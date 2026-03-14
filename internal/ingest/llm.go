package ingest

import (
	"fmt"
	"os"
)

// NewLLMClient returns the appropriate LLM client based on env config.
// If INGEST_LLM_URL is set, returns an OpenAI-compatible client.
// Otherwise, returns the Ollama client (backward compatible).
func NewLLMClient() LLMClient {
	if os.Getenv("INGEST_LLM_URL") != "" {
		return NewOpenAIClient()
	}

	return NewOllamaClient()
}

// LLMProviderName returns a human-readable name for the active provider.
func LLMProviderName(client LLMClient) string {
	switch c := client.(type) {
	case *OpenAIClient:
		return fmt.Sprintf("OpenAI-compatible (%s, model: %s)", c.BaseURL, c.Model)
	case *OllamaClient:
		return fmt.Sprintf("Ollama (%s, model: %s)", c.URL, c.Model)
	default:
		return "unknown"
	}
}
