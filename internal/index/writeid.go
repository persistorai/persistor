package index

import (
	"fmt"
	"path/filepath"
	"strings"
)

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
