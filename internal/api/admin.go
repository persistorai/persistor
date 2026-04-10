package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/service"
)

// AdminHandler serves administrative endpoints.
type AdminHandler struct {
	repo        AdminService
	embedWorker *service.EmbedWorker
	log         *logrus.Logger
}

// NewAdminHandler creates an AdminHandler.
func NewAdminHandler(repo AdminService, embedWorker *service.EmbedWorker, log *logrus.Logger) *AdminHandler {
	return &AdminHandler{repo: repo, embedWorker: embedWorker, log: log}
}

// BackfillEmbeddings queues embedding generation for all nodes with NULL embeddings.
func (h *AdminHandler) BackfillEmbeddings(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	nodes, err := h.repo.ListNodesWithoutEmbeddings(c.Request.Context(), tenantID, 1000)
	if err != nil {
		h.log.WithError(err).Error("listing nodes without embeddings")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	if h.embedWorker == nil {
		respondError(c, http.StatusServiceUnavailable, ErrCodeInternalError, "embedding worker not available")

		return
	}

	for _, n := range nodes {
		h.embedWorker.Enqueue(service.EmbedJob{
			TenantID: tenantID,
			NodeID:   n.ID,
			Text:     n.EmbeddingText(),
		})
	}

	h.log.WithFields(logrus.Fields{
		"action":    "admin.backfill_embeddings",
		"tenant_id": tenantID,
		"queued":    len(nodes),
	}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"queued": len(nodes)})
}

// ReprocessNodes rewrites search text and/or requeues embeddings for a batch of nodes.
func (h *AdminHandler) ReprocessNodes(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	var req models.ReprocessNodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}
	if !req.SearchText && !req.Embeddings {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "at least one of search_text or embeddings must be true")
		return
	}

	result, err := h.repo.ReprocessNodes(c.Request.Context(), tenantID, req)
	if err != nil {
		h.log.WithError(err).Error("reprocessing nodes")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}

	h.log.WithFields(logrus.Fields{
		"action":         "admin.reprocess_nodes",
		"tenant_id":      tenantID,
		"scanned":        result.Scanned,
		"updated_search": result.UpdatedSearch,
		"queued_embed":   result.QueuedEmbed,
	}).Info("audit")

	c.JSON(http.StatusOK, result)
}
