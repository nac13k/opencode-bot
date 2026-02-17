# AGENTS.md

Operational guidance for coding agents in this repository.

## Project Snapshot

- Stack: Node.js + TypeScript + ESM.
- Telegram runtime: `grammy`.
- Config: `.env` loaded via `dotenv`.
- Persistence: JSON files in `DATA_DIR` (default `./data`).
- OpenCode integration: HTTP server (`OPENCODE_SERVER_URL`).
- Global plugin template: `plugin-global/telegram-relay`.

Read `IMPLEMENTATION_PLAN.md` before major architectural changes.

## Rule Sources Checked

- `.cursor/rules/`: not present.
- `.cursorrules`: not present.
- `.github/copilot-instructions.md`: not present.

If these files appear later, treat them as high-priority constraints and update this document.

## Install / Build / Lint / Test

Use `npm` in the repo root.

- Install deps: `npm install`
- Dev mode: `npm run dev`
- Setup wizard: `npm run setup`
- Typecheck: `npm run typecheck`
- Lint (currently same as typecheck): `npm run lint`
- Build: `npm run build`
- Tests: `npm test`

## Running a Single Test (Important)

Current `test` script uses Node's built-in runner. Use these directly:

- Single file: `node --test path/to/file.test.ts`
- By test name: `node --test --test-name-pattern "your test name"`

If the test framework changes (Vitest/Jest), update this section immediately.

## Runtime Commands (Operational)

- Start bot in dev: `npm run dev`
- Run installer: `npm run setup`
- Production start (after build): `npm run build && npm start`

## Environment Variables

Required:

- `BOT_TOKEN`
- `ADMIN_USER_IDS` (comma-separated numeric IDs)

Optional:

- `BOT_TRANSPORT` (`polling` default, `webhook` accepted but not implemented in runtime)
- `DATA_DIR` (`./data` default)
- `OPENCODE_SERVER_URL` (`http://127.0.0.1:4096` default)
- `OPENCODE_SERVER_USERNAME` (`opencode` default)
- `OPENCODE_SERVER_PASSWORD` (optional)
- `DEFAULT_SESSION_ID` (optional)
- `OPENCODE_TIMEOUT_MS` (`120000` default)

## Directory Guide

- `src/bot`: Telegram bot setup, middleware, commands.
- `src/auth`: authorization service.
- `src/store`: JSON storage and schema guards.
- `src/opencode`: OpenCode HTTP client and session/queue logic.
- `src/resolver`: username best-effort resolver.
- `src/relay`: direct Telegram sending utility.
- `setup`: interactive installer + preflight + file writers.
- `plugin-global/telegram-relay`: global OpenCode plugin source template.

## Core Behavior Constraints

- Auth source of truth is `telegramUserId` only.
- Username resolution is convenience-only; never use username for auth.
- Admin-only commands: `/allow`, `/deny`, `/list`, `/status`, `/resolve`.
- JSON writes must remain atomic and recoverable.
- Never log tokens/secrets.

## Code Style Rules

### Language & Types

- Write new runtime code in TypeScript.
- Keep `strict` compatibility.
- Prefer type inference, but define types at module boundaries.
- Avoid `any`; if unavoidable, narrow quickly with guards.

### Imports

- Order: Node built-ins, third-party packages, internal modules.
- Use `import type` for type-only symbols.
- Prefer explicit imports; avoid broad wildcard exports.

### Naming

- `camelCase` for variables/functions.
- `PascalCase` for classes/types.
- `UPPER_SNAKE_CASE` for env constants.
- Use descriptive names (`getOrCreateSession`, `isAllowed`).

### Functions and Files

- Keep functions small and single-purpose.
- Keep files focused by domain concern.
- Split files if they start mixing unrelated responsibilities.

### Formatting

- Preserve existing formatting in edited files.
- Do not create formatting-only churn.
- If formatter is introduced later, follow formatter output as source of truth.

### Error Handling

- Fail fast for invalid startup configuration.
- Do not swallow errors silently.
- Return safe user-facing error messages in Telegram responses.
- Include technical detail in internal errors, excluding secrets.

### Data Safety

- Validate parsed JSON before use.
- Keep backup/recovery behavior in storage layer.
- Avoid breaking persisted schema without migration notes.

## Plugin-Specific Notes

- Plugin target install path is global: `~/.config/opencode/plugin/telegram-relay/`.
- Installer writes `config.json` for plugin runtime (`dataDir`, `botToken`).
- Event relay uses `message.updated` cache + `session.idle` dispatch flow.
- Do not hardcode machine-specific paths in plugin logic.

## Agent Workflow Expectations

1. Read relevant files before editing (`src/...` + `IMPLEMENTATION_PLAN.md`).
2. Make minimal targeted changes.
3. Run `npm run typecheck` after meaningful edits.
4. Run `npm run build` for integration checks.
5. Run tests if you add tests or modify test-covered behavior.
6. Update `README.md` or this file when commands/flows change.

## Security Checklist for Changes

- No secret values committed.
- Auth path still keyed by numeric Telegram IDs.
- Admin-only commands still gated.
- Logs do not include bot token or sensitive payloads.
- External command execution remains bounded by timeout.
