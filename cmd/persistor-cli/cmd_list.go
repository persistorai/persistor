package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// listOpts holds the resolved inputs for an enumeration query (list/namespaces).
type listOpts struct {
	databaseURL       string
	tenantID          string
	namespace         string
	limit             int
	offset            int
	includeSuperseded bool
}

// newListCmd builds `persistor list`: enumerate a tenant's notes as summaries
// (no bodies), optionally filtered to one namespace and paginated. The operator
// counterpart of the memory_list MCP tool.
func newListCmd() *cobra.Command {
	o := listOpts{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a tenant's notes as summaries (no bodies)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().StringVar(&o.namespace, "namespace", "", "Restrict to one namespace (default: all)")
	cmd.Flags().IntVar(&o.limit, "limit", 50, "Max notes to return (page size)")
	cmd.Flags().IntVar(&o.offset, "offset", 0, "Notes to skip, for paging")
	cmd.Flags().BoolVar(&o.includeSuperseded, "include-superseded", false, "Include superseded notes (history)")
	return cmd
}

func runList(ctx context.Context, o *listOpts) error {
	store, closeFn, err := openListStore(ctx, &o.databaseURL, o.tenantID)
	if err != nil {
		return err
	}
	defer closeFn()

	summaries, err := store.ListNotes(ctx, o.tenantID, index.ListOpts{
		Namespace:         o.namespace,
		Limit:             o.limit,
		Offset:            o.offset,
		IncludeSuperseded: o.includeSuperseded,
	})
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	return emitSummaries(summaries)
}

// newNamespacesCmd builds `persistor namespaces`: list the tenant's namespaces
// with their live-note counts. The operator counterpart of memory_namespaces.
func newNamespacesCmd() *cobra.Command {
	o := listOpts{}
	cmd := &cobra.Command{
		Use:   "namespaces",
		Short: "List a tenant's namespaces with note counts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNamespaces(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func runNamespaces(ctx context.Context, o *listOpts) error {
	store, closeFn, err := openListStore(ctx, &o.databaseURL, o.tenantID)
	if err != nil {
		return err
	}
	defer closeFn()

	counts, err := store.Namespaces(ctx, o.tenantID)
	if err != nil {
		return fmt.Errorf("namespaces: %w", err)
	}
	return emitNamespaces(counts)
}

// openListStore resolves the database URL/tenant (falling back to DATABASE_URL /
// PERSISTOR_TENANT_ID), opens a pool, and returns a store plus its close func.
// Shared by the list and namespaces commands.
func openListStore(ctx context.Context, dbURL *string, tenantID string) (*index.Store, func(), error) {
	if *dbURL == "" {
		*dbURL = os.Getenv("DATABASE_URL")
	}
	if *dbURL == "" {
		return nil, nil, fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}
	if tenantID == "" {
		return nil, nil, fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}
	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, *dbURL, 4)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}
	return index.NewStore(pool, log), pool.Close, nil
}

// displayID renders a summary's id for the table, flagging superseded notes.
func displayID(s *index.NoteSummary) string {
	if s.Superseded {
		return s.ID + " (superseded)"
	}
	return s.ID
}

func emitSummaries(ss []index.NoteSummary) error {
	if flagFmt == "json" {
		out, err := json.MarshalIndent(ss, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tTIER\tKIND\tVER\tID\tTITLE")
	for i := range ss {
		s := &ss[i]
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n", s.Namespace, s.Tier, s.Kind, s.Version, displayID(s), s.Title)
	}
	return w.Flush()
}

func emitNamespaces(counts []index.NamespaceCount) error {
	if flagFmt == "json" {
		out, err := json.MarshalIndent(counts, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNOTES")
	for i := range counts {
		fmt.Fprintf(w, "%s\t%d\n", counts[i].Namespace, counts[i].Count)
	}
	return w.Flush()
}
