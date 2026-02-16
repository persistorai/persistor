package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/metrics"
)

// PrometheusMiddleware records HTTP request duration and count.
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath() // route pattern, not actual path (avoids cardinality explosion)
		if path == "" {
			path = "unknown"
		}
		metrics.RequestDuration.WithLabelValues(c.Request.Method, path, status).Observe(duration)
		metrics.RequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
	}
}
