package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/eval"
	"github.com/persistorai/persistor/internal/index"
)

// evalOpts holds the DB/tenant inputs for `persistor eval`.
type evalOpts struct {
	databaseURL string
	tenantID    string
}

// evalRow is one baseline's score line in the scoreboard.
type evalRow struct {
	System       string  `json:"system"`
	RecallAtK    float64 `json:"recall_at_k"`
	PrecisionAtK float64 `json:"precision_at_k"`
	Passed       int     `json:"passed"`
	Questions    int     `json:"questions"`
}

// newEvalCmd builds `persistor eval`: run a retrieval fixture against the live
// index for the baselines and print the scoreboard — does full-text search over
// the notes beat a static always-loaded surface? The fixture (which may contain
// personal facts) is supplied by the caller and lives in the private notes repo,
// never here.
func newEvalCmd() *cobra.Command {
	o := evalOpts{}
	var fixturePath string
	cmd := &cobra.Command{
		Use:   "eval --fixture <path>",
		Short: "Score the index against retrieval baselines",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEval(cmd.Context(), &o, fixturePath)
		},
	}
	cmd.Flags().StringVar(&fixturePath, "fixture", "", "Path to the eval fixture JSON (required)")
	cmd.Flags().StringVar(&o.tenantID, "tenant", os.Getenv("PERSISTOR_TENANT_ID"), "Tenant UUID (env PERSISTOR_TENANT_ID)")
	cmd.Flags().StringVar(&o.databaseURL, "database-url", "", "Postgres URL (env DATABASE_URL)")
	return cmd
}

func runEval(ctx context.Context, o *evalOpts, fixturePath string) error {
	if fixturePath == "" {
		return fmt.Errorf("--fixture is required")
	}
	if o.databaseURL == "" {
		o.databaseURL = os.Getenv("DATABASE_URL")
	}
	if o.databaseURL == "" {
		return fmt.Errorf("no database URL (set --database-url or DATABASE_URL)")
	}
	if o.tenantID == "" {
		return fmt.Errorf("no tenant (set --tenant or PERSISTOR_TENANT_ID)")
	}

	fixture, err := eval.LoadFixture(fixturePath)
	if err != nil {
		return fmt.Errorf("loading fixture: %w", err)
	}

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	pool, err := dbpool.NewPool(ctx, o.databaseURL, 4)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()
	store := index.NewStore(pool, log)

	systems := []struct {
		name     string
		searcher eval.SearchClient
	}{
		{"persistor (all notes)", index.AllNotesSearcher(store, o.tenantID)},
		{"static MEMORY.md (core only)", index.StaticMemorySearcher(store, o.tenantID)},
		{"no memory (cold)", index.NoMemorySearcher()},
	}

	rows := make([]evalRow, 0, len(systems))
	for _, sys := range systems {
		report, err := eval.NewRunner(sys.searcher).Run(ctx, fixture)
		if err != nil {
			return fmt.Errorf("eval %q: %w", sys.name, err)
		}
		rows = append(rows, evalRow{
			System: sys.name, RecallAtK: report.RecallAtK, PrecisionAtK: report.PrecisionAtK,
			Passed: report.Passed, Questions: report.QuestionCount,
		})
	}

	return printEval(fixture.Name, rows)
}

func printEval(fixtureName string, rows []evalRow) error {
	if flagFmt == "json" {
		out, err := json.MarshalIndent(map[string]any{"fixture": fixtureName, "scoreboard": rows}, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}
	fmt.Printf("Fixture: %s\n\n", fixtureName)
	fmt.Printf("%-32s  %-9s  %-9s  %s\n", "SYSTEM", "RECALL@5", "PREC@5", "PASS")
	for i := range rows {
		r := &rows[i]
		fmt.Printf("%-32s  %-9.3f  %-9.3f  %d/%d\n", r.System, r.RecallAtK, r.PrecisionAtK, r.Passed, r.Questions)
	}
	return nil
}
