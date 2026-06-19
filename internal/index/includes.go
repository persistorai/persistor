package index

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// NotesIncludesEnv is the environment variable that overrides the curated
// include-list for the notes root. It holds a colon-separated list of
// repo-relative subpaths (e.g. "memory/MEMORY.md:memory/daily:memory/reference").
// Unset or empty means use defaultNotesIncludes unchanged.
const NotesIncludesEnv = "PERSISTOR_NOTES_INCLUDES"

// defaultNotesIncludes is the curated include-list used when NotesIncludesEnv is
// unset or empty. This is the backward-compatible default — when the env var is
// absent the indexer behaves exactly as it did before it was configurable.
var defaultNotesIncludes = []string{
	"memory/MEMORY.md",
	"memory/daily",
	"memory/atomic", // consolidation output — atomic notes
	"identity",
	"TOOLS.md",
	"AGENTS.md",
}

// InvalidIncludeError reports an include entry that fails the path guardrails:
// it is absolute, escapes the notes root, is empty/the root itself, or resolves
// (via a symlink) outside the root. The whole config is rejected — failing loud
// instead of silently dropping the offending entry.
type InvalidIncludeError struct {
	Entry  string
	Reason string
}

func (e *InvalidIncludeError) Error() string {
	return fmt.Sprintf("invalid notes include %q: %s (set %s to colon-separated repo-relative subpaths)",
		e.Entry, e.Reason, NotesIncludesEnv)
}

func invalidInclude(entry, reason string) error {
	return &InvalidIncludeError{Entry: entry, Reason: reason}
}

// resolveIncludes returns the validated include-list for the notes root rooted
// at dir. With NotesIncludesEnv unset it returns defaultNotesIncludes unchanged;
// when set it parses the colon-separated entries and rejects the entire list
// (returning *InvalidIncludeError) if any entry fails the guardrails.
func resolveIncludes(dir string) ([]string, error) {
	raw := os.Getenv(NotesIncludesEnv)
	if raw == "" {
		return defaultNotesIncludes, nil
	}
	parts := strings.Split(raw, ":")
	includes := make([]string, 0, len(parts))
	for _, entry := range parts {
		if err := validateInclude(dir, entry); err != nil {
			return nil, err
		}
		includes = append(includes, path.Clean(filepath.ToSlash(entry)))
	}
	return includes, nil
}

// validateInclude enforces the shape/escape guardrails on a single entry. It is
// about shape, not existence: a well-formed relative subpath that doesn't exist
// on disk yet is allowed (the walker tolerates missing includes).
func validateInclude(dir, entry string) error {
	switch {
	case entry == "":
		return invalidInclude(entry, "entry is empty")
	case strings.TrimSpace(entry) != entry:
		return invalidInclude(entry, "entry has leading or trailing whitespace")
	case hasWindowsVolume(entry):
		return invalidInclude(entry, "Windows drive or UNC paths are not allowed")
	case strings.HasPrefix(entry, "/") || filepath.IsAbs(entry):
		return invalidInclude(entry, "absolute paths are not allowed")
	}

	clean := path.Clean(filepath.ToSlash(entry))
	switch {
	case clean == ".":
		return invalidInclude(entry, "the notes root itself cannot be an include")
	case clean == ".." || strings.HasPrefix(clean, "../"):
		return invalidInclude(entry, "entry escapes the notes root via ..")
	}
	return checkWithinRoot(dir, entry)
}

// checkWithinRoot confirms entry resolves to a path inside dir. It follows
// symlinks when the target exists so a symlink pointing to an absolute location
// or outside the root is rejected; a not-yet-existing target is allowed.
func checkWithinRoot(dir, entry string) error {
	abs := filepath.Join(dir, filepath.FromSlash(entry))
	if !within(dir, abs) {
		return invalidInclude(entry, "resolves outside the notes root")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // shape is already validated; existence is not required
		}
		return invalidInclude(entry, fmt.Sprintf("resolving symlinks: %v", err))
	}
	if !within(resolveRoot(dir), resolved) {
		return invalidInclude(entry, "symlink target escapes the notes root")
	}
	return nil
}

// resolveRoot returns dir with its own symlinks resolved, so the within check
// compares like-for-like after EvalSymlinks; it falls back to dir on error.
func resolveRoot(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return dir
}

// within reports whether target is root itself or nested beneath it.
func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// hasWindowsVolume reports whether entry looks like a Windows drive-letter path
// (e.g. "C:\notes") or a UNC path (e.g. "\\host\share"). These are rejected so
// the include-list stays repo-relative.
func hasWindowsVolume(entry string) bool {
	if strings.HasPrefix(entry, `\\`) || strings.HasPrefix(entry, "//") {
		return true
	}
	if len(entry) >= 2 && entry[1] == ':' {
		c := entry[0]
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	return false
}
