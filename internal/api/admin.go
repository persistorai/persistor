package api

import (
	"net/http"
	"strconv"

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
		h.embedWorker.Enqueue(service.EmbedJob{TenantID: tenantID, NodeID: n.ID, Text: n.EmbeddingText()})
	}

	h.log.WithFields(logrus.Fields{"action": "admin.backfill_embeddings", "tenant_id": tenantID, "queued": len(nodes)}).Info("audit")
	c.JSON(http.StatusOK, gin.H{"queued": len(nodes)})
}

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

	h.log.WithFields(logrus.Fields{"action": "admin.reprocess_nodes", "tenant_id": tenantID, "scanned": result.Scanned, "updated_search": result.UpdatedSearch, "queued_embed": result.QueuedEmbed}).Info("audit")
	c.JSON(http.StatusOK, result)
}

// RunMaintenance performs an explicit maintenance pass for refresh/reprocess work.
func (h *AdminHandler) RunMaintenance(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	var req models.MaintenanceRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}
	if !req.RefreshSearchText && !req.RefreshEmbeddings && !req.ScanStaleFacts && !req.IncludeDuplicateCandidates {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "select at least one maintenance action")
		return
	}

	result, err := h.repo.RunMaintenance(c.Request.Context(), tenantID, req)
	if err != nil {
		h.log.WithError(err).Error("running maintenance")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}

	h.log.WithFields(logrus.Fields{"action": "admin.run_maintenance", "tenant_id": tenantID, "scanned": result.Scanned, "updated_search_text": result.UpdatedSearchText, "queued_embeddings": result.QueuedEmbeddings, "stale_fact_nodes": result.StaleFactNodes, "superseded_nodes": result.SupersededNodes, "duplicate_candidate_pairs": result.DuplicateCandidatePairs}).Info("audit")
	c.JSON(http.StatusOK, result)
}

// ListMergeSuggestions returns explainable duplicate candidates for manual review.
func (h *AdminHandler) ListMergeSuggestions(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	opts := models.MergeSuggestionListOpts{Type: c.Query("type")}
	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid limit")
			return
		}
		opts.Limit = limit
	}
	if minScoreStr := c.Query("min_score"); minScoreStr != "" {
		minScore, err := strconv.ParseFloat(minScoreStr, 64)
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid min_score")
			return
		}
		opts.MinScore = minScore
	}

	suggestions, err := h.repo.ListMergeSuggestions(c.Request.Context(), tenantID, opts)
	if err != nil {
		h.log.WithError(err).Error("listing merge suggestions")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

func (h *AdminHandler) RecordRetrievalFeedback(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	var req models.RetrievalFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")
		return
	}
	item, err := h.repo.RecordRetrievalFeedback(c.Request.Context(), tenantID, req)
	if err != nil {
		h.log.WithError(err).Warn("recording retrieval feedback")
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *AdminHandler) GetRetrievalFeedbackSummary(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	opts := models.RetrievalFeedbackListOpts{}
	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid limit")
			return
		}
		opts.Limit = limit
	}
	summary, err := h.repo.GetRetrievalFeedbackSummary(c.Request.Context(), tenantID, opts)
	if err != nil {
		h.log.WithError(err).Error("getting retrieval feedback summary")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")
		return
	}
	c.JSON(http.StatusOK, summary)
}
