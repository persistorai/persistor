<p align="center">
  <img src=".github/logo.png" alt="Persistor" width="120" height="120" style="border-radius: 24px;">
</p>

<h1 align="center">Persistor</h1>

<p align="center">
  <strong>Persistent knowledge graph + vector memory for AI agents</strong><br>
  <a href="https://persistor.ai">Website</a> · <a href="INTEGRATION.md">API Docs</a> · <a href="CHANGELOG.md">Changelog</a>
</p>

<p align="center">
  <img alt="License" src="https://img.shields.io/badge/license-AGPL--3.0-blue">
  <img alt="Go" src="https://img.shields.io/badge/go-1.25+-00ADD8?logo=go&logoColor=white">
  <img alt="PostgreSQL" src="https://img.shields.io/badge/PostgreSQL-16+-336791?logo=postgresql&logoColor=white">
</p>

Persistent knowledge graph and vector memory for AI agents.
Persistent knowledge graph and vector memory for AI agents. Durable, searchable,
graph-structured memory across sessions.

## Architecture

- **Go + Gin** — Fast, type-safe REST API
- **PostgreSQL 18 + pgvector** — Knowledge graph storage with vector similarity search
- **AES-256-GCM encryption** — All node/edge properties encrypted at rest, transparent to API consumers
- **Ollama embeddings** — Automatic vector generation for semantic search (qwen3-embedding:0.6b)
- **Row-Level Security** — Complete tenant isolation, one API key = one tenant
- **WebSocket** — Real-time change notifications via PostgreSQL LISTEN/NOTIFY

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 18 with pgvector extension
- Ollama running locally (for embedding generation)

### Setup

```bash
# Clone and build
git clone https://github.com/persistorai/persistor.git
cd persistor

# Configure environment (or use systemd EnvironmentFile — see Deployment)
export DATABASE_URL="postgres://persistor:<password>@localhost:5432/persistor?sslmode=disable"
export ENCRYPTION_PROVIDER=static
export ENCRYPTION_KEY="<64-hex-char-key>"  # 32 bytes, hex-encoded
export PORT=3030

# Build and run (migrations run automatically on startup)
make build
make run
```

### Verify

```bash
curl http://localhost:3030/api/v1/health
# {"status": "ok"}

curl http://localhost:3030/api/v1/ready
# {"status":"ok","checks":{"database":"ok","schema":"ok","ollama":"ok"}}
```

## Configuration

| Variable              | Default                  | Description                                     |
| --------------------- | ------------------------ | ----------------------------------------------- |
| `DATABASE_URL`        | — (required)             | PostgreSQL connection string                    |
| `PORT`                | `3030`                   | HTTP listen port                                |
| `LISTEN_HOST`         | `127.0.0.1`              | Listen address (must be loopback)               |
| `CORS_ORIGINS`        | `http://localhost:3002`  | Comma-separated allowed origins                 |
| `OLLAMA_URL`          | `http://localhost:11434` | Ollama API endpoint (must be localhost)         |
| `EMBEDDING_MODEL`     | `qwen3-embedding:0.6b`   | Embedding model name                            |
| `LOG_LEVEL`           | `info`                   | Log level                                       |
| `ENCRYPTION_PROVIDER` | `static`                 | `static` (env key) or `vault` (HashiCorp Vault) |
| `ENCRYPTION_KEY`      | — (required if static)   | 64 hex chars (32-byte AES key)                  |
| `VAULT_ADDR`          | `http://127.0.0.1:8200`  | Vault address (if provider=vault)               |
| `VAULT_TOKEN`         | — (required if vault)    | Vault token                                     |

## API Documentation

See **[INTEGRATION.md](./INTEGRATION.md)** for the complete API reference
with curl examples, data model documentation, and agent-specific usage patterns.

### Endpoint Summary

| Group     | Endpoints                                                                                                    |
| --------- | ------------------------------------------------------------------------------------------------------------ |
| Health    | `GET /health`, `GET /ready`                                                                                  |
| Nodes     | `GET/POST /nodes`, `GET/PUT/DELETE /nodes/:id`                                                               |
| Edges     | `GET/POST /edges`, `PUT/DELETE /edges/:source/:target/:relation`                                             |
| Search    | `GET /search`, `GET /search/semantic`, `GET /search/hybrid`                                                  |
| Graph     | `GET /graph/neighbors/:id`, `GET /graph/traverse/:id`, `GET /graph/context/:id`, `GET /graph/path/:from/:to` |
| Bulk      | `POST /bulk/nodes`, `POST /bulk/edges`                                                                       |
| Salience  | `POST /salience/boost/:id`, `POST /salience/supersede`, `POST /salience/recalc`                              |
| WebSocket | `GET /ws`                                                                                                    |
| Admin     | `POST /admin/backfill-embeddings`                                                                            |
| Audit     | `GET /audit`, `DELETE /audit`                                                                                |
| History   | `GET /nodes/:id/history`                                                                                     |
| Stats     | `GET /stats`                                                                                                 |
| Metrics   | `GET /metrics` (Prometheus, outside `/api/v1/`)                                                              |
| GraphQL   | `POST /graphql`, `GET /graphql/playground`                                                                   |

All under `/api/v1/` unless noted.

## Development

```bash
make build          # Build binary to bin/server
make run            # Build and run
make test           # Run tests with race detection
make test-coverage  # Tests + HTML coverage report
make lint           # golangci-lint
make lint-fix       # Auto-fix lint issues
make format         # gofmt + goimports
make vet            # go vet
make ci             # Full CI: format → vet → lint → test + coverage
make deps           # Download dependencies
make tidy           # go mod tidy
make setup-hooks    # Install git pre-commit hook
```

## Deployment

### systemd

```ini
# /etc/systemd/system/persistor.service
[Unit]
Description=Persistor
After=postgresql.service ollama.service
Requires=postgresql.service

[Service]
Type=simple
User=persistor
ExecStart=/usr/local/bin/persistor-server
EnvironmentFile=/etc/persistor.env
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Place environment variables in `/etc/persistor.env` (chmod 600, owned by root).

### Production Keys via Vault

For production, use the Vault encryption provider instead of a static key:

```bash
export ENCRYPTION_PROVIDER=vault
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=<your-vault-token>
```

The service fetches the encryption key from Vault at startup, avoiding plaintext keys in environment variables.

### Backup / Restore

```bash
./scripts/backup.sh              # pg_dump + verify + encrypt + rotate
./scripts/restore.sh <file>      # Restore from encrypted backup
./scripts/health-check.sh        # Quick health check
```

## License

This project is licensed under AGPL-3.0. See [LICENSE](LICENSE) for details. The Go SDK (`client/`) is licensed under Apache-2.0 to allow unrestricted client integration.
