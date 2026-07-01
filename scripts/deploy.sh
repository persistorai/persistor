#!/usr/bin/env bash
# deploy.sh — build, push, and roll out persistor to production (DigitalOcean
# App Platform, mcp.persistor.ai) in one command. No manual doctl choreography.
#
#   scripts/deploy.sh                # deploy v$(cat VERSION)
#   scripts/deploy.sh --skip-gate    # skip the test gate (gate already green)
#   scripts/deploy.sh --image-only   # build + push the image, no rollout
#
# Steps: gate -> docker build (amd64, both binaries) -> push to DOCR ->
# bump the image tag in the live app spec -> doctl apps update -> wait for the
# deployment to go ACTIVE -> verify /healthz through Cloudflare.
#
# Secrets: the DO token comes from Vault at run time and is never written to
# disk. The app spec's encrypted secrets (EV[1:...] refs) round-trip through
# `doctl apps spec get` unchanged, so no secret ever needs re-supplying here.
#
# Version bookkeeping is enforced: the git tree must be clean, VERSION is the
# source of truth, and the release tag is created on the deployed commit if it
# doesn't already exist.
set -euo pipefail

cd "$(dirname "$0")/.." || exit 2

APP_ID=41846218-8bd4-415a-9b82-5e160d3f9643
REGISTRY=registry.digitalocean.com/persistor/persistor
HEALTH_URL=https://mcp.persistor.ai/healthz
DEPLOY_TIMEOUT_SECS=900

SKIP_GATE=0
IMAGE_ONLY=0
for arg in "$@"; do
  case "$arg" in
    --skip-gate)  SKIP_GATE=1 ;;
    --image-only) IMAGE_ONLY=1 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

step() { echo; echo "=== $* ==="; }

step "preflight"
if [ -n "$(git status --porcelain)" ]; then
  echo "FATAL: git tree is dirty — commit or stash before deploying" >&2
  exit 1
fi
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
  echo "FATAL: deploys ship from main (on: $BRANCH)" >&2
  exit 1
fi
VERSION="v$(tr -d '[:space:]' < VERSION)"
IMAGE="$REGISTRY:$VERSION"
echo "version: $VERSION  commit: $(git rev-parse --short HEAD)"

if [ "$SKIP_GATE" -eq 0 ]; then
  step "build gate"
  scripts/gate.sh
else
  echo "(gate skipped)"
fi

step "docker build $IMAGE"
docker build -f deploy/do/Dockerfile --platform linux/amd64 \
  --build-arg VERSION="$VERSION" -t "$IMAGE" .

step "authenticate (Vault -> DO token, in-memory only)"
# shellcheck disable=SC1090
source ~/.scout/secrets/vault.env
DIGITALOCEAN_ACCESS_TOKEN=$(vault kv get -field=token secret/digitalocean/pat)
export DIGITALOCEAN_ACCESS_TOKEN
doctl registry login --expiry-seconds 600 >/dev/null

step "push"
docker push "$IMAGE"

# Tag the release on the deployed commit (idempotent).
if ! git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
  git tag "$VERSION"
  echo "tagged $VERSION"
fi

if [ "$IMAGE_ONLY" -eq 1 ]; then
  echo "DEPLOY: image pushed ($IMAGE), rollout skipped (--image-only)"
  exit 0
fi

step "roll out spec with tag $VERSION"
SPEC=$(mktemp)
trap 'rm -f "$SPEC"' EXIT
doctl apps spec get "$APP_ID" > "$SPEC"
# Both the migrate job and the server service pin the same image tag.
sed -i -E "s|^(\s*tag:) .*$|\1 $VERSION|" "$SPEC"
grep -n 'tag:' "$SPEC"
doctl apps update "$APP_ID" --spec "$SPEC" >/dev/null

step "wait for deployment"
DEPLOY_ID=$(doctl apps list-deployments "$APP_ID" --format ID --no-header | head -1)
echo "deployment: $DEPLOY_ID"
start=$(date +%s)
while :; do
  PHASE=$(doctl apps get-deployment "$APP_ID" "$DEPLOY_ID" --format Phase --no-header)
  case "$PHASE" in
    ACTIVE) echo "phase: ACTIVE"; break ;;
    ERROR|CANCELED|SUPERSEDED)
      echo "FATAL: deployment ended in $PHASE" >&2
      doctl apps get-deployment "$APP_ID" "$DEPLOY_ID" --format Progress --no-header >&2 || true
      exit 1 ;;
    *) printf 'phase: %s\r' "$PHASE" ;;
  esac
  if [ $(( $(date +%s) - start )) -gt "$DEPLOY_TIMEOUT_SECS" ]; then
    echo "FATAL: deployment did not go ACTIVE within ${DEPLOY_TIMEOUT_SECS}s" >&2
    exit 1
  fi
  sleep 10
done

step "verify"
for i in 1 2 3; do
  code=$(curl -sS -o /dev/null -w '%{http_code}' -m 10 "$HEALTH_URL") && [ "$code" = "200" ] && break
  [ "$i" = 3 ] && { echo "FATAL: $HEALTH_URL returned $code" >&2; exit 1; }
  sleep 5
done
echo "healthz: 200"
echo
echo "DEPLOY: GREEN — $VERSION live on mcp.persistor.ai"
echo "Reminder: push the release tag (git push origin main --tags)."
