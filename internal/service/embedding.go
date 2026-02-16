// Package services provides business logic for the persistor.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const embeddingTimeout = 30 * time.Second

// Circuit breaker configuration.
const (
	cbFailureThreshold = 5
	cbCooldown         = 30 * time.Second
)

// Circuit breaker states.
const (
	cbClosed   = iota // Normal operation.
	cbOpen            // Fail fast.
	cbHalfOpen        // Probe with one request.
)

// ErrCircuitOpen is returned when the circuit breaker is open and requests
// are being rejected without calling the embedding service.
var ErrCircuitOpen = errors.New("embedding circuit breaker is open")

// EmbeddingService generates vector embeddings via the Ollama API.
type EmbeddingService struct {
	ollamaURL string
	model     string
	client    *http.Client

	mu              sync.Mutex
	cbState         int
	cbFailures      int
	cbLastFailureAt time.Time
}

type embeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embeddingResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// NewEmbeddingService creates an EmbeddingService for the given Ollama endpoint and model.
func NewEmbeddingService(ollamaURL, model string) *EmbeddingService {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}

			// Resolve hostname to IPs and verify all are loopback.
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolving embedding host: %w", err)
			}

			for _, ip := range ips {
				if !ip.IP.IsLoopback() {
					return nil, fmt.Errorf("embedding service connections restricted to localhost")
				}
			}

			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	return &EmbeddingService{
		ollamaURL: ollamaURL,
		model:     model,
		client:    &http.Client{Timeout: embeddingTimeout, Transport: transport},
		cbState:   cbClosed,
	}
}

// Generate produces a vector embedding for the given text.
// It uses a circuit breaker to fail fast when the embedding service is down.
func (s *EmbeddingService) Generate(ctx context.Context, text string) ([]float32, error) {
	if err := s.cbAllow(); err != nil {
		return nil, err
	}

	result, err := s.doGenerate(ctx, text)
	if err != nil {
		s.cbRecordFailure()

		return nil, err
	}

	s.cbRecordSuccess()

	return result, nil
}

func (s *EmbeddingService) doGenerate(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(embeddingRequest{Model: s.model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("marshaling embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ollamaURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling ollama embed API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain body so the connection can be reused.
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20)) //nolint:errcheck // best-effort drain before close.
		return nil, fmt.Errorf("ollama embed API returned status %d", resp.StatusCode)
	}

	var result embeddingResponse

	limited := io.LimitReader(resp.Body, 10<<20) // 10 MB
	if err := json.NewDecoder(limited).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding embedding response: %w", err)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned empty embeddings")
	}

	return result.Embeddings[0], nil
}

// cbAllow checks whether the circuit breaker permits a request.
// In closed state, all requests pass. In open state, requests are rejected
// until the cooldown expires, at which point we transition to half-open.
// In half-open state, one probe request is allowed.
func (s *EmbeddingService) cbAllow() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.cbState {
	case cbClosed:
		return nil
	case cbOpen:
		if time.Since(s.cbLastFailureAt) >= cbCooldown {
			s.cbState = cbHalfOpen

			return nil
		}

		return ErrCircuitOpen
	case cbHalfOpen:
		// Already probing — reject additional requests.
		return ErrCircuitOpen
	}

	return nil
}

// cbRecordSuccess records a successful call. In half-open state this closes
// the circuit breaker, restoring normal operation.
func (s *EmbeddingService) cbRecordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cbFailures = 0
	s.cbState = cbClosed
}

// cbRecordFailure records a failed call. After reaching the failure threshold
// the circuit breaker transitions to open state.
func (s *EmbeddingService) cbRecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cbFailures++
	s.cbLastFailureAt = time.Now()

	if s.cbFailures >= cbFailureThreshold || s.cbState == cbHalfOpen {
		s.cbState = cbOpen
	}
}
