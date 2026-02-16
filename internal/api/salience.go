package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

const (
	recalcLocksMax = 1000
	recalcLocksTTL = 30 * time.Minute
)

// recalcEntry holds a per-tenant mutex with a last-used timestamp for TTL eviction.
type recalcEntry struct {
	mu       sync.Mutex
	lastUsed time.Time
}

// recalcLockMap is a bounded, TTL-evicting map of per-tenant recalculation locks.
type recalcLockMap struct {
	mu      sync.Mutex
	entries map[string]*recalcEntry
}

func newRecalcLockMap(ctx context.Context) *recalcLockMap {
	m := &recalcLockMap{entries: make(map[string]*recalcEntry)}
	go m.cleanup(ctx)
	return m
}

// get returns the recalcEntry for tenantID, creating one if needed.
// Evicts the oldest entry if the map exceeds recalcLocksMax.
func (m *recalcLockMap) get(tenantID string) *recalcEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.entries[tenantID]; ok {
		e.lastUsed = time.Now()
		return e
	}

	// Evict oldest if at capacity.
	if len(m.entries) >= recalcLocksMax {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range m.entries {
			if oldestKey == "" || v.lastUsed.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.lastUsed
			}
		}
		delete(m.entries, oldestKey)
	}

	e := &recalcEntry{lastUsed: time.Now()}
	m.entries[tenantID] = e
	return e
}

// refreshLastUsed updates the lastUsed timestamp for the given tenant under the map lock.
func (m *recalcLockMap) refreshLastUsed(tenantID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.entries[tenantID]; ok {
		e.lastUsed = time.Now()
	}
}

func (m *recalcLockMap) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.mu.Lock()
			for k, v := range m.entries {
				if now.Sub(v.lastUsed) > recalcLocksTTL {
					delete(m.entries, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

// SalienceHandler serves salience management endpoints.
type SalienceHandler struct {
	repo  SalienceRepository
	log   *logrus.Logger
	locks *recalcLockMap
}

// NewSalienceHandler creates a SalienceHandler with the given repository and logger.
func NewSalienceHandler(ctx context.Context, repo SalienceRepository, log *logrus.Logger) *SalienceHandler {
	return &SalienceHandler{repo: repo, log: log, locks: newRecalcLockMap(ctx)}
}

// Boost handles POST /api/salience/boost/:id.
func (h *SalienceHandler) Boost(c *gin.Context) {
	nodeID := c.Param("id")
	if err := validatePathID(nodeID); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidRequest, err.Error())

		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	node, err := h.repo.BoostNode(c.Request.Context(), tenantID, nodeID)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("boosting node salience")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "salience.boost", "tenant_id": tenantID, "node_id": nodeID}).Info("audit")

	c.JSON(http.StatusOK, node)
}

// Supersede handles POST /api/salience/supersede.
func (h *SalienceHandler) Supersede(c *gin.Context) {
	var req models.SupersedeRequest
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

	err := h.repo.SupersedeNode(c.Request.Context(), tenantID, req.OldID, req.NewID)
	if err != nil {
		if errors.Is(err, models.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "node not found")

			return
		}

		h.log.WithError(err).Error("superseding node")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "salience.supersede", "tenant_id": tenantID, "old_id": req.OldID, "new_id": req.NewID}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"superseded": true})
}

// Recalculate handles POST /api/salience/recalc.
func (h *SalienceHandler) Recalculate(c *gin.Context) {
	tenantID := getTenantID(c)
	if tenantID == "" {
		return
	}

	entry := h.locks.get(tenantID)
	if !entry.mu.TryLock() {
		respondError(c, http.StatusConflict, "conflict", "recalculation already in progress")
		return
	}
	defer entry.mu.Unlock()

	// Refresh lastUsed after acquiring the lock to prevent eviction during long recalculations.
	h.locks.refreshLastUsed(tenantID)

	count, err := h.repo.RecalculateSalience(c.Request.Context(), tenantID)
	if err != nil {
		h.log.WithError(err).Error("recalculating salience")
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal server error")

		return
	}

	h.log.WithFields(logrus.Fields{"action": "salience.recalculate", "tenant_id": tenantID, "updated": count}).Info("audit")

	c.JSON(http.StatusOK, gin.H{"updated": count})
}
