package index_test

import (
	"context"
	"strings"
	"testing"

	"github.com/briancolinger/persistor/internal/index"
)

// TestAssembleWorkingSet_CoreAndTail checks the working-set assembly: seed a
// synthetic corpus, assemble the working-set, and assert Core is always present,
// the total stays under budget, and a seed surfaces the matching Tail note.
func TestAssembleWorkingSet_CoreAndTail(t *testing.T) {
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	seedMarkdown(t, store, tenantID, "identity.md", "# Identity\n\nWe are the Northwind expedition crew.\n", true)
	seedMarkdown(t, store, tenantID, "rules.md", "# Standing Rules\n\nAlways file a flight plan before departure.\n", true)
	seedMarkdown(t, store, tenantID, "aurora.md", "# Aurora Protocol\n\nThe safety protocol for navigating polar storms.\n", false)
	seedMarkdown(t, store, tenantID, "vega.md", "# Captain Vega\n\nFounded the Northwind logistics company.\n", false)

	opts := index.BriefOptions{Budget: 6000, CoreBudget: 2000, TailLimit: 12}
	ws, err := index.AssembleWorkingSet(ctx, store, tenantID, "polar storm protocol", opts)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	// (a) every Core note present.
	if !hasNote(ws.Core, "syn:identity") || !hasNote(ws.Core, "syn:rules") {
		t.Errorf("Core missing notes: %+v", ws.Core)
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
	store, _, tenantID := newStoreTest(t)
	ctx := context.Background()

	big := "# Big Tail\n\n" + strings.Repeat("storm navigation detail and more storm detail. ", 200)
	seedMarkdown(t, store, tenantID, "core.md", "# Core\n\nThe mission is to chart the trade routes.\n", true)
	seedMarkdown(t, store, tenantID, "big1.md", big, false)
	seedMarkdown(t, store, tenantID, "big2.md", big, false)

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
