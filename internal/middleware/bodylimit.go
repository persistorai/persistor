package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MaxBodySize returns middleware that limits request body size.
func MaxBodySize(maxBytes int64) gin.HandlerFunc {
	return MaxBodySizeByPath(maxBytes, nil)
}

// MaxBodySizeByPath returns middleware that limits request body size, with
// explicit per-path overrides keyed by URL path prefix.
func MaxBodySizeByPath(defaultMaxBytes int64, pathPrefixOverrides map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := defaultMaxBytes
		for prefix, override := range pathPrefixOverrides {
			if strings.HasPrefix(c.Request.URL.Path, prefix) {
				maxBytes = override
				break
			}
		}

		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}

		c.Next()
	}
}
