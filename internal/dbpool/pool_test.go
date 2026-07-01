package dbpool_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/briancolinger/persistor/internal/dbpool"
)

// TestAssertNonOwner_RejectsOwnerConnection verifies the production boot guard
// fires when the daemon would connect as the role that owns the tenant tables.
// The integration test DB connects as the schema-owning role (TEST_DATABASE_URL
// mirrors the single-role/CI posture), so AssertNonOwner must reject it — that
// is exactly the misconfiguration the guard exists to catch in the split-role
// production posture.
func TestAssertNonOwner_RejectsOwnerConnection(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := dbpool.NewPool(ctx, dbURL, 2)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	err = pool.AssertNonOwner(ctx)
	if err == nil {
		t.Fatal("AssertNonOwner returned nil for an owner connection; expected it to reject")
	}
	if !strings.Contains(err.Error(), "owns the notes table") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// A ping failure must be classified transient (ErrDBUnreachable) so the daemon
// boot retry engages; a malformed URL must not be (retrying config is useless).
func TestNewPoolErrorClassification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// A routable-but-closed local port: connection refused = unreachable.
	_, err := dbpool.NewPool(ctx, "postgres://u:p@127.0.0.1:1/db?sslmode=disable", 2)
	if err == nil {
		t.Fatal("expected error connecting to a closed port")
	}
	if !errors.Is(err, dbpool.ErrDBUnreachable) {
		t.Errorf("closed-port error = %v, want ErrDBUnreachable", err)
	}

	_, err = dbpool.NewPool(ctx, "not a url \x00", 2)
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if errors.Is(err, dbpool.ErrDBUnreachable) {
		t.Errorf("malformed-URL error wrongly classified transient: %v", err)
	}
}
