# P3 — Stytch OIDC setup (the morning checklist)

Where P3 stands and exactly what's left to take the remote MCP daemon from
static bearer tokens (P2) to real OAuth via Stytch (P3). Everything in the
backend is built, tested, and committed; what remains is Stytch dashboard
config, a live browser check of the consent page, and one exposure decision.

## What's already done (committed on `remote-mcp-model1`)

- **OIDC token verifier** — validates Stytch JWTs against the IdP's JWKS (pinned
  algorithms, issuer/audience/expiry), derives tenant as `uuidv5(iss|sub)`.
- **OIDC daemon mode** — `PERSISTOR_AUTH_MODE=oidc` builds that verifier and
  serves RFC 9728 protected-resource metadata at
  `/.well-known/oauth-protected-resource`.
- **Consent page** — served at `/authorize` (the Stytch "Authorization URL"),
  mounting Stytch's vanilla-js `IdentityProvider` component. First draft —
  needs the live browser check below.
- **DCR** — already enabled in the Stytch dashboard (done the night before).

## Stytch dashboard — remaining clicks

Test project: `seemly-sunset-9833` · domain `https://seemly-sunset-9833.customers.stytch.dev`.

1. **Connected Apps → Settings → Authorization URL** — set it to the consent
   page the daemon serves, e.g. `http://<daemon-host>:8088/authorize`
   (the Tailscale host for local testing). This is the field whose absence makes
   the discovery endpoint return `authorization_endpoint_not_configured`.
2. **Frontend SDK** — enable the SDK and authorize the daemon's origin
   (`http://<daemon-host>:8088`) so the consent page's SDK calls are allowed.
3. **Login method** — enable at least one (Email Magic Link or Google OAuth) so
   a user can authenticate on the consent page before consenting.
4. Leave **Dynamic client registration** ON (already done) and the access-token
   template as `{}`.

## Run the daemon in OIDC mode

Once the Authorization URL is set, the discovery endpoint goes live. Grab the
`jwks_uri` from it and run:

```bash
ISSUER=https://seemly-sunset-9833.customers.stytch.dev
curl -sS "$ISSUER/.well-known/oauth-authorization-server" | python3 -m json.tool   # read jwks_uri + registration_endpoint

DATABASE_URL="postgres://persistor:$(cat /tmp/.pgpw_persistor)@localhost:5432/persistor_test?sslmode=disable" \
PERSISTOR_NOTES_DIR=/tmp/persistor-remote-test \
PERSISTOR_LISTEN_ADDR="$(tailscale ip -4 | head -1):8088" \
PERSISTOR_PUBLIC_URL="http://$(tailscale ip -4 | head -1):8088" \
PERSISTOR_AUTH_MODE=oidc \
PERSISTOR_OIDC_ISSUER="$ISSUER" \
PERSISTOR_OIDC_AUDIENCE="http://$(tailscale ip -4 | head -1):8088" \
PERSISTOR_OIDC_JWKS_URL="<jwks_uri from discovery>" \
PERSISTOR_STYTCH_PUBLIC_TOKEN="public-token-test-c4002df6-6768-4b5b-9db2-478009c984d0" \
/home/user/code/persistor/bin/persistor-server
```

The **audience** is the resource identifier the client requests (RFC 8707) — it
should match `PERSISTOR_PUBLIC_URL`. Confirm it against a real token at
pre-flight (decode the JWT's `aud`) and adjust `PERSISTOR_OIDC_AUDIENCE` if Stytch
uses a different value.

## DCR pre-flight (the one live check that gates the rest)

With discovery live, confirm an unauthenticated public-client registration
succeeds — exactly what claude.ai's connector does:

```bash
REG=$(curl -sS "$ISSUER/.well-known/oauth-authorization-server" | python3 -c 'import json,sys;print(json.load(sys.stdin)["registration_endpoint"])')
curl -sS -X POST "$REG" -H 'Content-Type: application/json' -d '{
  "client_name": "persistor-preflight",
  "redirect_uris": ["http://localhost:9999/callback"],
  "token_endpoint_auth_method": "none",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"]
}'
```

A `201` with a `client_id` (and no client secret) means public-client DCR works.
The new client appears in the Connected Apps list.

## Live browser check of the consent page

Open `http://<daemon-host>:8088/authorize` in a browser on the tailnet. Expect
the Stytch login UI, then the consent dialog. If the SDK fails to load or the
login config is off, that's the known first-draft risk — likely fixes:

- swap the esm.sh import for Stytch's official script/CDN or a bundled SDK;
- adjust the `mountLogin` config shape (product/option field names) to the SDK
  version in use.

## The exposure decision (mine flagged, yours to make)

The P3 gate is "an MCP client completes the browser OAuth flow and calls tools."
Two ways to get there:

- **Option A — local Claude Code on the tailnet (recommended first).** Claude
  Code runs on a tailnet device, so it reaches the daemon's `/mcp` over the
  tailnet; the browser reaches `/authorize` (tailnet) and Stytch (public). **No
  public exposure.** This proves the full OAuth + tools path within the existing
  tailnet boundary.
- **Option B — claude.ai web connector.** claude.ai's servers must reach the
  daemon, so the daemon (and `/authorize`) must be **publicly reachable** — that
  crosses the "no exposure beyond the tailnet" stop-gate. Defer to after the P5
  hardening pass.

Recommendation: do **Option A** to close P3, and treat public exposure for
claude.ai as a deliberate, separate decision after hardening.

## Open items

- Consent page: live browser verification + likely SDK-load/login-config tweaks.
- Confirm the token `aud` and set `PERSISTOR_OIDC_AUDIENCE` to match.
- `esm.sh` is an external CDN; consider self-hosting the Stytch SDK before any
  non-spike use.
