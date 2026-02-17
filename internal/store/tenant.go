package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/persistorai/persistor/internal/dbpool"
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
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	hash := sha256.Sum256([]byte(apiKey))
	apiKeyHash := hex.EncodeToString(hash[:])

	var tenantID string

	err := s.Pool.QueryRow(ctx, "SELECT id FROM tenants WHERE api_key_hash = $1", apiKeyHash).Scan(&tenantID)
	if err != nil {
		return "", fmt.Errorf("looking up tenant by API key: %w", err)
	}

	return tenantID, nil
}
