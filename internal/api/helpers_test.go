package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const testTenantID = "00000000-0000-0000-0000-000000000001"

func init() {
	gin.SetMode(gin.TestMode)
}

func testLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.ErrorLevel)

	return l
}

// newTestRouter creates a gin engine with tenant_id middleware for testing.
func newTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", testTenantID)
		c.Next()
	})

	return r
}

// doRequest performs an HTTP request against the test router and returns the recorder.
func doRequest(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}
