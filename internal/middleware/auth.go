package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// authTimingFloor is the minimum response time for auth endpoints to prevent
// timing oracle attacks that could distinguish valid from invalid API keys.
const authTimingFloor = 50 * time.Millisecond

// TenantLookup is the interface for looking up a tenant by API key.
type TenantLookup interface {
	GetTenantByAPIKey(ctx context.Context, apiKey string) (string, error)
}

// truncateKey returns at most the first 4 characters of key followed by "...".
func truncateKey(key string) string {
	if len(key) > 4 {
		return key[:4] + "..."
	}
	return key
}

// enforceTimingFloor sleeps if needed so the response takes at least authTimingFloor.
func enforceTimingFloor(start time.Time) {
	if elapsed := time.Since(start); elapsed < authTimingFloor {
		time.Sleep(authTimingFloor - elapsed)
	}
}

// AuthMiddleware returns Gin middleware that authenticates requests via Bearer token.
// If a BruteForceGuard is provided, failed attempts are tracked per key hash.
func AuthMiddleware(lookup TenantLookup, log *logrus.Logger, guards ...*BruteForceGuard) gin.HandlerFunc {
	var guard *BruteForceGuard
	if len(guards) > 0 {
		guard = guards[0]
	}

	return func(c *gin.Context) {
		start := time.Now()
		defer func() {
			if c.Writer.Status() == http.StatusUnauthorized {
				enforceTimingFloor(start)
			}
		}()

		apiKey := ExtractBearerToken(c)
		if apiKey == "" {
			respondError(c, http.StatusUnauthorized, "unauthorized", "missing or invalid authorization header")
			return
		}

		tenantID, err := lookup.GetTenantByAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			logAuthFailure(log, c, apiKey)

			if guard != nil {
				guard.RecordFailure(apiKey)
			}

			respondError(c, http.StatusUnauthorized, "unauthorized", "invalid api key")
			return
		}

		if guard != nil {
			guard.ResetKey(apiKey)
		}

		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

// ExtractBearerToken extracts the API key from the Authorization header.
func ExtractBearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" || !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(header, "Bearer ")
}

// logAuthFailure logs a failed authentication attempt.
func logAuthFailure(log *logrus.Logger, c *gin.Context, apiKey string) {
	log.WithFields(logrus.Fields{
		"client_ip":  c.ClientIP(),
		"method":     c.Request.Method,
		"path":       c.Request.URL.Path,
		"user_agent": c.Request.UserAgent(),
		"request_id": c.GetString("request_id"),
		"key_prefix": truncateKey(apiKey),
	}).Warn("authentication failed: invalid api key")
}
