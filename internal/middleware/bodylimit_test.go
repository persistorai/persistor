package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/persistorai/persistor/internal/middleware"
)

func TestMaxBodySizeByPath_UsesOverrideForImportRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.MaxBodySizeByPath(8, map[string]int64{
		"/api/v1/import": 32,
	}))
	r.POST("/api/v1/import", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}

		c.Status(http.StatusOK)
	})
	r.POST("/api/v1/nodes", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}

		c.Status(http.StatusOK)
	})

	largeForDefault := strings.Repeat("a", 16)

	importReq := httptest.NewRequest(http.MethodPost, "/api/v1/import", strings.NewReader(largeForDefault))
	importW := httptest.NewRecorder()
	r.ServeHTTP(importW, importReq)
	if importW.Code != http.StatusOK {
		t.Fatalf("import route code = %d, want %d", importW.Code, http.StatusOK)
	}

	nodesReq := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", strings.NewReader(largeForDefault))
	nodesW := httptest.NewRecorder()
	r.ServeHTTP(nodesW, nodesReq)
	if nodesW.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("nodes route code = %d, want %d", nodesW.Code, http.StatusRequestEntityTooLarge)
	}
}
