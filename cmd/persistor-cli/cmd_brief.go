package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/index"
)

// briefOpts holds the resolved inputs for assembling a working-set.
type briefOpts struct {
	databaseURL string
	tenantID    string
	cwd         string
	topics      string
	asJSON      bool
	opt         index.BriefOptions
}

// newBriefCmd builds `persistor brief`: assemble the dynamic working-set — every
// Core note plus the Tail notes most relevant to the current project, under a
// hard token budget. Emitted as a markdown block for injection at session start.
// Notes live in Postgres now, so the working-set is assembled entirely from the
// database (no disk degrade path).
func newBriefCmd() *cobra.Command {
	o := briefOpts{}
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "Assemble the dynamic memory working-set (Core + retrieved Tail)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBrief(cmd.Context(), &o)
		},
	}
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
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
	if o.databaseURL == "" {
		return fmt.Errorf("a database URL is required (--database-url or DATABASE_URL)")
	}
	if o.tenantID == "" {
		return fmt.Errorf("a tenant is required (--tenant or PERSISTOR_TENANT_ID)")
	}

	seed := index.SeedQuery(o.cwd, o.topics)

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, o.databaseURL, 4)
	if err != nil {
		return err
	}
	defer pool.Close()

	store := index.NewStore(pool, log)
	ws, err := index.AssembleWorkingSet(ctx, store, o.tenantID, seed, o.opt)
	if err != nil {
		return err
	}
	return emitWorkingSet(&ws, o.asJSON)
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
