package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Root is a watched directory tree of prose notes. Name is recorded in
// sources.root (e.g. "demo", "claude"). Includes lists relative files or
// directories to index; a directory entry pulls in every *.md beneath it, and
// an empty Includes means "all *.md under Dir". CorePaths marks files that
// belong to the always-loaded Core tier.
type Root struct {
	Name      string
	Dir       string
	Includes  []string
	CorePaths []string
}

// DiscoveredFile is one indexable file located under a Root.
type DiscoveredFile struct {
	Root    string
	AbsPath string
	RelPath string // path relative to Root.Dir, slash-separated, stable across machines
	IsCore  bool
}

// Discover walks a Root and returns every indexable markdown file. It is
// deterministic (sorted by RelPath) so index runs and tests are reproducible.
func Discover(root *Root) ([]DiscoveredFile, error) {
	core := make(map[string]bool, len(root.CorePaths))
	for _, p := range root.CorePaths {
		core[filepath.ToSlash(p)] = true
	}

	seen := make(map[string]DiscoveredFile)
	targets := root.Includes
	if len(targets) == 0 {
		targets = []string{"."}
	}

	for _, inc := range targets {
		if err := collect(root, inc, core, seen); err != nil {
			return nil, err
		}
	}

	out := make([]DiscoveredFile, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out, nil
}

// collect adds the markdown files reachable from one include entry (a file or a
// directory) to seen, keyed by absolute path so overlapping includes dedupe.
func collect(root *Root, inc string, core map[string]bool, seen map[string]DiscoveredFile) error {
	abs := filepath.Join(root.Dir, inc)
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // a configured include that isn't present is not an error
		}
		return fmt.Errorf("stat %q: %w", abs, err)
	}

	if !info.IsDir() {
		addFile(root, abs, core, seen)
		return nil
	}

	return filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		addFile(root, path, core, seen)
		return nil
	})
}

func addFile(root *Root, abs string, core map[string]bool, seen map[string]DiscoveredFile) {
	rel, err := filepath.Rel(root.Dir, abs)
	if err != nil {
		rel = filepath.Base(abs)
	}
	rel = filepath.ToSlash(rel)
	seen[abs] = DiscoveredFile{
		Root:    root.Name,
		AbsPath: abs,
		RelPath: rel,
		IsCore:  core[rel],
	}
}

// HashContent returns the lowercase hex sha256 of file content — the change
// signal for incremental re-indexing.
func HashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// less orders files by RelPath then Root for deterministic output.
func less(a, b DiscoveredFile) bool {
	if a.RelPath != b.RelPath {
		return a.RelPath < b.RelPath
	}
	return a.Root < b.Root
}
