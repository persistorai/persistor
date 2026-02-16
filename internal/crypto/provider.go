// Package crypto provides tenant-aware AES-256-GCM encryption.
package crypto

import "context"

// KeyProvider returns AES-256 encryption keys for tenants.
type KeyProvider interface {
	// GetKey returns the 32-byte AES-256 key for the given tenant.
	GetKey(ctx context.Context, tenantID string) ([]byte, error)
}
