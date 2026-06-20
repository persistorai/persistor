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
| Key file         | `/root/.persistor-luks.key` (root, 0600)         |
| Mapper / mount   | `/dev/mapper/persistor-enc` → `/mnt/persistor-enc` |
| PG cluster       | `18/persistorenc` on **:5434**, data dir `/mnt/persistor-enc/pgdata` |
| Cluster start    | **manual** (`/etc/postgresql/18/persistorenc/start.conf`) so a reboot never fail-starts on the absent mount |
| DB role          | `persistor` — `NOSUPERUSER NOBYPASSRLS` (RLS is the tenant boundary) |
| Daemon DSN       | `~/.persistor/server.env` → `DATABASE_URL=…@127.0.0.1:5434/persistor` |

### Operate it

```bash
deploy/persistor-enc-unlock.sh   # after a reboot: open + mount + start :5434
deploy/persistor-enc-lock.sh     # stop + unmount + lock (raw img becomes ciphertext)
```

The cluster is manual-start, so after a reboot run the unlock script, then
(re)start the daemon (`~/.persistor/server.env` + `bin/persistor-server`, or the
systemd unit in `deploy/persistor-server.service`).

### At-rest proof (verified 2026-06-20)

Write a note via the remote MCP, `CHECKPOINT`, then lock the volume and grep the
raw container: the plaintext is absent from the ciphertext, and reappears after
unlock. (See the P4 gate notes — a unique marker written through the OIDC MCP
path was not found in `volume.img` while locked, and survived the lock/unlock.)

### ⚠️ Key-management caveat (hardening for P5 / production)

The LUKS key currently sits at `/root/.persistor-luks.key` — on the **same disk**
as the encrypted data. That protects against *nothing* if the whole disk is
stolen, because the thief gets the key too. It only meaningfully protects a
detached/again-mounted volume and keeps plaintext out of backups of the image.

For real disk-theft protection, pick one before production:

- **Manual passphrase** — add a passphrase keyslot (`cryptsetup luksAddKey`) and
  unlock interactively; store nothing on disk. Most secure, least convenient.
- **Removable key** — keep the key file on a USB key inserted only at unlock.
- **KMS/Vault** — fetch the key at unlock from a secret manager whose root of
  trust is not this disk. (On AWS this is just RDS encryption + KMS.)

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
