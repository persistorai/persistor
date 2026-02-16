package api

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/middleware"
	"github.com/persistorai/persistor/internal/ws"
)

// getTenantID extracts the authenticated tenant ID from the Gin context
// and validates it is a proper UUID.
func getTenantID(c *gin.Context) string {
	tid := c.GetString("tenant_id")

	if _, err := uuid.Parse(tid); err != nil {
		respondError(c, 400, ErrCodeInvalidRequest, "invalid tenant id")

		return ""
	}

	return tid
}

func wsHandler(appCtx context.Context, log *logrus.Logger, hub *ws.Hub, corsOrigins []string, lookup middleware.TenantLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := getTenantID(c)
		if tenantID == "" {
			return
		}

		// Extract the raw API key for periodic re-validation.
		apiKey := middleware.ExtractBearerToken(c)

		// CORS origins are reused as WebSocket origin patterns. The config
		// validator ensures these are safe host patterns (no wildcards etc.).
		conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
			OriginPatterns:       corsOrigins,
			CompressionMode:      websocket.CompressionContextTakeover,
			CompressionThreshold: 128,
		})
		if err != nil {
			log.WithError(err).Error("websocket accept failed")

			return
		}

		client := ws.NewClient(hub, conn, lookup, apiKey)
		client.TenantID = tenantID
		hub.Register(client)

		// Derive a context that cancels when either the server shuts down or the request ends.
		wsCtx, wsCancel := context.WithCancel(appCtx)
		go func() {
			select {
			case <-c.Request.Context().Done():
				wsCancel()
			case <-wsCtx.Done():
			}
		}()

		go client.WritePump(wsCtx)
		client.ReadPump(wsCtx)
		wsCancel()
	}
}

func ginLogger(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		fields := logrus.Fields{
			"method":   c.Request.Method,
			"path":     c.Request.URL.Path,
			"status":   c.Writer.Status(),
			"duration": time.Since(start).String(),
			"client":   c.ClientIP(),
		}
		if rid, exists := c.Get(middleware.RequestIDKey); exists {
			fields["request_id"] = rid
		}
		if tid := c.GetString("tenant_id"); tid != "" {
			fields["tenant_id"] = tid
		}
		log.WithFields(fields).Info("request")
	}
}

// maxPaginationLimit caps the maximum number of items per page.
const maxPaginationLimit = 1000

// maxPaginationOffset caps the maximum offset for paginated queries.
const maxPaginationOffset = 100000

func parseInt(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return fallback
	}

	if v > maxPaginationLimit {
		return maxPaginationLimit
	}

	return v
}

func parseOffset(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return 0
	}

	if v > maxPaginationOffset {
		return maxPaginationOffset
	}

	return v
}

// validatePathID checks that a path parameter ID is non-empty and within length limits.
func validatePathID(id string) error {
	if id == "" {
		return fmt.Errorf("id must not be empty")
	}
	if len(id) > 255 {
		return fmt.Errorf("id exceeds maximum length of 255")
	}
	return nil
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}

	return v
}
