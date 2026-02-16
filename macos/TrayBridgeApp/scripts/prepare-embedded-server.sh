#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

EMBEDDED_SERVER_DIR="$APP_ROOT/embedded-server"

NPM_BIN="$(command -v npm || true)"
if [ -z "$NPM_BIN" ]; then
  echo "npm not found on PATH. Install Node/npm to prepare embedded server."
  exit 1
fi

if [ ! -d "$REPO_ROOT/dist" ]; then
  echo "dist/ missing. Running npm run build..."
  if [ ! -d "$REPO_ROOT/node_modules" ]; then
    echo "node_modules missing. Running npm install..."
    "$NPM_BIN" install --prefix "$REPO_ROOT"
  fi
  "$NPM_BIN" run build --prefix "$REPO_ROOT"
fi

echo "Preparing embedded server payload..."
rm -rf "$EMBEDDED_SERVER_DIR"
mkdir -p "$EMBEDDED_SERVER_DIR"
cp -R "$REPO_ROOT/dist" "$EMBEDDED_SERVER_DIR/dist"
cp "$REPO_ROOT/package.json" "$REPO_ROOT/package-lock.json" "$EMBEDDED_SERVER_DIR/"

"$NPM_BIN" ci --omit=dev --prefix "$EMBEDDED_SERVER_DIR"

NODE_BIN="$(command -v node || true)"
if [ -z "$NODE_BIN" ]; then
  echo "Node not found on PATH. Install Node to run the server."
  exit 1
fi

echo "Embedded server prepared."
