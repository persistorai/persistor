package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/middleware"
	"github.com/persistorai/persistor/internal/security"
)

func newTestGuard() (*security.BruteForceGuard, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	return security.NewBruteForceGuard(ctx, log), cancel
}

func TestBruteForce_MiddlewareBlocks(t *testing.T) {
	guard, cancel := newTestGuard()
	defer cancel()

	for range 5 {
		guard.RecordFailure("blockedtoken")
	}

	r := gin.New()
	r.Use(middleware.BruteForceMiddleware(guard))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.Header.Set("Authorization", "Bearer blockedtoken")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestBruteForce_MiddlewarePassesNoToken(t *testing.T) {
	guard, cancel := newTestGuard()
	defer cancel()

	r := gin.New()
	r.Use(middleware.BruteForceMiddleware(guard))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("no token should pass through, got %d", w.Code)
	}
}

func TestBruteForce_MiddlewareAllowsUnblockedToken(t *testing.T) {
	guard, cancel := newTestGuard()
	defer cancel()

	r := gin.New()
	r.Use(middleware.BruteForceMiddleware(guard))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.Header.Set("Authorization", "Bearer goodtoken")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unblocked token should pass, got %d", w.Code)
	}
}
