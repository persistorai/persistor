package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// charsPerToken is a deterministic, model-agnostic token estimate: English text
// averages ~4 characters per token. This is an approximation used only for
// budgeting the working-set; it never needs to match a real tokenizer exactly.
const charsPerToken = 4

// EstimateTokens approximates the token count of text (chars / 4, rounded up).
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

// BriefOptions bounds the working-set.
type BriefOptions struct {
	Budget     int // total token budget for the whole working-set
	CoreBudget int // max tokens for the always-loaded Core tier
	TailLimit  int // max Tail notes to consider before budgeting
}

// WorkingSet is the assembled, bounded context block: pinned Core + retrieved
// Tail under a token budget.
type WorkingSet struct {
	Seed        string
	Core        []NoteRecord
	Tail        []NoteRecord
	CoreTokens  int
	TailTokens  int
	TotalTokens int
	Truncated   bool // some body was truncated to fit the budget
	Degraded    bool // assembled from disk because the index was unreachable
}

// AssembleWorkingSet builds the working-set from the index: every Core note
// (always present, bodies truncated to fit CoreBudget) plus the top Tail notes
// retrieved for seed, filling whatever of Budget remains.
func AssembleWorkingSet(ctx context.Context, store *Store, tenantID, seed string, opts BriefOptions) (WorkingSet, error) {
	core, err := store.CoreNotes(ctx, tenantID)
	if err != nil {
		return WorkingSet{}, err
	}

	ws := WorkingSet{Seed: seed}
	var coreTrunc bool
	ws.Core, ws.CoreTokens, coreTrunc = packNotes(core, opts.CoreBudget, true)

	remaining := opts.Budget - ws.CoreTokens
	if seed != "" && remaining > 0 {
		hits, err := store.SearchNotes(ctx, tenantID, seed, SearchOpts{Limit: opts.TailLimit, Tier: tierTail})
		if err != nil {
			return WorkingSet{}, err
		}
		ids := make([]string, len(hits))
		for i := range hits {
			ids[i] = hits[i].ID
		}
		tailNotes, err := store.LoadNotes(ctx, tenantID, ids)
		if err != nil {
			return WorkingSet{}, err
		}
		var tailTrunc bool
		ws.Tail, ws.TailTokens, tailTrunc = packNotes(tailNotes, remaining, false)
		ws.Truncated = ws.Truncated || tailTrunc
	}

	ws.Truncated = ws.Truncated || coreTrunc
	ws.TotalTokens = ws.CoreTokens + ws.TailTokens
	return ws, nil
}

// packNotes fits notes into a token budget. With keepAll=true (Core) every note
// is included — bodies are truncated, and once the budget is spent later notes
// become title-only — so the Core tier is never dropped. With keepAll=false
// (Tail) notes are added until the next one wouldn't fit, then truncated/stopped.
func packNotes(notes []NoteRecord, budget int, keepAll bool) ([]NoteRecord, int, bool) {
	out := make([]NoteRecord, 0, len(notes))
	tokens := 0
	truncated := false

	for i := range notes {
		n := notes[i]
		titleTok := EstimateTokens(n.Title)
		if !keepAll && tokens+titleTok >= budget {
			break // Tail: no room for even the next title
		}
		remaining := budget - tokens - titleTok
		if remaining < 0 {
			remaining = 0
		}
		if EstimateTokens(n.Body) > remaining {
			n.Body = truncateToTokens(n.Body, remaining)
			truncated = true
		}
		out = append(out, n)
		tokens += titleTok + EstimateTokens(n.Body)
	}
	return out, tokens, truncated
}

// truncateToTokens cuts text to at most maxTokens (so the result, marker
// included, never exceeds the budget), preferring a line boundary, and appends
// an ellipsis marker when it trims. maxTokens<=0 yields "".
func truncateToTokens(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	limit := maxTokens * charsPerToken
	if len(text) <= limit {
		return text
	}
	const marker = "\n…(truncated)"
	keep := limit - len(marker)
	if keep <= 0 {
		return "" // budget too small even for the marker; emit title-only
	}
	cut := strings.ToValidUTF8(text[:keep], "") // never split a multi-byte rune
	if nl := strings.LastIndexByte(cut, '\n'); nl > keep/2 {
		cut = cut[:nl]
	}
	return strings.TrimRight(cut, " \n") + marker
}

// CoreFromDisk reads the Core notes straight from their source files — the
// degrade path for when the index/DB is unreachable. The notes are
// the source of truth, so Core is always recoverable without the database.
func CoreFromDisk(roots []Root) ([]NoteRecord, error) {
	var out []NoteRecord
	for ri := range roots {
		root := &roots[ri]
		for _, rel := range root.CorePaths {
			abs := filepath.Join(root.Dir, rel)
			content, err := os.ReadFile(abs)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("reading core file %q: %w", abs, err)
			}
			n := ParseNote(root.Name, rel, string(content), true)
			out = append(out, NoteRecord{
				ID: n.ID, Kind: n.Kind, Tier: tierCore, Title: n.Title,
				Body: n.Body, SourcePath: root.Name + "/" + rel,
			})
		}
	}
	return out, nil
}

// BuildDegradedWorkingSet packs disk-read Core notes into a Core-only working-set
// (no Tail — the index is unreachable), marked Degraded. Used by the brief
// degrade path.
func BuildDegradedWorkingSet(core []NoteRecord, seed string, coreBudget int) WorkingSet {
	packed, tokens, truncated := packNotes(core, coreBudget, true)
	return WorkingSet{
		Seed:        seed,
		Core:        packed,
		CoreTokens:  tokens,
		TotalTokens: tokens,
		Truncated:   truncated,
		Degraded:    true,
	}
}

// RenderMarkdown emits the working-set as a compact markdown block for injection
// at session start.
func RenderMarkdown(ws *WorkingSet) string {
	var b strings.Builder
	b.WriteString("# Memory working set\n")
	fmt.Fprintf(&b, "<!-- core=%d tok, tail=%d tok, total=%d tok%s%s -->\n\n",
		ws.CoreTokens, ws.TailTokens, ws.TotalTokens,
		flag(ws.Truncated, ", truncated"), flag(ws.Degraded, ", degraded(core-from-disk)"))

	b.WriteString("## Core\n\n")
	writeNotes(&b, ws.Core)

	if len(ws.Tail) > 0 {
		b.WriteString("## Relevant context\n\n")
		writeNotes(&b, ws.Tail)
	}
	return b.String()
}

func writeNotes(b *strings.Builder, notes []NoteRecord) {
	for i := range notes {
		n := &notes[i]
		fmt.Fprintf(b, "### %s\n\n", n.Title)
		body := strings.TrimRight(n.Body, "\n")
		if body != "" {
			b.WriteString(body)
			b.WriteString("\n\n")
		}
	}
}

func flag(on bool, s string) string {
	if on {
		return s
	}
	return ""
}
