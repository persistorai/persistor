package ws

import (
	"sync"
	"time"
)

const (
	defaultBufferMaxLen = 1000
	defaultBufferMaxAge = 1 * time.Hour
)

// EventBuffer stores recent events per tenant for replay on reconnect.
type EventBuffer struct {
	mu     sync.RWMutex
	events map[string][]Event
	maxAge time.Duration
	maxLen int
	stop   chan struct{}
}

// NewEventBuffer creates an EventBuffer with the given limits and starts
// a background goroutine that removes stale tenant entries every 10 minutes.
func NewEventBuffer(maxLen int, maxAge time.Duration) *EventBuffer {
	eb := &EventBuffer{
		events: make(map[string][]Event),
		maxAge: maxAge,
		maxLen: maxLen,
		stop:   make(chan struct{}),
	}
	go eb.cleanupLoop()
	return eb
}

// Stop halts the background cleanup goroutine.
func (eb *EventBuffer) Stop() {
	close(eb.stop)
}

func (eb *EventBuffer) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-eb.stop:
			return
		case <-ticker.C:
			eb.evictStaleTenants()
		}
	}
}

func (eb *EventBuffer) evictStaleTenants() {
	cutoff := time.Now().Add(-eb.maxAge)

	eb.mu.Lock()
	defer eb.mu.Unlock()

	for tenant, buf := range eb.events {
		if len(buf) == 0 || buf[len(buf)-1].Time.Before(cutoff) {
			delete(eb.events, tenant)
		}
	}
}

// Append stores an event for potential replay, evicting old entries.
func (eb *EventBuffer) Append(tenantID string, event *Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	buf := eb.events[tenantID]

	// Evict expired events from the front.
	cutoff := time.Now().Add(-eb.maxAge)
	start := 0
	for start < len(buf) && buf[start].Time.Before(cutoff) {
		start++
	}
	if start > 0 {
		buf = buf[start:]
	}

	// Append and enforce max length.
	buf = append(buf, *event)
	if len(buf) > eb.maxLen {
		buf = buf[len(buf)-eb.maxLen:]
	}

	eb.events[tenantID] = buf
}

// Since returns all events for a tenant with ID > lastEventID.
// Returns nil if the tenant has no buffered events.
func (eb *EventBuffer) Since(tenantID string, lastEventID uint64) []Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	buf := eb.events[tenantID]
	if len(buf) == 0 {
		return nil
	}

	// Binary search for the first event with ID > lastEventID.
	lo, hi := 0, len(buf)
	for lo < hi {
		mid := (lo + hi) / 2
		if buf[mid].ID <= lastEventID {
			lo = mid + 1
		} else {
			hi = mid
		}
	}

	if lo >= len(buf) {
		return nil
	}

	// Return a copy to avoid holding the lock via slice reference.
	result := make([]Event, len(buf)-lo)
	copy(result, buf[lo:])
	return result
}

// OldestID returns the oldest buffered event ID for a tenant, or 0 if empty.
func (eb *EventBuffer) OldestID(tenantID string) uint64 {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	buf := eb.events[tenantID]
	if len(buf) == 0 {
		return 0
	}
	return buf[0].ID
}
