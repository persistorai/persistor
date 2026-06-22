// Package mcpengine is the transport-agnostic MCP layer for Persistor: the
// memory Engine the tools call, the tool schemas/handlers, and a NewServer
// constructor. The remote HTTP daemon (cmd/persistor-server) is a thin transport
// over this one source of truth. The Engine type, options, and constructor live
// here; the read tools (search/get/list/namespaces/brief) in engine_read.go and
// the write tools (write/delete/restore) in engine_write.go.
package mcpengine

import (
	"errors"

	"github.com/persistorai/persistor/internal/index"
)

// ErrReadOnly is returned by mutating tools when the caller's identity has the
// readonly role. It is a typed sentinel so a transport can map it to the right
// status; the message is user-facing.
var ErrReadOnly = errors.New("identity is read-only: memory_write is not permitted")

// ErrRateLimited is returned by a tool when the tenant has exceeded its
// per-tenant rate (a tighter cap on writes, a looser one on reads). The caller
// should back off and retry.
var ErrRateLimited = errors.New("rate limit exceeded for this tenant: slow down and retry")

// defaultSurface labels a write whose transport did not set one (the audit
// surface in note_versions). The remote daemon overrides it per session.
const defaultSurface = "mcp"

// Engine is the memory backend the MCP tools call. It wraps the PG-native index
// store directly, so the tools search, read, write, and brief over the same
// Postgres full-text index — no filesystem. Each Engine is bound to one tenant.
type Engine struct {
	store       *index.Store
	tenantID    string
	surface     string
	readOnly    bool
	limiter     *KeyLimiter // write/delete/restore cap (tighter)
	readLimiter *KeyLimiter // search/get/list/namespaces/brief cap (looser)
}

// EngineOption configures optional Engine behavior.
type EngineOption func(*Engine)

// WithReadOnly marks the engine read-only, rejecting mutating tools. The remote
// daemon sets this for an identity whose resolved role is "readonly".
func WithReadOnly(ro bool) EngineOption {
	return func(e *Engine) { e.readOnly = ro }
}

// WithSurface sets the audit surface recorded for every write (which client/
// identity made it). It lands in note_versions.surface, the write audit trail.
func WithSurface(surface string) EngineOption {
	return func(e *Engine) {
		if surface != "" {
			e.surface = surface
		}
	}
}

// WithWriteLimiter attaches a shared per-tenant write rate limiter. The mutating
// tools consult it before touching the store. A nil limiter (the default) means
// no limiting.
func WithWriteLimiter(l *KeyLimiter) EngineOption {
	return func(e *Engine) { e.limiter = l }
}

// WithReadLimiter attaches a shared per-tenant read rate limiter. The read tools
// (search/get/list/namespaces/brief) consult it so a single tenant cannot run
// unbounded FTS/assembly against the daemon. A nil limiter (the default) means
// no limiting — used by the local single-user CLI path.
func WithReadLimiter(l *KeyLimiter) EngineOption {
	return func(e *Engine) { e.readLimiter = l }
}

// NewEngine builds an Engine over the given store for one tenant. Writes go
// straight to Postgres (no notes dir, no roots): the tenant from the verified
// token is the only write boundary.
func NewEngine(store *index.Store, tenantID string, opts ...EngineOption) *Engine {
	e := &Engine{store: store, tenantID: tenantID, surface: defaultSurface}
	for _, opt := range opts {
		opt(e)
	}
	return e
}
