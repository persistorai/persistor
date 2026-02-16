package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	// RequestIDKey is the gin context key for the request ID.
	RequestIDKey = "request_id"

	// RequestIDHeader is the HTTP header used to propagate the request ID.
	RequestIDHeader = "X-Request-ID"
)

// RequestID always generates a fresh server-side UUID for the canonical request ID.
// If the client provides an X-Request-ID header, it is logged as a separate
// "client_request_id" field but never used as the canonical ID.
func RequestID(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.New().String()

		if clientID := c.GetHeader(RequestIDHeader); clientID != "" {
			log.WithFields(logrus.Fields{
				"request_id":        id,
				"client_request_id": clientID,
			}).Debug("client provided request ID mapped to server ID")
			c.Set("client_request_id", clientID)
		}

		c.Set(RequestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}
