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
