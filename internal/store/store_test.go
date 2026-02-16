package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/crypto"
	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/store"
)

// testHexKey is a valid 64-char hex string (32 bytes) for test encryption.
const testHexKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// testEnv holds shared test infrastructure (single pool across all tests).
type testEnv struct {
	pool *dbpool.Pool
	log  *logrus.Logger
}

var sharedEnv *testEnv

func getTestEnv(t *testing.T) *testEnv {
	t.Helper()

	if sharedEnv != nil {
		return sharedEnv
	}

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()

	pool, err := dbpool.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to test DB: %v", err)
	}

	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	sharedEnv = &testEnv{
		pool: pool,
		log:  log,
	}

	return sharedEnv
}

// newCryptoService creates a fresh crypto.Service (StaticProvider locks to first tenant).
func newCryptoService(t *testing.T) *crypto.Service {
	t.Helper()

	provider, err := crypto.NewStaticProvider(testHexKey)
	if err != nil {
		t.Fatalf("creating static provider: %v", err)
	}

	return crypto.NewService(provider)
}

// setupTestBase creates a Base with a fresh test tenant, cleaned up after the test.
func setupTestBase(t *testing.T) (_ store.Base, _ string) {
	t.Helper()

	env := getTestEnv(t)
	tenantID := uuid.New().String()
	ctx := context.Background()

	// Insert test tenant directly (no RLS on tenants table inserts).
	apiKey := "test-key-" + tenantID
	hash := sha256.Sum256([]byte(apiKey))
	apiKeyHash := hex.EncodeToString(hash[:])

	_, err := env.pool.Exec(ctx,
		"INSERT INTO tenants (id, name, api_key_hash) VALUES ($1, $2, $3)",
		tenantID, fmt.Sprintf("test-tenant-%s", tenantID[:8]), apiKeyHash,
	)
	if err != nil {
		t.Fatalf("creating test tenant: %v", err)
	}

	t.Cleanup(func() {
		cleanCtx := context.Background()
		// Delete in dependency order: audit, property_history, edges, nodes, tenant.
		env.pool.Exec(cleanCtx, "DELETE FROM kg_audit_log WHERE tenant_id = $1", tenantID)        //nolint:errcheck // best-effort cleanup
		env.pool.Exec(cleanCtx, "DELETE FROM kg_property_history WHERE tenant_id = $1", tenantID) //nolint:errcheck // best-effort cleanup
		env.pool.Exec(cleanCtx, "DELETE FROM kg_edges WHERE tenant_id = $1", tenantID)            //nolint:errcheck // best-effort cleanup
		env.pool.Exec(cleanCtx, "DELETE FROM kg_nodes WHERE tenant_id = $1", tenantID)            //nolint:errcheck // best-effort cleanup
		env.pool.Exec(cleanCtx, "DELETE FROM tenants WHERE id = $1", tenantID)                    //nolint:errcheck // best-effort cleanup
	})

	base := store.Base{Pool: env.pool, Log: env.log, Crypto: newCryptoService(t)}

	return base, tenantID
}
