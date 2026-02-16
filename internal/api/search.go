package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// maxSearchQueryLen caps the length of search query strings.
const maxSearchQueryLen = 2000

// SearchHandler serves search endpoints.
type SearchHandler struct {
	repo SearchRepository
	log  *logrus.Logger
}

// NewSearchHandler creates a SearchHandler with the given repository and logger.
func NewSearchHandler(repo SearchRepository, log *logrus.Logger) *SearchHandler {
	return &SearchHandler{repo: repo, log: log}
}

// FullText handles GET /api/search.
func (h *SearchHandler) FullText(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "query parameter q is required")

		return
	}

	if len(q) > maxSearchQueryLen {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "query parameter q exceeds maximum length")

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}
	typeFilter := c.Query("type")
	minSalience := parseFloat(c.DefaultQuery("min_salience", "0"))
	limit := parseInt(c.DefaultQuery("limit", "20"), 20)

	nodes, err := h.repo.FullTextSearch(c.Request.Context(), tenantID, q, typeFilter, minSalience, limit)
	if err != nil {
		h.log.WithError(err).Error("full-text search")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "search.fulltext", "tenant_id": tenantID, "results": len(nodes)}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "total": len(nodes)})
}

// Semantic handles GET /api/search/semantic.
func (h *SearchHandler) Semantic(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "query parameter q is required")

		return
	}

	if len(q) > maxSearchQueryLen {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "query parameter q exceeds maximum length")

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}
	limit := parseInt(c.DefaultQuery("limit", "10"), 10)

	results, err := h.repo.SemanticSearch(c.Request.Context(), tenantID, q, limit)
	if err != nil {
		h.log.WithError(err).Error("semantic search")
		respondError(c, http.StatusBadGateway, ErrCodeInternalError, "search unavailable")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "search.semantic", "tenant_id": tenantID, "results": len(results)}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"nodes": results, "total": len(results)})
}

// Hybrid handles GET /api/search/hybrid.
func (h *SearchHandler) Hybrid(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "query parameter q is required")

		return
	}

	if len(q) > maxSearchQueryLen {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "query parameter q exceeds maximum length")

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}
	limit := parseInt(c.DefaultQuery("limit", "10"), 10)

	nodes, err := h.repo.HybridSearch(c.Request.Context(), tenantID, q, limit)
	if err != nil {
		// Embedding failed — fall back to full-text search.
		h.log.WithError(err).Warn("hybrid search failed, falling back to full-text")

		nodes, ftErr := h.repo.FullTextSearch(c.Request.Context(), tenantID, q, "", 0, limit)
		if ftErr != nil {
			h.log.WithError(ftErr).Error("full-text fallback in hybrid search")
			respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

			return
		}

		h.log.WithFields(logrus.Fields{"action": "search.hybrid_fallback", "tenant_id": tenantID, "results": len(nodes)}).Info("audit")

		c.JSON(http.StatusOK, gin.H{"nodes": nodes, "total": len(nodes)})

		return
	}

	h.log.WithFields(logrus.Fields{"action": "search.hybrid", "tenant_id": tenantID, "results": len(nodes)}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "total": len(nodes)})
}
