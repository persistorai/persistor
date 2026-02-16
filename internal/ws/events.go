package ws

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// Event is the structured message sent to WebSocket clients.
type Event struct {
	Type     string          `json:"type"`
	ID       uint64          `json:"id"`
	TenantID string          `json:"-"`
	Data     json.RawMessage `json:"data"`
	Time     time.Time       `json:"time"`
}

// SubscribeMsg is sent by the client on connect to request event replay.
type SubscribeMsg struct {
	Type        string `json:"type"`
	LastEventID uint64 `json:"last_event_id"`
}

// ResetMsg tells the client to do a full refresh (requested events too old).
type ResetMsg struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// EventSequence tracks monotonic event IDs per tenant.
type EventSequence struct {
	mu       sync.Mutex
	counters map[string]*atomic.Uint64
}

// NewEventSequence creates a new EventSequence.
func NewEventSequence() *EventSequence {
	return &EventSequence{
		counters: make(map[string]*atomic.Uint64),
	}
}

// Next returns the next sequence number for a tenant.
func (es *EventSequence) Next(tenantID string) uint64 {
	es.mu.Lock()
	counter, ok := es.counters[tenantID]
	if !ok {
		counter = &atomic.Uint64{}
		es.counters[tenantID] = counter
	}
	es.mu.Unlock()

	return counter.Add(1)
}
