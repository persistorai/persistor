package main

import (
	"bytes"
	"net/http"
	"strconv"
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

// contentLengthBuffer buffers a handler's full response so the server sends a
// definite Content-Length instead of chunked transfer encoding. Some HTTP client
// stacks handle chunked responses unreliably — read behavior that degrades as the
// body grows — which showed up as flaky failures on larger tool results through a
// proxy, while the server itself returned the complete body every time.
//
// Buffering is safe because the MCP transport runs in JSON-response
// (non-streaming) mode and every other route is small. As a guard, if a handler
// ever streams (Content-Type text/event-stream), the writer switches to
// pass-through so an SSE response is never withheld.
func contentLengthBuffer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bw := &bufferingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(bw, r)
		bw.finish()
	})
}

// bufferingWriter accumulates the response body, then emits it with an explicit
// Content-Length on finish — unless the handler streams, in which case it passes
// through untouched.
type bufferingWriter struct {
	http.ResponseWriter
	buf         bytes.Buffer
	status      int
	passthrough bool
	wroteHeader bool
}

// streaming reports whether the handler declared a streaming content type, in
// which case the response must not be buffered.
func (b *bufferingWriter) streaming() bool {
	return b.Header().Get("Content-Type") == "text/event-stream"
}

func (b *bufferingWriter) WriteHeader(code int) {
	b.status = code
	if b.streaming() {
		b.passthrough = true
		b.wroteHeader = true
		b.ResponseWriter.WriteHeader(code)
	}
}

func (b *bufferingWriter) Write(p []byte) (int, error) {
	if !b.passthrough && !b.wroteHeader && b.streaming() {
		b.passthrough = true
		b.wroteHeader = true
		b.ResponseWriter.WriteHeader(b.status)
	}
	if b.passthrough {
		return b.ResponseWriter.Write(p)
	}
	return b.buf.Write(p)
}

// finish emits the buffered response with a definite Content-Length. It is a
// no-op when the response streamed (already sent) or had no body.
func (b *bufferingWriter) finish() {
	if b.passthrough {
		return
	}
	b.Header().Set("Content-Length", strconv.Itoa(b.buf.Len()))
	b.ResponseWriter.WriteHeader(b.status)
	if b.buf.Len() > 0 {
		if _, err := b.ResponseWriter.Write(b.buf.Bytes()); err != nil {
			return
		}
	}
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
