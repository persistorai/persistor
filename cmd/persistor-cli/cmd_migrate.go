package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
)

// newMigrateCmd builds `persistor migrate`: apply all pending schema migrations.
//
// This is the explicit migration step for the production split-role posture:
// run it as the schema-owning migrator role (DDL needs ownership), then run the
// daemon as a separate non-owner app role with PERSISTOR_AUTO_MIGRATE=false. In
// the single-role self-host posture the daemon auto-migrates at boot and this
// command is optional.
func newMigrateCmd() *cobra.Command {
	var databaseURL string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending schema migrations (run as the schema-owning migrator role)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMigrate(cmd.Context(), databaseURL)
		},
	}
	cmd.Flags().StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"),
		"Postgres URL of the migrator/owner role (env DATABASE_URL)")
	return cmd
}

func runMigrate(ctx context.Context, databaseURL string) error {
	if databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}
	log := logrus.New()
	log.SetOutput(os.Stderr)
	log.SetLevel(logrus.InfoLevel)

	pool, err := dbpool.NewPool(ctx, databaseURL, 2)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	fmt.Fprintln(os.Stderr, "migrations applied")
	return nil
}
