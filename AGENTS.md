# AGENTS.md

Operational guidance for coding agents in this repository.

## Project Snapshot

- Runtime: Go (`go-bridge`) + Swift tray app (`macos/opencode-bot`).
- Persistence: SQLite (`bridge.db`) under `DATA_DIR`.
- Telegram runtime: polling or webhook from Go bridge.
- OpenCode integration: HTTP API + optional SSE relay.
- Local control API: Unix socket by default (`/tmp/opencode-bot.sock`).

Primary architecture/spec document: `GO_MIGRATION_PLAN.md`.

## Install / Build / Test

- Go tests: `cd go-bridge && go test ./...`
- Build bridge binary: `cd go-bridge && go build ./cmd/bridge`
- Run bridge: `cd go-bridge && go run ./cmd/bridge serve`
- Migrate DB: `cd go-bridge && go run ./cmd/bridge --migrate`
- Import legacy JSON data: `cd go-bridge && go run ./cmd/bridge import-json`
- Build macOS app bundle: `bash ./scripts/build-tray-bridge.sh`

## Runtime Commands (Go CLI)

- `bridge serve`
- `bridge --migrate`
- `bridge import-json`
- `bridge resolve --usernames @a,@b`
- `bridge bootstrap --env-file .env`

## Environment Variables

Required:

- `BOT_TOKEN`
- `ADMIN_USER_IDS`

Important optional:

- `ALLOWED_USER_IDS`
- `BOT_TRANSPORT` (`polling` or `webhook`)
- `WEBHOOK_URL`, `WEBHOOK_LISTEN_ADDR`
- `BOT_POLLING_INTERVAL_SECONDS`
- `DATA_DIR`
- `OPENCODE_SERVER_URL`, `OPENCODE_SERVER_USERNAME`, `OPENCODE_SERVER_PASSWORD`
- `OPENCODE_BINARY`, `OPENCODE_CLI_WORKDIR`
- `DEFAULT_SESSION_ID`
- `OPENCODE_TIMEOUT_MS`
- `RELAY_MODE`, `RELAY_FALLBACK`, `RELAY_FALLBACK_DELAY_MS`, `RELAY_SSE_ENABLED`
- `SESSIONS_LIST_LIMIT`, `SESSIONS_SOURCE`, `SESSIONS_SHOW_ID_LIST`
- `CONTROL_WEB_SERVER`, `CONTROL_SOCKET_PATH`, `HEALTH_PORT`

## Directory Guide

- `go-bridge/cmd/bridge`: CLI entrypoint.
- `go-bridge/internal/service`: bridge/control/relay use cases.
- `go-bridge/internal/opencode`: OpenCode API + CLI session listing client.
- `go-bridge/internal/storage`: SQLite migrations and repositories.
- `go-bridge/internal/app`: health/control API server.
- `go-bridge/internal/telegram`: Telegram transport and resolver.
- `macos/opencode-bot`: Swift tray app.

## Core Behavior Constraints

- Authorization uses numeric Telegram user IDs only.
- Admin-only commands remain gated (`/allow`, `/deny`, `/list`).
- Avoid logging secrets.
- Unix socket control mode is default; TCP mode is optional.

## Agent Workflow Expectations

1. Read relevant files before edits.
2. Make minimal, focused changes.
3. Run `go test ./...` in `go-bridge` after meaningful Go changes.
4. Update docs when commands/configuration change.
