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
	"github.com/briancolinger/persistor/internal/identity"
)

// adminOpts holds the resolved inputs for the admin subcommands.
type adminOpts struct {
	databaseURL string
	label       string
	issuer      string
	subject     string
	tenantID    string
	role        string
}

// newAdminCmd builds `persistor admin`: tenant + identity management for the
// remote MCP daemon's onboarding (Phase P4). Tenants are memory namespaces;
// identities route an IdP (issuer, subject) to a tenant. First login
// auto-provisions a personal tenant, so these commands are for the admin-assign
// path (e.g. pointing a work login at a shared tenant) and for offboarding.
func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Tenant & identity administration for the remote MCP daemon"}
	cmd.AddCommand(newTenantCmd(), newIdentityCmd())
	return cmd
}

func newTenantCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tenant", Short: "Manage tenants (memory namespaces)"}
	cmd.AddCommand(newTenantCreateCmd(), newTenantListCmd())
	return cmd
}

func newTenantCreateCmd() *cobra.Command {
	o := adminOpts{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tenant and print its UUID",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runTenantCreate(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.label, "label", "", "Human label for the tenant")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func newTenantListCmd() *cobra.Command {
	o := adminOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tenants",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runTenantList(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func newIdentityCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "identity", Short: "Map IdP identities to tenants"}
	cmd.AddCommand(newIdentitySetCmd(), newIdentityListCmd(), newIdentityDeleteCmd())
	return cmd
}

func newIdentitySetCmd() *cobra.Command {
	o := adminOpts{}
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Map an IdP (issuer, subject) to a tenant (upsert)",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runIdentitySet(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.issuer, "issuer", "", "IdP issuer (iss claim)")
	cmd.Flags().StringVar(&o.subject, "subject", "", "IdP subject (sub claim)")
	cmd.Flags().StringVar(&o.tenantID, "tenant", "", "Tenant UUID to route this identity to")
	cmd.Flags().StringVar(&o.role, "role", identity.RoleOwner, "Role: owner|member|readonly")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func newIdentityListCmd() *cobra.Command {
	o := adminOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List identity->tenant mappings",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runIdentityList(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func newIdentityDeleteCmd() *cobra.Command {
	o := adminOpts{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Remove an identity mapping (offboard); does NOT delete notes",
		RunE:  func(cmd *cobra.Command, _ []string) error { return runIdentityDelete(cmd.Context(), &o) },
	}
	cmd.Flags().StringVar(&o.issuer, "issuer", "", "IdP issuer (iss claim)")
	cmd.Flags().StringVar(&o.subject, "subject", "", "IdP subject (sub claim)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func runTenantCreate(ctx context.Context, o *adminOpts) error {
	store, closeStore, err := openIdentityStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()
	id, err := store.CreateTenant(ctx, o.label)
	if err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func runTenantList(ctx context.Context, o *adminOpts) error {
	store, closeStore, err := openIdentityStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()
	tenants, err := store.ListTenants(ctx)
	if err != nil {
		return err
	}
	for _, t := range tenants {
		fmt.Printf("%s\t%s\n", t.ID, t.Label)
	}
	return nil
}

func runIdentitySet(ctx context.Context, o *adminOpts) error {
	if o.issuer == "" || o.subject == "" || o.tenantID == "" {
		return fmt.Errorf("--issuer, --subject, and --tenant are required")
	}
	if _, err := uuid.Parse(o.tenantID); err != nil {
		return fmt.Errorf("invalid tenant UUID %q: %w", o.tenantID, err)
	}
	switch o.role {
	case identity.RoleOwner, identity.RoleMember, identity.RoleReadonly:
	default:
		return fmt.Errorf("invalid role %q (want owner|member|readonly)", o.role)
	}
	store, closeStore, err := openIdentityStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()
	if err := store.SetIdentity(ctx, identity.Identity{
		Issuer: o.issuer, Subject: o.subject, TenantID: o.tenantID, Role: o.role,
	}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "mapped %s|%s -> %s (%s)\n", o.issuer, o.subject, o.tenantID, o.role)
	return nil
}

func runIdentityList(ctx context.Context, o *adminOpts) error {
	store, closeStore, err := openIdentityStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()
	idents, err := store.ListIdentities(ctx)
	if err != nil {
		return err
	}
	for _, i := range idents {
		fmt.Printf("%s\t%s\t%s\t%s\n", i.Issuer, i.Subject, i.TenantID, i.Role)
	}
	return nil
}

func runIdentityDelete(ctx context.Context, o *adminOpts) error {
	if o.issuer == "" || o.subject == "" {
		return fmt.Errorf("--issuer and --subject are required")
	}
	store, closeStore, err := openIdentityStore(ctx, o.databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()
	n, err := store.DeleteIdentity(ctx, o.issuer, o.subject)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "deleted %d identity mapping(s)\n", n)
	return nil
}

// openIdentityStore connects to Postgres, applies migrations (the CLI is the
// migrator), and returns an identity store plus a close func.
func openIdentityStore(ctx context.Context, databaseURL string) (store *identity.Store, closeStore func(), err error) {
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
	return identity.NewStore(pool), pool.Close, nil
}
