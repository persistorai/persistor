package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/middleware"
)

// TenantStore handles tenant lookups (API key → tenant ID).
type TenantStore struct {
	Pool *dbpool.Pool
}

// NewTenantStore creates a new TenantStore.
func NewTenantStore(pool *dbpool.Pool) *TenantStore {
	return &TenantStore{Pool: pool}
}

// GetTenantByAPIKey looks up a tenant ID by API key hash.
func (s *TenantStore) GetTenantByAPIKey(ctx context.Context, apiKey string) (string, error) {
	principal, err := s.GetAuthPrincipalByAPIKey(ctx, apiKey)
	if err != nil {
		return "", err
	}

	return principal.TenantID, nil
}

// GetAuthPrincipalByAPIKey looks up the tenant ID and auth scope for an API key.
func (s *TenantStore) GetAuthPrincipalByAPIKey(ctx context.Context, apiKey string) (middleware.AuthPrincipal, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	hash := sha256.Sum256([]byte(apiKey))
	apiKeyHash := hex.EncodeToString(hash[:])

	var principal middleware.AuthPrincipal

	err := s.Pool.QueryRow(ctx, "SELECT id, api_key_scope FROM tenants WHERE api_key_hash = $1", apiKeyHash).Scan(&principal.TenantID, &principal.Scope)
	if err != nil {
		return middleware.AuthPrincipal{}, fmt.Errorf("looking up tenant by API key: %w", err)
	}

	return principal, nil
}
