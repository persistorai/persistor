package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

)

// HistoryHandler serves property history endpoints.
type HistoryHandler struct {
	repo HistoryService
	log  *logrus.Logger
}

// NewHistoryHandler creates a HistoryHandler with the given repository and logger.
func NewHistoryHandler(repo HistoryService, log *logrus.Logger) *HistoryHandler {
	return &HistoryHandler{repo: repo, log: log}
}

// GetHistory handles GET /api/v1/nodes/:id/history.
func (h *HistoryHandler) GetHistory(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	propertyKey := c.Query("property")
	limit := parseInt(c.DefaultQuery("limit", "50"), 50)
	offset := parseOffset(c.DefaultQuery("offset", "0"))

	changes, hasMore, err := h.repo.GetPropertyHistory(c.Request.Context(), tenantID, nodeID, propertyKey, limit, offset)
	if err != nil {
		h.log.WithError(err).Error("getting property history")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{
		"action":    "history.get",
		"tenant_id": tenantID,
		"node_id":   nodeID,
		"count":     len(changes),
	}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"changes": changes, "has_more": hasMore})
}
