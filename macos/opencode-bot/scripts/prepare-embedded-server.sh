#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

EMBEDDED_SERVER_DIR="$APP_ROOT/embedded-server"
GO_BRIDGE_DIR="$REPO_ROOT/go-bridge"

GO_BIN="$(command -v go || true)"
if [ -z "$GO_BIN" ]; then
  echo "go not found on PATH. Install Go to prepare embedded bridge."
  exit 1
fi

echo "Building Go bridge binary..."
rm -rf "$EMBEDDED_SERVER_DIR"
mkdir -p "$EMBEDDED_SERVER_DIR"

(cd "$GO_BRIDGE_DIR" && "$GO_BIN" build -o "$EMBEDDED_SERVER_DIR/bridge" ./cmd/bridge)

chmod +x "$EMBEDDED_SERVER_DIR/bridge"

echo "Embedded Go bridge prepared at $EMBEDDED_SERVER_DIR/bridge"
