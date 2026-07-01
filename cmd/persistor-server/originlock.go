package main

import (
	"crypto/subtle"
	"net/http"
)

// originSecretHeader carries the shared secret Cloudflare injects on proxied
// traffic (a Transform Rule adds it at the edge). Requests direct to the bare
// App Platform origin never carry it.
const originSecretHeader = "X-Origin-Secret"

// originLock makes the Cloudflare edge mandatory: when secret is non-empty,
// every route except /healthz requires the matching X-Origin-Secret header and
// mismatches are rejected 403 before any other work. This closes the
// direct-to-origin bypass (the bare *.ondigitalocean.app host is discoverable
// in certificate-transparency logs), restoring the edge WAF/rate limits as a
// real boundary — and it is what makes CF-Connecting-IP trustworthy for the
// per-IP limiters (see perIPLimit): a request that passed the lock provably
// came through Cloudflare.
//
// /healthz stays open because the App Platform liveness probe hits the origin
// directly, not through Cloudflare. It returns a bare 200 and nothing else.
//
// With secret empty the handler is returned unchanged (self-host/tailnet
// deployments have no edge in front and no lock).
func originLock(secret string, next http.Handler) http.Handler {
	if secret == "" {
		return next
	}
	secretBytes := []byte(secret)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got := []byte(r.Header.Get(originSecretHeader))
		if subtle.ConstantTimeCompare(got, secretBytes) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
