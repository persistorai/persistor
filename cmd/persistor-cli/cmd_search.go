package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/index"
)

// searchOpts holds the resolved inputs for a retrieval query.
type searchOpts struct {
	databaseURL       string
	tenantID          string
	query             string
	limit             int
	tier              string
	includeSuperseded bool
}

// newSearchCmd builds `persistor search`: run the full-text retrieval
// and print the ranked notes. Default
// retrieval excludes superseded notes; --include-superseded shows history.
func newSearchCmd() *cobra.Command {
	o := searchOpts{}
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the index (full-text over note chunks)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.query = strings.Join(args, " ")
			return runSearch(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	cmd.Flags().IntVar(&o.limit, "limit", 10, "Max notes to return")
	cmd.Flags().StringVar(&o.tier, "tier", "", "Restrict to a tier: core|tail (default: any)")
	cmd.Flags().BoolVar(&o.includeSuperseded, "include-superseded", false, "Include superseded notes (history)")
	return cmd
}

func runSearch(ctx context.Context, o *searchOpts) error {
	if o.databaseURL == "" {
		o.databaseURL = os.Getenv("DATABASE_URL")
	}
	if o.databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)

	pool, err := dbpool.NewPool(ctx, o.databaseURL, 4)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	store := index.NewStore(pool, log)
	hits, err := store.SearchNotes(ctx, o.tenantID, o.query, index.SearchOpts{
		Limit:             o.limit,
		Tier:              o.tier,
		IncludeSuperseded: o.includeSuperseded,
	})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	return emitHits(hits)
}

func emitHits(hits []index.NoteHit) error {
	if flagFmt == "json" {
		out, err := json.MarshalIndent(hits, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "RANK\tTIER\tKIND\tID\tTITLE")
	for i := range hits {
		h := &hits[i]
		id := h.ID
		if h.Superseded {
			id += " (superseded)"
		}
		fmt.Fprintf(w, "%.4f\t%s\t%s\t%s\t%s\n", h.Rank, h.Tier, h.Kind, id, h.Title)
	}
	return w.Flush()
}
