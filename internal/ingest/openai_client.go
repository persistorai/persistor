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

// OpenAIClient talks to any OpenAI-compatible API (xAI, OpenAI, Anthropic, etc).
type OpenAIClient struct {
	BaseURL string
	Model   string
	APIKey  string
	client  *http.Client
}

// NewOpenAIClient creates a client from INGEST_LLM env vars.
func NewOpenAIClient() *OpenAIClient {
	return &OpenAIClient{
		BaseURL: os.Getenv("INGEST_LLM_URL"),
		Model:   os.Getenv("INGEST_LLM_MODEL"),
		APIKey:  os.Getenv("INGEST_LLM_API_KEY"),
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

// NewOpenAIClientWithConfig creates a client with explicit configuration.
func NewOpenAIClientWithConfig(baseURL, model, apiKey string) *OpenAIClient {
	return &OpenAIClient{
		BaseURL: baseURL,
		Model:   model,
		APIKey:  apiKey,
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

// openaiRequest is the request body for the chat completions endpoint.
type openaiRequest struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	Temperature         float64       `json:"temperature,omitempty"`
	MaxTokens           int           `json:"max_tokens,omitempty"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
}

// openaiResponse is the response from the chat completions endpoint.
type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

// openaiChoice is a single choice in the response.
type openaiChoice struct {
	Message chatMessage `json:"message"`
}

// openaiError is an error returned by the API.
type openaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Chat sends a prompt and returns the response text.
func (c *OpenAIClient) Chat(ctx context.Context, prompt string) (string, error) {
	body, err := c.buildRequest(prompt)
	if err != nil {
		return "", fmt.Errorf("marshaling openai request: %w", err)
	}

	return c.doRequest(ctx, body)
}

func (c *OpenAIClient) buildRequest(prompt string) ([]byte, error) {
	req := openaiRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	if usesCompletionTokens(c.Model) {
		req.MaxCompletionTokens = 4096
	} else {
		req.Temperature = 0.3
		req.MaxTokens = 4096
	}

	return json.Marshal(req)
}

func (c *OpenAIClient) doRequest(ctx context.Context, body []byte) (string, error) {
	url := c.BaseURL + "/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling LLM API: %w", err)
	}
	defer resp.Body.Close()

	return parseOpenAIResponse(resp)
}

func usesCompletionTokens(model string) bool {
	return len(model) >= 5 && model[:5] == "gpt-5"
}

func parseOpenAIResponse(resp *http.Response) (string, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result openaiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM API returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}

// HealthCheck does a lightweight validation of the API configuration.
func (c *OpenAIClient) HealthCheck(ctx context.Context) error {
	if c.BaseURL == "" {
		return fmt.Errorf("INGEST_LLM_URL is not set")
	}
	if c.Model == "" {
		return fmt.Errorf("INGEST_LLM_MODEL is not set")
	}
	if c.APIKey == "" {
		return fmt.Errorf("INGEST_LLM_API_KEY is not set")
	}

	return nil
}
