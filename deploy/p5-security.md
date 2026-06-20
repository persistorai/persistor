# P5 — Security posture & attack suite

Phase P5 is the "hammer it before AWS" gate for the remote MCP daemon. This is
the threat model, the test-backed attack suite, and the residual risks that gate
any exposure beyond the tailnet.

## Threat model (Model 1, tailnet)

**Protected:**
- **Cross-tenant access.** One tenant can never read or write another's memory.
- **Forged / replayed / confused tokens.** Only valid, unexpired, correctly-signed
  IdP tokens authenticate; algorithm-confusion and `alg=none` are rejected.
- **Memory poisoning via supersede.** A write cannot silently mark arbitrary
  (or non-existent) notes as superseded without an audit trail.
- **At-rest disclosure** of the product data on a detached/again-mounted volume.
- **Transport eavesdropping** on the tailnet (TLS).

**NOT protected (by design, this phase):**
- An attacker with **root on the running host** (the LUKS key is reachable; see
  P4 caveat). Whole-disk theft while the key file is on the same disk.
- Exposure **beyond the tailnet** — not done; a separate post-P5 decision.
- A **compromised IdP** or a second Connected App under the same Stytch project
  (see `aud`=project-id residual risk below).

## Attack suite (all run as the NOSUPERUSER NOBYPASSRLS `persistor` role)

| Threat | Test | What it asserts |
| --- | --- | --- |
| Cross-tenant read/write (notes) | `index.TestTenantBleed_Notes` | export/state/search/delete/update/insert all isolated; raw RLS WITH CHECK + USING block direct probes; role is non-superuser |
| Cross-tenant version log | `index.TestNoteVersions_TenantIsolation` | B sees none of A's history; WITH CHECK blocks a cross-tenant version insert |
| Forged signature | `mcpauth.TestOIDCAuth_Verify/bad signature` | token signed by another key → 401 |
| Expired token | `…/expired` | past-`exp` token → 401 |
| Wrong issuer / audience | `…/wrong issuer`, `…/wrong audience` | rejected |
| Algorithm confusion / `alg=none` | `…/alg confusion` | HS256-with-pubkey-as-secret rejected before the key func runs |
| Missing subject / garbage | `…/no subject`, `…/garbage` | rejected |
| Revoked static key | `mcpauth.TestStaticTokenAuth_Verify` | revoked/unknown key → `ErrInvalidToken` (401) |
| Storage error ≠ bad token | `mcpauth.TestOIDCAuth_TenantResolver` | a resolver/DB failure surfaces as 500, not a misleading 401 |
| Self-supersede no-op | `mcpengine.TestEngine_WriteRejectsSelfSupersede` | rejected |
| Supersede of a non-existent note | `mcpengine.TestEngine_WriteRejectsMissingSupersedeTarget` | rejected (no dangling-pointer poisoning) |

Run it:

```bash
TEST_DATABASE_URL="postgres://persistor:<pw>@localhost:5432/persistor_test?sslmode=disable" \
  go test ./... && go vet ./... && golangci-lint run ./... && govulncheck ./...
```

## Standing controls

- **RLS** is `FORCE`d on every tenant table; the pool refuses to start as a
  SUPERUSER/BYPASSRLS role (`dbpool.assertRLSEnforceable`), and the bleed test
  re-asserts it. `api_keys`/`identities`/`tenants` are deliberate RLS-EXEMPT
  admin tables (they hold no note content; they are the auth oracle consulted
  before a tenant is known).
- **Audit log:** `note_versions` is append-only (a trigger blocks UPDATE/DELETE)
  and records every create/update/delete/restore with `op` + `surface` (the
  client), so a poisoning attempt is always attributable.
- **Parameterized queries only** — no string-built SQL from user input.
- **Token validation** pins asymmetric algs, requires `exp`, checks `iss`/`aud`;
  validated statelessly against the IdP JWKS (no credentials stored). Session
  hijack is bounded by `TokenInfo.UserID` (the tenant), per the go-sdk.
- **Transport:** the go-sdk Streamable HTTP handler has DNS-rebind protection;
  TLS via `tailscale serve`; bound to the Tailscale IP, never a public listener.
- **Dependency scan:** `govulncheck ./...` clean.
- **Secret hygiene:** no secrets in the repo; live secrets (DB password, LUKS
  key, Stytch token) live outside the tree (`~/.persistor/`, `/root`, `~/.scout`).

## Residual risks (must be closed before public exposure)

1. **`aud` = Stytch project id**, not the resource URL — the RS trusts every token
   the project issues. Give Persistor its own Stytch project (or bind tokens to
   the resource) before adding a second Connected App. See the P3 docs.
2. **LUKS key on the local disk** — no whole-disk-theft protection. Move to a
   passphrase / removable / KMS before production. See `p4-encryption-tls.md`.
3. **Static at-rest only** — index text is plaintext in the DB (FTS needs it);
   Model 2 (per-tenant keys / enclave) is the future seam, not built.

## AWS / public-exposure checkpoint

Everything above is **tailnet-only**. Exposing the daemon beyond the tailnet
(public `claude.ai`, AWS, `tailscale funnel`) is a deliberate decision that must
first close residual risks #1 and #2 and is tracked in a separate hosting doc —
**do not enable it as part of this loop.**
