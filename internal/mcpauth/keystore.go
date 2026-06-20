// Package mcpauth is the pluggable authentication layer for the remote MCP
// daemon. Phase P2 provides StaticTokenAuth: opaque per-tenant bearer tokens
// checked against the RLS-exempt api_keys table. Phase P3 swaps in a JWT/JWKS
// verifier behind the same auth.TokenVerifier seam — nothing IdP-specific leaks
// past this package.
package mcpauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/persistorai/persistor/internal/dbpool"
)

const (
	// tokenPrefix marks Persistor static keys so a leaked string is identifiable.
	tokenPrefix = "psk_"
	// tokenBytes is the entropy of a raw token before encoding.
	tokenBytes = 32
)

// ErrKeyNotFound is returned when a token hash matches no active (non-revoked)
// key.
var ErrKeyNotFound = errors.New("api key not found")

// PGKeyStore is data access for the RLS-exempt api_keys table. That table holds
// no tenant content and the lookup runs before a tenant is known, so it is
// queried directly on the pool with no app.tenant_id set.
type PGKeyStore struct {
	pool *dbpool.Pool
}

// NewPGKeyStore returns a key store over the given pool.
func NewPGKeyStore(pool *dbpool.Pool) *PGKeyStore {
	return &PGKeyStore{pool: pool}
}

// NewToken generates a random opaque token and returns it with its hash. The raw
// token is shown to the operator once and never stored; only the hash is.
func NewToken() (raw, hash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generating token: %w", err)
	}
	raw = tokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken returns the hex-encoded sha256 of a raw token (64 chars), the form
// stored in api_keys.key_hash. Tokens are high-entropy random, so an unsalted
// SHA-256 is appropriate here (unlike passwords).
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// CreateAPIKey mints a key for a tenant and returns the raw token. The token is
// unrecoverable afterward, so the caller must surface it once.
func (s *PGKeyStore) CreateAPIKey(ctx context.Context, tenantID, label string) (string, error) {
	raw, hash, err := NewToken()
	if err != nil {
		return "", err
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (tenant_id, key_hash, label) VALUES ($1, $2, $3)`,
		tenantID, hash, label); err != nil {
		return "", fmt.Errorf("inserting api key: %w", err)
	}
	return raw, nil
}

// ResolveAPIKey returns the tenant a raw token authenticates, recording the use
// in last_used_at. A missing or revoked key returns ErrKeyNotFound.
func (s *PGKeyStore) ResolveAPIKey(ctx context.Context, raw string) (string, error) {
	var tenantID string
	err := s.pool.QueryRow(ctx,
		`UPDATE api_keys SET last_used_at = NOW()
		   WHERE key_hash = $1 AND revoked = FALSE
		 RETURNING tenant_id::text`, HashToken(raw)).Scan(&tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrKeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolving api key: %w", err)
	}
	return tenantID, nil
}

// RevokeAPIKey disables every active key whose hash matches raw, returning the
// number revoked.
func (s *PGKeyStore) RevokeAPIKey(ctx context.Context, raw string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked = TRUE WHERE key_hash = $1 AND revoked = FALSE`,
		HashToken(raw))
	if err != nil {
		return 0, fmt.Errorf("revoking api key: %w", err)
	}
	return tag.RowsAffected(), nil
}
