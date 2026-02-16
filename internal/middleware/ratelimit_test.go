package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := middleware.NewRateLimiter(ctx, 10, 5)

	r := gin.New()
	r.Use(rl.Handler())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.RemoteAddr = "1.2.3.4:1234"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRateLimiter_BlocksExceedingLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := middleware.NewRateLimiter(ctx, 1, 2) // burst=2

	r := gin.New()
	r.Use(rl.Handler())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := range 3 {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)

		if i < 2 && w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d: expected 429, got %d", i, w.Code)
		}
	}
}

func TestRateLimiter_IndependentBuckets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rl := middleware.NewRateLimiter(ctx, 1, 1)

	r := gin.New()
	r.Use(rl.Handler())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Use IP A's token
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.RemoteAddr = "1.1.1.1:1000"
	r.ServeHTTP(w, req)

	// IP B should still work
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req2.RemoteAddr = "2.2.2.2:1000"
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("different IP should not be rate limited, got %d", w2.Code)
	}
}

func TestRateLimiter_TokensRefillOverTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// High rate so even tiny elapsed time refills tokens
	rl := middleware.NewRateLimiter(ctx, 1000000, 2)

	r := gin.New()
	r.Use(rl.Handler())
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Exhaust burst
	for range 2 {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
		req.RemoteAddr = "5.5.5.5:1000"
		r.ServeHTTP(w, req)
	}

	// With 1M/sec rate, next request should refill immediately
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.RemoteAddr = "5.5.5.5:1000"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected tokens to refill, got %d", w.Code)
	}
}
