package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeout wraps each request's context with a deadline so that slow
// downstream operations (DB queries, RPC calls, etc.) are bounded.  Place
// this after the recovery middleware so panics are still caught, but before
// any route handlers so every handler inherits the deadline.
func RequestTimeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
