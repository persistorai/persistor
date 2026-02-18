package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// EdgeHandler serves edge CRUD endpoints.
type EdgeHandler struct {
	repo EdgeService
	log  *logrus.Logger
}

// NewEdgeHandler creates an EdgeHandler with the given service and logger.
func NewEdgeHandler(repo EdgeService, log *logrus.Logger) *EdgeHandler {
	return &EdgeHandler{repo: repo, log: log}
}

// List handles GET /api/edges.
func (h *EdgeHandler) List(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}
	source := c.Query("source")
	target := c.Query("target")
	relation := c.Query("relation")
	limit := parseInt(c.DefaultQuery("limit", "50"), 50)
	offset := parseOffset(c.DefaultQuery("offset", "0"))

	edges, hasMore, err := h.repo.ListEdges(c.Request.Context(), tenantID, source, target, relation, limit, offset)
	if err != nil {
		h.log.WithError(err).Error("listing edges")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	c.JSON(http.StatusOK, gin.H{"edges": edges, "has_more": hasMore})
}

// Create handles POST /api/edges.
func (h *EdgeHandler) Create(c *gin.Context) {
	var req models.CreateEdgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	if err := req.Validate(); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeValidationError, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	edge, err := h.repo.CreateEdge(c.Request.Context(), tenantID, req)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

			return
		}

		if errors.Is(err, models.ErrDuplicateKey) {
			respondError(c, http.StatusConflict, "conflict", "edge with this source/target/relation already exists")

			return
		}

		h.log.WithError(err).Error("creating edge")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "edge.create", "tenant_id": tenantID, "source": req.Source, "target": req.Target, "relation": req.Relation}).Info("audit")

	c.JSON(http.StatusCreated, edge)
}

// Update handles PUT /api/edges/:source/:target/:relation.
func (h *EdgeHandler) Update(c *gin.Context) {
	source := c.Param("source")
	target := c.Param("target")
	relation := c.Param("relation")

	for _, pair := range []struct{ name, val string }{{"source", source}, {"target", target}, {"relation", relation}} {
		if err := validatePathID(pair.val); err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid "+pair.name+": "+err.Error())
			return
		}
	}

	var req models.UpdateEdgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	if err := req.Validate(); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeValidationError, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	edge, err := h.repo.UpdateEdge(c.Request.Context(), tenantID, source, target, relation, req)
	if err != nil {
		if errors.Is(err, models.ErrEdgeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "edge not found")

			return
		}

		h.log.WithError(err).Error("updating edge")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "edge.update", "tenant_id": tenantID, "source": source, "target": target, "relation": relation}).Info("audit")

	c.JSON(http.StatusOK, edge)
}

// PatchProperties handles PATCH /api/edges/:source/:target/:relation/properties.
func (h *EdgeHandler) PatchProperties(c *gin.Context) {
	source := c.Param("source")
	target := c.Param("target")
	relation := c.Param("relation")

	for _, pair := range []struct{ name, val string }{{"source", source}, {"target", target}, {"relation", relation}} {
		if err := validatePathID(pair.val); err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid "+pair.name+": "+err.Error())

			return
		}
	}

	var req models.PatchPropertiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	if err := req.Validate(); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeValidationError, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	edge, err := h.repo.PatchEdgeProperties(c.Request.Context(), tenantID, source, target, relation, req)
	if err != nil {
		if errors.Is(err, models.ErrEdgeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "edge not found")

			return
		}

		h.log.WithError(err).Error("patching edge properties")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{
		"action": "edge.patch_properties", "tenant_id": tenantID,
		"source": source, "target": target, "relation": relation,
	}).Info("audit")

	c.JSON(http.StatusOK, edge)
}

// Delete handles DELETE /api/edges/:source/:target/:relation.
func (h *EdgeHandler) Delete(c *gin.Context) {
	source := c.Param("source")
	target := c.Param("target")
	relation := c.Param("relation")

	for _, pair := range []struct{ name, val string }{{"source", source}, {"target", target}, {"relation", relation}} {
		if err := validatePathID(pair.val); err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid "+pair.name+": "+err.Error())
			return
		}
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	err := h.repo.DeleteEdge(c.Request.Context(), tenantID, source, target, relation)
	if err != nil {
		if errors.Is(err, models.ErrEdgeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "edge not found")

			return
		}

		h.log.WithError(err).Error("deleting edge")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "edge.delete", "tenant_id": tenantID, "source": source, "target": target, "relation": relation}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
