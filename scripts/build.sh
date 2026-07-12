#!/usr/bin/env bash
# Cross-compiles the tollan server and agent into dist/ for the release matrix.
# Builds the embedded web UI first (unless dist assets already exist), then the
# Go binaries with CGO disabled for clean static cross-compilation.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w \
  -X github.com/t0mer/tollan/internal/version.Version=${VERSION} \
  -X github.com/t0mer/tollan/internal/version.Commit=${COMMIT} \
  -X github.com/t0mer/tollan/internal/version.Date=${DATE}"

# Build the frontend so it is embedded into the binary.
if [ ! -f web/dist/index.html ] || [ "${REBUILD_UI:-0}" = "1" ]; then
  echo ">> building web UI"
  (cd web && npm ci && npm run build)
  touch web/dist/.gitkeep
fi

# OS/ARCH targets: linux (amd64, arm64, armv7, armv6, 386),
# darwin (amd64, arm64), windows (amd64, arm64).
TARGETS=(
  "linux/amd64" "linux/arm64" "linux/arm/7" "linux/arm/6" "linux/386"
  "darwin/amd64" "darwin/arm64"
  "windows/amd64" "windows/arm64"
)

rm -rf dist
mkdir -p dist

for bin in tollan tollan-agent; do
  for t in "${TARGETS[@]}"; do
    IFS=/ read -r GOOS GOARCH GOARM <<<"$t"
    ext=""
    [ "$GOOS" = "windows" ] && ext=".exe"
    suffix="$GOARCH"
    [ -n "${GOARM:-}" ] && suffix="armv${GOARM}"
    out="dist/${bin}_${VERSION}_${GOOS}_${suffix}${ext}"
    echo ">> ${out}"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" GOARM="${GOARM:-}" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$out" "./cmd/${bin}"
  done
done

echo ">> done"
ls -1 dist
