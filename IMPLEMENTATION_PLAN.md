# Implementation Plan (Archived)

This document is kept only as historical context from the previous Node/TypeScript MVP.

The active implementation and architecture are Go-first and documented in:

- `GO_MIGRATION_PLAN.md`
- `AGENTS.md`

Current runtime components:

- `go-bridge` (Telegram/OpenCode bridge, SQLite, control API)
- `macos/opencode-bot` (Swift tray app)

Node/TypeScript runtime and plugin-based relay are no longer used.
