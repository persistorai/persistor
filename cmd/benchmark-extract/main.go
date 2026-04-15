package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/persistorai/persistor/internal/ingest"
)

type benchmarkResult struct {
	File   string                   `json:"file"`
	Model  string                   `json:"model"`
	Source string                   `json:"source"`
	Result *ingest.ExtractionResult `json:"result,omitempty"`
	Error  string                   `json:"error,omitempty"`
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "usage: %s <provider: ollama|openai> <model> <file> [file...]\n", os.Args[0])
		os.Exit(2)
	}

	provider := os.Args[1]
	model := os.Args[2]
	files := os.Args[3:]

	var llm ingest.LLMClient
	switch provider {
	case "ollama":
		llm = ingest.NewOllamaClientWithURL(envOr("OLLAMA_URL", "http://localhost:11434"), model)
	case "openai":
		llm = ingest.NewOpenAIClientWithConfig(envOr("INGEST_LLM_URL", "https://api.openai.com/v1"), model, envOr("INGEST_LLM_API_KEY", os.Getenv("OPENAI_API_KEY")))
	default:
		fmt.Fprintf(os.Stderr, "unknown provider: %s\n", provider)
		os.Exit(2)
	}

	extractor := ingest.NewExtractor(llm)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			_ = enc.Encode(benchmarkResult{File: file, Model: model, Source: provider, Error: err.Error()})
			continue
		}

		result := ingest.BenchmarkExtract(context.Background(), extractor, string(content), 600, nil)
		_ = enc.Encode(benchmarkResult{
			File:   filepath.Base(file),
			Model:  model,
			Source: provider,
			Result: result,
		})
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
