package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/identity"
	"github.com/persistorai/persistor/internal/mcpauth"
)

const (
	authModeStatic = "static"
	authModeOIDC   = "oidc"
)

// authBundle is everything the mux needs to enforce authentication for the
// configured mode: the token verifier, the bearer-middleware options, and (OIDC
// only) the protected-resource metadata document to publish.
type authBundle struct {
	verify   auth.TokenVerifier
	opts     *auth.RequireBearerTokenOptions
	metadata *protectedResourceMetadata
}

// buildAuth assembles the authenticator for the configured mode. static mode
// validates per-tenant API keys against Postgres; oidc mode validates
// IdP-issued JWTs against the IdP's JWKS and advertises protected-resource
// metadata so MCP clients can discover the Authorization Server. Both resolve a
// request to a tenant the same way downstream — nothing IdP-specific leaks past
// this point (Non-corner-painting rule 2).
func buildAuth(ctx context.Context, cfg *serverConfig, pool *dbpool.Pool) (authBundle, error) {
	metadataURL := cfg.publicURL + "/.well-known/oauth-protected-resource"
	opts := &auth.RequireBearerTokenOptions{ResourceMetadataURL: metadataURL}

	switch cfg.authMode {
	case authModeStatic:
		v := mcpauth.NewStaticTokenAuth(mcpauth.NewPGKeyStore(pool))
		return authBundle{verify: v.Verify, opts: opts}, nil
	case authModeOIDC:
		keyFunc, err := mcpauth.NewJWKSKeyFunc(ctx, cfg.oidcJWKSURL)
		if err != nil {
			return authBundle{}, err
		}
		// The identities table resolves/provisions the tenant per login (P4).
		v := mcpauth.NewOIDCAuth(keyFunc, cfg.oidcIssuer, cfg.oidcAudience).
			WithTenantResolver(identity.NewStore(pool))
		return authBundle{
			verify: v.Verify,
			opts:   opts,
			metadata: &protectedResourceMetadata{
				Resource:             cfg.publicURL,
				AuthorizationServers: []string{cfg.oidcIssuer},
			},
		}, nil
	default:
		return authBundle{}, fmt.Errorf("unknown PERSISTOR_AUTH_MODE %q (want %q or %q)", cfg.authMode, authModeStatic, authModeOIDC)
	}
}
