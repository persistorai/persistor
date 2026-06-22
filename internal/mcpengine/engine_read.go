package mcpengine

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/index"
)

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
	if !e.readLimiter.Allow(e.tenantID) {
		return SearchOutput{}, ErrRateLimited
	}
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
	if !e.readLimiter.Allow(e.tenantID) {
		return GetOutput{}, ErrRateLimited
	}
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

// ListInput is the memory_list argument shape.
type ListInput struct {
	Namespace         string `json:"namespace,omitempty" jsonschema:"Restrict to one namespace (e.g. scout, claude, work). Omit to list across all namespaces."`
	Limit             int    `json:"limit,omitempty" jsonschema:"Max notes to return (page size). Default 50, capped at 500."`
	Offset            int    `json:"offset,omitempty" jsonschema:"Number of notes to skip, for paging through large namespaces. Default 0."`
	IncludeSuperseded bool   `json:"include_superseded,omitempty" jsonschema:"Include superseded (corrected/stale) notes. Default false."`
}

// ListEntry is one note summary in a memory_list result: metadata only, no body
// (fetch the body with memory_get once you've chosen a note).
type ListEntry struct {
	ID         string `json:"id"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Tier       string `json:"tier"`
	Title      string `json:"title"`
	Version    int    `json:"version"`
	Superseded bool   `json:"superseded"`
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
func (e *Engine) List(ctx context.Context, in ListInput) (ListOutput, error) {
	if !e.readLimiter.Allow(e.tenantID) {
		return ListOutput{}, ErrRateLimited
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultListPageLimit
	}
	limit = min(limit, maxListPageLimit)
	offset := max(in.Offset, 0)
	summaries, err := e.store.ListNotes(ctx, e.tenantID, index.ListOpts{
		Namespace:         in.Namespace,
		Limit:             limit,
		Offset:            offset,
		IncludeSuperseded: in.IncludeSuperseded,
	})
	if err != nil {
		return ListOutput{}, fmt.Errorf("list: %w", err)
	}
	out := ListOutput{Notes: make([]ListEntry, len(summaries)), Count: len(summaries), Limit: limit, Offset: offset}
	for i := range summaries {
		out.Notes[i] = ListEntry{
			ID: summaries[i].ID, Namespace: summaries[i].Namespace, Kind: summaries[i].Kind,
			Tier: summaries[i].Tier, Title: summaries[i].Title, Version: summaries[i].Version,
			Superseded: summaries[i].Superseded,
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
		return NamespacesOutput{}, fmt.Errorf("namespaces: %w", err)
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
