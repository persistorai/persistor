package middleware

import (
	"context"
	"errors"
	"testing"
	"time"
)

type cacheTestLookup struct {
	principal AuthPrincipal
	err       error
}

func (m *cacheTestLookup) GetTenantByAPIKey(_ context.Context, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.principal.TenantID, nil
}

func (m *cacheTestLookup) GetAuthPrincipalByAPIKey(_ context.Context, _ string) (AuthPrincipal, error) {
	if m.err != nil {
		return AuthPrincipal{}, m.err
	}
	return m.principal, nil
}

func TestCachedTenantLookupCachesScopeAndRefreshesQuickly(t *testing.T) {
	lookup := &cacheTestLookup{principal: AuthPrincipal{TenantID: "tenant-1", Scope: ScopeAdmin}}
	cache := NewCachedTenantLookup(context.Background(), lookup)

	principal, err := cache.GetAuthPrincipalByAPIKey(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("first lookup: %v", err)
	}
	if principal.Scope != ScopeAdmin {
		t.Fatalf("scope = %q, want %q", principal.Scope, ScopeAdmin)
	}

	lookup.err = errors.New("revoked")

	if _, err := cache.GetAuthPrincipalByAPIKey(context.Background(), "key-1"); err != nil {
		t.Fatalf("cached lookup before ttl: %v", err)
	}

	time.Sleep(tenantCacheTTL + 200*time.Millisecond)

	if _, err := cache.GetAuthPrincipalByAPIKey(context.Background(), "key-1"); err == nil {
		t.Fatal("expected revoked key lookup to fail after ttl")
	}
	if _, err := cache.GetAuthPrincipalByAPIKey(context.Background(), "key-1"); !errors.Is(err, errCachedNotFound) {
		t.Fatalf("expected cached negative lookup, got %v", err)
	}
}
