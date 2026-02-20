# syntax=docker/dockerfile:1

# ── Stage 1: Builder ──────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# build-base provides gcc/musl for CGO (required by mattn/go-sqlite3)
RUN apk add --no-cache build-base

WORKDIR /src

# Cache module downloads as a separate layer from source changes
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build-time metadata injected via ldflags (matches Makefile conventions)
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN --mount=type=cache,target=/root/.cache/go-build \
    go build \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
      -o /out/persistor-server \
      ./cmd/server

RUN --mount=type=cache,target=/root/.cache/go-build \
    go build \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
      -o /out/persistor \
      ./cmd/persistor-cli

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.21

# ca-certificates: TLS for outbound calls (Vault, Ollama remote)
# tzdata: correct timezone handling in logs
# wget: used by HEALTHCHECK
RUN apk add --no-cache ca-certificates tzdata wget

# Non-root user; UID/GID in the 100-range avoids conflicts with host users
RUN addgroup -S -g 10001 persistor \
 && adduser  -S -u 10001 -G persistor persistor

WORKDIR /app

COPY --from=builder --chown=persistor:persistor /out/persistor-server ./
COPY --from=builder --chown=persistor:persistor /out/persistor         ./

USER persistor

# ── Runtime defaults ─────────────────────────────────────────────────────────
# LISTEN_HOST=0.0.0.0 is required so the container accepts connections routed
# by Docker; the config validator explicitly allows it for containerised use.
# Secrets (DATABASE_URL, ENCRYPTION_KEY) must be supplied at runtime.
ENV PORT=3030 \
    LISTEN_HOST=0.0.0.0 \
    METRICS_PORT=9091 \
    LOG_LEVEL=info \
    DB_MAX_CONNS=21 \
    EMBED_WORKERS=4 \
    EMBEDDING_DIMENSIONS=1024 \
    EMBEDDING_MODEL=qwen3-embedding:0.6b \
    ENCRYPTION_PROVIDER=static \
    ENABLE_PLAYGROUND=false \
    CORS_ORIGINS=http://localhost:3002

# Server port (metrics always binds to 127.0.0.1 inside the container)
EXPOSE 3030

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://localhost:3030/api/v1/health || exit 1

ENTRYPOINT ["./persistor-server"]
