package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// AuditHandler serves audit log endpoints.
type AuditHandler struct {
	repo AuditRepository
	log  *logrus.Logger
}

// NewAuditHandler creates an AuditHandler.
func NewAuditHandler(repo AuditRepository, log *logrus.Logger) *AuditHandler {
	return &AuditHandler{repo: repo, log: log}
}

// Query handles GET /api/v1/audit.
func (h *AuditHandler) Query(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	opts := models.AuditQueryOpts{
		EntityType: c.Query("entity_type"),
		EntityID:   c.Query("entity_id"),
		Action:     c.Query("action"),
		Limit:      parseInt(c.Query("limit"), 50),
		Offset:     parseOffset(c.Query("offset")),
	}

	if since := c.Query("since"); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid since format, use RFC3339")
			return
		}
		opts.Since = &t
	}

	entries, hasMore, err := h.repo.QueryAudit(c.Request.Context(), tenantID, opts)
	if err != nil {
		h.log.WithError(err).Error("failed to query audit log")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "failed to query audit log")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     entries,
		"has_more": hasMore,
	})
}

// Purge handles DELETE /api/v1/audit.
func (h *AuditHandler) Purge(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	retentionDays := 90
	if rd := c.Query("retention_days"); rd != "" {
		v, err := strconv.Atoi(rd)
		if err != nil || v < 1 {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "retention_days must be a positive integer")
			return
		}
		retentionDays = v
	}

	deleted, err := h.repo.PurgeOldEntries(c.Request.Context(), tenantID, retentionDays)
	if err != nil {
		h.log.WithError(err).Error("failed to purge audit entries")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "failed to purge audit entries")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deleted":        deleted,
		"retention_days": retentionDays,
	})
}
