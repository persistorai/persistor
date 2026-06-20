package mcpauth_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/mcpauth"
)

// fakeResolver is a KeyResolver stand-in for the no-DB verifier tests.
type fakeResolver struct {
	tenant string
	err    error
}

func (f fakeResolver) ResolveAPIKey(_ context.Context, _ string) (string, error) {
	return f.tenant, f.err
}

func TestStaticTokenAuth_Verify(t *testing.T) {
	ctx := context.Background()
	tenant := uuid.New().String()

	// Valid token -> TokenInfo carrying the tenant as UserID, future expiry.
	ok := mcpauth.NewStaticTokenAuth(fakeResolver{tenant: tenant})
	ti, err := ok.Verify(ctx, "psk_whatever", nil)
	if err != nil {
		t.Fatalf("verify valid: %v", err)
	}
	if ti.UserID != tenant {
		t.Fatalf("UserID = %q, want tenant %q", ti.UserID, tenant)
	}
	if !ti.Expiration.After(time.Now()) {
		t.Fatalf("expiration %v not in the future", ti.Expiration)
	}

	// Unknown/revoked key -> unwraps to auth.ErrInvalidToken (middleware 401).
	bad := mcpauth.NewStaticTokenAuth(fakeResolver{err: mcpauth.ErrKeyNotFound})
	if _, err := bad.Verify(ctx, "psk_nope", nil); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("verify unknown: want auth.ErrInvalidToken, got %v", err)
	}

	// Backend failure -> NOT an auth failure (middleware maps to 500).
	boom := mcpauth.NewStaticTokenAuth(fakeResolver{err: errors.New("db down")})
	if _, err := boom.Verify(ctx, "psk_x", nil); err == nil || errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("verify backend error: want non-auth error, got %v", err)
	}
}

func TestPGKeyStore_CreateResolveRevoke(t *testing.T) {
	store, tenant := newKeyStore(t)
	ctx := context.Background()

	raw, err := store.CreateAPIKey(ctx, tenant, "laptop")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.HasPrefix(raw, "psk_") {
		t.Fatalf("token %q missing psk_ prefix", raw)
	}

	got, err := store.ResolveAPIKey(ctx, raw)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != tenant {
		t.Fatalf("resolved tenant = %q, want %q", got, tenant)
	}

	// A token that was never minted resolves to ErrKeyNotFound.
	if _, err := store.ResolveAPIKey(ctx, "psk_bogus"); !errors.Is(err, mcpauth.ErrKeyNotFound) {
		t.Fatalf("resolve bogus: want ErrKeyNotFound, got %v", err)
	}

	// After revocation the key no longer resolves.
	n, err := store.RevokeAPIKey(ctx, raw)
	if err != nil || n != 1 {
		t.Fatalf("revoke = (%d, %v), want (1, nil)", n, err)
	}
	if _, err := store.ResolveAPIKey(ctx, raw); !errors.Is(err, mcpauth.ErrKeyNotFound) {
		t.Fatalf("resolve revoked: want ErrKeyNotFound, got %v", err)
	}
}

func newKeyStore(t *testing.T) (store *mcpauth.PGKeyStore, tenant string) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := dbpool.NewPool(ctx, dbURL, 2)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)

	tenant = uuid.New().String()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM api_keys WHERE tenant_id = $1", tenant)
	})
	return mcpauth.NewPGKeyStore(pool), tenant
}
