package crypto

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
)

// StaticProvider returns a single key from a hex-encoded string for all tenants.
// Intended for dev/test single-tenant use.
type StaticProvider struct {
	key      []byte
	firstTID string
	tidOnce  sync.Once
}

// NewStaticProvider creates a StaticProvider from a hex-encoded 32-byte key.
func NewStaticProvider(hexKey string) (*StaticProvider, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("crypto/static: invalid hex key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("crypto/static: key must be 32 bytes, got %d", len(key))
	}

	return &StaticProvider{key: key}, nil
}

// GetKey returns a copy of the static key for the first tenant seen.
// Returns an error if called with a different tenant ID — multi-tenant
// use requires the vault provider.
func (p *StaticProvider) GetKey(_ context.Context, tenantID string) ([]byte, error) {
	p.tidOnce.Do(func() { p.firstTID = tenantID })

	if tenantID != p.firstTID {
		return nil, fmt.Errorf("crypto/static: multi-tenant use requires vault provider; saw tenant %s after %s", tenantID, p.firstTID)
	}

	out := make([]byte, len(p.key))
	copy(out, p.key)
	return out, nil
}
