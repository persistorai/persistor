package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/metrics"
)

// StatsHandler serves the knowledge graph statistics endpoint.
type StatsHandler struct {
	pool *dbpool.Pool
	log  *logrus.Logger
}

// NewStatsHandler creates a StatsHandler with the given dependencies.
func NewStatsHandler(pool *dbpool.Pool, log *logrus.Logger) *StatsHandler {
	return &StatsHandler{pool: pool, log: log}
}

// statsResponse is the JSON payload returned by the stats endpoint.
type statsResponse struct {
	Nodes              int     `json:"nodes"`
	Edges              int     `json:"edges"`
	EntityTypes        int     `json:"entity_types"`
	AvgSalience        float64 `json:"avg_salience"`
	EmbeddingsComplete int     `json:"embeddings_complete"`
	EmbeddingsPending  int     `json:"embeddings_pending"`
}

// GetStats handles GET /api/v1/stats — returns aggregate KG statistics.
func (h *StatsHandler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetString("tenant_id")

	if _, err := uuid.Parse(tenantID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid tenant id")
		return
	}

	// Start a read-only transaction with tenant RLS.
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		h.log.WithError(err).Error("stats: begin tx")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck // read-only tx, rollback is cleanup.

	// Set tenant context for RLS.
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		h.log.WithError(err).Error("stats: set tenant")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}

	var resp statsResponse

	// Single consolidated query for all tenant-scoped stats.
	if err := tx.QueryRow(ctx,
		`SELECT
			COUNT(*),
			COALESCE(AVG(salience_score), 0),
			COUNT(DISTINCT type),
			COUNT(*) FILTER (WHERE embedding IS NOT NULL),
			COUNT(*) FILTER (WHERE embedding IS NULL),
			(SELECT COUNT(*) FROM kg_edges WHERE tenant_id = current_setting('app.tenant_id')::uuid) AS edge_count
		 FROM kg_nodes`,
	).Scan(
		&resp.Nodes, &resp.AvgSalience, &resp.EntityTypes,
		&resp.EmbeddingsComplete, &resp.EmbeddingsPending,
		&resp.Edges,
	); err != nil {
		h.log.WithError(err).Error("stats: consolidated query")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}

	// Round avg_salience to 2 decimal places for cleaner output.
	resp.AvgSalience = float64(int(resp.AvgSalience*100+0.5)) / 100

	// Update Prometheus gauges with fresh counts.
	metrics.NodeCount.Set(float64(resp.Nodes))
	metrics.EdgeCount.Set(float64(resp.Edges))

	c.JSON(http.StatusOK, resp)
}
