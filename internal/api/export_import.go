package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// ExportImportHandler serves backup and restore endpoints.
type ExportImportHandler struct {
	repo ExportImportService
	log  *logrus.Logger
}

// NewExportImportHandler creates an ExportImportHandler.
func NewExportImportHandler(repo ExportImportService, log *logrus.Logger) *ExportImportHandler {
	return &ExportImportHandler{repo: repo, log: log}
}

// Export handles GET /api/v1/export.
// Returns the full tenant export as a JSON file attachment.
func (h *ExportImportHandler) Export(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	data, err := h.repo.Export(c.Request.Context(), tenantID)
	if err != nil {
		h.log.WithError(err).Error("exporting knowledge graph")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "export failed")

		return
	}

	hostname, _ := os.Hostname()
	ts := time.Now().UTC().Format("20060102T150405Z")
	filename := fmt.Sprintf("persistor-export-%s-%s.json", hostname, ts)

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	h.log.WithFields(logrus.Fields{
		"action":     "export",
		"tenant_id":  tenantID,
		"node_count": data.Stats.NodeCount,
		"edge_count": data.Stats.EdgeCount,
	}).Info("audit")

	c.JSON(http.StatusOK, data)
}

// Import handles POST /api/v1/import.
// Accepts an ExportFormat JSON body and writes it into the tenant graph.
func (h *ExportImportHandler) Import(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	var data models.ExportFormat
	if err := c.ShouldBindJSON(&data); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	opts := models.ImportOptions{
		OverwriteExisting:    c.Query("overwrite") == "true",
		DryRun:               c.Query("dry_run") == "true",
		RegenerateEmbeddings: c.Query("regenerate_embeddings") == "true",
		ResetUsage:           c.Query("reset_usage") == "true",
	}

	result, err := h.repo.Import(c.Request.Context(), tenantID, &data, opts)
	if err != nil {
		h.log.WithError(err).Error("importing knowledge graph")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "import failed")

		return
	}

	h.log.WithFields(logrus.Fields{
		"action":        "import",
		"tenant_id":     tenantID,
		"nodes_created": result.NodesCreated,
		"edges_created": result.EdgesCreated,
		"dry_run":       opts.DryRun,
	}).Info("audit")

	c.JSON(http.StatusOK, result)
}

// Validate handles POST /api/v1/import/validate.
// Checks the payload for consistency errors without writing to the database.
func (h *ExportImportHandler) Validate(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	var data models.ExportFormat
	if err := c.ShouldBindJSON(&data); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	errs, err := h.repo.ValidateImport(c.Request.Context(), tenantID, &data)
	if err != nil {
		h.log.WithError(err).Error("validating import payload")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "validation failed")

		return
	}

	c.JSON(http.StatusOK, gin.H{"errors": errs, "valid": len(errs) == 0})
}
