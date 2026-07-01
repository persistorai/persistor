# Deploying Persistor to DigitalOcean

Production target: **Cloudflare (proxied `mcp.persistor.ai`) → App Platform service → DO Managed PostgreSQL 18 (single-node)**. See `../../../planning-docs/persistor/digitalocean-deploy-plan.md` for the full rationale.

Files here:

- `Dockerfile` — multi-stage, ships both `persistor-server` and the `persistor` CLI; static amd64 build.
- `app.yaml` — App Platform spec (service + pre-deploy `persistor migrate` job + DB attach).

All `doctl` calls authenticate via the Vault-sourced token (never written to disk):

```bash
source ~/.scout/secrets/vault.env
export DIGITALOCEAN_ACCESS_TOKEN=$(vault kv get -field=token secret/digitalocean/pat)
```

> **$ = billable.** Steps marked `[$]` create paid resources. Nothing here runs without an explicit go-ahead.

## Provisioning order

### 1. Container registry + image  `[$ ~free]`
DOCR Starter tier is free (1 repo, 500 MB — our image is tiny).

```bash
doctl registry create persistor --subscription-tier starter
doctl registry login
docker build -f deploy/do/Dockerfile --platform linux/amd64 \
  --build-arg VERSION=v0.9.2 \
  -t registry.digitalocean.com/persistor/persistor:v0.9.2 .
docker push registry.digitalocean.com/persistor/persistor:v0.9.2
```

### 2. Managed PostgreSQL 18, single-node  `[$ $15/mo]`

```bash
doctl databases create persistor-db \
  --engine pg --version 18 --region nyc \
  --size db-s-1vcpu-1gb --num-nodes 1
# wait for "online":
doctl databases get persistor-db --format Status
# create the app database (alongside the auto-created defaultdb):
doctl databases db create persistor-db persistor
```

### 3. Bootstrap roles + extension (one time, as doadmin)
Generate two strong role passwords, persist them in Vault (Brian's hands — Scout's
token is read-only), then run `provision-roles.sql` Phase 1 against the new DB as
`doadmin`. btree_gin + the `persistor_migrator` (owner) and `persistor_app`
(non-owner, NOSUPERUSER NOBYPASSRLS) roles get created here. Then run
`persistor migrate` as the migrator, then Phase 2 grants. Connection string from:

```bash
doctl databases connection persistor-db --format Host,Port,User,Password,Database
```

Vault paths for the role creds (durable + rotatable):
`secret/persistor/db/migrator` and `secret/persistor/db/app` (field `password`).

### 4. App  `[$ $12/mo]`
Render a temp spec with secrets filled from Vault into the scratchpad, apply, shred:
the three `SET-AT-DEPLOY` values are the two role `DATABASE_URL`s and the Stytch
public token. Then:

```bash
doctl apps create --spec /path/to/rendered-spec.yaml   # in scratchpad, never committed
```

### 5. Cloudflare `mcp` record
After the app reports its default `*.ondigitalocean.app` hostname:

```
CNAME  mcp.persistor.ai -> <app-default-hostname>   (proxied / orange)
```
Set the zone SSL/TLS mode to **Full (strict)** (App Platform serves a valid cert).
Scout does this via the Cloudflare API token.

### 6. Stytch — authorize the prod origin (Brian, Test project for now)
Add `https://mcp.persistor.ai` to the Stytch project's SDK **Authorized domains**
and the OAuth **redirect URL**, or the consent page errors with
`bad_domain_for_stytch_sdk`. This applies to the Test project we bring up on; redo
for the Live project at go-live.

## Test → Live (Stytch) cutover, later
Live is an env-only change: swap the four `PERSISTOR_OIDC_*` / public-token values
to the Stytch **Live** project. Because `tenant = uuidv5(issuer | subject)` and Live
has a different issuer *and* a separate user pool, the **prod tenant UUID changes** —
prod starts empty. To carry memories over: `persistor export` (old tenant) →
`persistor import` (new). Intentional, clean cutover.

## Verify
```bash
curl -sf https://mcp.persistor.ai/healthz        # liveness
curl -sf https://mcp.persistor.ai/readyz         # DB-checked readiness
# then an authenticated MCP round-trip from a real client (memory_write/search).
```

## Backups + restore

Nightly at 03:30 America/Chicago, `scripts/backup-prod.sh` (systemd user timer
`persistor-backup.timer` on warp-core; units in `deploy/backup/`) pulls every
tenant's notes OFF DigitalOcean — DO's own backups live inside the account and
die with it. Each run: short DB-firewall window for warp-core's IP (removed on
exit) → `persistor export` per tenant (tenant-scoped, RLS-satisfied — do NOT
substitute a plain pg_dump: a non-BYPASSRLS role silently dumps zero rows from
every RLS table) → tenants/identities CSVs → tar → age-encrypt to
`/mnt/storage/backups/persistor/` (newest 14 kept). Encryption key:
`~/.scout/secrets/backup-pubkey.txt`; private key `backup.age` (DR copy in
1Password).

### Restore from backup

1. Provision infra + schema as in this README (roles, `persistor migrate`).
2. Decrypt + unpack:
   `age -d -i ~/.scout/secrets/backup.age <backup>.tar.age | tar -x`
3. Recreate tenants/identities routing from the CSVs (`\copy ... FROM` as the
   migrator), or let OIDC re-provision and map identities via `persistor admin`.
4. Per tenant: `persistor import --tenant <id> --namespace <ns> <dir>` for each
   namespace subdirectory of `tenant-<id>/` (frontmatter ids are preserved, so
   ids and supersedes chains survive; version history does not — the backup
   captures current notes, not the audit log).
5. Verify: `persistor namespaces --tenant <id>` counts match the export, and a
   live `memory_search` through `/mcp` returns a known note.

Verified 2026-07-01: full round-trip (prod write → backup → decrypt →
frontmatter-intact .md) with a canary note.
