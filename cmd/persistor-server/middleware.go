package main

import (
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// securityHeaders sets conservative security response headers on every route.
// Clickjacking is the concrete concern: the /authorize consent page issues an
// OAuth authorization, so it must never be framable. These headers are safe for
// the JSON API and the (top-level, never-framed) consent page alike; they do not
// constrain script/connect sources, so they don't risk the consent page's
// cross-origin Stytch calls (tighter script-src/SRI is tracked as a follow-up).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response status code for access logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// requestLogger logs one structured line per request (method, path, status,
// duration) for a long-running network daemon that otherwise has no per-call
// visibility. It deliberately logs only the path, never the query string or
// headers, to avoid recording bearer tokens or other sensitive values.
func requestLogger(log *logrus.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.WithFields(logrus.Fields{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rec.status,
			"duration": time.Since(start).String(),
		}).Info("request")
	})
}
