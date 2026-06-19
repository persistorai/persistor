package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// briefOpts holds the resolved inputs for assembling a working-set.
type briefOpts struct {
	databaseURL string
	tenantID    string
	notesDir    string
	cwd         string
	topics      string
	asJSON      bool
	opt         index.BriefOptions
}

// newBriefCmd builds `persistor brief`: assemble the dynamic working-set — every
// Core note plus the Tail notes most relevant to the current project, under a
// hard token budget. Emitted as a markdown block for injection at session start.
// Degrades to Core-read-from-disk when the index is unreachable, so it never
// hard-fails.
func newBriefCmd() *cobra.Command {
	o := briefOpts{}
	cmd := &cobra.Command{
		Use:              "brief",
		Short:            "Assemble the dynamic memory working-set (Core + retrieved Tail)",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {}, // skip HTTP client
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBrief(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().StringVar(&o.notesDir, "notes-dir", os.Getenv("PERSISTOR_NOTES_DIR"), "Notes repo root (env PERSISTOR_NOTES_DIR) — used for the degrade path")
	cmd.Flags().StringVar(&o.cwd, "cwd", "", "Project dir to seed retrieval from (default: current dir)")
	cmd.Flags().StringVar(&o.topics, "topics", "", "Optional extra seed topics (e.g. last session's topics)")
	cmd.Flags().IntVar(&o.opt.Budget, "budget", 6000, "Total working-set token budget")
	cmd.Flags().IntVar(&o.opt.CoreBudget, "core-budget", 2000, "Max tokens for the Core tier")
	cmd.Flags().IntVar(&o.opt.TailLimit, "tail-limit", 12, "Max Tail notes to consider")
	cmd.Flags().BoolVar(&o.asJSON, "json", false, "Emit the working-set as JSON instead of markdown")
	return cmd
}

func runBrief(ctx context.Context, o *briefOpts) error {
	if o.cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolving cwd: %w", err)
		}
		o.cwd = wd
	}
	if o.databaseURL == "" {
		o.databaseURL = os.Getenv("DATABASE_URL")
	}

	seed := index.SeedQuery(o.cwd, o.topics)

	ws, err := assembleFromIndex(ctx, o, seed)
	if err != nil {
		// Degrade: the notes are the source of truth, so Core is
		// recoverable from disk even with the index down. Never hard-fail.
		fmt.Fprintf(os.Stderr, "warning: index unavailable (%v); degrading to Core-from-disk\n", err)
		ws, err = assembleFromDisk(o, seed)
		if err != nil {
			return err
		}
	}

	return emitWorkingSet(&ws, o.asJSON)
}

// assembleFromIndex builds the working-set from the database.
func assembleFromIndex(ctx context.Context, o *briefOpts, seed string) (index.WorkingSet, error) {
	if o.databaseURL == "" {
		return index.WorkingSet{}, fmt.Errorf("no database URL")
	}
	if o.tenantID == "" {
		return index.WorkingSet{}, fmt.Errorf("no tenant")
	}
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)

	pool, err := dbpool.NewPool(ctx, o.databaseURL, 4)
	if err != nil {
		return index.WorkingSet{}, err
	}
	defer pool.Close()

	store := index.NewStore(pool, log)
	return index.AssembleWorkingSet(ctx, store, o.tenantID, seed, o.opt)
}

// assembleFromDisk builds a Core-only working-set straight from the note files.
func assembleFromDisk(o *briefOpts, seed string) (index.WorkingSet, error) {
	if o.notesDir == "" {
		return index.WorkingSet{}, fmt.Errorf("index unavailable and no --notes-dir/PERSISTOR_NOTES_DIR for the degrade path")
	}
	notesRoot, err := index.NotesRoot(o.notesDir)
	if err != nil {
		return index.WorkingSet{}, err
	}
	core, err := index.CoreFromDisk([]index.Root{notesRoot})
	if err != nil {
		return index.WorkingSet{}, err
	}
	ws := index.BuildDegradedWorkingSet(core, seed, o.opt.CoreBudget)
	return ws, nil
}

func emitWorkingSet(ws *index.WorkingSet, asJSON bool) error {
	if asJSON {
		out, err := json.MarshalIndent(ws, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}
	fmt.Print(strings.TrimRight(index.RenderMarkdown(ws), "\n") + "\n")
	return nil
}
