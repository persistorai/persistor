package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	tenantCacheTTL     = 5 * time.Minute
	negativeCacheTTL   = 30 * time.Second
	maxCacheEntries    = 10000
	cacheCleanupPeriod = 60 * time.Second
)

// negativeSentinel is stored in tenantID to indicate a cached lookup failure.
const negativeSentinel = "\x00negative"

// errCachedNotFound is returned for negative cache hits.
var errCachedNotFound = errors.New("tenant not found (cached)")

type cachedTenant struct {
	tenantID  string
	fetchedAt time.Time
}

// isNegative returns true if this entry represents a cached lookup failure.
func (ct cachedTenant) isNegative() bool {
	return ct.tenantID == negativeSentinel
}

// ttl returns the appropriate TTL for this entry.
func (ct cachedTenant) ttl() time.Duration {
	if ct.isNegative() {
		return negativeCacheTTL
	}
	return tenantCacheTTL
}

// hashKey returns a hex-encoded SHA-256 hash of the API key so raw keys
// are never stored in memory.
func hashKey(apiKey string) string {
	h := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(h[:])
}

// CachedTenantLookup wraps a TenantLookup with a bounded in-memory cache.
type CachedTenantLookup struct {
	inner TenantLookup
	mu    sync.RWMutex
	cache map[string]cachedTenant
}

// NewCachedTenantLookup creates a caching wrapper around the given TenantLookup.
// The provided context controls the lifetime of the background eviction goroutine.
func NewCachedTenantLookup(ctx context.Context, inner TenantLookup) *CachedTenantLookup {
	c := &CachedTenantLookup{
		inner: inner,
		cache: make(map[string]cachedTenant),
	}
	go c.evictLoop(ctx)
	return c
}

// evictLoop periodically removes expired entries from the cache.
func (c *CachedTenantLookup) evictLoop(ctx context.Context) {
	ticker := time.NewTicker(cacheCleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for k, v := range c.cache {
				if now.Sub(v.fetchedAt) >= v.ttl() {
					delete(c.cache, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// GetTenantByAPIKey returns a cached tenant ID or delegates to the inner lookup.
// Failed lookups are negatively cached for 30s to prevent brute-force DB hammering.
func (c *CachedTenantLookup) GetTenantByAPIKey(ctx context.Context, apiKey string) (string, error) {
	hk := hashKey(apiKey)

	// Read path — RLock for concurrent cache hits.
	c.mu.RLock()
	entry, ok := c.cache[hk]
	if ok && time.Since(entry.fetchedAt) < entry.ttl() {
		c.mu.RUnlock()
		if entry.isNegative() {
			return "", errCachedNotFound
		}
		return entry.tenantID, nil
	}
	c.mu.RUnlock()

	// Cache miss or expired — fetch from inner.
	tenantID, err := c.inner.GetTenantByAPIKey(ctx, apiKey)
	if err != nil {
		// Negative cache: store failed lookup with short TTL.
		c.mu.Lock()
		c.cache[hk] = cachedTenant{tenantID: negativeSentinel, fetchedAt: time.Now()}
		c.mu.Unlock()
		return "", err
	}

	c.mu.Lock()
	if len(c.cache) >= maxCacheEntries {
		// Evict expired entries, then trim if still over limit.
		now := time.Now()
		for k, v := range c.cache {
			if now.Sub(v.fetchedAt) >= v.ttl() {
				delete(c.cache, k)
			}
		}
		for k := range c.cache {
			if len(c.cache) < maxCacheEntries {
				break
			}
			delete(c.cache, k)
		}
	}
	c.cache[hk] = cachedTenant{tenantID: tenantID, fetchedAt: time.Now()}
	c.mu.Unlock()

	return tenantID, nil
}
