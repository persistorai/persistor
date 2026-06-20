// Package index implements the memory engine. Notes live in Postgres (the source
// of truth) as versioned rows; this package writes them, maintains the full-text
// chunk projection over them, and serves search/brief. No entity extraction, no
// graph — just notes in, searchable chunks out. ParseNote/RenderNote convert
// between the prose-with-frontmatter form (import, export) and the stored note.
package index

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Note is the parsed, normalized result of one source file. In this phase one
// source file maps to one note (one note per source file); later
// consolidation may write multiple atomic notes per file.
type Note struct {
	ID         string
	Kind       string
	Tier       string
	Title      string
	Body       string // prose with frontmatter stripped
	SourcePath string
	Supersedes string
	// Links is reserved: it is parsed from frontmatter and threaded through the
	// write path, but RenderNote omits it and IndexFile never persists it (the
	// links table is an intentional placeholder). Not yet a queryable edge.
	Links []string
}

// frontmatter mirrors the YAML block at the top of a note. All fields are
// optional; missing values get derived defaults in ParseNote.
type frontmatter struct {
	ID         string   `yaml:"id"`
	Kind       string   `yaml:"kind"`
	Tier       string   `yaml:"tier"`
	Title      string   `yaml:"title"`
	Supersedes string   `yaml:"supersedes,omitempty"`
	Links      []string `yaml:"links,omitempty"`
}

const (
	defaultKind = "fact"
	tierCore    = "core"
	tierTail    = "tail"
)

// validKinds are the note kinds the schema accepts (chk_note_kind).
var validKinds = map[string]bool{
	"fact": true, "decision": true, "episode": true, "reference": true, "preference": true,
}

// ValidKind reports whether kind is an accepted note kind. An empty kind is
// accepted (the store defaults it to "fact").
func ValidKind(kind string) bool { return kind == "" || validKinds[kind] }

// ValidTier reports whether tier is core or tail. An empty tier is accepted (the
// store defaults it to "tail").
func ValidTier(tier string) bool { return tier == "" || tier == tierCore || tier == tierTail }

// DeriveTitle returns the trimmed title when set, else the note body's first
// markdown heading, else fallback. The PG-native write path uses it so a note
// without an explicit title still gets a sensible one (the old file path derived
// this in ParseNote).
func DeriveTitle(title, body, fallback string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	if t := titleFromBody(body, ""); t != "" {
		return t
	}
	return fallback
}

// ParseNote parses one markdown file's content into a normalized Note. namespace
// (the root name, e.g. "demo") prefixes derived ids so the same relative path
// under different roots can't collide. relPath is the file path relative to its
// root (used to derive a stable id/title when frontmatter omits them). isCore
// marks files in the always-loaded Core tier unless frontmatter overrides.
func ParseNote(namespace, relPath, content string, isCore bool) Note {
	fmText, body := splitFrontmatter(content)

	var fm frontmatter
	if fmText != "" {
		// Malformed frontmatter is non-fatal: fall back to derived defaults
		// rather than dropping the note.
		if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
			fm = frontmatter{}
		}
	}

	n := Note{
		ID:         strings.TrimSpace(fm.ID),
		Kind:       strings.TrimSpace(fm.Kind),
		Tier:       strings.TrimSpace(fm.Tier),
		Title:      strings.TrimSpace(fm.Title),
		Body:       strings.TrimRight(body, "\n") + "\n",
		SourcePath: relPath,
		Supersedes: strings.TrimSpace(fm.Supersedes),
		Links:      fm.Links,
	}
	normalize(&n, namespace, relPath, body, isCore)
	return n
}

// normalize fills in derived defaults for any field the frontmatter omitted.
func normalize(n *Note, namespace, relPath, body string, isCore bool) {
	if n.ID == "" {
		n.ID = namespace + ":" + slugFromPath(relPath)
	}
	if !validKinds[n.Kind] {
		n.Kind = defaultKind
	}
	if n.Tier != tierCore && n.Tier != tierTail {
		if isCore {
			n.Tier = tierCore
		} else {
			n.Tier = tierTail
		}
	}
	if n.Title == "" {
		n.Title = titleFromBody(body, relPath)
	}
}

// RenderNote renders a note back to .md content (YAML frontmatter + body) — the
// inverse of ParseNote for the persisted fields. Links are intentionally omitted:
// they are derived from the body's [[id]] references on reindex, so a re-imported
// export reconstructs them. ParseNote(RenderNote(n)) round-trips
// id/kind/tier/title/supersedes and the body.
func RenderNote(n *Note) (string, error) {
	y, err := yaml.Marshal(frontmatter{
		ID:         n.ID,
		Kind:       n.Kind,
		Tier:       n.Tier,
		Title:      n.Title,
		Supersedes: n.Supersedes,
	})
	if err != nil {
		return "", fmt.Errorf("marshaling frontmatter: %w", err)
	}
	body := strings.TrimRight(n.Body, "\n") + "\n"
	// Closing fence immediately before the body: splitFrontmatter does not trim a
	// leading blank line, so an exact round-trip requires no extra newline here.
	return "---\n" + string(y) + "---\n" + body, nil
}

// splitFrontmatter separates a leading `---`-fenced YAML block from the body.
// Returns ("", content) when there is no well-formed frontmatter.
func splitFrontmatter(content string) (fmText, body string) {
	const fence = "---"
	trimmed := strings.TrimLeft(content, "\ufeff") // tolerate a UTF-8 BOM
	if !strings.HasPrefix(trimmed, fence+"\n") && !strings.HasPrefix(trimmed, fence+"\r\n") {
		return "", content
	}
	rest := trimmed[len(fence):]
	rest = strings.TrimLeft(rest, "\r\n")
	// Find the closing fence at the start of a line.
	lines := strings.Split(rest, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == fence {
			fmText = strings.Join(lines[:i], "\n")
			body = strings.Join(lines[i+1:], "\n")
			return fmText, body
		}
	}
	return "", content // no closing fence — treat whole thing as body
}

// slugFromPath derives a stable note id from a file's relative path, e.g.
// "memory/daily/2026/06/2026-06-17.md" -> "memory-daily-2026-06-2026-06-17".
func slugFromPath(relPath string) string {
	p := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	p = strings.ReplaceAll(p, string(filepath.Separator), "-")
	p = strings.ReplaceAll(p, "/", "-")
	p = strings.ReplaceAll(p, " ", "-")
	return strings.Trim(strings.ToLower(p), "-")
}

// titleFromBody uses the first markdown heading as the title, falling back to
// the file's base name.
func titleFromBody(body, relPath string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			return strings.TrimSpace(strings.TrimLeft(t, "# "))
		}
		if t != "" {
			break // first non-blank line isn't a heading; use the filename
		}
	}
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
