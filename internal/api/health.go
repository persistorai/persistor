// Package api provides HTTP handlers for the persistor.
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/ws"
)

// HealthHandler serves health check endpoints.
type HealthHandler struct {
	pool       *dbpool.Pool
	hub        *ws.Hub
	log        *logrus.Logger
	httpClient *http.Client
	version    string
	startTime  time.Time
	ollamaURL  string
}

// NewHealthHandler creates a HealthHandler with the given dependencies.
func NewHealthHandler(pool *dbpool.Pool, hub *ws.Hub, log *logrus.Logger, version, ollamaURL string) *HealthHandler {
	return &HealthHandler{
		pool:       pool,
		hub:        hub,
		log:        log,
		httpClient: &http.Client{Timeout: 2 * time.Second},
		version:    version,
		startTime:  time.Now(),
		ollamaURL:  ollamaURL,
	}
}

// readinessResponse is the JSON payload returned by the readiness endpoint.
type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Liveness handles GET /api/health — always returns ok for liveness probes.
func (h *HealthHandler) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "version": h.version})
}

// Readiness handles GET /api/ready — checks DB, schema, and Ollama.
func (h *HealthHandler) Readiness(c *gin.Context) {
	checks := map[string]string{
		"database": "ok",
		"schema":   "ok",
		"ollama":   "ok",
	}
	status := "ready"
	statusCode := http.StatusOK

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// Check database connectivity.
	if err := h.pool.HealthCheck(ctx); err != nil {
		h.log.WithError(err).Error("readiness: database health check failed")
		checks["database"] = "error"
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	// Check schema by querying tenants table.
	if checks["database"] == "ok" {
		if err := h.checkSchema(ctx); err != nil {
			h.log.WithError(err).Error("readiness: schema check failed")
			checks["schema"] = "error"
			status = "not_ready"
			statusCode = http.StatusServiceUnavailable
		}
	} else {
		checks["schema"] = "unknown"
	}

	// Check Ollama (best-effort, non-blocking).
	if err := h.checkOllama(); err != nil {
		h.log.WithError(err).Warn("readiness: ollama check failed")
		checks["ollama"] = "degraded"
	}

	c.JSON(statusCode, readinessResponse{
		Status: status,
		Checks: checks,
	})
}

// checkSchema verifies the database schema by querying the tenants table.
func (h *HealthHandler) checkSchema(ctx context.Context) error {
	var count int
	err := h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM tenants").Scan(&count)
	if err != nil {
		return fmt.Errorf("schema check: %w", err)
	}

	return nil
}

// checkOllama does a best-effort connectivity check to the Ollama API.
func (h *HealthHandler) checkOllama() error {
	if h.ollamaURL == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.ollamaURL+"/api/version", http.NoBody)
	if err != nil {
		return fmt.Errorf("ollama request: %w", err)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	return nil
}
