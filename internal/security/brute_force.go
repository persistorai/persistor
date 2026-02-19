package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	BruteForceMaxAttempts = 5
	BruteForceWindow      = 15 * time.Minute
	BruteForceLockout     = 5 * time.Minute
	bruteForceCleanup     = 60 * time.Second
	bruteForceMaxRecords  = 10000
)

type failureRecord struct {
	attempts  int
	firstFail time.Time
	lockedAt  time.Time
}

// BruteForceGuard tracks per-key-hash authentication failures and blocks
// keys that exceed the failure threshold within the tracking window.
type BruteForceGuard struct {
	mu      sync.Mutex
	records map[string]*failureRecord
	log     *logrus.Logger
}

// NewBruteForceGuard creates a new guard and starts a background cleanup goroutine
// that stops when ctx is cancelled.
func NewBruteForceGuard(ctx context.Context, log *logrus.Logger) *BruteForceGuard {
	g := &BruteForceGuard{
		records: make(map[string]*failureRecord),
		log:     log,
	}
	go g.cleanupLoop(ctx)
	return g
}

func keyHash(apiKey string) string {
	h := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(h[:])
}

// IsBlocked returns true if the given API key hash is currently locked out.
func (g *BruteForceGuard) IsBlocked(apiKey string) bool {
	kh := keyHash(apiKey)
	g.mu.Lock()
	defer g.mu.Unlock()

	rec, ok := g.records[kh]
	if !ok {
		return false
	}

	if !rec.lockedAt.IsZero() && time.Since(rec.lockedAt) < BruteForceLockout {
		return true
	}

	return false
}

// RecordFailure records a failed authentication attempt for the given API key.
func (g *BruteForceGuard) RecordFailure(apiKey string) {
	kh := keyHash(apiKey)
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	rec, ok := g.records[kh]
	if !ok {
		g.records[kh] = &failureRecord{attempts: 1, firstFail: now}
		return
	}

	// Reset if outside the tracking window.
	if now.Sub(rec.firstFail) > BruteForceWindow {
		rec.attempts = 1
		rec.firstFail = now
		rec.lockedAt = time.Time{}
		return
	}

	rec.attempts++
	if rec.attempts >= BruteForceMaxAttempts {
		rec.lockedAt = now
		g.log.WithField("key_hash", kh[:16]+"...").Warn("api key locked out due to repeated auth failures")
	}
}

// ResetKey clears failure tracking for a key (call on successful auth).
func (g *BruteForceGuard) ResetKey(apiKey string) {
	kh := keyHash(apiKey)
	g.mu.Lock()
	delete(g.records, kh)
	g.mu.Unlock()
}

func (g *BruteForceGuard) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(bruteForceCleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			g.mu.Lock()
			for k, rec := range g.records {
				if !rec.lockedAt.IsZero() && now.Sub(rec.lockedAt) >= BruteForceLockout {
					delete(g.records, k)
				} else if now.Sub(rec.firstFail) >= BruteForceWindow {
					delete(g.records, k)
				}
			}
			if len(g.records) > bruteForceMaxRecords {
				g.evictOldest(len(g.records) - bruteForceMaxRecords)
			}
			g.mu.Unlock()
		}
	}
}

// evictOldest removes n entries with the oldest firstFail times.
// Caller must hold g.mu. Complexity: O(m log m) via sort.
func (g *BruteForceGuard) evictOldest(n int) {
	type entry struct {
		key  string
		time time.Time
	}
	entries := make([]entry, 0, len(g.records))
	for k, rec := range g.records {
		entries = append(entries, entry{k, rec.firstFail})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].time.Before(entries[j].time)
	})
	for i := range n {
		delete(g.records, entries[i].key)
	}
}
