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
	return cmd
}

func printEvalTable(report *eval.Report) {
	headers := []string{"PROMPT", "MODE", "PASS", "FOUND", "EXPECTED", "RETURNED", "LATENCY_MS"}
	rows := make([][]string, 0, len(report.Results))
	for _, result := range report.Results {
		rows = append(rows, []string{
			result.Prompt,
			result.SearchMode,
			fmt.Sprintf("%t", result.Passed),
			fmt.Sprintf("%d", result.FoundExpectedCount),
			fmt.Sprintf("%d", result.ExpectedCount),
			fmt.Sprintf("%d", result.ReturnedCount),
			fmt.Sprintf("%.0f", result.LatencyMs),
		})
	}
	formatTable(headers, rows)
}
