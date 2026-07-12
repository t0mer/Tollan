#!/usr/bin/env bash
# Runs the Go backend and the Vite dev server together with hot reload. The Vite
# server proxies /api, /health and /metrics to the Go backend on :8080.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT INT TERM

echo ">> backend on :8080 (TOLLAN_DATA_DIR=./data)"
TOLLAN_DATA_DIR="${TOLLAN_DATA_DIR:-./data}" \
  go run ./cmd/tollan run --log-level debug &

echo ">> frontend on :5173"
(cd web && npm run dev) &

wait
