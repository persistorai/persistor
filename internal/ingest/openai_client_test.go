package ingest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/persistorai/persistor/internal/ingest"
)

func TestOpenAIClient_Chat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		assert.Equal(t, "test-model", req["model"])

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "world"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ingest.NewOpenAIClientWithConfig(server.URL, "test-model", "test-key")

	result, err := client.Chat(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "world", result)
}

func TestOpenAIClient_Chat_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	client := ingest.NewOpenAIClientWithConfig(server.URL, "test-model", "bad-key")

	_, err := client.Chat(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestOpenAIClient_Chat_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"choices": []map[string]any{}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ingest.NewOpenAIClientWithConfig(server.URL, "test-model", "test-key")

	_, err := client.Chat(context.Background(), "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices")
}

func TestOpenAIClient_HealthCheck(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		client := ingest.NewOpenAIClientWithConfig("https://api.example.com/v1", "gpt-4", "sk-test")
		err := client.HealthCheck(context.Background())
		assert.NoError(t, err)
	})

	t.Run("missing URL", func(t *testing.T) {
		client := ingest.NewOpenAIClientWithConfig("", "gpt-4", "sk-test")
		err := client.HealthCheck(context.Background())
		assert.ErrorContains(t, err, "INGEST_LLM_URL")
	})

	t.Run("missing model", func(t *testing.T) {
		client := ingest.NewOpenAIClientWithConfig("https://api.example.com", "", "sk-test")
		err := client.HealthCheck(context.Background())
		assert.ErrorContains(t, err, "INGEST_LLM_MODEL")
	})

	t.Run("missing API key", func(t *testing.T) {
		client := ingest.NewOpenAIClientWithConfig("https://api.example.com", "gpt-4", "")
		err := client.HealthCheck(context.Background())
		assert.ErrorContains(t, err, "INGEST_LLM_API_KEY")
	})
}
