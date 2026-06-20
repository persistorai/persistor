package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed authorize.html
var authorizeHTML string

// newConsentHandler renders the Stytch Connected Apps consent page — the OAuth
// "Authorization URL" the IdP requires. It is served OPEN (it is a browser page,
// not an API) and injects the publishable Stytch token. This page is the IdP's
// consent UI: deliberately the one IdP-specific surface, kept entirely outside
// the resource-server auth path (the token verifier stays IdP-agnostic).
func newConsentHandler(publicToken string) (http.HandlerFunc, error) {
	tmpl, err := template.New("authorize").Parse(authorizeHTML)
	if err != nil {
		return nil, fmt.Errorf("parsing authorize template: %w", err)
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, struct{ PublicToken string }{PublicToken: publicToken}); err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
		}
	}, nil
}
