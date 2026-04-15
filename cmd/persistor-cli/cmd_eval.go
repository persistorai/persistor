package main

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/internal/eval"
	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run memory evaluation benchmarks",
	}
	cmd.AddCommand(newEvalRunCmd())
	return cmd
}

func newEvalRunCmd() *cobra.Command {
	var fixturePath string
	var compareProfiles []string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an evaluation fixture against the current Persistor instance",
		Run: func(cmd *cobra.Command, args []string) {
			if fixturePath == "" {
				fatal("eval run", fmt.Errorf("--fixture is required"))
			}

			fixture, err := eval.LoadFixture(fixturePath)
			if err != nil {
				fatal("load fixture", err)
			}

			runner := eval.NewRunner(apiClient.Search)
			if len(compareProfiles) > 0 {
				report, err := runner.ComparePrototypeProfiles(context.Background(), fixture, compareProfiles)
				if err != nil {
					fatal("run eval comparison", err)
				}
				output(report, "")
				return
			}

			report, err := runner.Run(context.Background(), fixture)
			if err != nil {
				fatal("run eval", err)
			}

			if flagFmt == "table" {
				printEvalTable(report)
				return
			}

			output(report, "")
		},
	}

	cmd.Flags().StringVar(&fixturePath, "fixture", "", "Path to evaluation fixture JSON")
	cmd.Flags().StringSliceVar(&compareProfiles, "compare-rerank-profile", nil, "Additional prototype rerank weighting profiles to compare against the default rerank profile")
	return cmd
}

func printEvalTable(report *eval.Report) {
	headers := []string{"PROMPT", "CATEGORY", "MODE", "PROFILE", "PASS", "TOP1", "FOUND", "EXPECTED", "RETURNED", "LATENCY_MS"}
	rows := make([][]string, 0, len(report.Results))
	for _, result := range report.Results {
		top1 := ""
		if result.PreferredFirstExpectation != "" {
			top1 = fmt.Sprintf("%t", result.PreferredFirstMatched)
		}
		rows = append(rows, []string{
			result.Prompt,
			result.Category,
			result.SearchMode,
			result.InternalRerankProfile,
			fmt.Sprintf("%t", result.Passed),
			top1,
			fmt.Sprintf("%d", result.FoundExpectedCount),
			fmt.Sprintf("%d", result.ExpectedCount),
			fmt.Sprintf("%d", result.ReturnedCount),
			fmt.Sprintf("%.0f", result.LatencyMs),
		})
	}
	formatTable(headers, rows)
}
