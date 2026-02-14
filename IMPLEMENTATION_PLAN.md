# Telegram + OpenCode Integration Plan (MVP JSON)

## Objective

Build a Telegram bot that:

- Authorizes users strictly by `telegramUserId`.
- Receives instructions from authorized users and forwards them to OpenCode.
- Sends OpenCode's final response back to Telegram when the session is idle.
- Includes an interactive installer that generates configuration and installs a global OpenCode plugin.

## Decisions Locked

- `userId` is the source of truth for auth.
- Plugin scope is global.
- Persistence is JSON (MVP).

## High-Level Architecture

1. `telegram-bridge` service
   - Handles Telegram updates and admin commands.
   - Validates authorization by `from.id`.
   - Links Telegram chats/users to OpenCode sessions.
   - Sends user prompts to OpenCode.

2. OpenCode global plugin: `telegram-relay`
   - Installed in `~/.config/opencode/plugin/telegram-relay/`.
   - Listens to OpenCode events (`message.updated`, `session.idle`).
   - Stores latest useful assistant text per session.
   - On idle, forwards latest message to Telegram chat/user mapped to that session.

3. Interactive installer (`setup`)
   - Collects required values via prompts.
   - Validates Telegram token with `getMe`.
   - Creates `.env` + JSON data files.
   - Installs/configures global plugin.
   - Runs preflight checks.

## Proposed Project Structure

```
.
├── src/
│   ├── bot/
│   │   ├── index.ts
│   │   ├── middleware.ts
│   │   └── commands.ts
│   ├── auth/
│   │   └── authz.ts
│   ├── store/
│   │   ├── jsonStore.ts
│   │   ├── files.ts
│   │   └── schemas.ts
│   ├── opencode/
│   │   ├── client.ts
│   │   ├── sessions.ts
│   │   └── queue.ts
│   ├── relay/
│   │   └── telegramSender.ts
│   ├── resolver/
│   │   └── usernameResolver.ts
│   └── main.ts
├── setup/
│   ├── installer.ts
│   ├── prompts.ts
│   ├── preflight.ts
│   └── writers.ts
├── plugin-global/
│   └── telegram-relay/
│       ├── index.ts
│       ├── types.ts
│       ├── state.ts
│       ├── telegram.ts
│       └── hooks/
│           └── event.ts
└── IMPLEMENTATION_PLAN.md
```

## Data Model (JSON)

All files stored under a configurable data directory (default: `./data`).

- `allowed-users.json`
  - `[{ telegramUserId: number, alias?: string, addedBy: number, createdAt: string }]`
- `admins.json`
  - `[{ telegramUserId: number, createdAt: string }]`
- `session-links.json`
  - `[{ telegramChatId: number, telegramUserId: number, opencodeSessionId: string, updatedAt: string }]`
- `last-messages.json`
  - `[{ opencodeSessionId: string, text: string, updatedAt: string }]`
- `username-index.json` (aux only)
  - `[{ username: string, telegramUserId: number, updatedAt: string }]`

## Core Flows

### 1) Incoming Telegram Message

1. Receive update.
2. Check if `from.id` exists in `allowed-users.json`.
3. If unauthorized: deny with short message and log.
4. If authorized:
   - Resolve/create linked OpenCode session.
   - Send prompt to OpenCode.
   - Optionally reply "processing...".

### 2) OpenCode Idle Relay

1. Plugin captures `message.updated` and caches latest useful assistant text by `sessionID`.
2. Plugin receives `session.idle`.
3. Plugin finds Telegram mapping for that `sessionID`.
4. Plugin sends cached response to mapped Telegram chat.
5. Plugin clears temporary per-session relay state.

### 3) Admin Command Flow

- `/allow <userId>`: add authorized user.
- `/deny <userId>`: remove authorized user.
- `/list`: show allowed users/admins summary.
- `/status`: show bridge/plugin health basics.
- `/resolve @username`: best-effort map username to userId for convenience only.

## Username Resolution Strategy

- Username resolution is best-effort and non-authoritative.
- Authorization always uses `telegramUserId` only.
- Resolver may update `username-index.json` when users interact.
- If a username cannot be resolved, admin must use explicit `userId`.

## JSON Reliability Strategy

- Atomic writes (`temp file -> rename`).
- One-writer lock per file in process.
- Schema validation on read.
- Recovery strategy:
  - If parse fails, attempt `.bak` fallback.
  - If no backup, initialize empty valid structure and log error.

## Security Requirements

- Never authenticate by username.
- Restrict admin commands to `admins.json`.
- Keep secrets in `.env` only.
- Avoid logging tokens or sensitive values.
- Sanitize error messages returned to end users.

## Installer Scope and Responsibilities

Installer command (example): `npm run setup`.

Prompts:

1. `BOT_TOKEN` (required)
2. `ADMIN_USER_IDS` (required, comma-separated)
3. Mode: `polling` or `webhook` (default `polling`)
4. Data directory path (default `./data`)
5. OpenCode command/path and timeout

Installer actions:

1. Validate `BOT_TOKEN` with Telegram `getMe`.
2. Create `.env`.
3. Initialize JSON files.
4. Install global plugin files into `~/.config/opencode/plugin/telegram-relay/`.
5. Print instructions to ensure OpenCode loads global plugin if needed.
6. Run preflight checks and report results.

## OpenCode Plugin Design (Global)

Plugin file target:

- `~/.config/opencode/plugin/telegram-relay/index.ts`

Plugin behavior:

- `event` hook:
  - On `message.updated`, update last assistant text by session.
  - On `session.idle`, dispatch Telegram send via bot API.

Implementation notes:

- Keep plugin modular (`index.ts`, `state.ts`, `hooks/event.ts`, `telegram.ts`, `types.ts`).
- Keep files small and focused.
- Read shared JSON files safely and defensively.

## Validation Checklist

1. Authorized user sends prompt -> OpenCode receives.
2. Session completes -> Telegram receives final response on idle.
3. Unauthorized user is denied.
4. Admin commands work (`allow`, `deny`, `list`, `status`).
5. Process restart retains state from JSON.
6. Installer can bootstrap from empty environment.

## MVP Defaults

- Runtime: Node.js + TypeScript.
- Telegram library: `grammy`.
- Transport default: polling.
- Global plugin path: `~/.config/opencode/plugin/telegram-relay/`.

## Out-of-Scope for MVP

- Billing/quotas.
- Multi-tenant isolation.
- MTProto user-account based lookup.
- Rich markdown formatting guarantees beyond basic escaping.

## Next Implementation Steps

1. Scaffold Node/TS project with scripts.
2. Implement JSON store layer + schemas.
3. Implement Telegram auth/admin handlers.
4. Implement OpenCode session bridge.
5. Implement global plugin relay hooks.
6. Implement interactive installer + preflight.
7. Run end-to-end manual test and document runbook.
