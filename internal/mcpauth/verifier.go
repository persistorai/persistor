package mcpauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// staticTokenTTL is the lifetime stamped on a verified static key's TokenInfo.
// Static keys do not actually expire — revocation is the kill switch — but the
// MCP auth middleware requires a non-zero, future expiration. The verifier runs
// on every request, so this value only needs to outlast a single request.
const staticTokenTTL = time.Hour

// KeyResolver resolves a raw bearer token to the tenant it authenticates. It is
// defined here, where the verifier consumes it, and implemented by PGKeyStore.
type KeyResolver interface {
	ResolveAPIKey(ctx context.Context, raw string) (tenantID string, err error)
}

// StaticTokenAuth verifies opaque per-tenant bearer tokens against a KeyResolver.
type StaticTokenAuth struct {
	keys KeyResolver
}

// NewStaticTokenAuth returns a verifier over the given resolver.
func NewStaticTokenAuth(keys KeyResolver) *StaticTokenAuth {
	return &StaticTokenAuth{keys: keys}
}

// Verify matches auth.TokenVerifier. It resolves the token to a tenant and
// returns a TokenInfo whose UserID is that tenant; the Streamable HTTP transport
// reuses UserID to pin a session to its tenant (anti-hijack). An unknown or
// revoked token unwraps to auth.ErrInvalidToken, which the middleware turns into
// a 401; a backend failure is returned as-is (the middleware maps it to 500).
func (a *StaticTokenAuth) Verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	tenantID, err := a.keys.ResolveAPIKey(ctx, token)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil, fmt.Errorf("%w: unknown or revoked key", auth.ErrInvalidToken)
		}
		return nil, fmt.Errorf("verifying token: %w", err)
	}
	return &auth.TokenInfo{
		UserID:     tenantID,
		Expiration: time.Now().Add(staticTokenTTL),
	}, nil
}
