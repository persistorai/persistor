package index_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestApplyPlan_WritesProseNotes is the deterministic-apply unit test: a plan
// renders the expected frontmatter + body, derives clean relative paths, and is
// re-readable by ParseNote (round-trip). No DB needed.
func TestApplyPlan_WritesProseNotes(t *testing.T) {
	dir := t.TempDir()
	plan := &index.Plan{Notes: []index.PlanNote{
		{
			Path: "weight-2026.md", Kind: "fact", Tier: "tail",
			Title: "Weight 2026", Supersedes: "scout:weight",
			Body: "# Weight 2026\n\nWeighs 230 pounds now.",
		},
		{
			ID: "scout:core-summary", Path: "core/summary.md", Tier: "core",
			Body: "# Core Summary\n\nThe pinned always-loaded summary.",
		},
	}}

	written, err := index.ApplyPlan(plan, dir)
	if err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	if len(written) != 2 || written[0] != "weight-2026.md" || written[1] != "core/summary.md" {
		t.Fatalf("written = %v, want [weight-2026.md core/summary.md]", written)
	}

	// First note: frontmatter carries the typed fields, body follows.
	raw, err := os.ReadFile(filepath.Join(dir, "weight-2026.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(raw)
	for _, want := range []string{"kind: fact", "tier: tail", "supersedes: scout:weight", "Weighs 230 pounds now."} {
		mustContain(t, content, want)
	}

	// Round-trip: ParseNote reads back the tier + supersedes the plan wrote.
	n := index.ParseNote("scout", "weight-2026.md", content, false)
	if n.Tier != "tail" || n.Supersedes != "scout:weight" {
		t.Errorf("parsed note = %+v, want tier=tail supersedes=scout:weight", n)
	}

	// A core-tier note is written with tier: core in frontmatter, so Core can be a
	// small generated summary note rather than raw MEMORY.md.
	coreRaw, err := os.ReadFile(filepath.Join(dir, "core", "summary.md"))
	if err != nil {
		t.Fatalf("read core: %v", err)
	}
	mustContain(t, string(coreRaw), "tier: core")
}

// TestApplyPlan_RejectsBadPlans: validation fails loud and writes nothing.
func TestApplyPlan_RejectsBadPlans(t *testing.T) {
	cases := []struct {
		name string
		note index.PlanNote
	}{
		{"absolute path", index.PlanNote{Path: "/etc/passwd.md", Body: "x"}},
		{"traversal", index.PlanNote{Path: "../escape.md", Body: "x"}},
		{"not markdown", index.PlanNote{Path: "note.txt", Body: "x"}},
		{"empty body", index.PlanNote{Path: "note.md", Body: "  \n"}},
		{"bad kind", index.PlanNote{Path: "note.md", Kind: "nonsense", Body: "x"}},
		{"bad tier", index.PlanNote{Path: "note.md", Tier: "middle", Body: "x"}},
		{"id with uppercase", index.PlanNote{Path: "note.md", ID: "Big-Jerry", Body: "x"}},
		{"id with space", index.PlanNote{Path: "note.md", ID: "big jerry", Body: "x"}},
		{"id with slash", index.PlanNote{Path: "note.md", ID: "scout/big-jerry", Body: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := index.ApplyPlan(&index.Plan{Notes: []index.PlanNote{tc.note}}, dir); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			// Nothing should have been written.
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Errorf("bad plan wrote files: %v", entries)
			}
		})
	}
}

// TestConsolidate_RoundTrip checks the consolidation round-trip: apply a plan that adds a fact and
// a superseding correction, reindex, and confirm the correction is retrievable
// while the stale note is hidden from default retrieval — the full
// consolidation round-trip through the indexer.
func TestConsolidate_RoundTrip(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Seed an existing note via a plan, index it.
	seed := &index.Plan{Notes: []index.PlanNote{
		{ID: "scout:weight", Path: "weight.md", Body: "# Weight\n\nWeighs 250 pounds at the desk."},
	}}
	if _, err := index.ApplyPlan(seed, dir); err != nil {
		t.Fatalf("apply seed: %v", err)
	}
	roots := []index.Root{{Name: "scout", Dir: dir}}
	if _, err := ix.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("reindex seed: %v", err)
	}

	// Consolidation: a correction supersedes the old note.
	correction := &index.Plan{Notes: []index.PlanNote{
		{ID: "scout:weight-2026", Path: "weight-2026.md", Supersedes: "scout:weight",
			Body: "# Weight 2026\n\nWeighs 230 pounds now."},
	}}
	if _, err := index.ApplyPlan(correction, dir); err != nil {
		t.Fatalf("apply correction: %v", err)
	}
	rep, err := ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("reindex correction: %v", err)
	}
	if rep.Superseded != 1 {
		t.Errorf("want 1 supersession, got %d", rep.Superseded)
	}

	hits, err := store.SearchNotes(ctx, tenantID, "weighs pounds", index.SearchOpts{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if hasHit(hits, "scout:weight") {
		t.Errorf("stale note retrievable by default: %v", ids(hits))
	}
	if !hasHit(hits, "scout:weight-2026") {
		t.Errorf("correction not retrievable: %v", ids(hits))
	}
}

func TestCleanRelPathViaPlan(t *testing.T) {
	// A nested clean path is preserved and slash-normalized.
	dir := t.TempDir()
	plan := &index.Plan{Notes: []index.PlanNote{
		{Path: "a/b/c.md", Body: "# C\n\nbody"},
	}}
	written, err := index.ApplyPlan(plan, dir)
	if err != nil {
		t.Fatalf("ApplyPlan: %v", err)
	}
	if written[0] != "a/b/c.md" {
		t.Errorf("written = %q, want a/b/c.md", written[0])
	}
	if !strings.HasSuffix(written[0], ".md") {
		t.Errorf("not markdown: %q", written[0])
	}
}
