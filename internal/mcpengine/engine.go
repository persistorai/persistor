// Package mcpengine is the transport-agnostic MCP layer for Persistor: the
// memory Engine the tools call, the tool schemas/handlers, and a NewServer
// constructor. The stdio binary (cmd/persistor-mcp) and the remote HTTP daemon
// (cmd/persistor-server) are thin transports over this one source of truth.
package mcpengine

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/persistorai/persistor/internal/index"
)

// Engine is the memory backend the MCP tools call. It wraps the local index
// store directly, so the tools search, read, write, and brief over the same
// Postgres full-text index the CLI uses.
type Engine struct {
	store    *index.Store
	indexer  *index.Indexer
	tenantID string
	roots    []index.Root
	writeDir string
}

// NewEngine builds an Engine over the given store for one tenant. roots are the
// watched note roots (for memory_write's reindex); writeDir is where memory_write
// writes new notes (default <notesDir>/memory/atomic, a watched include).
func NewEngine(store *index.Store, indexer *index.Indexer, tenantID string, roots []index.Root, writeDir string) *Engine {
	return &Engine{store: store, indexer: indexer, tenantID: tenantID, roots: roots, writeDir: writeDir}
}

// SearchInput is the memory_search argument shape.
type SearchInput struct {
	Query             string `json:"query" jsonschema:"Search query: natural-language or keywords, matched against the prose notes via full-text search."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Max notes to return. Default 8."`
	IncludeSuperseded bool   `json:"include_superseded,omitempty" jsonschema:"Include superseded (corrected/stale) notes. Default false — retrieval returns current notes only."`
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
	ID string `json:"id" jsonschema:"The note id to fetch (as returned by memory_search)."`
}

// GetOutput is the memory_get result shape (the full note body).
type GetOutput struct {
	Found      bool   `json:"found"`
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Tier       string `json:"tier"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	SourcePath string `json:"source_path"`
}

// Get fetches one note's full record by id.
func (e *Engine) Get(ctx context.Context, in GetInput) (GetOutput, error) {
	if in.ID == "" {
		return GetOutput{}, fmt.Errorf("id is required")
	}
	recs, err := e.store.LoadNotes(ctx, e.tenantID, []string{in.ID})
	if err != nil {
		return GetOutput{}, fmt.Errorf("get: %w", err)
	}
	if len(recs) == 0 {
		return GetOutput{Found: false, ID: in.ID}, nil
	}
	r := recs[0]
	return GetOutput{
		Found: true, ID: r.ID, Kind: r.Kind, Tier: r.Tier,
		Title: r.Title, Body: r.Body, SourcePath: r.SourcePath,
	}, nil
}

// WriteInput is the memory_write argument shape — one note of a consolidation
// plan.
type WriteInput struct {
	Path       string   `json:"path" jsonschema:"File path for the note, relative to the notes dir, ending in .md (no absolute paths or ..)."`
	Body       string   `json:"body" jsonschema:"The note's markdown prose. Do not include a frontmatter block; it is rendered from the typed fields."`
	ID         string   `json:"id,omitempty" jsonschema:"Stable note id. Derived from the path when omitted."`
	Kind       string   `json:"kind,omitempty" jsonschema:"fact|decision|episode|reference|preference. Default fact."`
	Tier       string   `json:"tier,omitempty" jsonschema:"core (always-loaded) or tail (retrieved). Default tail."`
	Title      string   `json:"title,omitempty" jsonschema:"Short title. Derived from the first heading when omitted."`
	Supersedes string   `json:"supersedes,omitempty" jsonschema:"Id of the note this CORRECTS. Use only to replace a stale fact, not for a new point in a timeline."`
	Links      []string `json:"links,omitempty" jsonschema:"Optional related note ids."`
}

// WriteOutput is the memory_write result shape.
type WriteOutput struct {
	Written    string `json:"written"`    // relative path written
	Notes      int    `json:"notes"`      // total notes after the reindex
	Superseded int    `json:"superseded"` // supersession flips this reindex
}

// Write applies a single-note consolidation plan: write the prose file, then
// reindex (which reconciles supersession).
func (e *Engine) Write(ctx context.Context, in *WriteInput) (WriteOutput, error) {
	// Reject a self-supersede: a write whose supersedes resolves to the same note
	// it lands on forks no history and would otherwise silently do nothing.
	if err := index.CheckSelfSupersede(e.roots, e.writeDir, in.Path, in.ID, in.Supersedes); err != nil {
		return WriteOutput{}, err
	}
	// Reject a supersede of a non-existent note. Otherwise the reconcile marks
	// nothing and the new note is still written with a dangling supersedes pointer
	// — a silent no-op that a prompt-injected write could use to fake a correction.
	if in.Supersedes != "" {
		exists, err := e.store.NoteExists(ctx, e.tenantID, in.Supersedes)
		if err != nil {
			return WriteOutput{}, fmt.Errorf("checking supersedes target: %w", err)
		}
		if !exists {
			return WriteOutput{}, fmt.Errorf("supersedes target %q does not exist", in.Supersedes)
		}
	}
	plan := &index.Plan{Notes: []index.PlanNote{{
		ID: in.ID, Path: in.Path, Kind: in.Kind, Tier: in.Tier,
		Title: in.Title, Supersedes: in.Supersedes, Links: in.Links, Body: in.Body,
	}}}
	written, err := index.ApplyPlan(plan, e.writeDir)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("write: %w", err)
	}
	// Index just the file we wrote rather than walking and hashing every watched
	// file; supersession is still reconciled corpus-wide inside IndexPath.
	abs := filepath.Join(e.writeDir, filepath.FromSlash(written[0]))
	rep, err := e.indexer.IndexPath(ctx, e.tenantID, e.roots, abs)
	if err != nil {
		return WriteOutput{}, fmt.Errorf("indexing after write: %w", err)
	}
	return WriteOutput{Written: written[0], Notes: rep.Notes, Superseded: rep.Superseded}, nil
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

// DefaultWriteDir is where memory_write writes when no override is given.
func DefaultWriteDir(notesDir string) string {
	return filepath.Join(notesDir, "memory", "atomic")
}
