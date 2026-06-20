// Package mcpengine is the transport-agnostic MCP layer for Persistor: the
// memory Engine the tools call, the tool schemas/handlers, and a NewServer
// constructor. The remote HTTP daemon (cmd/persistor-server) is a thin transport
// over this one source of truth.
package mcpengine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/persistorai/persistor/internal/index"
)

// ErrReadOnly is returned by mutating tools when the caller's identity has the
// readonly role. It is a typed sentinel so a transport can map it to the right
// status; the message is user-facing.
var ErrReadOnly = errors.New("identity is read-only: memory_write is not permitted")

// ErrRateLimited is returned by mutating tools when the tenant has exceeded its
// per-tenant write rate. The caller should back off and retry.
var ErrRateLimited = errors.New("write rate limit exceeded for this tenant: slow down and retry")

// defaultSurface labels a write whose transport did not set one (the audit
// surface in note_versions). The remote daemon overrides it per session.
const defaultSurface = "mcp"

// Engine is the memory backend the MCP tools call. It wraps the PG-native index
// store directly, so the tools search, read, write, and brief over the same
// Postgres full-text index — no filesystem. Each Engine is bound to one tenant.
type Engine struct {
	store    *index.Store
	tenantID string
	surface  string
	readOnly bool
	limiter  *WriteLimiter
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
func WithWriteLimiter(l *WriteLimiter) EngineOption {
	return func(e *Engine) { e.limiter = l }
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

// SearchInput is the memory_search argument shape.
type SearchInput struct {
	Query             string `json:"query" jsonschema:"Search query: natural-language or keywords, matched against the prose notes via full-text search."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Max notes to return. Default 8."`
	IncludeSuperseded bool   `json:"include_superseded,omitempty" jsonschema:"Include superseded (corrected/stale) notes. Default false — retrieval returns current notes only."`
	Namespace         string `json:"namespace,omitempty" jsonschema:"Restrict results to one namespace (e.g. scout, claude, work). Omit to search all namespaces."`
}

// SearchHit is one ranked note in a search result.
type SearchHit struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Kind       string  `json:"kind"`
	Tier       string  `json:"tier"`
	Rank       float64 `json:"rank"`
	Superseded bool    `json:"superseded"`
}

// SearchOutput is the memory_search result shape.
type SearchOutput struct {
	Results []SearchHit `json:"results"`
}

const defaultSearchLimit = 8

// Search runs the full-text retrieval.
func (e *Engine) Search(ctx context.Context, in SearchInput) (SearchOutput, error) {
	if in.Query == "" {
		return SearchOutput{}, fmt.Errorf("query is required")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	hits, err := e.store.SearchNotes(ctx, e.tenantID, in.Query, index.SearchOpts{
		Limit:             limit,
		IncludeSuperseded: in.IncludeSuperseded,
		Namespace:         in.Namespace,
	})
	if err != nil {
		return SearchOutput{}, fmt.Errorf("search: %w", err)
	}
	out := SearchOutput{Results: make([]SearchHit, len(hits))}
	for i := range hits {
		out.Results[i] = SearchHit{
			ID: hits[i].ID, Title: hits[i].Title, Kind: hits[i].Kind,
			Tier: hits[i].Tier, Rank: hits[i].Rank, Superseded: hits[i].Superseded,
		}
	}
	return out, nil
}

// GetInput is the memory_get argument shape.
type GetInput struct {
	ID        string `json:"id" jsonschema:"The note id to fetch (as returned by memory_search)."`
	Namespace string `json:"namespace,omitempty" jsonschema:"Optional namespace guard — if set, the note is returned only when it belongs to this namespace."`
}

// GetOutput is the memory_get result shape (the full note body). Version is the
// note's current version — pass it back as expected_version on a memory_write
// update or a memory_delete to get optimistic-concurrency safety.
type GetOutput struct {
	Found     bool   `json:"found"`
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Tier      string `json:"tier"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Version   int    `json:"version"`
}

// Get fetches one note's full record by id, including its current version. A
// tombstoned note, or a note outside the requested namespace, reads as not-found.
func (e *Engine) Get(ctx context.Context, in GetInput) (GetOutput, error) {
	if in.ID == "" {
		return GetOutput{}, fmt.Errorf("id is required")
	}
	st, found, err := e.store.NoteState(ctx, e.tenantID, in.ID)
	if err != nil {
		return GetOutput{}, fmt.Errorf("get: %w", err)
	}
	if !found || st.Deleted || (in.Namespace != "" && st.Namespace != in.Namespace) {
		return GetOutput{Found: false, ID: in.ID}, nil
	}
	return GetOutput{
		Found: true, ID: st.ID, Namespace: st.Namespace, Kind: st.Kind, Tier: st.Tier,
		Title: st.Title, Body: st.Body, Version: st.Version,
	}, nil
}

// WriteInput is the memory_write argument shape — one durable note.
type WriteInput struct {
	Path            string   `json:"path,omitempty" jsonschema:"Optional .md path used to derive the note id when id is omitted (no absolute paths or ..). The note is stored in Postgres, not as a file."`
	Body            string   `json:"body" jsonschema:"The note's markdown prose. Do not include a frontmatter block; the typed fields carry the metadata."`
	ID              string   `json:"id,omitempty" jsonschema:"Stable note id. Derived from the path (namespace-prefixed) when omitted."`
	Namespace       string   `json:"namespace,omitempty" jsonschema:"Logical bucket for the note (e.g. scout, claude, work, personal). Defaults to 'default'."`
	Kind            string   `json:"kind,omitempty" jsonschema:"fact|decision|episode|reference|preference. Default fact."`
	Tier            string   `json:"tier,omitempty" jsonschema:"core (always-loaded) or tail (retrieved). Default tail."`
	Title           string   `json:"title,omitempty" jsonschema:"Short title. Derived from the first heading when omitted."`
	Supersedes      string   `json:"supersedes,omitempty" jsonschema:"Id of the note this CORRECTS. Use only to replace a stale fact, not for a new point in a timeline."`
	ExpectedVersion int      `json:"expected_version,omitempty" jsonschema:"Optimistic-concurrency guard. Omit (or 0) to CREATE a new note; pass the current version (from memory_get) to UPDATE an existing one. A mismatch is rejected as a version conflict."`
	Links           []string `json:"links,omitempty" jsonschema:"Optional related note ids (reserved)."`
}

// WriteOutput is the memory_write result shape.
type WriteOutput struct {
	ID         string `json:"id"`         // the note id written
	Version    int    `json:"version"`    // the note's new version
	Op         string `json:"op"`         // create | update
	Superseded int    `json:"superseded"` // supersession flips this write reconciled
}

// Write creates or updates a single PG-native note under optimistic concurrency,
// then reconciles supersession across the tenant. The tenant comes from the
// verified token, so the write is tenant-isolated with no shared directory.
func (e *Engine) Write(ctx context.Context, in *WriteInput) (WriteOutput, error) {
	if e.readOnly {
		return WriteOutput{}, ErrReadOnly
	}
	if !e.limiter.Allow(e.tenantID) {
		return WriteOutput{}, ErrRateLimited
	}
	if strings.TrimSpace(in.Body) == "" {
		return WriteOutput{}, fmt.Errorf("body is required")
	}
	if !index.ValidKind(in.Kind) {
		return WriteOutput{}, fmt.Errorf("invalid kind %q", in.Kind)
	}
	if !index.ValidTier(in.Tier) {
		return WriteOutput{}, fmt.Errorf("invalid tier %q (want core|tail)", in.Tier)
	}
	id, err := index.DeriveNoteID(in.Namespace, in.Path, in.ID)
	if err != nil {
		return WriteOutput{}, err
	}
	// Reject a self-supersede: a write whose supersedes is the very id it lands on
	// forks no history and would otherwise silently do nothing.
	if in.Supersedes != "" && in.Supersedes == id {
		return WriteOutput{}, &index.SelfSupersedeError{ID: id, Path: in.Path}
	}
	// Reject a supersede of a non-existent note. Otherwise the reconcile marks
	// nothing and the new note is still written with a dangling supersedes pointer
	// — a silent no-op a prompt-injected write could use to fake a correction.
	if in.Supersedes != "" {
		exists, err := e.store.NoteExists(ctx, e.tenantID, in.Supersedes)
		if err != nil {
			return WriteOutput{}, fmt.Errorf("checking supersedes target: %w", err)
		}
		if !exists {
			return WriteOutput{}, fmt.Errorf("supersedes target %q does not exist", in.Supersedes)
		}
	}

	res, err := e.store.WriteNote(ctx, e.tenantID, &index.PGNoteInput{
		ID:         id,
		Namespace:  in.Namespace,
		Kind:       in.Kind,
		Tier:       in.Tier,
		Title:      index.DeriveTitle(in.Title, in.Body, id),
		Body:       in.Body,
		Supersedes: in.Supersedes,
		Surface:    e.surface,
	}, in.ExpectedVersion)
	if err != nil {
		return WriteOutput{}, err
	}
	superseded, err := e.store.ReconcileSupersessions(ctx, e.tenantID)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("reconciling supersessions: %w", err)
	}
	return WriteOutput{ID: res.ID, Version: res.Version, Op: res.Op, Superseded: int(superseded)}, nil
}

// DeleteInput is the memory_delete argument shape.
type DeleteInput struct {
	ID              string `json:"id" jsonschema:"The note id to delete (tombstone)."`
	ExpectedVersion int    `json:"expected_version" jsonschema:"The note's current version (from memory_get). Guards against deleting a note that changed under you."`
}

// MutationOutput is the result shape for delete/restore.
type MutationOutput struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Op      string `json:"op"` // delete | restore
}

// Delete tombstones a note: history is preserved (memory_restore can undo it) but
// it leaves live retrieval. Optimistic via expected_version.
func (e *Engine) Delete(ctx context.Context, in DeleteInput) (MutationOutput, error) {
	if e.readOnly {
		return MutationOutput{}, ErrReadOnly
	}
	if !e.limiter.Allow(e.tenantID) {
		return MutationOutput{}, ErrRateLimited
	}
	if in.ID == "" {
		return MutationOutput{}, fmt.Errorf("id is required")
	}
	res, err := e.store.DeleteNote(ctx, e.tenantID, in.ID, in.ExpectedVersion, e.surface)
	if err != nil {
		return MutationOutput{}, err
	}
	if _, err := e.store.ReconcileSupersessions(ctx, e.tenantID); err != nil {
		return MutationOutput{}, fmt.Errorf("reconciling supersessions: %w", err)
	}
	return MutationOutput{ID: res.ID, Version: res.Version, Op: res.Op}, nil
}

// RestoreInput is the memory_restore argument shape.
type RestoreInput struct {
	ID              string `json:"id" jsonschema:"The note id to restore."`
	TargetVersion   int    `json:"target_version,omitempty" jsonschema:"The history version to restore. Omit (or 0) for the most recent non-delete version — the usual undo."`
	ExpectedVersion int    `json:"expected_version" jsonschema:"The note's current version (from memory_get). Guards against restoring over a concurrent change."`
}

// Restore copies a prior version's content forward as a new version — the undo
// for an accidental delete or a bad overwrite.
func (e *Engine) Restore(ctx context.Context, in RestoreInput) (MutationOutput, error) {
	if e.readOnly {
		return MutationOutput{}, ErrReadOnly
	}
	if !e.limiter.Allow(e.tenantID) {
		return MutationOutput{}, ErrRateLimited
	}
	if in.ID == "" {
		return MutationOutput{}, fmt.Errorf("id is required")
	}
	res, err := e.store.RestoreNote(ctx, e.tenantID, in.ID, in.TargetVersion, in.ExpectedVersion, e.surface)
	if err != nil {
		return MutationOutput{}, err
	}
	if _, err := e.store.ReconcileSupersessions(ctx, e.tenantID); err != nil {
		return MutationOutput{}, fmt.Errorf("reconciling supersessions: %w", err)
	}
	return MutationOutput{ID: res.ID, Version: res.Version, Op: res.Op}, nil
}

// BriefInput is the brief argument shape.
type BriefInput struct {
	Seed       string `json:"seed,omitempty" jsonschema:"Retrieval seed for the Tail tier (e.g. the current project/topic). Empty returns Core only."`
	Budget     int    `json:"budget,omitempty" jsonschema:"Total token budget. Default 6000."`
	CoreBudget int    `json:"core_budget,omitempty" jsonschema:"Max tokens for the Core tier. Default 2000."`
	TailLimit  int    `json:"tail_limit,omitempty" jsonschema:"Max Tail notes to consider. Default 12."`
}

// BriefOutput is the brief result shape.
type BriefOutput struct {
	Markdown    string `json:"markdown"`
	CoreTokens  int    `json:"core_tokens"`
	TailTokens  int    `json:"tail_tokens"`
	TotalTokens int    `json:"total_tokens"`
}

// Brief assembles the bounded working-set for the given seed.
func (e *Engine) Brief(ctx context.Context, in BriefInput) (BriefOutput, error) {
	opts := index.BriefOptions{Budget: in.Budget, CoreBudget: in.CoreBudget, TailLimit: in.TailLimit}
	if opts.Budget <= 0 {
		opts.Budget = 6000
	}
	if opts.CoreBudget <= 0 {
		opts.CoreBudget = 2000
	}
	if opts.TailLimit <= 0 {
		opts.TailLimit = 12
	}
	ws, err := index.AssembleWorkingSet(ctx, e.store, e.tenantID, in.Seed, opts)
	if err != nil {
		return BriefOutput{}, fmt.Errorf("brief: %w", err)
	}
	return BriefOutput{
		Markdown:    index.RenderMarkdown(&ws),
		CoreTokens:  ws.CoreTokens,
		TailTokens:  ws.TailTokens,
		TotalTokens: ws.TotalTokens,
	}, nil
}
