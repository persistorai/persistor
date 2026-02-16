// Package client provides a typed Go SDK for the Persistor REST API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is the top-level Persistor API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client

	Nodes    *NodeService
	Edges    *EdgeService
	Search   *SearchService
	Graph    *GraphService
	Bulk     *BulkService
	Salience *SalienceService
	Audit    *AuditService
	Admin    *AdminService
	History  *HistoryService
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *Client) { c.apiKey = key }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// New creates a Persistor client for the given base URL (e.g. "http://localhost:3030").
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	c.Nodes = &NodeService{c: c}
	c.Edges = &EdgeService{c: c}
	c.Search = &SearchService{c: c}
	c.Graph = &GraphService{c: c}
	c.Bulk = &BulkService{c: c}
	c.Salience = &SalienceService{c: c}
	c.Audit = &AuditService{c: c}
	c.Admin = &AdminService{c: c}
	c.History = &HistoryService{c: c}
	return c
}

// Health returns the liveness check response.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.get(ctx, "/api/v1/health", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Stats returns aggregate knowledge graph statistics.
func (c *Client) Stats(ctx context.Context) (*StatsResponse, error) {
	var resp StatsResponse
	if err := c.get(ctx, "/api/v1/stats", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// do executes an HTTP request and decodes the JSON response.
func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	u := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, respBody)
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// get is a convenience wrapper for GET requests with query parameters.
func (c *Client) get(ctx context.Context, path string, params url.Values, result any) error {
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	return c.do(ctx, http.MethodGet, path, nil, result)
}

// post is a convenience wrapper for POST requests.
func (c *Client) post(ctx context.Context, path string, body any, result any) error {
	return c.do(ctx, http.MethodPost, path, body, result)
}

// put is a convenience wrapper for PUT requests.
func (c *Client) put(ctx context.Context, path string, body any, result any) error {
	return c.do(ctx, http.MethodPut, path, body, result)
}

// del is a convenience wrapper for DELETE requests.
func (c *Client) del(ctx context.Context, path string, params url.Values, result any) error {
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	return c.do(ctx, http.MethodDelete, path, nil, result)
}
