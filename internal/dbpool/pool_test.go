package dbpool_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/persistorai/persistor/internal/dbpool"
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
