# Local dev stack

An **on-demand, throwaway** Persistor you can poke at — the same image DO runs,
with the same split-role RLS posture, pointed at the Stytch **test** project.

## When to use this (and when not to)

- **Day-to-day dev — use `make gate`, not this.** The gate self-provisions a
  disposable Postgres and runs the full test suite (including the RLS/tenant
  tests). That's the loop for changing code. It needs nothing here.
- **Use this stack only to poke a _running_ server** — reproduce a client bug,
  eyeball an endpoint, watch logs against real traffic.

There is **no systemd unit** and nothing starts on boot. Your real memory lives
in production (`mcp.persistor.ai`); local is always ephemeral.

## Quick start

```bash
cp deploy/local/.env.local.example deploy/local/.env.local   # then fill the Stytch test token
make dev-up      # build + migrate + serve, waits until healthy
make dev-logs    # tail the server
make dev-down    # stop (keeps the DB volume)
make dev-reset   # stop AND wipe the DB volume (fresh schema next up)
```

Once up:

- `http://localhost:8087/healthz` — process up (200).
- `http://localhost:8087/readyz` — DB-checked: proves the non-owner `persistor_app`
  role connects and the schema is current (200).
- `http://localhost:8087/.well-known/oauth-protected-resource` — advertises the
  Stytch **test** authorization server.
- `http://localhost:8087/mcp` — the MCP endpoint (needs a bearer token).

Postgres is on `127.0.0.1:5468` if you want the operator CLI against it:
`DATABASE_URL=postgres://persistor_app:persistor_app@127.0.0.1:5468/persistor?sslmode=disable`.

## Parity with production

| Aspect | Local stack | Production (DO) |
| --- | --- | --- |
| Image | `deploy/do/Dockerfile` (built) | same Dockerfile → DOCR |
| Migrations | `migrate` service, `persistor_migrator` (owner) | PRE_DEPLOY job, same role |
| Daemon role | `persistor_app`, NOSUPERUSER NOBYPASSRLS, `AUTO_MIGRATE=false` | same |
| RLS / tenant isolation | enforced (FORCE + NOBYPASSRLS) | same |
| OIDC | Stytch **test** (`.stytch.dev`) | Stytch **live** (`.stytch.com`) |
| Edge origin lock | off (no Cloudflare in front) | on (`X-Origin-Secret`) |

The one deliberate simplification: the local app role gets full DML on all tables
(via `ALTER DEFAULT PRIVILEGES`) instead of prod's tightened
`note_versions` INSERT-only grant. Append-only immutability still holds — it's
enforced by a DB trigger, not just the grant. See `initdb/00-roles.sql`.

## Getting a token (interactive)

The daemon is OIDC-only — there is no auth bypass, by design. To drive `/mcp`
from Claude Code locally, add it as an MCP server pointed at
`http://localhost:8087/mcp` and complete the browser OAuth flow against the Stytch
test project (you may need `claude mcp login <name> --no-browser`; the local
callback must be allowed in the test project's Connected Apps redirect URLs).
Tokens issued this way map to **test** tenants — separate from your Live memory.
