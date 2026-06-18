package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

// reindexOpts holds the resolved inputs for a reindex run.
type reindexOpts struct {
	databaseURL string
	tenantID    string
	notesDir    string
	claudeDir   string
	archiveDir  string
	chunkWords  int
}

// newReindexCmd builds `persistor reindex`: the file-sync indexer. It walks the
// notes repo and Claude Code's auto-memory dir, archives Claude's (local-only)
// notes into a durable git-tracked location, and re-indexes only files whose
// content changed.
//
// This is a local DB command: it connects straight to Postgres, so it skips the
// HTTP client setup that other subcommands use.
func newReindexCmd() *cobra.Command {
	o := reindexOpts{}
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the memory index from watched note roots",
		Long: `Walk the watched note roots (the notes repo and Claude Code's auto-memory
directory), hash every markdown file, and re-index only files whose content
changed since the last run. Claude's local-only notes are archived into a
durable git-tracked location first, so a fresh clone + reindex rebuilds the
whole index on a new machine.

Configuration (flags override env):
  --tenant        tenant UUID            (env PERSISTOR_TENANT_ID)
  --database-url  Postgres URL           (env DATABASE_URL)
  --notes-dir     notes repo root        (env PERSISTOR_NOTES_DIR)
  --claude-memory Claude auto-memory dir (env CLAUDE_MEMORY_DIR; optional)
  --archive-dir   durable archive target (default <notes-dir>/data/claude-memory)`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {}, // skip HTTP client
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReindex(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().StringVar(&o.notesDir, "notes-dir", os.Getenv("PERSISTOR_NOTES_DIR"), "Notes repo root (env PERSISTOR_NOTES_DIR)")
	cmd.Flags().StringVar(&o.claudeDir, "claude-memory", os.Getenv("CLAUDE_MEMORY_DIR"), "Claude auto-memory dir (env CLAUDE_MEMORY_DIR)")
	cmd.Flags().StringVar(&o.archiveDir, "archive-dir", "", "Durable archive dir for Claude notes (default <notes-dir>/data/claude-memory)")
	cmd.Flags().IntVar(&o.chunkWords, "chunk-words", index.DefaultChunkWords, "Target words per chunk")
	return cmd
}

func runReindex(ctx context.Context, o *reindexOpts) error {
	rep, err := reindexCorpus(ctx, o)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting report: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// reindexCorpus resolves options, applies migrations, and runs a full reindex of
// the watched roots. Shared by `reindex` and `consolidate` (which applies a plan
// then reindexes so supersession is reconciled).
func reindexCorpus(ctx context.Context, o *reindexOpts) (index.Report, error) {
	if err := resolveReindexOpts(o); err != nil {
		return index.Report{}, err
	}

	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)

	pool, err := dbpool.NewPool(ctx, o.databaseURL, 4)
	if err != nil {
		return index.Report{}, fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Apply migrations here: reindex is the migrator (there is no long-running
	// server). goose skips already-applied migrations, so this is a fast no-op
	// once the schema is current.
	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		return index.Report{}, fmt.Errorf("applying migrations: %w", err)
	}

	roots, err := buildRoots(o)
	if err != nil {
		return index.Report{}, err
	}

	ix := index.NewIndexer(index.NewStore(pool, log), log, o.chunkWords)
	rep, err := ix.Reindex(ctx, o.tenantID, roots)
	if err != nil {
		return index.Report{}, fmt.Errorf("reindex: %w", err)
	}
	return rep, nil
}

// resolveReindexOpts fills in env/defaults and validates required inputs.
func resolveReindexOpts(o *reindexOpts) error {
	if o.databaseURL == "" {
		// The index is plaintext, so reindex needs only the DB URL.
		o.databaseURL = os.Getenv("DATABASE_URL")
	}
	if o.databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	if o.notesDir == "" {
		return fmt.Errorf("no notes repo (set --notes-dir or PERSISTOR_NOTES_DIR)")
	}
	if o.archiveDir == "" {
		o.archiveDir = filepath.Join(o.notesDir, "data", "claude-memory")
	}
	return nil
}

// buildRoots assembles the watched roots from the resolved options, using the
// shared index root config so the CLI and MCP server index the same corpus.
func buildRoots(o *reindexOpts) ([]index.Root, error) {
	return index.BuildRoots(o.notesDir, o.claudeDir, o.archiveDir)
}
