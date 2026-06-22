package main

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
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

// maxResponseBuffer caps how much of a response contentLengthBuffer will hold in
// memory. Tool results are bounded (list pages cap at 500 summaries, note bodies
// at 1 MiB), so 8 MiB is well above any normal response; a handler that exceeds
// it falls back to chunked streaming rather than buffering without bound.
const maxResponseBuffer = 8 << 20

// contentLengthBuffer buffers a handler's full response so the server sends a
// definite Content-Length instead of chunked transfer encoding. Some HTTP client
// stacks handle chunked responses unreliably — read behavior that degrades as the
// body grows — which showed up as flaky failures on larger tool results through a
// proxy, while the server itself returned the complete body every time.
//
// Buffering is safe because the MCP transport runs in JSON-response
// (non-streaming) mode and every other route is small. Two guards keep it
// bounded: if a handler streams (Content-Type text/event-stream) or explicitly
// flushes, or if the buffered body exceeds maxResponseBuffer, the writer switches
// to pass-through so the response is neither withheld nor held in memory unbounded.
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
		b.switchToPassthrough()
	}
}

func (b *bufferingWriter) Write(p []byte) (int, error) {
	if !b.passthrough && b.streaming() {
		b.switchToPassthrough()
	}
	// Outsized response: stop withholding it, flush what we have, and stream the
	// rest (chunked) so memory stays bounded at the cost of the Content-Length.
	if !b.passthrough && b.buf.Len()+len(p) > maxResponseBuffer {
		b.switchToPassthrough()
	}
	if b.passthrough {
		return b.ResponseWriter.Write(p)
	}
	return b.buf.Write(p)
}

// switchToPassthrough emits the status line and any buffered bytes to the
// underlying writer, then routes all further writes straight through. Idempotent.
func (b *bufferingWriter) switchToPassthrough() {
	if b.passthrough {
		return
	}
	b.passthrough = true
	b.wroteHeader = true
	b.ResponseWriter.WriteHeader(b.status)
	buffered := b.buf.Bytes()
	b.buf.Reset()
	if len(buffered) > 0 {
		if _, err := b.ResponseWriter.Write(buffered); err != nil {
			return
		}
	}
}

// Flush satisfies http.Flusher: it switches to pass-through (sending the status
// and any buffered bytes) and flushes the underlying writer, so a handler that
// explicitly flushes — e.g. a future streaming path — is not silently withheld.
func (b *bufferingWriter) Flush() {
	b.switchToPassthrough()
	if f, ok := b.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying ResponseWriter to http.ResponseController so it
// can find Flusher/other optional interfaces down the wrapper chain.
func (b *bufferingWriter) Unwrap() http.ResponseWriter {
	return b.ResponseWriter
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

// Unwrap exposes the underlying ResponseWriter to http.ResponseController so
// optional interfaces (e.g. Flusher) resolve through this wrapper too.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// observe is the outermost middleware: it assigns a correlation id, threads
// per-request observability state through the context, records process metrics,
// and logs one structured line per request for a long-running daemon that
// otherwise has no per-call visibility. It deliberately logs only the path, never
// the query string or headers, to avoid recording bearer tokens. The tenant is
// filled in by tenantServer once the token resolves, so it appears in the log
// without the logger parsing the token itself.
func observe(log *logrus.Logger, m *metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = uuid.NewString()
		}
		st := &reqState{requestID: rid}
		r = r.WithContext(context.WithValue(r.Context(), reqStateKey, st))
		w.Header().Set("X-Request-Id", rid)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		durMs := time.Since(start).Milliseconds()

		m.record(rec.status, durMs)
		log.WithFields(logrus.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"status":     rec.status,
			"duration":   time.Since(start).String(),
			"request_id": rid,
			"tenant":     st.tenantID,
		}).Info("request")
	})
}
