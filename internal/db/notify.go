package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
)

// validChannel matches safe PostgreSQL LISTEN channel names.
var validChannel = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const (
	listenChannel     = "kg_changes"
	initialBackoff    = 1 * time.Second
	maxBackoff        = 30 * time.Second
	backoffMultiplier = 2
)

// Broadcaster sends messages to connected clients.
type Broadcaster interface {
	BroadcastToTenant(tenantID string, msg []byte)
	BroadcastEvent(eventType, tenantID string, data json.RawMessage)
}

// NotifyBridge subscribes to PostgreSQL LISTEN/NOTIFY on the kg_changes
// channel and forwards each payload to the WebSocket hub.
type NotifyBridge struct {
	log  *logrus.Logger
	pool *dbpool.Pool
	hub  Broadcaster
}

// NewNotifyBridge creates a NotifyBridge wired to the given pool and hub.
func NewNotifyBridge(log *logrus.Logger, pool *dbpool.Pool, hub Broadcaster) *NotifyBridge {
	return &NotifyBridge{
		log:  log,
		pool: pool,
		hub:  hub,
	}
}

// Start launches the LISTEN/NOTIFY loop in a background goroutine.
// It verifies the initial connection before returning. If the initial
// LISTEN fails, it returns an error. The background goroutine handles
// reconnection for subsequent failures.
func (b *NotifyBridge) Start(ctx context.Context) error {
	if !validChannel.MatchString(listenChannel) {
		return fmt.Errorf("notify bridge: invalid channel name %q", listenChannel)
	}

	if err := b.pool.Ping(ctx); err != nil {
		return fmt.Errorf("notify bridge: database not reachable: %w", err)
	}

	go b.listen(ctx)

	return nil
}

// listen is the main loop that acquires a connection, subscribes to the
// channel, and processes notifications until the context is cancelled.
func (b *NotifyBridge) listen(ctx context.Context) {
	backoff := initialBackoff

	for {
		if ctx.Err() != nil {
			return
		}

		err := b.subscribeAndForward(ctx)
		if err == nil || ctx.Err() != nil {
			return
		}

		b.log.WithError(err).WithField("retry_in", backoff).
			Warn("notify bridge connection lost, reconnecting")

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff = nextBackoff(backoff)
	}
}

// subscribeAndForward acquires a connection, issues LISTEN, and blocks on
// notifications until the connection fails or the context is cancelled.
func (b *NotifyBridge) subscribeAndForward(ctx context.Context) error {
	conn, err := b.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Release()

	// LISTEN requires the channel name inline (not a parameter), so we use
	// pgx.Identifier to safely quote/sanitize the channel name.
	sanitizedChannel := pgx.Identifier{listenChannel}.Sanitize()
	if _, err := conn.Exec(ctx, "LISTEN "+sanitizedChannel); err != nil {
		return fmt.Errorf("executing LISTEN: %w", err)
	}

	b.log.WithField("channel", listenChannel).Info("notify bridge listening")

	for {
		// Set a 2-minute read deadline so we periodically check ctx cancellation.
		if err := conn.Conn().PgConn().Conn().SetReadDeadline(time.Now().Add(2 * time.Minute)); err != nil {
			return fmt.Errorf("setting read deadline: %w", err)
		}

		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// On timeout, loop back to check context and retry.
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}

			return fmt.Errorf("waiting for notification: %w", err)
		}

		b.handleNotification(notification)
	}
}

// handleNotification forwards a single PG notification payload to the hub.
// Handles both statement-level payloads (with "count") and legacy per-row
// payloads (with "id") for backward compatibility.
func (b *NotifyBridge) handleNotification(n *pgconn.Notification) {
	b.log.WithFields(logrus.Fields{
		"channel": n.Channel,
		"pid":     n.PID,
	}).Debug("notification received")

	var payload struct {
		TenantID string `json:"tenant_id"`
		Type     string `json:"type,omitempty"`
		Count    *int64 `json:"count,omitempty"`
	}
	if err := json.Unmarshal([]byte(n.Payload), &payload); err != nil || payload.TenantID == "" {
		b.log.Warn("dropping notification without tenant_id")
		return
	}

	if payload.Count != nil {
		b.log.WithField("count", *payload.Count).Debug("statement-level notification")
	}

	eventType := payload.Type
	if eventType == "" {
		eventType = "kg.change"
	}

	b.hub.BroadcastEvent(eventType, payload.TenantID, json.RawMessage(n.Payload))
}

// nextBackoff doubles the current backoff duration with random jitter (±25%),
// capped at maxBackoff. Jitter prevents thundering herd on reconnect.
func nextBackoff(current time.Duration) time.Duration {
	next := current * backoffMultiplier
	if next > maxBackoff {
		next = maxBackoff
	}

	// Add ±25% jitter.
	jitter := float64(next) * (0.75 + rand.Float64()*0.5) //nolint:gosec // jitter doesn't need crypto rand.

	return time.Duration(jitter)
}
