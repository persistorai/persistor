package mcpengine

import (
	"sync"

	"golang.org/x/time/rate"
)

// WriteLimiter caps the sustained write rate per tenant at the MCP boundary so a
// runaway or prompt-injected agent cannot hammer the mutating tools (write,
// delete, restore). Each tenant gets its own token bucket; reads are never
// limited. One limiter is shared across all sessions of the daemon, so the cap
// is per tenant, not per connection.
//
// Buckets are created lazily and not evicted — fine for the current scale (a
// handful of tenants on one box). A multi-thousand-tenant deployment would add
// LRU eviction; noted, not built.
type WriteLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
	rate    rate.Limit
	burst   int
}

// NewWriteLimiter builds a limiter allowing perSecond sustained writes per tenant
// with the given burst. A non-positive perSecond disables limiting (Allow always
// true), which the local single-user path uses.
func NewWriteLimiter(perSecond float64, burst int) *WriteLimiter {
	return &WriteLimiter{
		buckets: make(map[string]*rate.Limiter),
		rate:    rate.Limit(perSecond),
		burst:   burst,
	}
}

// Allow reports whether the tenant may perform a write now, consuming a token if
// so. A nil limiter or a non-positive rate allows everything.
func (l *WriteLimiter) Allow(tenantID string) bool {
	if l == nil || l.rate <= 0 {
		return true
	}
	l.mu.Lock()
	b, ok := l.buckets[tenantID]
	if !ok {
		b = rate.NewLimiter(l.rate, l.burst)
		l.buckets[tenantID] = b
	}
	l.mu.Unlock()
	return b.Allow()
}
