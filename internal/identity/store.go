// Package identity maps IdP identities to tenants for the remote MCP daemon
// (Phase P4 onboarding). It owns the RLS-EXEMPT tenants/identities admin tables:
// the OIDC auth path resolves a token's (issuer, subject) to a tenant here,
// auto-provisioning a personal tenant on first login. It queries directly on the
// pool with no app.tenant_id set — resolving the identity is how the tenant is
// discovered in the first place.
package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/briancolinger/persistor/internal/dbpool"
)

// authQueryTimeout bounds the auth-path query on the request hot path, mirroring
// the index store's per-op timeout (the pool's statement_timeout is a backstop,
// but this also bounds connection acquisition).
const authQueryTimeout = 5 * time.Second

// lastSeenStaleAfterMinutes bounds how often the auth hot path refreshes
// identities.last_seen_at. Auth runs on EVERY request (reads included), so
// updating last_seen_at each time would turn every read into a row-locking
// write + WAL and serialize same-subject requests on that row. Instead the
// refresh fires at most this often, folded into the lookup statement so a fresh
// row produces a no-op write (the inner UPDATE matches nothing).
const lastSeenStaleAfterMinutes = 15

// Roles an identity can hold against its tenant.
const (
	RoleOwner    = "owner"
	RoleMember   = "member"
	RoleReadonly = "readonly"
)

// ErrIdentityNotFound is returned when no identity maps the given (issuer,
// subject).
var ErrIdentityNotFound = errors.New("identity not found")

// Identity is one IdP subject routed to a tenant.
type Identity struct {
	Issuer   string
	Subject  string
	TenantID string
	Role     string
}

// Tenant is a memory namespace.
type Tenant struct {
	ID    string
	Label string
}

// Store is data access for the RLS-exempt tenants/identities tables.
type Store struct {
	pool *dbpool.Pool
}

// NewStore returns an identity store over the given pool.
func NewStore(pool *dbpool.Pool) *Store { return &Store{pool: pool} }

// ResolveOrProvision returns the tenant (and role) for an IdP subject. On first
// sight it auto-provisions: a personal tenant whose id is defaultTenant (callers
// pass the claim-derived uuidv5(iss|sub) so pre-P4 users keep their memory) plus
// an owner identity row. Subsequent logins return the stored mapping, which an
// admin may have repointed to a different tenant. It runs in one transaction so a
// race between two first-logins cannot double-provision.
func (s *Store) ResolveOrProvision(ctx context.Context, issuer, subject, defaultTenant string) (tenantID, role string, err error) {
	ctx, cancel := context.WithTimeout(ctx, authQueryTimeout)
	defer cancel()

	// Hot path: one statement that looks the identity up and refreshes
	// last_seen_at only when stale (or never set). When the row is fresh the
	// data-modifying CTE's WHERE matches nothing — no row lock, no WAL — so a
	// steady-state request (the overwhelming majority, all reads included) does
	// not amplify into a write. The CTE always executes but prunes to a no-op.
	err = s.pool.QueryRow(ctx,
		`WITH touched AS (
		     UPDATE identities SET last_seen_at = NOW()
		      WHERE issuer = $1 AND subject = $2
		        AND (last_seen_at IS NULL OR last_seen_at < NOW() - make_interval(mins => $3))
		    RETURNING tenant_id::text AS tenant_id, role
		 )
		 SELECT tenant_id, role FROM touched
		 UNION ALL
		 SELECT tenant_id::text, role FROM identities
		  WHERE issuer = $1 AND subject = $2 AND NOT EXISTS (SELECT 1 FROM touched)
		 LIMIT 1`,
		issuer, subject, lastSeenStaleAfterMinutes).Scan(&tenantID, &role)
	switch {
	case err == nil:
		return tenantID, role, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return "", "", fmt.Errorf("looking up identity: %w", err)
	}

	// First login: provision in one transaction so a race between two
	// first-logins cannot double-provision.
	return s.provision(ctx, issuer, subject, defaultTenant)
}

// provision creates the personal tenant + owner identity on first login and
// returns the resolved tenant/role. ON CONFLICT makes both inserts idempotent
// under a concurrent first-login of the same subject; the loser's
// UPDATE-returning reads the winner's row.
func (s *Store) provision(ctx context.Context, issuer, subject, defaultTenant string) (tenantID, role string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", fmt.Errorf("begin: %w", err)
	}
	// Rollback is a no-op once Commit has run (returns ErrTxClosed); surface any
	// other rollback failure by joining it into the returned error.
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
	}()

	if _, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, label) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		defaultTenant, "auto:"+subject); err != nil {
		return "", "", fmt.Errorf("provisioning tenant: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO identities (issuer, subject, tenant_id, role, last_seen_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (issuer, subject)
		   DO UPDATE SET last_seen_at = NOW()
		 RETURNING tenant_id::text, role`,
		issuer, subject, defaultTenant, RoleOwner).Scan(&tenantID, &role); err != nil {
		return "", "", fmt.Errorf("provisioning identity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", fmt.Errorf("commit: %w", err)
	}
	return tenantID, role, nil
}

// CreateTenant mints a tenant with the given label and returns its id. Used by
// `persistor admin tenant create` to pre-create a tenant for admin assignment.
func (s *Store) CreateTenant(ctx context.Context, label string) (string, error) {
	var id string
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO tenants (label) VALUES ($1) RETURNING id::text`, label).Scan(&id); err != nil {
		return "", fmt.Errorf("creating tenant: %w", err)
	}
	return id, nil
}

// SetIdentity maps an IdP (issuer, subject) to a tenant with a role, upserting an
// existing mapping. This is the admin-assign path (e.g. routing a work login to a
// shared work tenant) and must run before that subject's first login to take
// effect.
func (s *Store) SetIdentity(ctx context.Context, ident Identity) error {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO tenants (id, label) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
		ident.TenantID, "assigned:"+ident.Subject); err != nil {
		return fmt.Errorf("ensuring tenant: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO identities (issuer, subject, tenant_id, role)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (issuer, subject)
		   DO UPDATE SET tenant_id = EXCLUDED.tenant_id, role = EXCLUDED.role`,
		ident.Issuer, ident.Subject, ident.TenantID, ident.Role); err != nil {
		return fmt.Errorf("setting identity: %w", err)
	}
	return nil
}

// DeleteIdentity removes an identity mapping (user offboard), returning the number
// of rows removed. It does not delete the tenant's notes — that is a separate,
// deliberate action.
func (s *Store) DeleteIdentity(ctx context.Context, issuer, subject string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM identities WHERE issuer = $1 AND subject = $2`, issuer, subject)
	if err != nil {
		return 0, fmt.Errorf("deleting identity: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListIdentities returns every identity mapping, newest first.
func (s *Store) ListIdentities(ctx context.Context) ([]Identity, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT issuer, subject, tenant_id::text, role FROM identities ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing identities: %w", err)
	}
	defer rows.Close()

	var out []Identity
	for rows.Next() {
		var i Identity
		if err := rows.Scan(&i.Issuer, &i.Subject, &i.TenantID, &i.Role); err != nil {
			return nil, fmt.Errorf("scanning identity: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating identities: %w", err)
	}
	return out, nil
}

// ListTenants returns every tenant, newest first.
func (s *Store) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, label FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	defer rows.Close()

	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Label); err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tenants: %w", err)
	}
	return out, nil
}
