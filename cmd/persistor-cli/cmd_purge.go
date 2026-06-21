package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// purgeOpts holds the resolved inputs for `persistor purge`.
type purgeOpts struct {
	databaseURL string
	tenantID    string
	noteID      string
	confirm     bool
}

// newPurgeCmd builds `persistor purge <id>`: irreversibly hard-delete ONE note —
// its live row, its full version history, and its chunks. The surgical
// counterpart to delete-tenant, gated behind an explicit --yes so it is never a
// one-keystroke mistake. The everyday, reversible path is memory_delete (a
// restorable tombstone); this scrubs the note and its history out of existence.
func newPurgeCmd() *cobra.Command {
	o := purgeOpts{}
	cmd := &cobra.Command{
		Use:   "purge <note-id> --tenant <id> --yes",
		Short: "Irreversibly hard-delete one note and its full history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.noteID = args[0]
			return runPurge(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().BoolVar(&o.confirm, "yes", false, "Confirm the irreversible purge (required)")
	return cmd
}

func runPurge(ctx context.Context, o *purgeOpts) error {
	if o.noteID == "" {
		return fmt.Errorf("no note id given")
	}
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	if _, err := uuid.Parse(o.tenantID); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", o.tenantID, err)
	}
	if !o.confirm {
		return fmt.Errorf("refusing to purge note %q without --yes (this is irreversible)", o.noteID)
	}
	databaseURL := o.databaseURL
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, databaseURL, 2)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	store := index.NewStore(pool, log)
	res, err := store.PurgeNote(ctx, o.tenantID, o.noteID)
	if err != nil {
		return err
	}
	if !res.Found {
		fmt.Fprintf(os.Stderr, "no such note %q in tenant %s (nothing purged)\n", o.noteID, o.tenantID)
		return nil
	}
	fmt.Fprintf(os.Stderr, "purged note %q: %d versions, %d chunks\n", o.noteID, res.Versions, res.Chunks)
	return nil
}
