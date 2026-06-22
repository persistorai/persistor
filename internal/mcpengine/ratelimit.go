package mcpengine

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// KeyLimiter is a per-key token-bucket rate limiter. Each key gets its own
// bucket, shared across all sessions of the daemon so the cap is per key, not
// per connection. Keys are tenant ids for the write/read caps, or client IPs for
// the registration proxy.
//
// Buckets are created lazily and swept when idle, so an unbounded key space — an
// open signup minting many tenants, or many distinct client IPs — cannot grow
// the map without bound. A bucket untouched for limiterIdleTTL is dropped on the
// next sweep; a returning key just gets a fresh full bucket. The idle TTL is far
// longer than any bucket's refill time, so an actively-throttled key is never
// evicted mid-limit.
type KeyLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	rate      rate.Limit
	burst     int
	lastSweep time.Time
	now       func() time.Time // injectable clock for tests
}

type bucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

const (
	// A bucket idle this long is evicted; the sweep runs at most this often.
	limiterIdleTTL       = 30 * time.Minute
	limiterSweepInterval = 10 * time.Minute
)

// NewKeyLimiter builds a limiter allowing perSecond sustained events per key with
// the given burst. A non-positive perSecond disables limiting (Allow always
// true), which the local single-user path uses.
func NewKeyLimiter(perSecond float64, burst int) *KeyLimiter {
	return &KeyLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate.Limit(perSecond),
		burst:   burst,
		now:     time.Now,
	}
}

// Allow reports whether the key may proceed now, consuming a token if so. A nil
// limiter or a non-positive rate allows everything.
func (l *KeyLimiter) Allow(key string) bool {
	if l == nil || l.rate <= 0 {
		return true
	}
	now := l.now()
	l.mu.Lock()
	l.sweepLocked(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.rate, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = now
	lim := b.lim
	l.mu.Unlock()
	// Allow() outside the map lock: rate.Limiter is internally synchronized, so
	// concurrent keys don't serialize on the limiter map.
	return lim.Allow()
}

// sweepLocked drops buckets idle beyond limiterIdleTTL. It is amortized — it does
// real work at most once per limiterSweepInterval. The caller holds l.mu.
func (l *KeyLimiter) sweepLocked(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < limiterSweepInterval {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if now.Sub(b.lastSeen) > limiterIdleTTL {
			delete(l.buckets, k)
		}
	}
}
