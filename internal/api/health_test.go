package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/api"
)

func TestLiveness_ReturnsOK(t *testing.T) {
	t.Parallel()

	h := api.NewHealthHandler(nil, nil, testLogger(), "test-v1", "")

	r := gin.New()
	r.GET("/health", h.Liveness)

	w := doRequest(r, http.MethodGet, "/health", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", body["status"])
	}

	if body["version"] != "test-v1" {
		t.Errorf("expected version 'test-v1', got %v", body["version"])
	}
}
