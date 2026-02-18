package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

// NodeHandler serves node CRUD endpoints.
type NodeHandler struct {
	repo NodeService
	log  *logrus.Logger
}

// NewNodeHandler creates a NodeHandler with the given service and logger.
func NewNodeHandler(repo NodeService, log *logrus.Logger) *NodeHandler {
	return &NodeHandler{repo: repo, log: log}
}

// List handles GET /api/nodes.
func (h *NodeHandler) List(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}
	typeFilter := c.Query("type")
	minSalience := parseFloat(c.DefaultQuery("min_salience", "0"))
	limit := parseInt(c.DefaultQuery("limit", "50"), 50)
	offset := parseOffset(c.DefaultQuery("offset", "0"))

	nodes, hasMore, err := h.repo.ListNodes(c.Request.Context(), tenantID, typeFilter, minSalience, limit, offset)
	if err != nil {
		h.log.WithError(err).Error("listing nodes")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "node.list", "tenant_id": tenantID, "type": typeFilter, "count": len(nodes)}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "has_more": hasMore})
}

// Get handles GET /api/nodes/:id.
func (h *NodeHandler) Get(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	node, err := h.repo.GetNode(c.Request.Context(), tenantID, nodeID)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("getting node")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "node.get", "tenant_id": tenantID, "node_id": nodeID}).Info("audit")

	c.JSON(http.StatusOK, node)
}

// Create handles POST /api/nodes.
func (h *NodeHandler) Create(c *gin.Context) {
	var req models.CreateNodeRequest
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

	node, err := h.repo.CreateNode(c.Request.Context(), tenantID, req)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateKey) {
			respondError(c, http.StatusConflict, "conflict", "node with this ID already exists")

			return
		}

		h.log.WithError(err).Error("creating node")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "node.create", "tenant_id": tenantID, "node_id": node.ID}).Info("audit")

	c.JSON(http.StatusCreated, node)
}

// Update handles PUT /api/nodes/:id.
func (h *NodeHandler) Update(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	var req models.UpdateNodeRequest
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

	node, err := h.repo.UpdateNode(c.Request.Context(), tenantID, nodeID, req)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("updating node")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "node.update", "tenant_id": tenantID, "node_id": nodeID}).Info("audit")

	c.JSON(http.StatusOK, node)
}

// PatchProperties handles PATCH /api/nodes/:id/properties.
func (h *NodeHandler) PatchProperties(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
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

	node, err := h.repo.PatchNodeProperties(c.Request.Context(), tenantID, nodeID, req)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("patching node properties")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "node.patch_properties", "tenant_id": tenantID, "node_id": nodeID}).Info("audit")

	c.JSON(http.StatusOK, node)
}

// Migrate handles POST /api/nodes/:id/migrate.
func (h *NodeHandler) Migrate(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	var req models.MigrateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, "invalid request body")

		return
	}

	if req.NewID == "" {
		respondError(c, http.StatusBadRequest, ErrCodeValidationError, "new_id is required")

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	result, err := h.repo.MigrateNode(c.Request.Context(), tenantID, nodeID, req)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		if errors.Is(err, models.ErrDuplicateKey) {
			respondError(c, http.StatusConflict, "conflict", "node with new_id already exists")

			return
		}

		h.log.WithError(err).Error("migrating node")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{
		"action": "node.migrate", "tenant_id": tenantID,
		"old_id": nodeID, "new_id": req.NewID,
	}).Info("audit")

	c.JSON(http.StatusOK, result)
}

// Delete handles DELETE /api/nodes/:id.
func (h *NodeHandler) Delete(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	err := h.repo.DeleteNode(c.Request.Context(), tenantID, nodeID)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("deleting node")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "node.delete", "tenant_id": tenantID, "node_id": nodeID}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
