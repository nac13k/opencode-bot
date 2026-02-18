#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Building Go bridge"
cd "$ROOT_DIR/go-bridge"
go test ./...
go build ./cmd/bridge

echo "==> Preparing embedded server"
bash "$ROOT_DIR/macos/opencode-bot/scripts/prepare-embedded-server.sh"

echo "==> Building opencode-bot app"
bash "$ROOT_DIR/macos/opencode-bot/scripts/build-app.sh"

echo "==> Done"
echo "App bundle: $ROOT_DIR/macos/opencode-bot/dist/opencode-bot.app"
