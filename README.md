# OpenCode Telegram Bridge (MVP)

Telegram bot that gates access by `telegramUserId`, forwards prompts to OpenCode, and relays final answers through a global OpenCode plugin.

## Quickstart

1. Install dependencies:

```bash
npm install
```

2. Run interactive setup:

```bash
npm run setup
```

3. Start bot:

```bash
npm run dev
```

## Admin Commands

- `/allow <userId>`
- `/deny <userId>`
- `/list`
- `/status`
- `/resolve @username` (best-effort helper only)

## Session Selector

- `/session` show current linked OpenCode session
- `/session list` list recent OpenCode sessions with interactive buttons
- `/session use <ses_...>` switch current chat/user to an existing session
- `/session new` clear session link and force a new one on next message
- `/sessions` alias to list recent sessions with interactive buttons

## Notes

- Auth is strictly by numeric `telegramUserId`.
- Username resolution is convenience-only and never used as auth source.
- Installer writes plugin files to `~/.config/opencode/plugin/telegram-relay/`.

## macOS Tray App

This repository includes a macOS menu bar app at `macos/TrayBridgeApp`.

- Menu bar icon shows running/stopped status.
- Menu supports Start, Stop, Restart, and opening settings.
- Settings window allows changing project path, command, env vars, and viewing logs.
- Build process generates a custom app icon and embeds app description metadata.

Run locally:

```bash
cd macos/TrayBridgeApp
swift run TrayBridgeApp
```

Build a double-clickable `.app` bundle:

```bash
./macos/TrayBridgeApp/scripts/build-app.sh
open ./macos/TrayBridgeApp/dist/TrayBridgeApp.app
```

### macOS signing and startup errors

If macOS blocks startup because of signature/trust:

```bash
xattr -dr com.apple.quarantine ./macos/TrayBridgeApp/dist/TrayBridgeApp.app
codesign --force --deep --sign - ./macos/TrayBridgeApp/dist/TrayBridgeApp.app
open ./macos/TrayBridgeApp/dist/TrayBridgeApp.app
```

For distribution with a real Apple certificate:

```bash
APPLE_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" ./macos/TrayBridgeApp/scripts/build-app.sh
```

## CI/CD on `main`

This project includes a GitHub Actions workflow at `.github/workflows/build-and-release.yml`.

- Trigger: every push to `main`.
- Security: runs `gitleaks` secret scan before build/release steps.
- Build steps: `npm ci`, `npm run typecheck`, `npm run build`, and macOS tray app packaging.
- Version source: `package.json` field `version`.
- Tag: `v${version}` is created if it does not exist.
- Publish: a GitHub Release is created/updated with build artifacts.

If you want a new tag/release version, update `version` in `package.json`.
