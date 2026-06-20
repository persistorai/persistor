package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

// deleteTenantOpts holds the resolved inputs for `persistor delete-tenant`.
type deleteTenantOpts struct {
	databaseURL string
	tenantID    string
	confirm     bool
}

// newDeleteTenantCmd builds `persistor delete-tenant`: irreversibly purge ALL of
// a tenant's data — notes, version history, chunks, API keys, and identity
// mappings ("delete my account"). Per the no-soft-delete doctrine this is a hard
// delete, gated behind an explicit --yes so it is never a one-keystroke mistake.
func newDeleteTenantCmd() *cobra.Command {
	o := deleteTenantOpts{}
	cmd := &cobra.Command{
		Use:   "delete-tenant --tenant <id> --yes",
		Short: "Irreversibly delete ALL of a tenant's data",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runDeleteTenant(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().BoolVar(&o.confirm, "yes", false, "Confirm the irreversible purge (required)")
	return cmd
}

func runDeleteTenant(ctx context.Context, o *deleteTenantOpts) error {
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	if _, err := uuid.Parse(o.tenantID); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", o.tenantID, err)
	}
	if !o.confirm {
		return fmt.Errorf("refusing to delete tenant %s without --yes (this is irreversible)", o.tenantID)
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
	res, err := store.DeleteTenant(ctx, o.tenantID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr,
		"deleted tenant %s: %d notes, %d versions, %d chunks, %d api keys, %d identities\n",
		o.tenantID, res.Notes, res.Versions, res.Chunks, res.APIKeys, res.Identities)
	return nil
}
