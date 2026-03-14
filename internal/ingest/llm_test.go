package ingest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/persistorai/persistor/internal/ingest"
)

func TestNewLLMClient_DefaultsToOllama(t *testing.T) {
	t.Setenv("INGEST_LLM_URL", "")

	client := ingest.NewLLMClient()
	_, ok := client.(*ingest.OllamaClient)
	assert.True(t, ok, "expected OllamaClient when INGEST_LLM_URL is not set")
}

func TestNewLLMClient_UsesOpenAIWhenConfigured(t *testing.T) {
	t.Setenv("INGEST_LLM_URL", "https://api.x.ai/v1")
	t.Setenv("INGEST_LLM_MODEL", "grok-beta")
	t.Setenv("INGEST_LLM_API_KEY", "xai-test-key")

	client := ingest.NewLLMClient()
	c, ok := client.(*ingest.OpenAIClient)
	assert.True(t, ok, "expected OpenAIClient when INGEST_LLM_URL is set")
	assert.Equal(t, "https://api.x.ai/v1", c.BaseURL)
	assert.Equal(t, "grok-beta", c.Model)
	assert.Equal(t, "xai-test-key", c.APIKey)
}

func TestLLMProviderName(t *testing.T) {
	ollama := ingest.NewOllamaClientWithURL("http://localhost:11434", "qwen3.5:9b")
	assert.Contains(t, ingest.LLMProviderName(ollama), "Ollama")

	openai := ingest.NewOpenAIClientWithConfig("https://api.x.ai/v1", "grok-beta", "key")
	assert.Contains(t, ingest.LLMProviderName(openai), "OpenAI-compatible")
}
