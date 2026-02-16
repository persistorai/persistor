package main

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/client"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var mode string
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the knowledge graph",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			query := args[0]

			switch mode {
			case "text":
				opts := &client.SearchOptions{Limit: limit}
				nodes, err := apiClient.Search.FullText(ctx, query, opts)
				if err != nil {
					fatal("search", err)
				}
				if flagFmt == "table" {
					printNodeTable(nodes)
					return
				}
				output(nodes, "")

			case "vector":
				scored, err := apiClient.Search.Semantic(ctx, query, limit)
				if err != nil {
					fatal("search", err)
				}
				if flagFmt == "table" {
					headers := []string{"ID", "LABEL", "TYPE", "SCORE"}
					var rows [][]string
					for _, n := range scored {
						rows = append(rows, []string{n.ID, n.Label, n.Type, fmt.Sprintf("%.4f", n.Score)})
					}
					formatTable(headers, rows)
					return
				}
				output(scored, "")

			default: // hybrid
				opts := &client.SearchOptions{Limit: limit}
				nodes, err := apiClient.Search.Hybrid(ctx, query, opts)
				if err != nil {
					fatal("search", err)
				}
				if flagFmt == "table" {
					printNodeTable(nodes)
					return
				}
				output(nodes, "")
			}
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "hybrid", "Search mode: text|vector|hybrid")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	return cmd
}

func printNodeTable(nodes []client.Node) {
	headers := []string{"ID", "LABEL", "TYPE", "SALIENCE"}
	var rows [][]string
	for _, n := range nodes {
		rows = append(rows, []string{n.ID, n.Label, n.Type, fmt.Sprintf("%.2f", n.Salience)})
	}
	formatTable(headers, rows)
}
