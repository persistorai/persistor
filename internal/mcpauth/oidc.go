package mcpauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// tenantNamespace is the fixed UUIDv5 namespace for deriving a tenant id from an
// OIDC subject. It must NEVER change: a different namespace would remap every
// existing user to a new tenant and orphan their memory.
var tenantNamespace = uuid.MustParse("9e6f1b2c-3d4a-5b6c-7d8e-9f0a1b2c3d4e")

// allowedSigningMethods restricts JWT verification to asymmetric algorithms.
// Pinning these defeats "alg=none" and HS/RS confusion attacks, where an
// attacker re-signs a token with the public key as an HMAC secret.
var allowedSigningMethods = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}

// OIDCAuth verifies OIDC access tokens (JWTs) issued by a delegated Authorization
// Server (Stytch in P3). It is the Phase P3 Authenticator behind the same
// auth.TokenVerifier seam as StaticTokenAuth: claude.ai/mobile obtain a token via
// the browser OAuth flow, and Persistor validates it statelessly against the
// IdP's JWKS — Persistor never handles credentials or talks to the IdP per call.
type OIDCAuth struct {
	keyFunc  jwt.Keyfunc
	issuer   string
	audience string
	parser   *jwt.Parser
}

// NewOIDCAuth builds a verifier. keyFunc supplies the IdP's signing keys (a JWKS
// fetcher in production, a static key in tests); issuer and audience are the
// values every token must carry.
func NewOIDCAuth(keyFunc jwt.Keyfunc, issuer, audience string) *OIDCAuth {
	parser := jwt.NewParser(
		jwt.WithValidMethods(allowedSigningMethods),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
	)
	return &OIDCAuth{keyFunc: keyFunc, issuer: issuer, audience: audience, parser: parser}
}

// Verify matches auth.TokenVerifier. It validates the JWT's signature (via the
// IdP's keys), issuer, audience, and expiry, then derives a stable tenant from
// iss|sub. Any failure unwraps to auth.ErrInvalidToken, which the middleware
// turns into a 401.
func (a *OIDCAuth) Verify(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	var claims jwt.RegisteredClaims
	if _, err := a.parser.ParseWithClaims(token, &claims, a.keyFunc); err != nil {
		return nil, fmt.Errorf("%w: %w", auth.ErrInvalidToken, err)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: token has no subject", auth.ErrInvalidToken)
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return nil, fmt.Errorf("%w: token has no expiration", auth.ErrInvalidToken)
	}
	return &auth.TokenInfo{
		UserID:     TenantForSubject(a.issuer, claims.Subject),
		Expiration: exp.Time,
	}, nil
}

// TenantForSubject derives the stable tenant UUID for an IdP subject. It is a
// pure function of (issuer, subject), so the same login always maps to the same
// tenant with no stored mapping. Explicit identity mapping (work/personal
// separation, admin assignment) is a later refinement layered on top.
func TenantForSubject(issuer, subject string) string {
	return uuid.NewSHA1(tenantNamespace, []byte(issuer+"|"+subject)).String()
}

// NewJWKSKeyFunc returns a jwt.Keyfunc that fetches and caches the IdP's signing
// keys from a JWKS URL, refreshing automatically on key rotation. Used to build
// the production OIDCAuth; tests pass a static key instead.
func NewJWKSKeyFunc(ctx context.Context, jwksURL string) (jwt.Keyfunc, error) {
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("building JWKS key function for %q: %w", jwksURL, err)
	}
	return k.Keyfunc, nil
}
