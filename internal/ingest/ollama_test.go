package ingest_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/persistorai/persistor/internal/ingest"
)

func TestOllamaChat_RequestConstruction(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}

		if err := json.Unmarshal(body, &receivedBody); err != nil {
			t.Fatalf("parsing body: %v", err)
		}

		resp := map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "test response",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	defer server.Close()

	client := ingest.NewOllamaClientWithURL(server.URL, "test-model")

	result, err := client.Chat(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "test response" {
		t.Errorf("expected 'test response', got %q", result)
	}

	assertRequestField(t, receivedBody, "model", "test-model")
	assertStreamFalse(t, receivedBody)
	assertThinkFalse(t, receivedBody)
	assertOptions(t, receivedBody)
	assertMessages(t, receivedBody, "hello world")
}

func assertRequestField(t *testing.T, body map[string]any, key, expected string) {
	t.Helper()

	val, ok := body[key].(string)
	if !ok || val != expected {
		t.Errorf("expected %s=%q, got %v", key, expected, body[key])
	}
}

func assertStreamFalse(t *testing.T, body map[string]any) {
	t.Helper()

	stream, ok := body["stream"].(bool)
	if !ok || stream {
		t.Error("expected stream=false")
	}
}

func assertThinkFalse(t *testing.T, body map[string]any) {
	t.Helper()

	think, ok := body["think"].(bool)
	if !ok || think {
		t.Error("expected think=false")
	}
}

func assertOptions(t *testing.T, body map[string]any) {
	t.Helper()

	opts, ok := body["options"].(map[string]any)
	if !ok {
		t.Fatal("missing options")
	}

	numPredict, ok := opts["num_predict"].(float64)
	if !ok || numPredict != 2048 {
		t.Errorf("expected num_predict=2048, got %v", opts["num_predict"])
	}

	temp, ok := opts["temperature"].(float64)
	if !ok || temp != 0.3 {
		t.Errorf("expected temperature=0.3, got %v", opts["temperature"])
	}
}

func assertMessages(t *testing.T, body map[string]any, expectedContent string) {
	t.Helper()

	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %v", body["messages"])
	}

	msg, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatal("message is not an object")
	}

	content, ok := msg["content"].(string)
	if !ok || content != expectedContent {
		t.Errorf("expected content %q, got %v", expectedContent, msg["content"])
	}
}

func TestOllamaChat_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := ingest.NewOllamaClientWithURL(server.URL, "test-model")

	_, err := client.Chat(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestOllamaChat_ValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": `{"entities": [], "relationships": [], "facts": []}`,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	defer server.Close()

	client := ingest.NewOllamaClientWithURL(server.URL, "test-model")

	result, err := client.Chat(context.Background(), "extract stuff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !json.Valid([]byte(result)) {
		t.Errorf("expected valid JSON response, got %q", result)
	}
}
