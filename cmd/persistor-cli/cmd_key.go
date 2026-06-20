package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/db"
	"github.com/briancolinger/persistor/internal/db/migrations"
	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/mcpauth"
)

// keyOpts holds the resolved inputs for the key subcommands.
type keyOpts struct {
	databaseURL string
	tenantID    string
	label       string
	token       string
}

// newKeyCmd builds `persistor key`: mint and revoke the bearer tokens the remote
// MCP daemon (persistor-server) authenticates. A token is shown once at creation
// and stored only as a hash, so losing it means minting a new one.
func newKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage remote MCP API keys (bearer tokens)",
	}
	cmd.AddCommand(newKeyCreateCmd(), newKeyRevokeCmd())
	return cmd
}

func newKeyCreateCmd() *cobra.Command {
	o := keyOpts{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint an API key for a tenant (printed once)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runKeyCreate(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.label, "label", "", "Human label for the key")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func newKeyRevokeCmd() *cobra.Command {
	o := keyOpts{}
	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke an API key by its token",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runKeyRevoke(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.token, "token", "", "The raw token to revoke")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func runKeyCreate(ctx context.Context, o *keyOpts) error {
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	if _, err := uuid.Parse(o.tenantID); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", o.tenantID, err)
	}
	store, closeStore, err := openKeyStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()

	raw, err := store.CreateAPIKey(ctx, o.tenantID, o.label)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "API key created. Copy it now — it is shown only once:")
	fmt.Println(raw)
	return nil
}

func runKeyRevoke(ctx context.Context, o *keyOpts) error {
	if o.token == "" {
		return fmt.Errorf("no token (set --token)")
	}
	store, closeStore, err := openKeyStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()

	n, err := store.RevokeAPIKey(ctx, o.token)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "revoked %d key(s)\n", n)
	return nil
}

// openKeyStore connects to Postgres, applies migrations (the CLI is the
// migrator), and returns a key store plus a close func.
func openKeyStore(ctx context.Context, databaseURL string) (store *mcpauth.PGKeyStore, closeStore func(), err error) {
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		return nil, nil, fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, databaseURL, 2)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("applying migrations: %w", err)
	}
	return mcpauth.NewPGKeyStore(pool), pool.Close, nil
}
