package index_test

import (
	"context"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// TestAssembleWorkingSet_CoreAndTail checks the working-set assembly: seed a synthetic corpus,
// assemble the working-set, and assert Core is always present, the total stays
// under budget, and a seed surfaces the matching Tail note.
func TestAssembleWorkingSet_CoreAndTail(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	writeFile(t, dir, "identity.md", "# Identity\n\nWe are the Northwind expedition crew.\n")
	writeFile(t, dir, "rules.md", "# Standing Rules\n\nAlways file a flight plan before departure.\n")
	writeFile(t, dir, "aurora.md", "# Aurora Protocol\n\nThe safety protocol for navigating polar storms.\n")
	writeFile(t, dir, "vega.md", "# Captain Vega\n\nFounded the Northwind logistics company.\n")
	roots := []index.Root{{Name: "syn", Dir: dir, CorePaths: []string{"identity.md", "rules.md"}}}
	if _, err := ix.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	opts := index.BriefOptions{Budget: 6000, CoreBudget: 2000, TailLimit: 12}
	ws, err := index.AssembleWorkingSet(ctx, store, tenantID, "polar storm protocol", opts)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	// (a) every Core note present.
	if !hasNote(ws.Core, "syn:identity") || !hasNote(ws.Core, "syn:rules") {
		t.Errorf("Core missing notes: %+v", ws.Core)
	}
	if ws.Degraded {
		t.Errorf("DB path should not be degraded")
	}
	// (b) total tokens under budget.
	if ws.TotalTokens > opts.Budget {
		t.Errorf("total tokens %d > budget %d", ws.TotalTokens, opts.Budget)
	}
	// (c) the seed surfaces the matching Tail note.
	if !hasNote(ws.Tail, "syn:aurora") {
		t.Errorf("Tail missing seed match syn:aurora: %+v", ws.Tail)
	}
	// Core notes must not leak into Tail.
	if hasNote(ws.Tail, "syn:identity") {
		t.Errorf("Core note leaked into Tail")
	}
}

// TestAssembleWorkingSet_RespectsBudget: a tiny budget drops Tail and never
// exceeds the cap.
func TestAssembleWorkingSet_RespectsBudget(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	dir := t.TempDir()
	writeFile(t, dir, "core.md", "# Core\n\nThe mission is to chart the trade routes.\n")
	big := "# Big Tail\n\n" + strings.Repeat("storm navigation detail and more storm detail. ", 200)
	writeFile(t, dir, "big1.md", big)
	writeFile(t, dir, "big2.md", big)
	roots := []index.Root{{Name: "syn", Dir: dir, CorePaths: []string{"core.md"}}}
	if _, err := ix.Reindex(ctx, tenantID, roots); err != nil {
		t.Fatalf("reindex: %v", err)
	}

	opts := index.BriefOptions{Budget: 120, CoreBudget: 60, TailLimit: 12}
	ws, err := index.AssembleWorkingSet(ctx, store, tenantID, "storm navigation", opts)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if ws.TotalTokens > opts.Budget {
		t.Errorf("total %d exceeds budget %d", ws.TotalTokens, opts.Budget)
	}
	if !hasNote(ws.Core, "syn:core") {
		t.Errorf("Core dropped under tight budget")
	}
}

// TestBriefDegradesToDisk: (d) Core is recoverable from disk with no database.
func TestBriefDegradesToDisk(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "identity.md", "# Identity\n\nWho we are.\n")
	writeFile(t, dir, "rules.md", "# Rules\n\nStanding rules.\n")
	root := index.Root{Name: "syn", Dir: dir, CorePaths: []string{"identity.md", "rules.md"}}

	core, err := index.CoreFromDisk([]index.Root{root})
	if err != nil {
		t.Fatalf("CoreFromDisk: %v", err)
	}
	if len(core) != 2 {
		t.Fatalf("want 2 core notes from disk, got %d", len(core))
	}
	ws := index.BuildDegradedWorkingSet(core, "anything", 2000)
	if !ws.Degraded {
		t.Errorf("degraded working-set should be marked Degraded")
	}
	if !hasNote(ws.Core, "syn:identity") || !hasNote(ws.Core, "syn:rules") {
		t.Errorf("degraded Core missing notes: %+v", ws.Core)
	}
	if len(ws.Tail) != 0 {
		t.Errorf("degraded set should have no Tail")
	}
	md := index.RenderMarkdown(&ws)
	if !strings.Contains(md, "degraded") || !strings.Contains(md, "## Core") {
		t.Errorf("rendered markdown missing degrade marker / Core section:\n%s", md)
	}
}

func TestEstimateTokensAndTruncate(t *testing.T) {
	if got := index.EstimateTokens(""); got != 0 {
		t.Errorf("empty = %d, want 0", got)
	}
	if got := index.EstimateTokens("12345678"); got != 2 { // 8 chars / 4
		t.Errorf("8 chars = %d, want 2", got)
	}
}

func hasNote(notes []index.NoteRecord, id string) bool {
	for i := range notes {
		if notes[i].ID == id {
			return true
		}
	}
	return false
}
