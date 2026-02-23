package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the full knowledge graph to a JSON file",
		Long: `Export all nodes, edges, embeddings, and metadata to a portable JSON file.
The export is full-fidelity: embeddings, access counts, salience scores, and
all properties are preserved. Use 'persistor import' to restore.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			data, err := apiClient.Export(ctx)
			if err != nil {
				return fmt.Errorf("export failed: %w", err)
			}

			out, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return fmt.Errorf("marshalling export: %w", err)
			}

			if outputPath == "" {
				outputPath = fmt.Sprintf("persistor-export-%s.json",
					time.Now().UTC().Format("20060102T150405Z"))
			}

			if outputPath == "-" {
				_, err = os.Stdout.Write(out)
				return err
			}

			if err := os.WriteFile(outputPath, out, 0o600); err != nil {
				return fmt.Errorf("writing export file: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Exported %d nodes, %d edges to %s\n",
				data.Stats.NodeCount, data.Stats.EdgeCount, outputPath)

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: persistor-export-<timestamp>.json, use - for stdout)")

	return cmd
}
