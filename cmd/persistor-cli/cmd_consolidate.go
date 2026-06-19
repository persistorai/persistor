package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/index"
)

// consolidateOpts holds the resolved inputs for applying a consolidation plan.
// The embedded reindexOpts.notesDir is the notes repo root; writeDir is where the
// plan's note files are written (a watched location inside that repo).
type consolidateOpts struct {
	reindexOpts
	planPath string
	writeDir string
}

// consolidateResult is the command's structured output.
type consolidateResult struct {
	Written []string     `json:"written"` // relative note paths the plan wrote
	Report  index.Report `json:"report"`  // the reindex that followed
}

// newConsolidateCmd builds `persistor consolidate`: apply a consolidation plan
// deterministically. The judgment — reading a transcript and deciding what's
// durable and what supersedes what — happens in the harness; this command only
// writes the note files the plan describes and then reindexes, which reconciles
// supersession. Notes are written under <write-dir> (default
// <notes-dir>/memory/atomic), which the reindexer watches.
func newConsolidateCmd() *cobra.Command {
	o := consolidateOpts{}
	cmd := &cobra.Command{
		Use:   "consolidate",
		Short: "Apply a consolidation plan: write notes, then reindex",
		Long: `Apply a consolidation plan deterministically.

The plan is JSON. Each note carries prose plus typed frontmatter fields;
consolidate renders the .md files under the write dir and reindexes the corpus,
which marks any superseded targets stale.

  persistor consolidate --plan plan.json
  cat plan.json | persistor consolidate --plan -

Plan shape:
  {"notes": [
    {"path": "topic.md", "kind": "fact", "tier": "tail",
     "supersedes": "notes:old-topic-id",
     "body": "# Topic\n\nLatest understanding ..."}
  ]}`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {}, // skip HTTP client
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsolidate(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().StringVar(&o.notesDir, "notes-dir", os.Getenv("PERSISTOR_NOTES_DIR"), "Notes repo root (env PERSISTOR_NOTES_DIR)")
	cmd.Flags().StringVar(&o.claudeDir, "claude-memory", os.Getenv("CLAUDE_MEMORY_DIR"), "Claude auto-memory dir (env CLAUDE_MEMORY_DIR)")
	cmd.Flags().StringVar(&o.archiveDir, "archive-dir", "", "Durable archive dir for Claude notes (default <notes-dir>/data/claude-memory)")
	cmd.Flags().IntVar(&o.chunkWords, "chunk-words", index.DefaultChunkWords, "Target words per chunk")
	cmd.Flags().StringVar(&o.planPath, "plan", "", "Path to the consolidation plan JSON, or - for stdin (required)")
	cmd.Flags().StringVar(&o.writeDir, "write-dir", os.Getenv("PERSISTOR_WRITE_DIR"), "Dir to write plan notes into (default <notes-dir>/memory/atomic)")
	return cmd
}

func runConsolidate(ctx context.Context, o *consolidateOpts) error {
	if o.planPath == "" {
		return fmt.Errorf("no plan (set --plan <file>|-)")
	}
	if o.notesDir == "" {
		return fmt.Errorf("no notes repo (set --notes-dir or PERSISTOR_NOTES_DIR)")
	}
	if o.writeDir == "" {
		o.writeDir = filepath.Join(o.notesDir, "memory", "atomic")
	}

	plan, err := readPlan(o.planPath)
	if err != nil {
		return err
	}

	// Reject a self-supersede before writing anything: a note whose supersedes
	// resolves to its own id forks no history.
	notesRoot, err := index.NotesRoot(o.notesDir)
	if err != nil {
		return err
	}
	checkRoots := []index.Root{notesRoot}
	for i := range plan.Notes {
		n := &plan.Notes[i]
		if err := index.CheckSelfSupersede(checkRoots, o.writeDir, n.Path, n.ID, n.Supersedes); err != nil {
			return err
		}
	}

	written, err := index.ApplyPlan(plan, o.writeDir)
	if err != nil {
		return fmt.Errorf("applying plan: %w", err)
	}

	rep, err := reindexCorpus(ctx, &o.reindexOpts)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(consolidateResult{Written: written, Report: rep}, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting result: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// readPlan loads a plan from a file path or, when path is "-", from stdin.
func readPlan(path string) (*index.Plan, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading plan: %w", err)
	}

	var plan index.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parsing plan JSON: %w", err)
	}
	return &plan, nil
}
