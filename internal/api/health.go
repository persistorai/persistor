// Package api provides HTTP handlers for the persistor.
package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/ws"
)

// HealthHandler serves health check endpoints.
type HealthHandler struct {
	pool                *dbpool.Pool
	hub                 *ws.Hub
	log                 *logrus.Logger
	httpClient          *http.Client
	version             string
	startTime           time.Time
	ollamaURL           string
	embeddingModel      string
	embeddingDimensions int
}

// NewHealthHandler creates a HealthHandler with the given dependencies.
func NewHealthHandler(pool *dbpool.Pool, hub *ws.Hub, log *logrus.Logger, version, ollamaURL, embeddingModel string, embeddingDimensions int) *HealthHandler {
	return &HealthHandler{
		pool:                pool,
		hub:                 hub,
		log:                 log,
		httpClient:          &http.Client{Timeout: 2 * time.Second},
		version:             version,
		startTime:           time.Now(),
		ollamaURL:           ollamaURL,
		embeddingModel:      embeddingModel,
		embeddingDimensions: embeddingDimensions,
	}
}

// readinessResponse is the JSON payload returned by the readiness endpoint.
type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// healthResponse is the JSON payload returned by the health/liveness endpoint.
type healthResponse struct {
	Status              string  `json:"status"`
	Version             string  `json:"version"`
	Database            string  `json:"database"`
	Embeddings          string  `json:"embeddings"`
	EmbeddingDimensions int     `json:"embedding_dimensions"`
	UptimeSeconds       float64 `json:"uptime_seconds"`
}

// Liveness handles GET /api/health — returns status with db, embeddings, and uptime info.
func (h *HealthHandler) Liveness(c *gin.Context) {
	resp := healthResponse{
		Status:              "ok",
		Version:             h.version,
		Database:            "connected",
		Embeddings:          "unavailable",
		EmbeddingDimensions: h.embeddingDimensions,
		UptimeSeconds:       time.Since(h.startTime).Seconds(),
	}

	// Best-effort database ping (non-fatal for liveness).
	if h.pool != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := h.pool.HealthCheck(ctx); err != nil {
			resp.Database = "disconnected"
		}
	} else {
		resp.Database = "not_configured"
	}

	// Report embedding availability.
	if h.embeddingModel != "" {
		resp.Embeddings = h.embeddingModel
	}

	c.JSON(http.StatusOK, resp)
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
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	return nil
}
