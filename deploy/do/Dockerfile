# Persistor container image — ships BOTH binaries:
#   /usr/local/bin/persistor-server  (the MCP daemon, default entrypoint)
#   /usr/local/bin/persistor          (the operator CLI, used for `persistor migrate`)
#
# The DO App Platform pre-deploy job runs `persistor migrate`; the service runs
# persistor-server. Same image, two roles — so the migrator and the daemon are
# byte-identical builds (no schema/binary skew between migrate and serve).
#
# Pure-Go build: modernc.org/sqlite + embedded migrations mean CGO_ENABLED=0, so
# the binaries are fully static and run on a minimal base. App Platform shared
# instances are amd64, so we target linux/amd64.
#
# Build (from repo root):
#   docker build -f deploy/do/Dockerfile --platform linux/amd64 -t persistor:vX.Y.Z .

FROM golang:1.25-alpine AS build
WORKDIR /src
# git only needed if any module is fetched via VCS; harmless otherwise.
RUN apk add --no-cache git
# Cache deps separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
        -ldflags="-s -w -X github.com/briancolinger/persistor/internal/config.Version=${VERSION}" \
        -o /out/persistor-server ./cmd/persistor-server \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
        -ldflags="-s -w -X github.com/briancolinger/persistor/internal/config.Version=${VERSION}" \
        -o /out/persistor ./cmd/persistor-cli

# Minimal runtime: alpine gives us /bin/sh (App Platform run_command needs a
# shell) + ca-certificates (the daemon fetches Stytch JWKS over HTTPS).
FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
 && adduser -D -H -u 10001 persistor
COPY --from=build /out/persistor-server /usr/local/bin/persistor-server
COPY --from=build /out/persistor        /usr/local/bin/persistor
USER persistor
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/persistor-server"]
