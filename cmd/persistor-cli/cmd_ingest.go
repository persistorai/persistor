package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newIngestCmd() *cobra.Command {
	var (
		dryRun        bool
		source        string
		scanDir       string
		reviewUnknown bool
		resolveID     string
		resolveAs     string
		chunkTokens   int
	)

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest markdown text into the knowledge graph",
		Long: `Extract entities and relationships from text and write them to the graph.

By default, reads from stdin. Use --scan to process a directory of .md files.
Requires a running Ollama instance for LLM extraction.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIngest(cmd, dryRun, source, scanDir, reviewUnknown, resolveID, resolveAs, chunkTokens)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be ingested without writing")
	cmd.Flags().StringVar(&source, "source", "", "Source tag (default: stdin or filename)")
	cmd.Flags().StringVar(&scanDir, "scan", "", "Directory to scan for .md files")
	cmd.Flags().IntVar(&chunkTokens, "chunk-tokens", 600, "Approximate max tokens per ingest chunk")
	cmd.Flags().BoolVar(&reviewUnknown, "review-unknown", false, "List unresolved unknown relation types")
	cmd.Flags().StringVar(&resolveID, "resolve", "", "Resolve an unknown relation by ID")
	cmd.Flags().StringVar(&resolveAs, "as", "", "Canonical type to resolve as (use with --resolve)")

	return cmd
}

func runIngest(
	cmd *cobra.Command,
	dryRun bool,
	source, scanDir string,
	reviewUnknown bool,
	resolveID, resolveAs string,
	chunkTokens int,
) error {
	if reviewUnknown {
		return handleReviewUnknown()
	}

	if resolveID != "" {
		return handleResolve(resolveID, resolveAs)
	}

	return runIngestion(cmd, dryRun, source, scanDir, chunkTokens)
}

func handleReviewUnknown() error {
	fmt.Fprintln(os.Stderr, "This feature requires the Persistor server API (coming soon).")
	fmt.Fprintln(os.Stderr, "Unknown relation types are logged during ingestion for later review.")
	return nil
}

func handleResolve(resolveID, resolveAs string) error {
	if resolveAs == "" {
		return fmt.Errorf("--as flag is required with --resolve")
	}

	fmt.Fprintf(os.Stderr, "This feature requires the Persistor server API (coming soon).\n")
	fmt.Fprintf(os.Stderr, "Would resolve %q as %q.\n", resolveID, resolveAs)

	return nil
}
