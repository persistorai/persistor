package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// authServerMetadata is the RFC 8414 OAuth 2.0 Authorization Server Metadata
// document served at THIS server's own origin. Some MCP clients — notably
// claude.ai web connectors (BETA) — do not follow the cross-origin
// authorization_servers pointer in our protected-resource metadata; they expect
// authorization-server discovery and Dynamic Client Registration at the MCP
// server's own origin (they probe /.well-known/oauth-authorization-server and
// POST DCR to /register here). This facade answers them: the issuer IS this
// server, the authorization endpoint is the local consent page, and the
// token/userinfo/jwks endpoints point at the upstream IdP (Stytch). Token
// validation is unaffected — the bearer is a Stytch-issued JWT verified against
// Stytch's issuer by mcpauth, and is opaque to the client.
type authServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
}

// stytchEndpoint joins the upstream IdP issuer base with an OAuth2 path.
func stytchEndpoint(issuer, path string) string {
	return strings.TrimRight(issuer, "/") + path
}

// newAuthServerMetadata builds the facade document. publicURL is this server's
// externally-visible base — it is the issuer the client fetched the doc from, so
// per RFC 8414 it MUST equal the metadata's issuer exactly. stytchIssuer is the
// upstream IdP base used to derive the token/userinfo endpoints; jwksURL is the
// upstream signing-key set. registration_endpoint points back at this server's
// same-origin /register proxy so clients that insist on same-origin DCR still
// work; the proxy relays to the IdP.
func newAuthServerMetadata(publicURL, stytchIssuer, jwksURL string) *authServerMetadata {
	return &authServerMetadata{
		Issuer:                            publicURL,
		AuthorizationEndpoint:             publicURL + "/authorize",
		TokenEndpoint:                     stytchEndpoint(stytchIssuer, "/v1/oauth2/token"),
		RegistrationEndpoint:              publicURL + "/register",
		UserinfoEndpoint:                  stytchEndpoint(stytchIssuer, "/v1/oauth2/userinfo"),
		JWKSURI:                           jwksURL,
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none", "client_secret_post", "client_secret_basic"},
		ScopesSupported:                   []string{"openid", "profile", "email", "offline_access"},
		SubjectTypesSupported:             []string{"public"},
		IDTokenSigningAlgValuesSupported:  []string{"RS256"},
	}
}

// serveHTTP writes the facade document as JSON. It is served OPEN (discovery
// metadata is public by design) and is the same body for both the OAuth
// authorization-server and OpenID Connect discovery paths.
func (m *authServerMetadata) serveHTTP(w http.ResponseWriter, _ *http.Request) {
	body, err := json.Marshal(m)
	if err != nil {
		http.Error(w, "metadata encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(body); err != nil {
		return
	}
}

// maxRegisterBody caps the request and relayed response of the open /register
// DCR proxy (defensive: it forwards to the IdP's registration endpoint).
const maxRegisterBody = 64 << 10

// newRegisterProxy relays a Dynamic Client Registration POST to the upstream IdP
// registration endpoint and returns the response verbatim. claude.ai's connector
// posts DCR to the resource origin (/register) instead of following the
// registration_endpoint cross-origin, so this same-origin proxy bridges to the
// IdP. Only POST is accepted and the body is size-capped.
func newRegisterProxy(client *http.Client, upstream string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRegisterBody))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream, bytes.NewReader(body))
		if err != nil {
			http.Error(w, "proxy error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "upstream unreachable", http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, io.LimitReader(resp.Body, maxRegisterBody)); err != nil {
			return
		}
	}
}
