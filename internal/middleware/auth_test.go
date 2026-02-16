package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/middleware"
)

type mockTenantLookup struct {
	validKeys map[string]string
}

func (m *mockTenantLookup) GetTenantByAPIKey(_ context.Context, apiKey string) (string, error) {
	if tid, ok := m.validKeys[apiKey]; ok {
		return tid, nil
	}
	return "", errors.New("invalid key")
}

func TestAuthMiddleware(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	lookup := &mockTenantLookup{validKeys: map[string]string{"good-key": "tenant-1"}}

	tests := []struct {
		name       string
		authHeader string
		wantCode   int
	}{
		{"valid token", "Bearer good-key", http.StatusOK},
		{"missing header", "", http.StatusUnauthorized},
		{"invalid token", "Bearer bad-key", http.StatusUnauthorized},
		{"no bearer prefix", "good-key", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(middleware.AuthMiddleware(lookup, log))
			r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			r.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("got %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestAuthMiddleware_SetsTenantID(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	lookup := &mockTenantLookup{validKeys: map[string]string{"k1": "t1"}}

	var gotTenant string
	r := gin.New()
	r.Use(middleware.AuthMiddleware(lookup, log))
	r.GET("/test", func(c *gin.Context) {
		v, _ := c.Get("tenant_id")
		gotTenant, _ = v.(string)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.Header.Set("Authorization", "Bearer k1")
	r.ServeHTTP(w, req)

	if gotTenant != "t1" {
		t.Fatalf("expected tenant_id=t1, got %q", gotTenant)
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer abc123", "abc123"},
		{"abc123", ""},
		{"", ""},
		{"Bearer ", ""},
		{"bearer abc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tt.header != "" {
				c.Request.Header.Set("Authorization", tt.header)
			}
			got := middleware.ExtractBearerToken(c)
			if got != tt.want {
				t.Errorf("ExtractBearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}
