package ws

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/sirupsen/logrus"
)

const (
	writeTimeout         = 10 * time.Second
	wsReadLimit          = 4096
	clientSendBuffer     = 256
	maxConnLifetime      = 4 * time.Hour    // safety-net lifetime (token refresh handles auth)
	tokenRefreshInterval = 15 * time.Minute // periodic re-validation of API key
	tokenRefreshTimeout  = 10 * time.Second
	pingInterval         = 30 * time.Second
	pingTimeout          = 10 * time.Second
	maxMissedPongs       = int32(2)
)

// TenantValidator validates that an API key still maps to a valid tenant.
type TenantValidator interface {
	GetTenantByAPIKey(ctx context.Context, apiKey string) (string, error)
}

// Client wraps a single WebSocket connection managed by the Hub.
type Client struct {
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	log         *logrus.Logger
	TenantID    string
	apiKey      string
	validator   TenantValidator
	closeOnce   sync.Once
	connectedAt time.Time
}

// closeSend safely closes the send channel exactly once.
func (c *Client) closeSend() {
	c.closeOnce.Do(func() { close(c.send) })
}

// NewClient creates a new Client for the given WebSocket connection.
func NewClient(hub *Hub, conn *websocket.Conn, validator TenantValidator, apiKey string) *Client {
	return &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, clientSendBuffer),
		log:         hub.log,
		apiKey:      apiKey,
		validator:   validator,
		connectedAt: time.Now(),
	}
}

// ReadPump reads messages from the WebSocket connection until it closes.
// The first message may be a subscribe request for event replay.
func (c *Client) ReadPump(ctx context.Context) {
	defer func() {
		c.hub.Unregister(c)
		c.conn.CloseNow() //nolint:errcheck // best-effort close on teardown
	}()

	c.conn.SetReadLimit(wsReadLimit)

	for {
		_, msgBytes, err := c.conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				c.log.WithField("status", websocket.CloseStatus(err)).Debug("client disconnected")
			}

			return
		}

		c.handleMessage(ctx, msgBytes)
	}
}

// sendPing sends a WebSocket ping and tracks missed pongs.
// Returns true if the connection should be closed.
func (c *Client) sendPing(ctx context.Context, missedPongs *atomic.Int32) bool {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err := c.conn.Ping(pingCtx)
	cancel()

	if err != nil {
		if missedPongs.Add(1) >= maxMissedPongs {
			c.log.Debug("closing: 2 consecutive missed pongs")

			return true
		}

		return false
	}

	missedPongs.Store(0)

	return false
}

// handleMessage processes an incoming client message.
func (c *Client) handleMessage(_ context.Context, msgBytes []byte) {
	var msg struct {
		Type        string `json:"type"`
		LastEventID uint64 `json:"last_event_id"`
	}
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		return
	}

	if msg.Type != "subscribe" {
		return
	}

	if !c.hub.ReplayEvents(c, msg.LastEventID) {
		resetMsg, err := json.Marshal(ResetMsg{
			Type:   "reset",
			Reason: "requested events no longer available, perform full refresh",
		})
		if err != nil {
			return
		}
		select {
		case c.send <- resetMsg:
		default:
		}
	}
}

// WritePump writes messages from the send channel to the WebSocket connection.
// It enforces a maximum connection lifetime and periodically re-validates the API key.
func (c *Client) WritePump(ctx context.Context) {
	defer c.conn.CloseNow() //nolint:errcheck // best-effort close on teardown

	lifetimeTimer := time.NewTimer(time.Until(c.connectedAt.Add(maxConnLifetime)))
	defer lifetimeTimer.Stop()

	refreshTicker := time.NewTicker(tokenRefreshInterval)
	defer refreshTicker.Stop()

	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	var missedPongs atomic.Int32

	for {
		select {
		case <-pingTicker.C:
			if c.sendPing(ctx, &missedPongs) {
				return
			}
		case msg, ok := <-c.send:
			if !ok {
				return
			}

			writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)

			err := c.conn.Write(writeCtx, websocket.MessageText, msg)

			cancel()

			if err != nil {
				c.log.WithError(err).Debug("write failed")

				return
			}
		case <-refreshTicker.C:
			if !c.refreshToken(ctx) {
				return
			}
		case <-lifetimeTimer.C:
			c.log.Info("closing WebSocket: max connection lifetime exceeded")
			c.conn.Close(websocket.StatusNormalClosure, "max connection lifetime exceeded") //nolint:errcheck // best-effort

			return
		}
	}
}

// refreshToken re-validates the API key. Returns true if valid, false if the connection should close.
func (c *Client) refreshToken(ctx context.Context) bool {
	if c.validator == nil {
		return true
	}

	refreshCtx, cancel := context.WithTimeout(ctx, tokenRefreshTimeout)
	_, err := c.validator.GetTenantByAPIKey(refreshCtx, c.apiKey)
	cancel()

	if err != nil {
		c.log.Info("closing WebSocket: token refresh failed")
		c.conn.Close(websocket.StatusPolicyViolation, "authentication expired") //nolint:errcheck // best-effort

		return false
	}

	return true
}
