package main

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/client"
	"github.com/spf13/cobra"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands",
	}
	cmd.AddCommand(adminHealthCmd())
	cmd.AddCommand(adminStatsCmd())
	cmd.AddCommand(adminBackfillCmd())
	return cmd
}

func adminHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check server health",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := apiClient.Health(context.Background())
			if err != nil {
				fatal("health", err)
			}
			output(resp, resp.Status)
		},
	}
}

func adminStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show knowledge graph statistics",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := apiClient.Stats(context.Background())
			if err != nil {
				fatal("stats", err)
			}
			if flagFmt == "table" {
				formatTable(
					[]string{"METRIC", "VALUE"},
					[][]string{
						{"Nodes", fmt.Sprintf("%d", resp.Nodes)},
						{"Edges", fmt.Sprintf("%d", resp.Edges)},
						{"Entity Types", fmt.Sprintf("%d", resp.EntityTypes)},
						{"Avg Salience", fmt.Sprintf("%.4f", resp.AvgSalience)},
						{"Embeddings Done", fmt.Sprintf("%d", resp.EmbeddingsComplete)},
						{"Embeddings Pending", fmt.Sprintf("%d", resp.EmbeddingsPending)},
					},
				)
				return
			}
			output(resp, "")
		},
	}
}

func adminBackfillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backfill-embeddings",
		Short: "Queue embedding generation for nodes without embeddings",
		Run: func(cmd *cobra.Command, args []string) {
			queued, err := apiClient.Admin.BackfillEmbeddings(context.Background())
			if err != nil {
				fatal("backfill", err)
			}
			output(map[string]int{"queued": queued}, fmt.Sprintf("%d", queued))
		},
	}
}

func newAuditCmd() *cobra.Command {
	var entityID, action string
	var limit int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Query audit logs",
		Run: func(cmd *cobra.Command, args []string) {
			opts := &client.AuditQueryOptions{
				EntityID: entityID,
				Action:   action,
				Limit:    limit,
			}
			entries, _, err := apiClient.Audit.Query(context.Background(), opts)
			if err != nil {
				fatal("audit query", err)
			}
			if flagFmt == "table" {
				headers := []string{"ID", "ACTION", "ENTITY_TYPE", "ENTITY_ID", "CREATED_AT"}
				var rows [][]string
				for _, e := range entries {
					rows = append(rows, []string{e.ID, e.Action, e.EntityType, e.EntityID, e.CreatedAt.Format("2006-01-02 15:04:05")})
				}
				formatTable(headers, rows)
				return
			}
			output(entries, "")
		},
	}
	cmd.Flags().StringVar(&entityID, "entity", "", "Filter by entity ID")
	cmd.Flags().StringVar(&action, "action", "", "Filter by action")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")

	cmd.AddCommand(auditPurgeCmd())
	return cmd
}

func auditPurgeCmd() *cobra.Command {
	var retentionDays int
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge old audit entries",
		Run: func(cmd *cobra.Command, args []string) {
			deleted, err := apiClient.Audit.Purge(context.Background(), retentionDays)
			if err != nil {
				fatal("audit purge", err)
			}
			output(map[string]int{"deleted": deleted}, fmt.Sprintf("%d", deleted))
		},
	}
	cmd.Flags().IntVar(&retentionDays, "retention-days", 90, "Delete entries older than N days")
	return cmd
}
