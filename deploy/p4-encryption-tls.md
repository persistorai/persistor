# P4 — Encryption at rest, TLS, and onboarding

This is the Phase P4 runbook for the remote MCP daemon on warp-core (Model 1:
storage-layer encryption + tailnet TLS). It records the encrypted-instance setup
that is already live, the TLS posture, and the tenant onboarding flow.

## At-rest encryption — a dedicated LUKS-backed Postgres instance

Rather than re-encrypting the system Postgres (which also holds Scout's live
memory on :5432), the remote-MCP product data runs in a **separate Postgres 18
cluster whose data directory lives on a LUKS volume**. This isolates the product
data, never disrupts the system instance, and mirrors the eventual AWS shape (a
dedicated, RDS-encrypted instance). Both `notes.body` and `chunks.text` stay
plaintext *inside* the DB (FTS needs them) but are encrypted *on disk*.

What was set up (idempotent — these are the live values):

| Piece            | Value                                            |
| ---------------- | ------------------------------------------------ |
| LUKS container   | `/var/lib/persistor-enc/volume.img` (luks2, 4G)  |
| LUKS key         | **1Password → Personal → "Persistor LUKS key (warp-core)"** (document). NO on-disk copy. |
| Mapper / mount   | `/dev/mapper/persistor-enc` → `/mnt/persistor-enc` |
| Notes dir        | `/mnt/persistor-enc/notes` (on the encrypted volume — note files never hit unencrypted disk) |
| PG cluster       | `18/persistorenc` on **:5434**, data dir `/mnt/persistor-enc/pgdata` |
| Cluster start    | **manual** (`/etc/postgresql/18/persistorenc/start.conf`) so a reboot never fail-starts on the absent mount |
| DB role          | `persistor` — `NOSUPERUSER NOBYPASSRLS` (RLS is the tenant boundary) |
| Daemon           | systemd `persistor-server.service` (User=brian, RW only to the notes dir); NOT boot-enabled |
| Daemon DSN       | `~/.persistor/server.env` → `DATABASE_URL=…@127.0.0.1:5434/persistor` |

### Operate it

```bash
deploy/persistor-enc-unlock.sh   # open (key from 1Password) + mount + start :5434 + start daemon
deploy/persistor-enc-lock.sh     # stop daemon + cluster + unmount + lock (raw img becomes ciphertext)
```

`unlock` streams the key from 1Password straight into `cryptsetup` over stdin (it
never lands on disk), waits for Postgres to accept connections, then starts the
daemon. It needs your interactive `op` session (unlocked 1Password app or
`op signin`). Break-glass: set `PERSISTOR_LUKS_KEYFILE=/path/to/key` to bypass
1Password if you have a copy of the raw key.

### At-rest proof (verified 2026-06-20)

Write a note via the remote MCP, `CHECKPOINT`, then lock the volume and grep the
raw container: the plaintext is absent from the ciphertext, and reappears after
unlock. (A unique marker written through the OIDC MCP path was not found in
`volume.img` while locked, and survived the lock/unlock.) Note **files** also
live on the encrypted volume (`/mnt/persistor-enc/notes`), so no note plaintext
exists outside it.

### Key management — disk-theft protection (done)

The LUKS key lives **only in 1Password** (Personal vault), not on this disk, so a
stolen powered-off disk cannot be unlocked — opening the volume requires Brian's
interactive 1Password session. This is the deliberate trade: unlock-after-reboot
is a manual step, which is correct for a key like this.

- **Recovery:** the key is in 1Password (cloud-synced, recoverable). The encrypted
  data is also rebuildable (notes are the source of truth; the index reindexes).
- **Optional break-glass:** add a second keyslot with a passphrase
  (`cryptsetup luksAddKey`) so losing 1Password access can't brick the volume.

Auto-unlock at boot via `/etc/crypttab` is deliberately NOT configured: it would
re-introduce the on-disk-key weakness and add a boot-critical dependency.

## TLS in transit

On the tailnet, TLS is terminated by **`tailscale serve`** (tailnet-only HTTPS
with a real cert), fronting the daemon on `100.96.84.118:8088`:

```bash
sudo tailscale serve --bg --https=443 http://100.96.84.118:8088
# https://warp-core.tailf32134.ts.net  (tailnet only — NOT `funnel`)
```

A standalone reverse proxy (Caddy/nginx) for TLS is only required once the
service is exposed beyond the tailnet — a separate, post-P5 decision.

## Onboarding a tenant

Tenancy is claim-driven. **First login auto-provisions**: the OIDC auth path
resolves the token's `(issuer, subject)` against the `identities` table and, on
first sight, creates a personal tenant (id = `uuidv5(iss|sub)`) + an `owner`
identity row — so a brand-new user simply logs in and starts writing memory.

Admin paths (the work-vs-personal / shared-tenant case), via the CLI against the
encrypted instance (`DATABASE_URL=…:5434/persistor`):

```bash
persistor admin tenant create --label "work"          # -> prints a tenant UUID
persistor admin identity set --issuer <iss> --subject <sub> \
        --tenant <uuid> --role member                 # pre-map BEFORE first login
persistor admin identity list
persistor admin identity delete --issuer <iss> --subject <sub>   # offboard (keeps notes)
```

## Backup / export

```bash
persistor export --tenant <uuid> --out <dir>   # every current note -> .md (0600)
```

The export is portable, git-trackable markdown that re-imports faithfully
(`RenderNote`/`ParseNote` round-trip). This is the Model 1 backup story.
