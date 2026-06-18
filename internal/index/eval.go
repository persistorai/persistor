package index

import (
	"context"

	"github.com/briancolinger/persistor/internal/eval"
)

// NoteSearcher adapts the full-text retrieval to the eval.SearchClient
// interface so the deterministic eval harness (recall@k, per-category) can score
// it. It also models the baselines via SearchOpts:
//   - persistor: opts{} (every note)
//   - static MEMORY.md: opts{Tier:"core"} (the always-loaded surface only)
//   - no memory: disabled (retrieves nothing)
type NoteSearcher struct {
	store    *Store
	tenantID string
	opts     SearchOpts
	disabled bool
}

// AllNotesSearcher scores the full index — the system under test.
func AllNotesSearcher(store *Store, tenantID string) *NoteSearcher {
	return &NoteSearcher{store: store, tenantID: tenantID}
}

// StaticMemorySearcher models "today's static MEMORY.md" baseline: retrieval is
// limited to the always-loaded Core tier, the analogue of what a static
// always-loaded surface can answer from.
func StaticMemorySearcher(store *Store, tenantID string) *NoteSearcher {
	return &NoteSearcher{store: store, tenantID: tenantID, opts: SearchOpts{Tier: tierCore}}
}

// NoMemorySearcher models the cold-agent baseline: no retrieval at all.
func NoMemorySearcher() *NoteSearcher {
	return &NoteSearcher{disabled: true}
}

// FullText runs the full-text retrieval and maps notes to eval.Node
// (ID = note id, Label = title, Type = kind) so the eval harness can match
// expected note ids.
func (ns *NoteSearcher) FullText(ctx context.Context, query string, opts *eval.SearchOptions) ([]eval.Node, error) {
	if ns.disabled {
		return nil, nil
	}
	searchOpts := ns.opts
	searchOpts.Limit = 5
	if opts != nil && opts.Limit > 0 {
		searchOpts.Limit = opts.Limit
	}
	hits, err := ns.store.SearchNotes(ctx, ns.tenantID, query, searchOpts)
	if err != nil {
		return nil, err
	}
	nodes := make([]eval.Node, len(hits))
	for i := range hits {
		nodes[i] = eval.Node{ID: hits[i].ID, Label: hits[i].Title, Type: hits[i].Kind}
	}
	return nodes, nil
}
