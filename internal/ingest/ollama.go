package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// OllamaClient talks to a local Ollama instance.
type OllamaClient struct {
	URL    string
	Model  string
	client *http.Client
}

// NewOllamaClient creates a client with defaults from env vars.
// OLLAMA_URL defaults to "http://localhost:11434".
// OLLAMA_MODEL defaults to "gemma4:e4b".
func NewOllamaClient() *OllamaClient {
	return NewOllamaClientWithURL(
		envOrDefault("OLLAMA_URL", "http://localhost:11434"),
		envOrDefault("OLLAMA_MODEL", "gemma4:e4b"),
	)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

// NewOllamaClientWithURL creates a client with explicit URL and model.
func NewOllamaClientWithURL(url, model string) *OllamaClient {
	return &OllamaClient{
		URL:   url,
		Model: model,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// chatRequest is the request body for the Ollama /api/chat endpoint.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  chatOptions   `json:"options"`
	Think    bool          `json:"think"`
}

// chatMessage is a single message in the chat request.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatOptions holds Ollama generation options.
type chatOptions struct {
	NumPredict  int     `json:"num_predict"`
	Temperature float64 `json:"temperature"`
}

// chatResponse is the response body from Ollama /api/chat.
type chatResponse struct {
	Message chatMessage `json:"message"`
}

// Chat sends a prompt and returns the raw response text.
func (o *OllamaClient) Chat(ctx context.Context, prompt string) (string, error) {
	body, err := buildChatRequest(o.Model, prompt)
	if err != nil {
		return "", fmt.Errorf("marshaling chat request: %w", err)
	}

	resp, err := o.doRequest(ctx, body)
	if err != nil {
		return "", err
	}

	return resp, nil
}

func buildChatRequest(model, prompt string) ([]byte, error) {
	req := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream:  false,
		Options: chatOptions{NumPredict: 2048, Temperature: 0.3},
		Think:   false,
	}

	return json.Marshal(req)
}

func (o *OllamaClient) doRequest(ctx context.Context, body []byte) (string, error) {
	url := o.URL + "/api/chat"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	return parseChatResponse(resp)
}

func parseChatResponse(resp *http.Response) (string, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	return chatResp.Message.Content, nil
}
