package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

// newTestIndexer connects to TEST_DATABASE_URL (skipping when unset, like the
// store package) and returns an Indexer plus a random tenant. The schema is
// assumed migrated (the loop gate runs the fresh-migration step first).
func newTestIndexer(t *testing.T) (*index.Indexer, *index.Store, string) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := dbpool.NewPool(ctx, dbURL, 4)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)

	tenantID := uuid.New().String()
	t.Cleanup(func() {
		cleanCtx := context.Background()
		tx, err := pool.Begin(cleanCtx)
		if err != nil {
			return
		}
		defer func() { _ = tx.Rollback(cleanCtx) }()
		if _, err := tx.Exec(cleanCtx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
			return
		}
		_, _ = tx.Exec(cleanCtx, "DELETE FROM chunks WHERE tenant_id = current_setting('app.tenant_id')::uuid")
		_, _ = tx.Exec(cleanCtx, "DELETE FROM notes WHERE tenant_id = current_setting('app.tenant_id')::uuid")
		_, _ = tx.Exec(cleanCtx, "DELETE FROM sources WHERE tenant_id = current_setting('app.tenant_id')::uuid")
		_ = tx.Commit(cleanCtx)
	})

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	store := index.NewStore(pool, log)
	return index.NewIndexer(store, log, 0), store, tenantID
}

func TestIndexer_FileSyncSemantics(t *testing.T) {
	ix, store, tenantID := newTestIndexer(t)
	ctx := context.Background()

	demoDir := t.TempDir()
	writeFile(t, demoDir, "MEMORY.md", "# Memory\n\nThe big picture.\n")
	writeFile(t, demoDir, "identity/SOUL.md", "# Soul\n\nWho I am.\n")
	writeFile(t, demoDir, "memory/daily/d1.md", "# Day One\n\nFirst day.\n")

	claudeDir := t.TempDir()
	writeFile(t, claudeDir, "MEMORY.md", "# Claude Memory\n\nAuto notes.\n")

	roots := []index.Root{
		{Name: "demo", Dir: demoDir, Includes: []string{"MEMORY.md", "identity", "memory"}, CorePaths: []string{"MEMORY.md", "identity/SOUL.md"}},
		{Name: "claude", Dir: claudeDir},
	}

	// First run: everything is new, every file represented, zero dropped.
	rep, err := ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	if rep.Discovered != 4 || rep.Indexed != 4 || rep.Skipped != 0 || rep.Notes != 4 {
		t.Fatalf("first run = %+v, want discovered/indexed/notes=4 skipped=0", rep)
	}
	assertPaths(t, rep.Paths, []string{
		"demo/MEMORY.md", "demo/identity/SOUL.md", "demo/memory/daily/d1.md", "claude/MEMORY.md",
	})

	// Second run, no file changes: a pure no-op (everything skipped).
	rep, err = ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("second reindex: %v", err)
	}
	if rep.Indexed != 0 || rep.Skipped != 4 || rep.Deleted != 0 || rep.Notes != 4 {
		t.Fatalf("no-op run = %+v, want indexed=0 skipped=4 deleted=0", rep)
	}

	// Edit one file: only that file reindexes.
	writeFile(t, demoDir, "memory/daily/d1.md", "# Day One\n\nFirst day, revised.\n")
	rep, err = ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("edit reindex: %v", err)
	}
	if rep.Indexed != 1 || rep.Skipped != 3 {
		t.Fatalf("edit run = %+v, want indexed=1 skipped=3", rep)
	}

	// Delete one file: its source/note/chunks are dropped.
	if err := os.Remove(filepath.Join(demoDir, "identity", "SOUL.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	rep, err = ix.Reindex(ctx, tenantID, roots)
	if err != nil {
		t.Fatalf("delete reindex: %v", err)
	}
	if rep.Deleted != 1 || rep.Notes != 3 {
		t.Fatalf("delete run = %+v, want deleted=1 notes=3", rep)
	}

	n, err := store.CountNotes(ctx, tenantID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Errorf("final note count = %d, want 3", n)
	}
}

func TestIndexer_IndexPath(t *testing.T) {
	ix, _, tenantID := newTestIndexer(t)
	ctx := context.Background()

	demoDir := t.TempDir()
	writeFile(t, demoDir, "memory/atomic/a.md", "---\nid: note-a\n---\n\nAlpha fact.\n")
	roots := []index.Root{{Name: "demo", Dir: demoDir, Includes: []string{"memory"}}}

	// Targeted index of a single written file, no full walk of the root.
	abs := filepath.Join(demoDir, "memory", "atomic", "a.md")
	rep, err := ix.IndexPath(ctx, tenantID, roots, abs)
	if err != nil {
		t.Fatalf("IndexPath: %v", err)
	}
	if rep.Discovered != 1 || rep.Indexed != 1 || rep.Notes != 1 {
		t.Fatalf("first IndexPath = %+v, want discovered/indexed/notes=1", rep)
	}
	assertPaths(t, rep.Paths, []string{"demo/memory/atomic/a.md"})

	// A note that supersedes the first: IndexPath writes only b.md but still
	// reconciles supersession corpus-wide, flipping note-a's flag.
	writeFile(t, demoDir, "memory/atomic/b.md", "---\nid: note-b\nsupersedes: note-a\n---\n\nAlpha, corrected.\n")
	abs = filepath.Join(demoDir, "memory", "atomic", "b.md")
	rep, err = ix.IndexPath(ctx, tenantID, roots, abs)
	if err != nil {
		t.Fatalf("IndexPath supersede: %v", err)
	}
	if rep.Indexed != 1 || rep.Notes != 2 || rep.Superseded != 1 {
		t.Fatalf("supersede IndexPath = %+v, want indexed=1 notes=2 superseded=1", rep)
	}

	// A path outside every root is rejected, not silently indexed.
	if _, err := ix.IndexPath(ctx, tenantID, roots, filepath.Join(t.TempDir(), "x.md")); err == nil {
		t.Fatal("IndexPath outside roots: want error, got nil")
	}
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	set := make(map[string]bool, len(got))
	for _, p := range got {
		set[p] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("missing source path %q in %v", w, got)
		}
	}
}
