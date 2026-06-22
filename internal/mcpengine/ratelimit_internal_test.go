package mcpengine

import (
	"testing"
	"time"
)

// TestKeyLimiterEvictsIdleBuckets verifies the sweep drops buckets idle beyond
// the TTL (so an unbounded key space can't grow the map without bound) while
// keeping recently-seen ones.
func TestKeyLimiterEvictsIdleBuckets(t *testing.T) {
	l := NewKeyLimiter(100, 100)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	// Two keys seen at t=0.
	l.Allow("idle")
	l.Allow("active")
	if got := l.bucketCount(); got != 2 {
		t.Fatalf("bucket count = %d, want 2", got)
	}

	// Advance past a sweep interval, but keep "active" fresh. "idle" has now been
	// untouched longer than the idle TTL and must be evicted on the next Allow's
	// amortized sweep; "active" survives.
	now = now.Add(limiterIdleTTL + time.Minute)
	l.Allow("active")
	if got := l.bucketCount(); got != 1 {
		t.Fatalf("after sweep bucket count = %d, want 1 (idle evicted)", got)
	}
	if _, ok := l.buckets["active"]; !ok {
		t.Error("active bucket was evicted; only idle ones should be")
	}
}

func (l *KeyLimiter) bucketCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
