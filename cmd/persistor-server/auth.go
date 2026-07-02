package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/briancolinger/persistor/internal/dbpool"
	"github.com/briancolinger/persistor/internal/identity"
	"github.com/briancolinger/persistor/internal/mcpauth"
)

// authBundle is everything the mux needs to enforce authentication: the token
// verifier, the bearer-middleware options, and the protected-resource metadata
// document to publish so MCP clients can discover the Authorization Server.
type authBundle struct {
	verify   auth.TokenVerifier
	opts     *auth.RequireBearerTokenOptions
	metadata *protectedResourceMetadata
}

// buildAuth assembles the OIDC authenticator: it validates IdP-issued JWTs
// against the IdP's JWKS, resolves/provisions the tenant per login via the
// identities table, and advertises protected-resource metadata. OIDC is the only
// auth path — local and remote clients both obtain a token through the browser
// OAuth flow, so the same Stytch identity maps to the same tenant on every
// device. Nothing IdP-specific leaks past this point.
func buildAuth(ctx context.Context, cfg *serverConfig, pool *dbpool.Pool) (authBundle, error) {
	metadataURL := cfg.publicURL + "/.well-known/oauth-protected-resource"
	opts := &auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL}

	keyFunc, err := mcpauth.NewJWKSKeyFunc(ctx, cfg.oidcJWKSURL)
	if err != nil {
		return authBundle{}, err
	}
	v := mcpauth.NewOIDCAuth(keyFunc, cfg.oidcIssuer, cfg.oidcAudience).
		WithTenantResolver(identity.NewStore(pool)).
		WithAllowedSubjects(cfg.allowedSubjects)
	return authBundle{
		verify: v.Verify,
		opts:   opts,
		metadata: &protectedResourceMetadata{
			Resource:             cfg.publicURL,
			AuthorizationServers: []string{cfg.oidcIssuer},
		},
	}, nil
}
