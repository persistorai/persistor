package index

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MirrorMarkdown makes dest a durable mirror of every *.md file under src
// : Claude Code's auto-memory lives in a local-only
// directory that isn't backed up, so we copy it into a git-tracked location
// before indexing. The mirror is exact — markdown files removed from src are
// pruned from dest — so deletions propagate to the index. Non-markdown files in
// dest are left untouched (the archive dir may be git-managed with a README).
//
// A nil src directory (missing) is a no-op rather than an error.
func MirrorMarkdown(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat source %q: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source %q is not a directory", src)
	}

	want := make(map[string]bool)
	if err := copyMarkdown(src, dest, want); err != nil {
		return err
	}
	return pruneMarkdown(dest, want)
}

// copyMarkdown copies every *.md under src into dest, preserving relative paths,
// and records the set of relative paths it wrote into want.
func copyMarkdown(src, dest string, want map[string]bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}
		want[filepath.ToSlash(rel)] = true

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %q: %w", path, err)
		}
		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %q: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return fmt.Errorf("writing %q: %w", target, err)
		}
		return nil
	})
}

// pruneMarkdown removes *.md files under dest whose relative path is not in want.
func pruneMarkdown(dest string, want map[string]bool) error {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(dest, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(dest, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}
		if !want[filepath.ToSlash(rel)] {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("pruning %q: %w", path, err)
			}
		}
		return nil
	})
}
