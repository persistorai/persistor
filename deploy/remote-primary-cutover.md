# Remote-primary deployment (warp-core)

The warp-core deployment where the **encrypted remote store is Scout's primary
memory**, shared by the local stdio MCP and remote OAuth clients (laptop/phone).
Cut over 2026-06-20. This is a deployment choice, not the product default — the
repo templates (`*.service`, `env.example`) still describe the general
encrypted-notes-dir default.

## Topology

- **Source of truth:** the git repo `/home/brian/code/scout` (+ the Claude
  memory dir), GitHub-backed. `.md` files are authoritative; the DB is a
  rebuildable index.
- **Shared index:** Postgres `:5434`, tenant `c6dc079c-…` (= uuidv5 of Brian's
  Stytch identity), data dir on the LUKS volume `/mnt/persistor-enc`.
- **warp-core stdio `persistor`** (`~/.persistor/env`): `DATABASE_URL` → `:5434`,
  `PERSISTOR_TENANT_ID` → the c6dc079c tenant. Read/writes the remote store.
- **remote daemon `persistor-server`** (`~/.persistor/server.env`):
  `PERSISTOR_NOTES_DIR` → the git repo, so remote writes land in git too
  (committed by the 30-min auto-commit cron). systemd unit `ReadWritePaths`
  covers the git + Claude memory dirs.
- **reindex cron** (`*/15`) sources `~/.persistor/env`, so it reindexes git →
  `:5434` automatically.

Both write paths write `.md` files into the git repo and index into the one
`:5434` tenant; everything reads that tenant. One memory, both directions.

## After a reboot

The LUKS volume is locked at boot, so **Scout has no memory on warp-core until
you unlock it**:

```bash
deploy/persistor-enc-unlock.sh   # 1Password prompts; opens volume, starts DB + daemon
```

Until then the stdio MCP and the cron can't reach `:5434` (they fail harmlessly).

## Rollback to the old local primary (`:5432`)

`:5432` (system Postgres, tenant `c20bafb4-…`) is kept as a frozen pre-cutover
snapshot. To revert:

```bash
cp ~/.persistor/env.bak-precutover        ~/.persistor/env
cp ~/.persistor/server.env.bak-precutover ~/.persistor/server.env
sudo cp /etc/systemd/system/persistor-server.service.bak-precutover \
        /etc/systemd/system/persistor-server.service
sudo systemctl daemon-reload && sudo systemctl restart persistor-server
```

Then restart Claude Code on warp-core. (Note: `:5432` is frozen at cutover time —
re-sync it with a `persistor reindex` against the old DSN if you need it current.)

## Caveats

- Remote writes go into the git repo (unencrypted on disk, but GitHub-private —
  Brian's established posture). The encrypted `:5434` DB encrypts the index/note
  bodies at rest; the git `.md` files are the portable source of truth.
- Single-user assumption: the daemon's one `PERSISTOR_NOTES_DIR` points at the
  git repo. Multi-tenant product serving needs per-tenant dirs (or PG-native,
  file-less writes) before another tenant shares this daemon.
