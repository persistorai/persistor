package index

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxNoteIDLen mirrors the chk_note_id_len DB constraint so an over-long id
// fails with a clear app-layer error instead of a constraint violation.
const maxNoteIDLen = 512

// noteIDPattern constrains an explicit note id to the shape slugFromPath
// produces: lowercase alphanumerics plus ':' (namespace), '.', '_', and '-'. It
// stops a model-supplied id from smuggling in path separators, whitespace, or
// uppercase that would collide with or shadow a derived id on reindex.
var noteIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9:._-]*$`)

// Plan is the deterministic output of the consolidation step: the LLM
// (the /consolidate harness skill) reads a transcript and emits this plan; Go
// applies it mechanically. The judgment lives in the harness; this package only
// writes the prose note files the plan describes, then the caller reindexes.
type Plan struct {
	Notes []PlanNote `json:"notes"`
}

// PlanNote is one note the plan writes. Body is markdown prose (no frontmatter —
// ApplyPlan renders the frontmatter from the typed fields). Path is the file's
// location relative to the notes dir; it must be a clean relative path.
type PlanNote struct {
	ID         string   `json:"id,omitempty"`         // optional; derived from path if absent
	Path       string   `json:"path"`                 // required, relative to notesDir
	Kind       string   `json:"kind,omitempty"`       // fact|decision|episode|reference|preference
	Tier       string   `json:"tier,omitempty"`       // core|tail (default tail)
	Title      string   `json:"title,omitempty"`      // optional; derived from body heading
	Supersedes string   `json:"supersedes,omitempty"` // id of the note this corrects
	Links      []string `json:"links,omitempty"`      // optional associations
	Body       string   `json:"body"`                 // required prose
}

// planFrontmatter is the YAML block ApplyPlan renders. Field order here is the
// emitted order; omitempty keeps unset fields out so notes stay clean.
type planFrontmatter struct {
	ID         string   `yaml:"id,omitempty"`
	Kind       string   `yaml:"kind,omitempty"`
	Tier       string   `yaml:"tier,omitempty"`
	Title      string   `yaml:"title,omitempty"`
	Supersedes string   `yaml:"supersedes,omitempty"`
	Links      []string `yaml:"links,omitempty"`
}

// ApplyPlan writes every note in the plan as a prose.md file under notesDir and
// returns the relative paths written, in plan order. It is deterministic: the
// same plan yields byte-identical files. It does NOT touch the index — the
// caller reindexes afterward (that is where supersession is reconciled).
// ApplyPlan validates each note and writes nothing if any note is invalid.
func ApplyPlan(plan *Plan, notesDir string) ([]string, error) {
	if len(plan.Notes) == 0 {
		return nil, fmt.Errorf("plan has no notes")
	}
	rendered := make([][]byte, len(plan.Notes))
	rels := make([]string, len(plan.Notes))
	for i := range plan.Notes {
		clean, content, err := renderPlanNote(&plan.Notes[i])
		if err != nil {
			return nil, fmt.Errorf("note %d (%s): %w", i, plan.Notes[i].Path, err)
		}
		rels[i] = clean
		rendered[i] = content
	}

	// Validation passed for all notes; now write (so a bad plan writes nothing).
	for i, rel := range rels {
		abs := filepath.Join(notesDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, fmt.Errorf("creating dir for %s: %w", rel, err)
		}
		if err := os.WriteFile(abs, rendered[i], 0o600); err != nil {
			return nil, fmt.Errorf("writing %s: %w", rel, err)
		}
	}
	return rels, nil
}

// renderPlanNote validates one note and returns its cleaned relative path and
// rendered file bytes (frontmatter + body).
func renderPlanNote(n *PlanNote) (cleanRel string, content []byte, err error) {
	cleanRel, err = cleanRelPath(n.Path)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(n.Body) == "" {
		return "", nil, fmt.Errorf("empty body")
	}
	if n.ID != "" {
		if len(n.ID) > maxNoteIDLen {
			return "", nil, fmt.Errorf("id too long: %d > %d", len(n.ID), maxNoteIDLen)
		}
		if !noteIDPattern.MatchString(n.ID) {
			return "", nil, fmt.Errorf("invalid id %q (want a lowercase slug of a-z, 0-9, and :._-)", n.ID)
		}
	}
	if n.Kind != "" && !validKinds[n.Kind] {
		return "", nil, fmt.Errorf("invalid kind %q", n.Kind)
	}
	if n.Tier != "" && n.Tier != tierCore && n.Tier != tierTail {
		return "", nil, fmt.Errorf("invalid tier %q (want core|tail)", n.Tier)
	}

	fm := planFrontmatter{
		ID: n.ID, Kind: n.Kind, Tier: n.Tier,
		Title: n.Title, Supersedes: n.Supersedes, Links: n.Links,
	}
	fmBytes, err := yaml.Marshal(&fm)
	if err != nil {
		return "", nil, fmt.Errorf("marshaling frontmatter: %w", err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(fmBytes)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(n.Body, "\n"))
	b.WriteString("\n")
	return cleanRel, []byte(b.String()), nil
}

// cleanRelPath rejects absolute paths and traversal, returning a slash-normalized
// relative path. Plans come from the harness, but a note must never escape the
// notes dir.
func cleanRelPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be relative: %q", p)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes notes dir: %q", p)
	}
	if !strings.HasSuffix(clean, ".md") {
		return "", fmt.Errorf("path must end in .md: %q", p)
	}
	return clean, nil
}
