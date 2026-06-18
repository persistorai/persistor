package index

import (
	"fmt"
	"path/filepath"
)

// NotesRoot defines which files in the notes repo are indexed and which belong
// to the always-loaded Core tier. Curated to the durable, high-signal note
// surfaces, plus memory/atomic where consolidation writes. The namespace is the
// directory's base name, so note ids are prefixed by the repo name. Shared by
// the reindex CLI and the MCP server so both index the same corpus.
func NotesRoot(dir string) Root {
	return Root{
		Name: filepath.Base(dir),
		Dir:  dir,
		Includes: []string{
			"memory/MEMORY.md",
			"memory/daily",
			"memory/atomic", // consolidation output — atomic notes
			"identity",
			"TOOLS.md",
			"AGENTS.md",
		},
		CorePaths: []string{
			"memory/MEMORY.md",
			"identity/IDENTITY.md",
			"identity/SOUL.md",
			"identity/USER.md",
			"TOOLS.md",
			"AGENTS.md",
		},
	}
}

// BuildRoots assembles the watched roots: the notes repo (curated includes) and,
// optionally, the durable archive of Claude Code's auto-memory (mirrored from the
// live dir first so a fresh clone + reindex rebuilds the whole index). claudeDir
// empty means the Claude root is skipped. archiveDir defaults to
// <notesDir>/data/claude-memory.
func BuildRoots(notesDir, claudeDir, archiveDir string) ([]Root, error) {
	roots := []Root{NotesRoot(notesDir)}
	if claudeDir == "" {
		return roots, nil
	}
	if archiveDir == "" {
		archiveDir = filepath.Join(notesDir, "data", "claude-memory")
	}
	if err := MirrorMarkdown(claudeDir, archiveDir); err != nil {
		return nil, fmt.Errorf("archiving Claude notes: %w", err)
	}
	return append(roots, Root{Name: "claude", Dir: archiveDir}), nil
}
