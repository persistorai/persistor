package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// GraphHandler serves graph traversal endpoints.
type GraphHandler struct {
	repo GraphService
	log  *logrus.Logger
}

// NewGraphHandler creates a GraphHandler with the given repository and logger.
func NewGraphHandler(repo GraphService, log *logrus.Logger) *GraphHandler {
	return &GraphHandler{repo: repo, log: log}
}

// Neighbors handles GET /api/graph/neighbors/:id.
func (h *GraphHandler) Neighbors(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	limit := parseInt(c.DefaultQuery("limit", "100"), 100)
	result, err := h.repo.Neighbors(c.Request.Context(), tenantID, nodeID, limit)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("getting neighbors")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	c.JSON(http.StatusOK, result)
}

// Traverse handles GET /api/graph/traverse/:id.
func (h *GraphHandler) Traverse(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}
	maxHops := parseInt(c.DefaultQuery("hops", "2"), 2)
	if maxHops > 10 {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "hops must be <= 10")

		return
	}

	result, err := h.repo.Traverse(c.Request.Context(), tenantID, nodeID, maxHops)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("traversing graph")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	c.JSON(http.StatusOK, result)
}

// Context handles GET /api/graph/context/:id.
func (h *GraphHandler) Context(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	result, err := h.repo.GraphContext(c.Request.Context(), tenantID, nodeID)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("getting graph context")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	c.JSON(http.StatusOK, result)
}

// Path handles GET /api/graph/path/:from/:to.
func (h *GraphHandler) Path(c *gin.Context) {
	from := c.Param("from")
	to := c.Param("to")

	if err := validatePathID(from); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid from: "+err.Error())

		return
	}

	if err := validatePathID(to); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid to: "+err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	nodes, err := h.repo.ShortestPath(c.Request.Context(), tenantID, from, to)
	if err != nil {
		h.log.WithError(err).Error("finding shortest path")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	if nodes == nil {
		respondError(c, http.StatusNotFound, ErrCodeNotFound, "no path found")

		return
	}

	c.JSON(http.StatusOK, gin.H{"path": nodes})
}
