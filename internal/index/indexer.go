package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// Indexer drives incremental, content-hash-based reindexing of the watched
// roots. It is the orchestration layer over Store and the parsing/
// chunking helpers.
type Indexer struct {
	store      *Store
	log        *logrus.Logger
	chunkWords int
}

// NewIndexer builds an Indexer. chunkWords <= 0 uses DefaultChunkWords.
func NewIndexer(store *Store, log *logrus.Logger, chunkWords int) *Indexer {
	if chunkWords <= 0 {
		chunkWords = DefaultChunkWords
	}
	return &Indexer{store: store, log: log, chunkWords: chunkWords}
}

// Report summarizes one reindex run.
type Report struct {
	Discovered int      `json:"discovered"` // files found across roots
	Indexed    int      `json:"indexed"`    // files (re)written (new or changed)
	Skipped    int      `json:"skipped"`    // files unchanged since last run
	Deleted    int      `json:"deleted"`    // sources removed (file gone)
	Superseded int      `json:"superseded"` // notes whose superseded flag changed this run
	Notes      int      `json:"notes"`      // total notes in the index after the run
	Roots      []string `json:"roots"`
	Paths      []string `json:"-"` // namespaced source paths represented (for assertions)
}

// Reindex walks every root, (re)indexes files whose content hash changed since
// the last run, and drops sources whose files have disappeared. It is
// incremental and idempotent: a run with no file changes is a no-op.
func (ix *Indexer) Reindex(ctx context.Context, tenantID string, roots []Root) (Report, error) {
	rep := Report{}
	for _, r := range roots {
		rep.Roots = append(rep.Roots, r.Name)
	}

	existing, err := ix.store.ListSourceHashes(ctx, tenantID)
	if err != nil {
		return rep, fmt.Errorf("loading existing sources: %w", err)
	}

	current := make(map[string]bool)
	for i := range roots {
		if err := ix.indexRoot(ctx, tenantID, &roots[i], existing, current, &rep); err != nil {
			return rep, err
		}
	}

	if err := ix.dropStale(ctx, tenantID, existing, current, &rep); err != nil {
		return rep, err
	}

	// Recompute supersession from the current corpus of supersedes pointers,
	// once after all files are indexed and stale sources dropped, so it is
	// independent of file order and self-heals when a superseding note is added
	// or removed.
	reconciled, err := ix.store.ReconcileSupersessions(ctx, tenantID)
	if err != nil {
		return rep, err
	}
	rep.Superseded = int(reconciled)

	notes, err := ix.store.CountNotes(ctx, tenantID)
	if err != nil {
		return rep, err
	}
	rep.Notes = notes
	return rep, nil
}

// IndexPath indexes a single file by absolute path: it locates the owning root,
// (re)writes that one note, then reconciles supersession and recounts. It is the
// targeted equivalent of Reindex for the common single-note memory_write, so a
// write costs one file read+hash instead of walking and hashing every watched
// file. The supersession reconcile still runs corpus-wide because a write can
// flip another note's superseded flag.
func (ix *Indexer) IndexPath(ctx context.Context, tenantID string, roots []Root, absPath string) (Report, error) {
	rep := Report{}
	root, rel, ok := ownerRoot(roots, absPath)
	if !ok {
		return rep, fmt.Errorf("indexing %q: not under any watched root", absPath)
	}
	rep.Roots = append(rep.Roots, root.Name)

	storedPath := root.Name + "/" + rel
	rep.Discovered = 1
	rep.Paths = append(rep.Paths, storedPath)

	f := DiscoveredFile{Root: root.Name, AbsPath: absPath, RelPath: rel, IsCore: isCorePath(root, rel)}
	changed, err := ix.indexOne(ctx, tenantID, f, storedPath, "")
	if err != nil {
		return rep, err
	}
	if changed {
		rep.Indexed = 1
	} else {
		rep.Skipped = 1
	}

	reconciled, err := ix.store.ReconcileSupersessions(ctx, tenantID)
	if err != nil {
		return rep, err
	}
	rep.Superseded = int(reconciled)

	notes, err := ix.store.CountNotes(ctx, tenantID)
	if err != nil {
		return rep, err
	}
	rep.Notes = notes
	return rep, nil
}

// ownerRoot returns the watched root that contains absPath and the file's
// slash-separated path relative to that root. When roots nest, the most specific
// (longest Dir) wins. ok is false when no root contains the path.
func ownerRoot(roots []Root, absPath string) (root *Root, rel string, ok bool) {
	best := -1
	for i := range roots {
		dir, err := filepath.Abs(roots[i].Dir)
		if err != nil {
			continue
		}
		r, err := filepath.Rel(dir, absPath)
		if err != nil {
			continue
		}
		r = filepath.ToSlash(r)
		if r == ".." || strings.HasPrefix(r, "../") {
			continue
		}
		if len(dir) > best {
			best, root, rel, ok = len(dir), &roots[i], r, true
		}
	}
	return root, rel, ok
}

// isCorePath reports whether rel (slash-separated, relative to the root) is one
// of the root's always-loaded Core files.
func isCorePath(root *Root, rel string) bool {
	for _, p := range root.CorePaths {
		if filepath.ToSlash(p) == rel {
			return true
		}
	}
	return false
}

// indexRoot discovers and (re)indexes every file under one root, updating the
// current-path set and the run report.
func (ix *Indexer) indexRoot(ctx context.Context, tenantID string, root *Root, existing map[string]string, current map[string]bool, rep *Report) error {
	files, err := Discover(root)
	if err != nil {
		return fmt.Errorf("discovering root %q: %w", root.Name, err)
	}
	for _, f := range files {
		storedPath := f.Root + "/" + f.RelPath
		current[storedPath] = true
		rep.Discovered++
		rep.Paths = append(rep.Paths, storedPath)

		changed, err := ix.indexOne(ctx, tenantID, f, storedPath, existing[storedPath])
		if err != nil {
			return err
		}
		if changed {
			rep.Indexed++
		} else {
			rep.Skipped++
		}
	}
	return nil
}

// dropStale removes sources whose backing files have disappeared.
func (ix *Indexer) dropStale(ctx context.Context, tenantID string, existing map[string]string, current map[string]bool, rep *Report) error {
	for path := range existing {
		if !current[path] {
			if err := ix.store.DeleteByPath(ctx, tenantID, path); err != nil {
				return fmt.Errorf("deleting stale source %q: %w", path, err)
			}
			rep.Deleted++
		}
	}
	return nil
}

// indexOne reads, hashes, and conditionally reindexes a single file. It returns
// true when the file was (re)written, false when skipped as unchanged.
func (ix *Indexer) indexOne(ctx context.Context, tenantID string, f DiscoveredFile, storedPath, priorHash string) (bool, error) {
	content, err := os.ReadFile(f.AbsPath)
	if err != nil {
		return false, fmt.Errorf("reading %q: %w", f.AbsPath, err)
	}
	hash := HashContent(content)
	if priorHash == hash {
		return false, nil // unchanged
	}

	note := ParseNote(f.Root, f.RelPath, string(content), f.IsCore)
	note.SourcePath = storedPath // namespaced, unique across roots
	chunks := Chunk(note.Title, note.Body, ix.chunkWords)

	if err := ix.store.IndexFile(ctx, tenantID, &IndexedFile{
		Root:    f.Root,
		RelPath: storedPath,
		SHA256:  hash,
		Note:    note,
		Chunks:  chunks,
	}); err != nil {
		return false, err
	}
	return true, nil
}
