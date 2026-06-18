package index_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/eval"
	"github.com/persistorai/persistor/internal/index"
)

// syntheticCorpus is a fully-fictional note set (no PII — product testdata stays
// synthetic). Key facts live in TAIL notes so the index can retrieve
// them while a core-only "static MEMORY.md" surface cannot. The fuel/captain
// pairs exercise supersession: the *-old notes are superseded by the
// frontmatter `supersedes:` pointers and must drop out of default retrieval, so
// the contradiction category proves current-vs-stale resolution. The payload
// pair are time-bound facts that coexist (neither supersedes the other).
var syntheticCorpus = []struct {
	rel, content string
	core         bool
}{
	{"memory.md", "# Mission\n\nThe overall mission and big picture: chart the northern trade routes by airship.\n", true},
	{"identity.md", "# Identity\n\nWe are the Northwind expedition crew.\n", true},
	{"aurora.md", "# Aurora Protocol\n\nThe safety protocol for navigating airships through polar storms.\n", false},
	{"vega.md", "# Captain Vega\n\nVeteran pilot who founded the Northwind logistics company in 1987.\n", false},
	{"helios.md", "# Helios Engine\n\nThe solar propulsion system that powers the cargo airships.\n", false},
	{"glacier.md", "# Glacier Outpost\n\nA remote research station at the edge of the northern ice field where the crew studies the climate.\n", false},
	{"ballast.md", "# Ballast System\n\nThe ballast system controls buoyancy, letting the airships adjust their altitude and stay aloft by venting gas and dropping weight.\n", false},
	{"raven.md", "# The Raven\n\nThe Raven is the flagship airship that leads the fleet on every expedition.\n", false},
	{"meridian.md", "# Meridian City\n\nMeridian City is the home port of the expedition fleet.\n", false},
	{"first-flight.md", "# Maiden Voyage\n\nThe maiden voyage of the Raven departed from Meridian City on March 3, 1989.\n", false},
	{"storm-1991.md", "# The Great Aurora Storm\n\nIn the winter of 1991 the fleet was grounded by the Great Aurora Storm for six weeks.\n", false},
	{"summit.md", "# The Meridian Accord\n\nThe Meridian Accord was signed at the 1995 logistics summit in Meridian City.\n", false},
	{"fuel-old.md", "---\nid: syn:fuel\n---\n# Lifting Gas\n\nThe airships are kept aloft by hydrogen lifting gas.\n", false},
	{"fuel-new.md", "---\nid: syn:fuel-2\nsupersedes: syn:fuel\n---\n# Lifting Gas\n\nThe airships were converted to helium lifting gas in 1992 for safety, replacing the older hydrogen.\n", false},
	{"captain-old.md", "---\nid: syn:captain\n---\n# Captain of the Raven\n\nCaptain Vega commands the Raven.\n", false},
	{"captain-new.md", "---\nid: syn:captain-2\nsupersedes: syn:captain\n---\n# Captain of the Raven\n\nCaptain Orion now commands the Raven after Vega retired.\n", false},
	{"payload-1989.md", "# Payload 1989\n\nIn 1989 the Raven's cargo payload capacity was two tons.\n", false},
	{"payload-1996.md", "# Payload 1996\n\nBy 1996, upgrades raised the Raven's cargo payload capacity to five tons.\n", false},
}

// supersededIDs are notes that must never appear in default retrieval after a
// reindex reconciles supersession.
var supersededIDs = []string{"syn:fuel", "syn:captain"}

// categoryTargets are the recall@5 bars per category. Ratchet, don't relax.
var categoryTargets = map[string]float64{
	"fact":          0.90,
	"episodic":      0.85,
	"association":   0.80,
	"contradiction": 1.00, // current note always retrieved; stale excluded
	"temporal":      0.80,
}

// TestEvalBeatsStaticBaseline is the CI gate: it seeds the synthetic corpus,
// runs the deterministic retrieval eval against the baselines, and asserts the
// index (a) strictly beats the core-only static surface and (b) hits every
// per-category target. No embedding backend required (text mode).
func TestEvalBeatsStaticBaseline(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	var corePaths []string
	for _, n := range syntheticCorpus {
		writeFile(t, dir, n.rel, n.content)
		if n.core {
			corePaths = append(corePaths, n.rel)
		}
	}
	roots := []index.Root{{Name: "syn", Dir: dir, CorePaths: corePaths}}
	if _, err := ix.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	fixture, err := eval.LoadFixture("testdata/synthetic-eval.json")
	if err != nil {
		t.Fatalf("loading fixture: %v", err)
	}

	indexReport := runReport(ctx, t, fixture, index.AllNotesSearcher(store, tenantID))
	static := runRecall(ctx, t, fixture, index.StaticMemorySearcher(store, tenantID))
	none := runRecall(ctx, t, fixture, index.NoMemorySearcher())

	t.Logf("recall@5 — persistor=%.3f  static(core)=%.3f  no-memory=%.3f",
		indexReport.RecallAtK, static, none)

	// (a) beats the static MEMORY.md baseline — the whole point.
	if indexReport.RecallAtK <= static {
		t.Errorf("persistor (%.3f) must beat static MEMORY.md (%.3f)", indexReport.RecallAtK, static)
	}
	if none != 0 {
		t.Errorf("no-memory baseline recall = %.3f, want 0", none)
	}

	// (b) every per-category target is met. Ratchet, don't relax.
	got := categoryRecall(indexReport)
	for cat, target := range categoryTargets {
		r, ok := got[cat]
		if !ok {
			t.Errorf("category %q missing from eval report", cat)
			continue
		}
		if r < target {
			t.Errorf("category %q recall@5 = %.3f, want >= %.2f (fix retrieval, not the target)", cat, r, target)
		} else {
			t.Logf("category %q recall@5 = %.3f (>= %.2f) ✓", cat, r, target)
		}
	}

	// (c) 0 contradiction-staleness failures: superseded notes never surface in
	// default retrieval, no matter the query.
	assertNoStaleHits(ctx, t, store, tenantID, fixture)
}

// assertNoStaleHits runs every fixture query and fails if any superseded note id
// appears in the default (current-only) results.
func assertNoStaleHits(ctx context.Context, t *testing.T, store *index.Store, tenantID string, fixture *eval.Fixture) {
	t.Helper()
	stale := make(map[string]bool, len(supersededIDs))
	for _, id := range supersededIDs {
		stale[id] = true
	}
	for i := range fixture.Questions {
		q := &fixture.Questions[i]
		hits, err := store.SearchNotes(ctx, tenantID, q.Prompt, index.SearchOpts{Limit: 5})
		if err != nil {
			t.Fatalf("search %q: %v", q.Prompt, err)
		}
		for j := range hits {
			if stale[hits[j].ID] {
				t.Errorf("staleness failure: superseded note %q surfaced for query %q", hits[j].ID, q.Prompt)
			}
		}
	}
}

// categoryRecall maps category name -> recall@5 from a report.
func categoryRecall(report *eval.Report) map[string]float64 {
	out := make(map[string]float64, len(report.Categories))
	for i := range report.Categories {
		out[report.Categories[i].Name] = report.Categories[i].RecallAtK
	}
	return out
}

func runReport(ctx context.Context, t *testing.T, fixture *eval.Fixture, searcher eval.SearchClient) *eval.Report {
	t.Helper()
	report, err := eval.NewRunner(searcher).Run(ctx, fixture)
	if err != nil {
		t.Fatalf("eval run: %v", err)
	}
	return report
}

func runRecall(ctx context.Context, t *testing.T, fixture *eval.Fixture, searcher eval.SearchClient) float64 {
	t.Helper()
	return runReport(ctx, t, fixture, searcher).RecallAtK
}
