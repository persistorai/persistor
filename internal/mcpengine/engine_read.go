package mcpengine

import (
	"context"
	"fmt"
	"time"

	"github.com/briancolinger/persistor/internal/index"
)

// SearchInput is the memory_search argument shape.
type SearchInput struct {
	Query             string `json:"query" jsonschema:"Search query: natural-language or keywords, matched against the prose notes via full-text search."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Max notes to return. Default 8, capped at 500."`
	IncludeSuperseded bool   `json:"include_superseded,omitempty" jsonschema:"Include superseded (corrected/stale) notes. Default false — retrieval returns current notes only."`
	Namespace         string `json:"namespace,omitempty" jsonschema:"Restrict results to one namespace (e.g. demo, claude, work). Omit to search all namespaces."`
	Since             string `json:"since,omitempty" jsonschema:"Only notes updated at/after this time (RFC3339 or YYYY-MM-DD). Resolve relative phrases like 'last week' to a date before calling."`
	Until             string `json:"until,omitempty" jsonschema:"Only notes updated at/before this time (RFC3339 or YYYY-MM-DD; a bare date means end of that day UTC)."`
	Kind              string `json:"kind,omitempty" jsonschema:"Restrict to one note kind: fact, decision, episode, reference, or preference. Omit for all kinds."`
}

// SearchHit is one ranked note in a search result. CreatedAt/UpdatedAt are
// RFC3339 — the temporal grounding that lets the caller answer "when did we
// decide this" without spelunking version history. Snippet is the matched
// excerpt, so relevance can be judged without a memory_get per candidate.
type SearchHit struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Kind       string  `json:"kind"`
	Tier       string  `json:"tier"`
	Rank       float64 `json:"rank"`
	Superseded bool    `json:"superseded"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	Snippet    string  `json:"snippet,omitempty"`
}

// parseTimeBound parses a since/until tool argument: RFC3339, or a bare
// YYYY-MM-DD date (interpreted as start of day UTC; endOfDay shifts it to
// 23:59:59.999… so "until: 2026-07-01" includes that whole day). Empty = nil.
func parseTimeBound(field, raw string, endOfDay bool) (*time.Time, error) {
	if raw == "" {
		return nil, nil //nolint:nilnil // nil time bound = unbounded, by contract
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q (want RFC3339 or YYYY-MM-DD)", field, raw)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return &t, nil
}

// SearchOutput is the memory_search result shape.
type SearchOutput struct {
	Results []SearchHit `json:"results"`
}

const (
	defaultSearchLimit = 8
	// maxSearchLimit caps a caller-supplied search/brief result limit so one
	// request can't force an unbounded FTS scan/sort and result serialization
	// (a single-request DoS). Mirrors maxListPageLimit's bound on memory_list.
	maxSearchLimit = 500
)

// Search runs the full-text retrieval.
func (e *Engine) Search(ctx context.Context, in *SearchInput) (SearchOutput, error) {
	if !e.readLimiter.Allow(e.tenantID) {
		return SearchOutput{}, ErrRateLimited
	}
	if in.Query == "" {
		return SearchOutput{}, fmt.Errorf("query is required")
	}
	if in.Kind != "" && !index.ValidKind(in.Kind) {
		return SearchOutput{}, fmt.Errorf("invalid kind %q", in.Kind)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	limit = min(limit, maxSearchLimit)
	since, err := parseTimeBound("since", in.Since, false)
	if err != nil {
		return SearchOutput{}, err
	}
	until, err := parseTimeBound("until", in.Until, true)
	if err != nil {
		return SearchOutput{}, err
	}
	hits, err := e.store.SearchNotes(ctx, e.tenantID, in.Query, &index.SearchOpts{
		Limit:             limit,
		IncludeSuperseded: in.IncludeSuperseded,
		Namespace:         in.Namespace,
		Since:             since,
		Until:             until,
		Kind:              in.Kind,
	})
	if err != nil {
		return SearchOutput{}, e.opError("search", err)
	}
	out := SearchOutput{Results: make([]SearchHit, len(hits))}
	surfaced := make([]string, len(hits))
	for i := range hits {
		surfaced[i] = hits[i].ID
		out.Results[i] = SearchHit{
			ID: hits[i].ID, Title: hits[i].Title, Kind: hits[i].Kind,
			Tier: hits[i].Tier, Rank: hits[i].Rank, Superseded: hits[i].Superseded,
			CreatedAt: hits[i].CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt: hits[i].UpdatedAt.UTC().Format(time.RFC3339),
			Snippet:   hits[i].Snippet,
		}
	}
	e.bumpAccess(ctx, surfaced)
	return out, nil
}

// bumpAccess records read telemetry, best-effort: losing a tick must never
// fail the read that triggered it, so errors are logged and swallowed.
func (e *Engine) bumpAccess(ctx context.Context, ids []string) {
	if len(ids) == 0 {
		return
	}
	if err := e.store.BumpAccess(ctx, e.tenantID, ids); err != nil && e.log != nil {
		e.log.WithError(err).Warn("access telemetry update failed")
	}
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
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Get fetches one note's full record by id, including its current version. A
// tombstoned note, or a note outside the requested namespace, reads as not-found.
func (e *Engine) Get(ctx context.Context, in GetInput) (GetOutput, error) {
	if !e.readLimiter.Allow(e.tenantID) {
		return GetOutput{}, ErrRateLimited
	}
	if in.ID == "" {
		return GetOutput{}, fmt.Errorf("id is required")
	}
	st, found, err := e.store.NoteState(ctx, e.tenantID, in.ID)
	if err != nil {
		return GetOutput{}, e.opError("get", err)
	}
	if !found || st.Deleted || (in.Namespace != "" && st.Namespace != in.Namespace) {
		return GetOutput{Found: false, ID: in.ID}, nil
	}
	e.bumpAccess(ctx, []string{st.ID})
	return GetOutput{
		Found: true, ID: st.ID, Namespace: st.Namespace, Kind: st.Kind, Tier: st.Tier,
		Title: st.Title, Body: st.Body, Version: st.Version,
		CreatedAt: st.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: st.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}

// ListInput is the memory_list argument shape.
type ListInput struct {
	Namespace         string `json:"namespace,omitempty" jsonschema:"Restrict to one namespace (e.g. demo, claude, work). Omit to list across all namespaces."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Max notes to return (page size). Default 50, capped at 500."`
	Offset            int    `json:"offset,omitempty" jsonschema:"Number of notes to skip, for paging through large namespaces. Default 0."`
	IncludeSuperseded bool   `json:"include_superseded,omitempty" jsonschema:"Include superseded (corrected/stale) notes. Default false."`
	Since             string `json:"since,omitempty" jsonschema:"Only notes updated at/after this time (RFC3339 or YYYY-MM-DD) — e.g. to review what changed recently."`
	Until             string `json:"until,omitempty" jsonschema:"Only notes updated at/before this time (RFC3339 or YYYY-MM-DD; a bare date means end of that day UTC)."`
	Kind              string `json:"kind,omitempty" jsonschema:"Restrict to one note kind: fact, decision, episode, reference, or preference. Omit for all kinds."`
}

// ListEntry is one note summary in a memory_list result: metadata only, no body
// (fetch the body with memory_get once you've chosen a note). Timestamps are
// RFC3339.
type ListEntry struct {
	ID         string `json:"id"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Tier       string `json:"tier"`
	Title      string `json:"title"`
	Version    int    `json:"version"`
	Superseded bool   `json:"superseded"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// ListOutput is the memory_list result shape. Limit/Offset echo the effective
// page applied (after defaulting/capping), so the caller can page correctly.
type ListOutput struct {
	Notes  []ListEntry `json:"notes"`
	Count  int         `json:"count"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

const (
	defaultListPageLimit = 50
	maxListPageLimit     = 500
)

// List enumerates the tenant's notes as summaries (no bodies), paginated and
// optionally filtered to one namespace — the browse/page path memory_search
// cannot serve, because search needs a query term. Bodies come from memory_get.
func (e *Engine) List(ctx context.Context, in *ListInput) (ListOutput, error) {
	if !e.readLimiter.Allow(e.tenantID) {
		return ListOutput{}, ErrRateLimited
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultListPageLimit
	}
	limit = min(limit, maxListPageLimit)
	offset := max(in.Offset, 0)
	if in.Kind != "" && !index.ValidKind(in.Kind) {
		return ListOutput{}, fmt.Errorf("invalid kind %q", in.Kind)
	}
	since, err := parseTimeBound("since", in.Since, false)
	if err != nil {
		return ListOutput{}, err
	}
	until, err := parseTimeBound("until", in.Until, true)
	if err != nil {
		return ListOutput{}, err
	}
	summaries, err := e.store.ListNotes(ctx, e.tenantID, index.ListOpts{
		Namespace:         in.Namespace,
		Limit:             limit,
		Offset:            offset,
		IncludeSuperseded: in.IncludeSuperseded,
		Since:             since,
		Until:             until,
		Kind:              in.Kind,
	})
	if err != nil {
		return ListOutput{}, e.opError("list", err)
	}
	out := ListOutput{Notes: make([]ListEntry, len(summaries)), Count: len(summaries), Limit: limit, Offset: offset}
	for i := range summaries {
		out.Notes[i] = ListEntry{
			ID: summaries[i].ID, Namespace: summaries[i].Namespace, Kind: summaries[i].Kind,
			Tier: summaries[i].Tier, Title: summaries[i].Title, Version: summaries[i].Version,
			Superseded: summaries[i].Superseded,
			CreatedAt:  summaries[i].CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:  summaries[i].UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	return out, nil
}

// NamespacesInput is the (empty) memory_namespaces argument shape — the tool
// takes no parameters; the tenant comes from the verified token.
type NamespacesInput struct{}

// NamespaceEntry is one namespace and its live-note count.
type NamespaceEntry struct {
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
}

// NamespacesOutput is the memory_namespaces result shape.
type NamespacesOutput struct {
	Namespaces []NamespaceEntry `json:"namespaces"`
}

// Namespaces returns the tenant's namespaces, each with its live-note count —
// the top-level map of where this tenant's memory lives. Useful to discover
// namespaces before listing or searching within one.
func (e *Engine) Namespaces(ctx context.Context) (NamespacesOutput, error) {
	if !e.readLimiter.Allow(e.tenantID) {
		return NamespacesOutput{}, ErrRateLimited
	}
	counts, err := e.store.Namespaces(ctx, e.tenantID)
	if err != nil {
		return NamespacesOutput{}, e.opError("namespaces", err)
	}
	out := NamespacesOutput{Namespaces: make([]NamespaceEntry, len(counts))}
	for i := range counts {
		out.Namespaces[i] = NamespaceEntry{Namespace: counts[i].Namespace, Count: counts[i].Count}
	}
	return out, nil
}

// BriefInput is the brief argument shape.
type BriefInput struct {
	Seed       string `json:"seed,omitempty" jsonschema:"Retrieval seed for the Tail tier (e.g. the current project/topic). Empty returns Core only."`
	Budget     int    `json:"budget,omitempty" jsonschema:"Total token budget. Default 6000."`
	CoreBudget int    `json:"core_budget,omitempty" jsonschema:"Max tokens for the Core tier. Default 2000."`
	TailLimit  int    `json:"tail_limit,omitempty" jsonschema:"Max Tail notes to consider. Default 12, capped at 500."`
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
	if !e.readLimiter.Allow(e.tenantID) {
		return BriefOutput{}, ErrRateLimited
	}
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
	opts.TailLimit = min(opts.TailLimit, maxSearchLimit)
	ws, err := index.AssembleWorkingSet(ctx, e.store, e.tenantID, in.Seed, opts)
	if err != nil {
		return BriefOutput{}, e.opError("brief", err)
	}
	return BriefOutput{
		Markdown:    index.RenderMarkdown(&ws),
		CoreTokens:  ws.CoreTokens,
		TailTokens:  ws.TailTokens,
		TotalTokens: ws.TotalTokens,
	}, nil
}
