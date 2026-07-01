package index

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// maxNoteIDLen mirrors the chk_note_id_len DB constraint so an over-long id
// fails with a clear app-layer error instead of a constraint violation.
const maxNoteIDLen = 512

// noteIDPattern constrains a note id to the shape slugFromPath produces:
// lowercase alphanumerics plus ':' (namespace), '.', '_', and '-'. It stops a
// model-supplied id from smuggling in path separators, whitespace, or uppercase.
var noteIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9:._-]*$`)

// DeriveNoteID computes the final id for a PG-native note write. An explicit id
// is validated and returned as-is; otherwise the id is derived from the note's
// path slug, prefixed with namespace when non-empty (e.g. "demo:memory-foo").
// The path is only a key for deriving a stable slug — PG-native notes have no
// backing file — so it must still be a clean relative .md path.
func DeriveNoteID(namespace, path, explicitID string) (string, error) {
	if explicitID != "" {
		if err := validateNoteID(explicitID); err != nil {
			return "", err
		}
		return explicitID, nil
	}
	clean, err := cleanRelPath(path)
	if err != nil {
		return "", fmt.Errorf("deriving id from path: %w", err)
	}
	id := slugFromPath(clean)
	if namespace != "" {
		id = namespace + ":" + id
	}
	if err := validateNoteID(id); err != nil {
		return "", err
	}
	return id, nil
}

// validateNoteID enforces the length and character-set constraints shared by the
// chk_note_id_len DB constraint and noteIDPattern, so a bad id fails with a clear
// app-layer error instead of a constraint violation.
func validateNoteID(id string) error {
	if len(id) > maxNoteIDLen {
		return fmt.Errorf("id too long: %d > %d", len(id), maxNoteIDLen)
	}
	if !noteIDPattern.MatchString(id) {
		return fmt.Errorf("invalid id %q (want a lowercase slug of a-z, 0-9, and :._-)", id)
	}
	return nil
}

// ValidateNoteID is the exported form of the note-id check, for write paths that
// carry a pre-derived id (the CLI importer's frontmatter ids) so they enforce
// the same charset/length rules as DeriveNoteID.
func ValidateNoteID(id string) error {
	return validateNoteID(id)
}

// maxNamespaceLen mirrors the chk_note_namespace_len DB constraint.
const maxNamespaceLen = 128

// namespacePattern constrains a namespace to a lowercase slug WITHOUT ':' —
// the colon is the namespace/slug separator inside note ids, so a namespace
// containing one would make "demo:memory-x" style ids ambiguous.
var namespacePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ValidateNamespace enforces the namespace slug shape at the write boundary
// (empty means "default" and is allowed). Data hygiene within a tenant: queries
// are parameterized and RLS-scoped regardless, but a free-form namespace would
// fragment listings and collide with the id derivation above.
func ValidateNamespace(ns string) error {
	if ns == "" {
		return nil
	}
	if len(ns) > maxNamespaceLen {
		return fmt.Errorf("namespace too long: %d > %d", len(ns), maxNamespaceLen)
	}
	if !namespacePattern.MatchString(ns) {
		return fmt.Errorf("invalid namespace %q (want a lowercase slug of a-z, 0-9, and ._- ; no ':')", ns)
	}
	return nil
}

// cleanRelPath rejects absolute paths and traversal, returning a slash-normalized
// relative .md path. A note id is derived from it, so it must stay inside a clean
// relative namespace.
func cleanRelPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be relative: %q", p)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes notes namespace: %q", p)
	}
	if !strings.HasSuffix(clean, ".md") {
		return "", fmt.Errorf("path must end in .md: %q", p)
	}
	return clean, nil
}

// SelfSupersedeError is returned when a write would supersede the same note it
// resolves to — almost always a mistake, since it forks no history.
type SelfSupersedeError struct {
	ID   string
	Path string
}

func (e *SelfSupersedeError) Error() string {
	return fmt.Sprintf("supersedes %q is this same note: to correct it in place omit supersedes; "+
		"to keep history, write a NEW id that supersedes %q", e.ID, e.ID)
}
