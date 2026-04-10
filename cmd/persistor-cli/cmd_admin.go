package main

import (
	"context"
	"fmt"

	"github.com/persistorai/persistor/client"
	clientmodels "github.com/persistorai/persistor/internal/models"
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
	cmd.AddCommand(adminReprocessCmd())
	cmd.AddCommand(adminMaintenanceCmd())
	cmd.AddCommand(adminMergeSuggestionsCmd())
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

func adminReprocessCmd() *cobra.Command {
	var batchSize int
	var searchText bool
	var embeddings bool

	cmd := &cobra.Command{
		Use:   "reprocess-nodes",
		Short: "Rebuild search text and/or embeddings for existing nodes in batches",
		Run: func(cmd *cobra.Command, args []string) {
			result, err := apiClient.Admin.ReprocessNodes(context.Background(), clientmodels.ReprocessNodesRequest{
				BatchSize:  batchSize,
				SearchText: searchText,
				Embeddings: embeddings,
			})
			if err != nil {
				fatal("reprocess-nodes", err)
			}
			output(result, fmt.Sprintf("scanned=%d updated_search=%d queued_embeddings=%d", result.Scanned, result.UpdatedSearch, result.QueuedEmbed))
		},
	}
	cmd.Flags().IntVar(&batchSize, "batch-size", 100, "Number of nodes to process in one batch")
	cmd.Flags().BoolVar(&searchText, "search-text", false, "Rebuild stored search_text for scanned nodes")
	cmd.Flags().BoolVar(&embeddings, "embeddings", false, "Queue embeddings for scanned nodes")
	return cmd
}

func adminMaintenanceCmd() *cobra.Command {
	var batchSize int
	var refreshSearchText bool
	var refreshEmbeddings bool
	var scanStaleFacts bool
	var includeDuplicateCandidates bool

	cmd := &cobra.Command{
		Use:   "maintenance-run",
		Short: "Run an explicit maintenance pass for refresh and consolidation scans",
		Run: func(cmd *cobra.Command, args []string) {
			result, err := apiClient.Admin.RunMaintenance(context.Background(), clientmodels.MaintenanceRunRequest{
				BatchSize:                  batchSize,
				RefreshSearchText:          refreshSearchText,
				RefreshEmbeddings:          refreshEmbeddings,
				ScanStaleFacts:             scanStaleFacts,
				IncludeDuplicateCandidates: includeDuplicateCandidates,
			})
			if err != nil {
				fatal("maintenance-run", err)
			}
			output(result, fmt.Sprintf("scanned=%d updated_search_text=%d queued_embeddings=%d stale_fact_nodes=%d", result.Scanned, result.UpdatedSearchText, result.QueuedEmbeddings, result.StaleFactNodes))
		},
	}
	cmd.Flags().IntVar(&batchSize, "batch-size", 100, "Number of nodes to inspect in one maintenance pass")
	cmd.Flags().BoolVar(&refreshSearchText, "refresh-search-text", false, "Refresh stored search_text when derived text has changed")
	cmd.Flags().BoolVar(&refreshEmbeddings, "refresh-embeddings", false, "Queue embeddings for nodes missing them in the scanned batch")
	cmd.Flags().BoolVar(&scanStaleFacts, "scan-stale-facts", false, "Scan fact evidence for stale or superseded entries")
	cmd.Flags().BoolVar(&includeDuplicateCandidates, "include-duplicate-candidates", false, "Count duplicate candidate pairs for the current tenant")
	return cmd
}

func adminMergeSuggestionsCmd() *cobra.Command {
	var limit int
	var minScore float64
	var typeFilter string

	cmd := &cobra.Command{
		Use:   "merge-suggestions",
		Short: "Inspect explainable duplicate candidate suggestions",
		Run: func(cmd *cobra.Command, args []string) {
			suggestions, err := apiClient.Admin.ListMergeSuggestions(context.Background(), clientmodels.MergeSuggestionListOpts{
				Type:     typeFilter,
				Limit:    limit,
				MinScore: minScore,
			})
			if err != nil {
				fatal("merge-suggestions", err)
			}
			if flagFmt == "table" {
				rows := make([][]string, 0, len(suggestions))
				for _, suggestion := range suggestions {
					reason := ""
					if len(suggestion.Reasons) > 0 {
						reason = suggestion.Reasons[0].Description
					}
					rows = append(rows, []string{
						suggestion.Canonical.ID,
						suggestion.Duplicate.ID,
						fmt.Sprintf("%.2f", suggestion.Score),
						reason,
					})
				}
				formatTable([]string{"CANONICAL", "DUPLICATE", "SCORE", "TOP_REASON"}, rows)
				return
			}
			output(map[string]any{"suggestions": suggestions}, fmt.Sprintf("%d", len(suggestions)))
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum number of suggestions to return")
	cmd.Flags().Float64Var(&minScore, "min-score", 0.6, "Minimum suggestion score to include")
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter to a single node type")
	return cmd
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
