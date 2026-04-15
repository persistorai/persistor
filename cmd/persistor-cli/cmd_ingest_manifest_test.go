package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIngestManifest_SkipCompletedUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	store, err := loadIngestManifest(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if err := store.markCompleted("note.md", info, 2); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	reloaded, err := loadIngestManifest(dir)
	if err != nil {
		t.Fatalf("reload manifest: %v", err)
	}
	if !reloaded.shouldSkip("note.md", info) {
		t.Fatal("expected unchanged file to be skipped")
	}
}

func TestIngestManifest_DoesNotSkipChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	store, err := loadIngestManifest(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if err := store.markCompleted("note.md", info, 2); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(path, []byte("hello again"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}
	updated, err := os.Stat(path)
	if err != nil {
		t.Fatalf("restat file: %v", err)
	}
	if store.shouldSkip("note.md", updated) {
		t.Fatal("expected changed file not to be skipped")
	}
}

func TestIngestManifest_MarkFailed(t *testing.T) {
	dir := t.TempDir()
	store, err := loadIngestManifest(dir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if err := store.markFailed("note.md", errors.New("boom")); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if got := store.data.Failed["note.md"].Error; got != "boom" {
		t.Fatalf("unexpected failure error %q", got)
	}
}
