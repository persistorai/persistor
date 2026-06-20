package index

import (
	"fmt"
	"path/filepath"
	"strings"
)

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

// ResolveWriteID computes the note id a plan note will have once written under
// notesDir and indexed: the explicit id if set, else `<root>:<slug(relpath)>`
// where relpath is the written file's path relative to the watched root that
// contains notesDir. ok is false when notesDir is not under any watched root, in
// which case callers should skip the self-supersede check rather than block the
// write. It lets the write path detect a note that supersedes the very id it
// resolves to before writing, instead of silently doing nothing.
func ResolveWriteID(roots []Root, notesDir, planRelPath, explicitID string) (id string, ok bool) {
	if explicitID != "" {
		return explicitID, true
	}
	abs := filepath.Join(notesDir, filepath.FromSlash(planRelPath))
	for i := range roots {
		rel, err := filepath.Rel(roots[i].Dir, abs)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			continue // abs is not under this root
		}
		return roots[i].Name + ":" + slugFromPath(rel), true
	}
	return "", false
}

// SelfSupersedeError is returned when a write would supersede the same note it
// resolves to — almost always a mistake, since it forks no history.
type SelfSupersedeError struct {
	ID   string
	Path string
}

func (e *SelfSupersedeError) Error() string {
	return fmt.Sprintf("supersedes %q is this same note (path %q resolves to it): "+
		"to correct it in place omit supersedes; to keep history, write a NEW path that supersedes %q",
		e.ID, e.Path, e.ID)
}

// CheckSelfSupersede returns a *SelfSupersedeError when the plan note's supersedes
// target is the same id the note will resolve to under notesDir. It returns nil
// when supersedes is empty or the id can't be resolved.
func CheckSelfSupersede(roots []Root, notesDir, planRelPath, explicitID, supersedes string) error {
	if supersedes == "" {
		return nil
	}
	resolved, ok := ResolveWriteID(roots, notesDir, planRelPath, explicitID)
	if !ok {
		return nil
	}
	if resolved == supersedes {
		return &SelfSupersedeError{ID: resolved, Path: planRelPath}
	}
	return nil
}
