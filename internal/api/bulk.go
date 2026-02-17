package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// BulkHandler serves batch operation endpoints.
type BulkHandler struct {
	repo BulkRepository
	log  *logrus.Logger
}

// NewBulkHandler creates a BulkHandler with the given repository and logger.
func NewBulkHandler(repo BulkRepository, log *logrus.Logger) *BulkHandler {
	return &BulkHandler{repo: repo, log: log}
}

// BulkNodes handles POST /api/bulk/nodes.
func (h *BulkHandler) BulkNodes(c *gin.Context) {
	var reqs []models.CreateNodeRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	if len(reqs) > 1000 {
		respondError(c, http.StatusBadRequest, ErrCodeValidationError, "bulk request exceeds maximum of 1000 items")

		return
	}

	for i, req := range reqs {
		if err := req.Validate(); err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeValidationError, "item "+strconv.Itoa(i)+": "+err.Error())

			return
		}
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	nodes, err := h.repo.BulkUpsertNodes(c.Request.Context(), tenantID, reqs)
	if err != nil {
		h.log.WithError(err).Error("bulk upserting nodes")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "bulk.nodes", "tenant_id": tenantID, "upserted": len(nodes)}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"upserted": len(nodes), "nodes": nodes})
}

// BulkEdges handles POST /api/bulk/edges.
func (h *BulkHandler) BulkEdges(c *gin.Context) {
	var reqs []models.CreateEdgeRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	if len(reqs) > 1000 {
		respondError(c, http.StatusBadRequest, ErrCodeValidationError, "bulk request exceeds maximum of 1000 items")

		return
	}

	for i, req := range reqs {
		if err := req.Validate(); err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeValidationError, "item "+strconv.Itoa(i)+": "+err.Error())

			return
		}
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	edges, err := h.repo.BulkUpsertEdges(c.Request.Context(), tenantID, reqs)
	if err != nil {
		h.log.WithError(err).Error("bulk upserting edges")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "bulk.edges", "tenant_id": tenantID, "upserted": len(edges)}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"upserted": len(edges), "edges": edges})
}
