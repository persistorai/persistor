// Package ws implements WebSocket hub and client management.
package ws

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/metrics"
)

// Hub channel buffer sizes.
const (
	broadcastBuffer = 256
	registerBuffer  = 64
)

// tenantBroadcast is sent through the broadcast channel to the Run goroutine.
type tenantBroadcast struct {
	tenantID string
	msg      []byte
}

// Hub manages active WebSocket clients and broadcasts messages.
// All client map mutations happen exclusively in the Run goroutine.
type Hub struct {
	clients     map[*Client]bool
	tenantCount map[string]int // O(1) per-tenant connection counting
	register    chan *Client
	unregister  chan *Client
	broadcast   chan tenantBroadcast
	shutdown    chan struct{} // signals Run to begin graceful drain
	done        chan struct{} // closed when Run has finished draining
	count       atomic.Int64
	log         *logrus.Logger
	seq         *EventSequence
	buffer      *EventBuffer
}

// NewHub creates a new Hub instance.
func NewHub(log *logrus.Logger) *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		tenantCount: make(map[string]int),
		register:    make(chan *Client, registerBuffer),
		unregister:  make(chan *Client, registerBuffer),
		broadcast:   make(chan tenantBroadcast, broadcastBuffer),
		shutdown:    make(chan struct{}),
		done:        make(chan struct{}),
		log:         log,
		seq:         NewEventSequence(),
		buffer:      NewEventBuffer(defaultBufferMaxLen, defaultBufferMaxAge),
	}
}

// drainTimeout is how long the hub waits for clients to flush after shutdown.
const drainTimeout = 3 * time.Second

// Run starts the hub event loop. It should be run as a goroutine.
// It exits when Shutdown is called or the context is cancelled.
func (h *Hub) Run(ctx context.Context) { //nolint:gocognit,gocyclo,cyclop // connection-limit checks add necessary branching.
	defer close(h.done)

	for {
		select {
		case <-ctx.Done():
			h.drainClients()

			return
		case <-h.shutdown:
			h.drainClients()

			return

		case client := <-h.register:
			// Global cap.
			if len(h.clients) >= 1000 {
				h.log.Warn("global connection limit reached, dropping client")
				client.closeSend()
				continue
			}
			// Per-tenant cap using O(1) counter.
			if h.tenantCount[client.TenantID] >= 50 {
				h.log.WithField("tenant_id", client.TenantID).Warn("per-tenant connection limit reached, dropping client")
				client.closeSend()
				continue
			}
			h.clients[client] = true
			h.tenantCount[client.TenantID]++
			h.count.Store(int64(len(h.clients)))
			metrics.WSConnections.Set(float64(len(h.clients)))
			h.log.WithField("total", len(h.clients)).Info("client registered")

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.closeSend()
				h.tenantCount[client.TenantID]--
				if h.tenantCount[client.TenantID] <= 0 {
					delete(h.tenantCount, client.TenantID)
				}
			}
			h.count.Store(int64(len(h.clients)))
			metrics.WSConnections.Set(float64(len(h.clients)))
			h.log.WithField("total", len(h.clients)).Info("client unregistered")

		case b := <-h.broadcast:
			for client := range h.clients {
				if client.TenantID != b.tenantID {
					continue
				}
				select {
				case client.send <- b.msg:
				default:
					client.closeSend()
					delete(h.clients, client)
					h.tenantCount[client.TenantID]--
					if h.tenantCount[client.TenantID] <= 0 {
						delete(h.tenantCount, client.TenantID)
					}
				}
			}
			h.count.Store(int64(len(h.clients)))
		}
	}
}

// maxBroadcastPayload is the maximum allowed notification payload size (4 KB).
const maxBroadcastPayload = 4096

// BroadcastToTenant sends a message only to clients belonging to the specified tenant.
// Payloads exceeding 4 KB are dropped with a warning log.
// The actual send is performed by the Run goroutine via a channel.
func (h *Hub) BroadcastToTenant(tenantID string, msg []byte) {
	if len(msg) > maxBroadcastPayload {
		h.log.WithFields(logrus.Fields{
			"tenant_id":    tenantID,
			"payload_size": len(msg),
			"max_size":     maxBroadcastPayload,
		}).Warn("dropping oversized broadcast payload")
		return
	}
	select {
	case h.broadcast <- tenantBroadcast{tenantID: tenantID, msg: msg}:
	default:
		h.log.Warn("broadcast channel full, dropping message")
	}
}

// Register adds a client to the hub.
func (h *Hub) Register(c *Client) {
	select {
	case h.register <- c:
	default:
		h.log.Warn("register channel full, dropping client")
		c.closeSend()
	}
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	default:
		// Run loop already exited; client cleanup happened in Run shutdown.
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	return int(h.count.Load())
}

// BroadcastEvent assigns a sequence ID, stores in the buffer, and broadcasts
// a typed event to all clients of the given tenant.
func (h *Hub) BroadcastEvent(eventType, tenantID string, data json.RawMessage) {
	evt := Event{
		Type:     eventType,
		ID:       h.seq.Next(tenantID),
		TenantID: tenantID,
		Data:     data,
		Time:     time.Now(),
	}

	msg, err := json.Marshal(evt)
	if err != nil {
		h.log.WithError(err).Error("failed to marshal event")
		return
	}

	h.buffer.Append(tenantID, &evt)
	h.BroadcastToTenant(tenantID, msg)
}

// Shutdown initiates a graceful WebSocket drain: sends a shutdown frame to
// every connected client, waits for their write pumps to flush, then closes
// all connections. It blocks until drain is complete or the timeout expires.
func (h *Hub) Shutdown() {
	close(h.shutdown)
	<-h.done
}

// drainClients sends a close frame to every client and waits for buffers to flush.
func (h *Hub) drainClients() {
	if len(h.clients) == 0 {
		return
	}

	h.log.WithField("clients", len(h.clients)).Info("draining WebSocket clients")

	// Send shutdown notification so clients know to reconnect.
	shutdownMsg := []byte(`{"type":"shutdown","message":"server shutting down"}`)
	for client := range h.clients {
		select {
		case client.send <- shutdownMsg:
		default:
		}
	}

	// Wait for send buffers to empty or timeout.
	deadline := time.After(drainTimeout)
	ticker := time.NewTicker(50 * time.Millisecond) //nolint:mnd // poll interval
	defer ticker.Stop()

	for {
		allDrained := true

		for client := range h.clients {
			if len(client.send) > 0 {
				allDrained = false

				break
			}
		}

		if allDrained {
			break
		}

		select {
		case <-deadline:
			h.log.Warn("WebSocket drain timeout, closing remaining clients")

			goto closeAll
		case <-ticker.C:
		}
	}

closeAll:
	for client := range h.clients {
		client.closeSend()
		delete(h.clients, client)
	}

	h.tenantCount = make(map[string]int)
	h.count.Store(0)
	metrics.WSConnections.Set(0)
}

// ReplayEvents sends buffered events since lastEventID to the client.
// Returns false if the requested ID is too old (not in buffer).
func (h *Hub) ReplayEvents(client *Client, lastEventID uint64) bool {
	oldest := h.buffer.OldestID(client.TenantID)
	if oldest > 0 && lastEventID > 0 && lastEventID < oldest {
		return false
	}

	events := h.buffer.Since(client.TenantID, lastEventID)
	for _, evt := range events {
		msg, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		select {
		case client.send <- msg:
		default:
			return true // channel full, stop replay
		}
	}
	return true
}
