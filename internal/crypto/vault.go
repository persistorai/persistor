package crypto

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/persistorai/persistor/internal/config"
)

// keyCacheTTL is how long a cached key is valid before re-fetching from Vault.
const keyCacheTTL = 15 * time.Minute

// uuidPattern validates tenant IDs to prevent SSRF via path traversal.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// cachedKey stores a key alongside its fetch timestamp for TTL expiration.
type cachedKey struct {
	key       []byte
	fetchedAt time.Time
}

// VaultProvider fetches tenant encryption keys from HashiCorp Vault.
type VaultProvider struct {
	addr   string
	token  config.Secret
	client *http.Client
	cache  sync.Map
	group  singleflight.Group
}

// NewVaultProvider creates a VaultProvider with the given Vault address and token.
func NewVaultProvider(addr, token string) *VaultProvider {
	return &VaultProvider{
		addr:  addr,
		token: config.Secret(token),
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
	}
}

// GetKey returns the cached AES-256 key for the tenant, fetching from Vault on first access.
// Cached keys expire after keyCacheTTL and are re-fetched.
func (p *VaultProvider) GetKey(ctx context.Context, tenantID string) ([]byte, error) {
	if cached, ok := p.cache.Load(tenantID); ok {
		entry, valid := cached.(cachedKey)
		if valid && time.Since(entry.fetchedAt) < keyCacheTTL {
			// Return a copy to prevent caller mutation of cached data.
			out := make([]byte, len(entry.key))
			copy(out, entry.key)
			return out, nil
		}
		// Expired — delete and re-fetch.
		p.cache.Delete(tenantID)
	}

	val, err, _ := p.group.Do(tenantID, func() (any, error) {
		// Double-check cache after winning the singleflight race.
		if cached, ok := p.cache.Load(tenantID); ok {
			entry, valid := cached.(cachedKey)
			if valid && time.Since(entry.fetchedAt) < keyCacheTTL {
				return entry.key, nil
			}
		}

		k, err := p.fetchKey(ctx, tenantID)
		if err != nil {
			return nil, err
		}

		p.cache.Store(tenantID, cachedKey{key: append([]byte(nil), k...), fetchedAt: time.Now()})
		return k, nil
	})
	if err != nil {
		return nil, err
	}

	key, ok := val.([]byte)
	if !ok {
		return nil, fmt.Errorf("crypto/vault: unexpected singleflight result type %T", val)
	}

	// Return a copy.
	out := make([]byte, len(key))
	copy(out, key)
	return out, nil
}

func (p *VaultProvider) fetchKey(ctx context.Context, tenantID string) ([]byte, error) {
	if !uuidPattern.MatchString(tenantID) {
		return nil, fmt.Errorf("crypto/vault: invalid tenant ID format: %q", tenantID)
	}

	reqURL := fmt.Sprintf("%s/v1/secret/data/persistor/tenant-keys/%s", p.addr, url.PathEscape(tenantID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("crypto/vault: create request: %w", err)
	}

	req.Header.Set("X-Vault-Token", p.token.Value())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crypto/vault: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit all body reads to 1 MB to prevent memory exhaustion.
	limitedBody := io.LimitReader(resp.Body, 1<<20)

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, limitedBody)
		return nil, fmt.Errorf("crypto/vault: no encryption key found for tenant %q — create it in Vault at secret/persistor/tenant-keys/%s", tenantID, tenantID)
	}

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(limitedBody)
		if readErr != nil {
			return nil, fmt.Errorf("crypto/vault: unexpected status %d (failed to read body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("crypto/vault: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return nil, fmt.Errorf("crypto/vault: decode response: %w", err)
	}

	b64Key, ok := result.Data.Data["encryption_key"]
	if !ok || b64Key == "" {
		return nil, fmt.Errorf("crypto/vault: encryption_key field missing for tenant %q", tenantID)
	}

	key, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, fmt.Errorf("crypto/vault: decode base64 key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("crypto/vault: key must be 32 bytes, got %d", len(key))
	}

	return key, nil
}
