package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/persistorai/persistor/internal/ingest"
	"github.com/spf13/cobra"
)

func runIngestion(cmd *cobra.Command, dryRun bool, source, scanDir string) error {
	llmClient := ingest.NewLLMClient()
	fmt.Fprintf(os.Stderr, "LLM provider: %s\n", ingest.LLMProviderName(llmClient))

	if err := checkLLMHealth(llmClient); err != nil {
		return err
	}

	ext := ingest.NewExtractor(llmClient)
	gc := ingest.NewPersistorClient(apiClient)

	if scanDir != "" {
		return scanAndIngest(cmd.Context(), ext, gc, scanDir, dryRun)
	}

	return ingestStdin(cmd.Context(), ext, gc, source, dryRun)
}

func checkLLMHealth(client ingest.LLMClient) error {
	if c, ok := client.(*ingest.OpenAIClient); ok {
		return c.HealthCheck(context.Background())
	}

	return checkOllamaHealth()
}

func checkOllamaHealth() error {
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ollamaURL+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("creating Ollama health check: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Ollama is not reachable at %s: %w\nMake sure Ollama is running (ollama serve)", ollamaURL, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain response body

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d at %s", resp.StatusCode, ollamaURL)
	}

	return nil
}

func ingestStdin(
	ctx context.Context,
	ext *ingest.Extractor,
	gc ingest.GraphClient,
	source string,
	dryRun bool,
) error {
	if source == "" {
		source = "stdin"
	}

	w := ingest.NewWriter(gc, source)
	fetcher := ingest.NewClientEntityFetcher(apiClient)
	ing := ingest.NewIngester(ext, w, fetcher)

	report, err := ing.Ingest(ctx, os.Stdin, ingest.IngestOpts{
		Source: source,
		DryRun: dryRun,
	})
	if err != nil {
		return fmt.Errorf("ingestion failed: %w", err)
	}

	printReport(source, report, dryRun)

	return nil
}

func scanAndIngest(
	ctx context.Context,
	ext *ingest.Extractor,
	gc ingest.GraphClient,
	dir string,
	dryRun bool,
) error {
	entries, err := findMarkdownFiles(dir)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No .md files found in", dir)
		return nil
	}

	for _, path := range entries {
		if err := ingestFile(ctx, ext, gc, path, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
		}
	}

	return nil
}

func findMarkdownFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning directory %s: %w", dir, err)
	}

	return files, nil
}

func ingestFile(
	ctx context.Context,
	ext *ingest.Extractor,
	gc ingest.GraphClient,
	path string,
	dryRun bool,
) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	source := filepath.Base(path)
	w := ingest.NewWriter(gc, source)
	fetcher := ingest.NewClientEntityFetcher(apiClient)
	ing := ingest.NewIngester(ext, w, fetcher)

	report, err := ing.Ingest(ctx, f, ingest.IngestOpts{
		Source: source,
		DryRun: dryRun,
	})
	if err != nil {
		return fmt.Errorf("ingesting %s: %w", path, err)
	}

	printReport(source, report, dryRun)

	return nil
}

func printReport(source string, report *ingest.IngestReport, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "(dry run) "
	}

	fmt.Fprintf(os.Stderr, "%sIngested: %s (%d chunks)\n", prefix, source, report.Chunks)
	fmt.Fprintf(os.Stderr, "  Created:  %d nodes, %d edges\n", report.CreatedNodes, report.CreatedEdges)
	fmt.Fprintf(os.Stderr, "  Updated:  %d nodes (merged properties)\n", report.UpdatedNodes)
	fmt.Fprintf(os.Stderr, "  Skipped:  %d edges (orphan entities)\n", report.SkippedEdges)
	fmt.Fprintf(os.Stderr, "  Unknown:  %d relation types logged\n", report.UnknownRelations)
	if report.CreatedEpisodes > 0 || report.CreatedEvents > 0 {
		fmt.Fprintf(os.Stderr, "  Episodic: %d episodes, %d events\n", report.CreatedEpisodes, report.CreatedEvents)
	}

	for _, e := range report.Errors {
		fmt.Fprintf(os.Stderr, "  Error: %s\n", e)
	}
}
