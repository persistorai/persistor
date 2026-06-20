package index_test

import (
	"context"
	"testing"

	"github.com/persistorai/persistor/internal/index"
)

// seedNote writes one live PG-native note in the given namespace.
func seedNote(t *testing.T, store *index.Store, tenant, ns, id, body string) int {
	t.Helper()
	res, err := store.WriteNote(context.Background(), tenant, &index.PGNoteInput{
		ID: id, Namespace: ns, Tier: "tail", Title: id, Body: body, Surface: "test",
	}, 0)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return res.Version
}

// summaryIDs projects a summary slice to its ids, preserving order.
func summaryIDs(ss []index.NoteSummary) []string {
	ids := make([]string, len(ss))
	for i := range ss {
		ids[i] = ss[i].ID
	}
	return ids
}

func TestListNotes_PaginationAndNamespace(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	seedNote(t, store, tenant, "scout", "scout:a", "alpha")
	seedNote(t, store, tenant, "scout", "scout:b", "bravo")
	seedNote(t, store, tenant, "scout", "scout:c", "charlie")
	seedNote(t, store, tenant, "claude", "claude:a", "delta")
	seedNote(t, store, tenant, "claude", "claude:b", "echo")

	// All namespaces, ordered by id.
	all, err := store.ListNotes(ctx, tenant, index.ListOpts{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("listed %d, want 5", len(all))
	}
	if all[0].ID != "claude:a" || all[4].ID != "scout:c" {
		t.Fatalf("unexpected order: %s ... %s", all[0].ID, all[4].ID)
	}
	if all[0].Namespace != "claude" || all[0].Version != 1 || all[0].Tier != "tail" {
		t.Fatalf("summary meta = %+v", all[0])
	}

	// Namespace filter.
	scout, err := store.ListNotes(ctx, tenant, index.ListOpts{Namespace: "scout"})
	if err != nil {
		t.Fatalf("list scout: %v", err)
	}
	if len(scout) != 3 {
		t.Fatalf("scout listed %d, want 3", len(scout))
	}
	for _, n := range scout {
		if n.Namespace != "scout" {
			t.Fatalf("got namespace %s in a scout-only list", n.Namespace)
		}
	}

	// Pagination within the namespace.
	p1, err := store.ListNotes(ctx, tenant, index.ListOpts{Namespace: "scout", Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	p2, err := store.ListNotes(ctx, tenant, index.ListOpts{Namespace: "scout", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p1) != 2 || len(p2) != 1 {
		t.Fatalf("pagination sizes = %d, %d; want 2, 1", len(p1), len(p2))
	}
	if p1[0].ID != "scout:a" || p1[1].ID != "scout:b" || p2[0].ID != "scout:c" {
		t.Fatalf("pagination order wrong: %v | %v", summaryIDs(p1), summaryIDs(p2))
	}
}

func TestListNotes_ExcludesDeletedAndSuperseded(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	seedNote(t, store, tenant, "work", "work:keep", "keep me")
	goneVer := seedNote(t, store, tenant, "work", "work:gone", "delete me")
	seedNote(t, store, tenant, "work", "work:old", "stale fact")

	// Correct work:old with a superseding write, then reconcile.
	if _, err := store.WriteNote(ctx, tenant, &index.PGNoteInput{
		ID: "work:new", Namespace: "work", Tier: "tail", Title: "new", Body: "fresh fact",
		Supersedes: "work:old", Surface: "test",
	}, 0); err != nil {
		t.Fatalf("supersede write: %v", err)
	}
	if _, err := store.ReconcileSupersessions(ctx, tenant); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := store.DeleteNote(ctx, tenant, "work:gone", goneVer, "test"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Default excludes both the tombstone and the superseded note.
	live, err := store.ListNotes(ctx, tenant, index.ListOpts{Namespace: "work"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ids := summaryIDs(live)
	if !contains(ids, "work:keep") || !contains(ids, "work:new") {
		t.Fatalf("live list missing expected ids: %v", ids)
	}
	if contains(ids, "work:gone") {
		t.Fatal("deleted note appeared in default list")
	}
	if contains(ids, "work:old") {
		t.Fatal("superseded note appeared in default list")
	}

	// IncludeSuperseded surfaces the superseded note but never the tombstone.
	withSup, err := store.ListNotes(ctx, tenant, index.ListOpts{Namespace: "work", IncludeSuperseded: true})
	if err != nil {
		t.Fatalf("list with superseded: %v", err)
	}
	supIDs := summaryIDs(withSup)
	if !contains(supIDs, "work:old") {
		t.Fatal("IncludeSuperseded did not surface the superseded note")
	}
	if contains(supIDs, "work:gone") {
		t.Fatal("deleted note appeared even with IncludeSuperseded")
	}
}

func TestNamespaces_Counts(t *testing.T) {
	store, _, tenant := newStoreTest(t)
	ctx := context.Background()

	// Empty tenant: no namespaces.
	if nss, err := store.Namespaces(ctx, tenant); err != nil {
		t.Fatalf("namespaces (empty): %v", err)
	} else if len(nss) != 0 {
		t.Fatalf("empty tenant has %d namespaces, want 0", len(nss))
	}

	seedNote(t, store, tenant, "scout", "scout:a", "x")
	seedNote(t, store, tenant, "scout", "scout:b", "x")
	seedNote(t, store, tenant, "claude", "claude:a", "x")

	nss, err := store.Namespaces(ctx, tenant)
	if err != nil {
		t.Fatalf("namespaces: %v", err)
	}
	if len(nss) != 2 {
		t.Fatalf("got %d namespaces, want 2: %+v", len(nss), nss)
	}
	// Ordered by namespace.
	if nss[0].Namespace != "claude" || nss[0].Count != 1 {
		t.Fatalf("nss[0] = %+v, want claude/1", nss[0])
	}
	if nss[1].Namespace != "scout" || nss[1].Count != 2 {
		t.Fatalf("nss[1] = %+v, want scout/2", nss[1])
	}
}
