package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/persistorai/persistor/internal/models"
	"github.com/spf13/cobra"
)

func newImportKGCmd() *cobra.Command {
	var (
		overwrite    bool
		dryRun       bool
		regenEmbed   bool
		resetUsage   bool
		validateOnly bool
	)

	cmd := &cobra.Command{
		Use:   "import-kg <file>",
		Short: "Import a knowledge graph export file",
		Long: `Import nodes and edges from a Persistor export JSON file.
By default, existing nodes/edges are skipped. Use --overwrite to update them.

Flags:
  --overwrite              Update existing nodes/edges instead of skipping
  --dry-run                Validate and count without writing
  --regenerate-embeddings  Clear imported embeddings so they get regenerated
  --reset-usage            Zero out access_count and last_accessed
  --validate               Only validate the file, don't import`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			filePath := args[0]

			raw, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}

			var data models.ExportFormat
			if err := json.Unmarshal(raw, &data); err != nil {
				return fmt.Errorf("parsing export file: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Export file: schema v%d, %d nodes, %d edges (Persistor %s)\n",
				data.SchemaVersion, data.Stats.NodeCount, data.Stats.EdgeCount, data.PersistorVersion)

			if validateOnly {
				errs, err := apiClient.ValidateImport(ctx, &data)
				if err != nil {
					return fmt.Errorf("validation failed: %w", err)
				}

				if len(errs) == 0 {
					fmt.Fprintln(os.Stderr, "✓ Validation passed — no errors found.")
					return nil
				}

				fmt.Fprintf(os.Stderr, "✗ %d validation error(s):\n", len(errs))

				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}

				return fmt.Errorf("validation failed with %d error(s)", len(errs))
			}

			opts := models.ImportOptions{
				OverwriteExisting:    overwrite,
				DryRun:               dryRun,
				RegenerateEmbeddings: regenEmbed,
				ResetUsage:           resetUsage,
			}

			result, err := apiClient.Import(ctx, &data, opts)
			if err != nil {
				return fmt.Errorf("import failed: %w", err)
			}

			prefix := ""
			if dryRun {
				prefix = "(dry run) "
			}

			fmt.Fprintf(os.Stderr, "%sNodes: %d created, %d updated, %d skipped\n",
				prefix, result.NodesCreated, result.NodesUpdated, result.NodesSkipped)
			fmt.Fprintf(os.Stderr, "%sEdges: %d created, %d updated, %d skipped\n",
				prefix, result.EdgesCreated, result.EdgesUpdated, result.EdgesSkipped)

			if len(result.Errors) > 0 {
				fmt.Fprintf(os.Stderr, "%d error(s):\n", len(result.Errors))

				for _, e := range result.Errors {
					fmt.Fprintf(os.Stderr, "  - %s\n", e)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Update existing nodes/edges")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and count without writing")
	cmd.Flags().BoolVar(&regenEmbed, "regenerate-embeddings", false, "Clear embeddings for regeneration")
	cmd.Flags().BoolVar(&resetUsage, "reset-usage", false, "Zero out access counts")
	cmd.Flags().BoolVar(&validateOnly, "validate", false, "Only validate, don't import")

	return cmd
}
