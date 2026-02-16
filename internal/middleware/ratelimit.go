// Package middleware provides HTTP middleware for the persistor.
package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// maxBuckets is the maximum number of tracked IPs to prevent memory exhaustion.
const maxBuckets = 100_000

// RateLimiter implements a simple token bucket rate limiter per IP.
type RateLimiter struct {
	buckets map[string]*bucket
	mu      sync.Mutex
	rate    int
	burst   int
}

// bucket represents a per-IP token bucket for rate limiting.
type bucket struct {
	tokens     int
	lastFill   time.Time
	ratePerSec int
	burst      int
}

func (b *bucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	refill := int(elapsed * float64(b.ratePerSec))

	if refill > 0 {
		b.tokens += refill
		if b.tokens > b.burst {
			b.tokens = b.burst
		}

		b.lastFill = now
	}

	if b.tokens > 0 {
		b.tokens--

		return true
	}

	return false
}

// NewRateLimiter creates a RateLimiter with the given requests per second and burst size.
// It starts a background goroutine to evict stale buckets, which stops when ctx is cancelled.
func NewRateLimiter(ctx context.Context, ratePerSec, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    ratePerSec,
		burst:   burst,
	}
	go rl.startCleanup(ctx)

	return rl
}

// startCleanup periodically evicts stale rate-limit buckets.
func (rl *RateLimiter) startCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	const maxAge = 10 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			rl.mu.Lock()
			for ip, b := range rl.buckets {
				if now.Sub(b.lastFill) > maxAge {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Handler returns Gin middleware that applies rate limiting per client IP.
func (rl *RateLimiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// c.ClientIP() is safe from X-Forwarded-For spoofing because
		// SetTrustedProxies(nil) in router.go disables proxy header trust.
		ip := c.ClientIP()

		rl.mu.Lock()
		b, ok := rl.buckets[ip]
		if !ok {
			// Reject new IPs when bucket table is full to prevent memory exhaustion.
			if len(rl.buckets) >= maxBuckets {
				rl.mu.Unlock()
				respondError(c, http.StatusTooManyRequests, "rate_limited", "too many clients")

				return
			}

			b = &bucket{
				tokens:     rl.burst,
				lastFill:   time.Now(),
				ratePerSec: rl.rate,
				burst:      rl.burst,
			}
			rl.buckets[ip] = b
		}

		allowed := b.allow()
		rl.mu.Unlock()

		if !allowed {
			respondError(c, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")

			return
		}

		c.Next()
	}
}
