package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/security"
)

// BruteForceMiddleware returns middleware that blocks requests from locked-out API keys.
func BruteForceMiddleware(guard *security.BruteForceGuard) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := ExtractBearerToken(c)
		if apiKey == "" {
			c.Next()
			return
		}
		if guard.IsBlocked(apiKey) {
			respondError(c, http.StatusTooManyRequests, "rate_limited", "too many failed authentication attempts")
			c.Abort()
			return
		}

		c.Next()
	}
}
