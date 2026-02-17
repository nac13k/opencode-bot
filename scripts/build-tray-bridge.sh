#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Building server (npm run build)"
cd "$ROOT_DIR"
npm run build

echo "==> Preparing embedded server"
"$ROOT_DIR/macos/TrayBridgeApp/scripts/prepare-embedded-server.sh"

echo "==> Building TrayBridgeApp"
"$ROOT_DIR/macos/TrayBridgeApp/scripts/build-app.sh"

echo "==> Done"
echo "App bundle: $ROOT_DIR/macos/TrayBridgeApp/dist/opencode-bot.app"
