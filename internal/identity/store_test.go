package identity_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/db"
	"github.com/persistorai/persistor/internal/db/migrations"
	"github.com/persistorai/persistor/internal/dbpool"
	"github.com/persistorai/persistor/internal/identity"
)

func newIdentityStore(t *testing.T) (store *identity.Store, issuer, subject string) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := dbpool.NewPool(ctx, dbURL, 2)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	t.Cleanup(pool.Close)

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	if err := db.RunMigrations(ctx, pool, log, migrations.FS); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// Unique issuer/subject per test so parallel/repeat runs don't collide.
	issuer = "https://test.example/" + uuid.NewString()
	subject = "sub-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM identities WHERE issuer = $1", issuer)
	})
	return identity.NewStore(pool), issuer, subject
}

func TestResolveOrProvisionFirstLogin(t *testing.T) {
	store, issuer, subject := newIdentityStore(t)
	ctx := context.Background()
	defaultTenant := uuid.NewString()

	tenant, role, err := store.ResolveOrProvision(ctx, issuer, subject, defaultTenant)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if tenant != defaultTenant {
		t.Fatalf("tenant = %q, want auto-provisioned default %q", tenant, defaultTenant)
	}
	if role != identity.RoleOwner {
		t.Fatalf("role = %q, want %q", role, identity.RoleOwner)
	}

	// Second login is stable: same tenant, no re-provision to a new id.
	tenant2, _, err := store.ResolveOrProvision(ctx, issuer, subject, uuid.NewString())
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if tenant2 != defaultTenant {
		t.Fatalf("second tenant = %q, want stable %q", tenant2, defaultTenant)
	}
}

func TestSetIdentityOverridesProvisioning(t *testing.T) {
	store, issuer, subject := newIdentityStore(t)
	ctx := context.Background()

	// Admin pre-assigns this subject to a chosen (work) tenant before first login.
	assigned := uuid.NewString()
	if err := store.SetIdentity(ctx, identity.Identity{
		Issuer: issuer, Subject: subject, TenantID: assigned, Role: identity.RoleMember,
	}); err != nil {
		t.Fatalf("set identity: %v", err)
	}

	// First login must honor the admin mapping, NOT the claim-derived default.
	tenant, role, err := store.ResolveOrProvision(ctx, issuer, subject, uuid.NewString())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if tenant != assigned {
		t.Fatalf("tenant = %q, want admin-assigned %q", tenant, assigned)
	}
	if role != identity.RoleMember {
		t.Fatalf("role = %q, want %q", role, identity.RoleMember)
	}
}

func TestCreateTenantAndList(t *testing.T) {
	store, _, _ := newIdentityStore(t)
	ctx := context.Background()

	id, err := store.CreateTenant(ctx, "p4-test-tenant")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("create tenant returned non-uuid %q: %v", id, err)
	}

	tenants, err := store.ListTenants(ctx)
	if err != nil {
		t.Fatalf("list tenants: %v", err)
	}
	found := false
	for _, tn := range tenants {
		if tn.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("created tenant %q not in list", id)
	}
}

func TestDeleteIdentity(t *testing.T) {
	store, issuer, subject := newIdentityStore(t)
	ctx := context.Background()

	if _, _, err := store.ResolveOrProvision(ctx, issuer, subject, uuid.NewString()); err != nil {
		t.Fatalf("provision: %v", err)
	}
	n, err := store.DeleteIdentity(ctx, issuer, subject)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted %d, want 1", n)
	}
	idents, err := store.ListIdentities(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, id := range idents {
		if id.Issuer == issuer && id.Subject == subject {
			t.Fatalf("identity still present after delete")
		}
	}
}
