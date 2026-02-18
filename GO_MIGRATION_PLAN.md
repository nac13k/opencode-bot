# Go Migration Plan (Hexagonal + Swift Tray)

This plan captures the agreed migration path from Node to Go, keeping the Swift tray UI and replacing the OpenCode plugin with SSE relay.

## Decisions

- Migrate all logic to a Go binary.
- Keep Swift tray as the primary UI/launcher.
- Persist data in SQLite.
- Support Telegram polling and webhook.
- Use OpenCode SSE `/event` for relay (no plugin).
- Relay modes: `last` or `final`.
- Fallback is configurable and only applies to `final`.
- Fallback delay is configurable (ms).
- Resolve usernames from tray via Go binary, batch supported.
- Resolve results are added immediately to Admin/Allowed lists.
- Logging to rotating file.
- Go binary exposes a health endpoint for tray monitoring.

## Configuration (env + settings)

- `BOT_TOKEN`
- `ADMIN_USER_IDS`
- `ALLOWED_USER_IDS`
- `BOT_TRANSPORT` (`polling` | `webhook`)
- `WEBHOOK_URL` (webhook only)
- `DATA_DIR` (SQLite path)
- `OPENCODE_SERVER_URL`
- `OPENCODE_SERVER_USERNAME`
- `OPENCODE_SERVER_PASSWORD`
- `OPENCODE_BINARY`
- `OPENCODE_CLI_WORKDIR`
- `DEFAULT_SESSION_ID`
- `OPENCODE_TIMEOUT_MS`
- `RELAY_MODE` (`last` | `final`)
- `RELAY_FALLBACK` (`true` | `false`)
- `RELAY_FALLBACK_DELAY_MS`
- `HEALTH_PORT`
- `LOG_LEVEL`

## Architecture (Hexagonal)

### Domain

- `User`
- `SessionLink`
- `SessionModel`

### Ports

- `AuthzRepository`
- `SessionLinkRepository`
- `SessionModelRepository`
- `UserResolver`
- `OpenCodeClient`
- `TelegramClient`
- `EventStream`

### Adapters

- SQLite repositories
- OpenCode HTTP client
- SSE event stream client
- Telegram adapter (polling + webhook)
- Env config loader
- Rotating file logger

### Application Services

- Queue by chat/user
- Relay service (SSE -> Telegram)
- Use cases: start, status, session list, set/clear session, models, compact
- Resolve usernames (tray-triggered)

## Relay (SSE) Behavior

- Cache last message by session ID.
- On `message.updated`: update cache.
- On `session.idle`:
  - `RELAY_MODE=last`: send immediately.
  - `RELAY_MODE=final`:
    - If final detected: send.
    - Else if `RELAY_FALLBACK=true`: wait `RELAY_FALLBACK_DELAY_MS`, then send last cached.

Final detection strategy:

- Prefer event payload markers.
- If missing, fetch `/session/:id/message` and use last assistant message.

## Telegram Bot

- Modes: polling + webhook.
- Commands:
  - `/start` (apply `DEFAULT_SESSION_ID`)
  - `/status`
  - `/session` / `/sessions`
  - `/models`
  - `/compact`
- Remove Telegram `/resolve` command (tray only).

## Tray Swift UI

- UI remains the primary launcher.
- Add settings:
  - Relay mode
  - Fallback toggle
  - Fallback delay (ms)
- Add “Resolve usernames” button:
  - Accepts multiple `@usernames` (comma/space separated).
  - Calls Go binary.
  - On success, add to Admin/Allowed immediately.
  - On failure, show manual steps:
    1) Ask user to message the bot.
    2) Use @userinfobot for ID.
    3) Add ID manually.

## Go Binary CLI

- `bridge --migrate` (create/update DB)
- `bridge import-json` (import legacy JSON files into SQLite)
- `bridge resolve --usernames @a,@b` (batch resolve)
- `bridge bootstrap --env-file .env` (write env template from effective config)
- `bridge serve` (default run)

## Health Endpoint

- `GET /health` with:
  - uptime
  - OpenCode connectivity
  - Telegram connectivity
  - relay mode + fallback status

## Packaging

- Build Go binary and embed in macOS app bundle.
- Swift tray executes the Go binary and passes env vars.
- Update build scripts accordingly.
