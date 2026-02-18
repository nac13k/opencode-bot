# opencode-bot

Go Telegram bridge for OpenCode with a macOS tray app.

## Architecture

- `go-bridge`: main runtime (Telegram bot, OpenCode client, control API, SQLite).
- `macos/opencode-bot`: Swift tray app to configure and run the bundled bridge.
- Control API defaults to Unix socket (`/tmp/opencode-bot.sock`) and can run on TCP when enabled.

See `GO_MIGRATION_PLAN.md` for full design.

## Required env

```env
BOT_TOKEN=<telegram-bot-token>
ADMIN_USER_IDS=<comma-separated-user-ids>
```

Common optional env:

```env
ALLOWED_USER_IDS=
BOT_TRANSPORT=polling
WEBHOOK_URL=
WEBHOOK_LISTEN_ADDR=:8090
BOT_POLLING_INTERVAL_SECONDS=2

DATA_DIR=./data
OPENCODE_SERVER_URL=http://127.0.0.1:4096
OPENCODE_SERVER_USERNAME=opencode
OPENCODE_SERVER_PASSWORD=
OPENCODE_BINARY=opencode
OPENCODE_CLI_WORKDIR=
DEFAULT_SESSION_ID=
OPENCODE_TIMEOUT_MS=120000

RELAY_MODE=last
RELAY_FALLBACK=true
RELAY_FALLBACK_DELAY_MS=3000
RELAY_SSE_ENABLED=false

SESSIONS_LIST_LIMIT=5
SESSIONS_SOURCE=both
SESSIONS_SHOW_ID_LIST=true

CONTROL_WEB_SERVER=false
CONTROL_SOCKET_PATH=/tmp/opencode-bot.sock
HEALTH_PORT=4097
```

## Run bridge locally

```bash
cd go-bridge
go test ./...
go run ./cmd/bridge --migrate
go run ./cmd/bridge serve
```

## Go CLI commands

- `go run ./cmd/bridge serve`
- `go run ./cmd/bridge --migrate`
- `go run ./cmd/bridge import-json`
- `go run ./cmd/bridge resolve --usernames @a,@b`
- `go run ./cmd/bridge bootstrap --env-file .env`

## Telegram commands

- `/start`
- `/status`
- `/session` / `/sessions`
- `/models`
- `/compact`
- `/allow` `/deny` `/list` (admin only)

## Legacy JSON migration

If you have old Node/TS JSON data under `DATA_DIR` (`admins.json`, `allowed-users.json`, `session-links.json`, `session-models.json`):

```bash
cd go-bridge
go run ./cmd/bridge import-json
```

`--migrate` and `serve` also attempt import automatically.

## macOS tray app

Run locally:

```bash
cd macos/opencode-bot
swift run OpencodeBot
```

Build `.app` bundle:

```bash
bash ./scripts/build-tray-bridge.sh
```

Or manually:

```bash
bash ./macos/opencode-bot/scripts/prepare-embedded-server.sh
bash ./macos/opencode-bot/scripts/build-app.sh
open ./macos/opencode-bot/dist/opencode-bot.app
```
