#!/usr/bin/env bash
# Build the single static Traffic-monitor-amd64 binary (linux/amd64) with the
# Nuxt frontend embedded. Usage: scripts/build.sh [version]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-dev}"
EMBED="$ROOT/backend/web/public"
OUT="$ROOT/Traffic-monitor-amd64"

clean_embed() {
  find "$EMBED" -mindepth 1 ! -name index.html -exec rm -rf {} + 2>/dev/null || true
}

echo "==> Building frontend (nuxt generate)"
cd "$ROOT/frontend"
[ -d node_modules ] || npm install --no-audit --no-fund
npm run generate

echo "==> Staging embedded assets into backend/web/public"
clean_embed
cp -r "$ROOT/frontend/.output/public/." "$EMBED/"

echo "==> Building Go binary (linux/amd64, CGO disabled, static)"
cd "$ROOT/backend"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=${VERSION}" \
  -o "$OUT" ./cmd/traffic-monitor

echo "==> Restoring working tree"
clean_embed
git -C "$ROOT" checkout -- backend/web/public/index.html 2>/dev/null || true

echo "Built ${OUT} (version ${VERSION})"
