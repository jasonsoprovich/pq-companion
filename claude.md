# PQ Companion — Claude Code Project Instructions

## Project Overview
PQ Companion is a desktop companion app for the EverQuest emulated server
"Project Quarm." It combines a database explorer, combat log parser, DPS
meter, spell/buff/DOT timer overlays, NPC info overlay, spell checklist,
config backup manager, and custom trigger system into a single application.

## Architecture
- **Go backend** (`backend/`): API server, log parser, timer engine, database
  queries, file watching, all core logic. Runs as a sidecar process.
- **Electron shell** (`electron/`): Desktop window management, overlay
  windows (transparent, always-on-top, click-through), sidecar lifecycle.
- **React frontend** (`frontend/`): All UI rendered in React with TypeScript
  and Tailwind CSS. Communicates with Go backend via REST API + WebSocket.
- **SQLite database** (`backend/data/quarm.db`): EverQuest game data
  converted from MySQL dumps. Read-only.
- **User database** (`~/.pq-companion/user.db`): User settings, triggers,
  backup history. Read-write.

## Tech Stack
- Go 1.22+ with `modernc.org/sqlite`, `go-chi/chi`, `gorilla/websocket`
- Node.js 20+ with Electron, React 18, TypeScript, Vite, Tailwind CSS
- SQLite for all data storage (no external DB dependency for end users)
- electron-builder for packaging, electron-updater for auto-updates

## Key Conventions

### Go Backend
- Use standard library where possible, minimize dependencies
- All database queries in `internal/db/queries.go`
- All models in `internal/db/models.go` with JSON struct tags
- API handlers in `internal/api/` — one file per resource
- Use structured logging (slog)
- Error handling: wrap errors with context, never panic in library code
- Tests: table-driven tests, test against real SQLite DB with test fixtures

### Frontend (React + TypeScript)
- Functional components only, hooks for state management
- Use shadcn/ui components where applicable
- WebSocket hook in `hooks/useWebSocket.ts` — singleton connection
- API client in `services/api.ts` — typed fetch wrappers
- Types mirror Go structs in `types/`
- Dark theme only (for now)
- Tailwind for all styling, no CSS modules

### Electron
- Main process is thin — just window management and sidecar lifecycle
- All business logic lives in Go, not in Electron main process
- Overlay windows: `transparent: true`, `alwaysOnTop: true`, frameless
- IPC only for window management commands (show/hide/resize overlays)

### Database
- Never modify the EQ game database at runtime
- User data goes in separate user.db
- All queries must use parameterized statements (no string concatenation)
- Add indexes for any column used in WHERE clauses or JOINs

### General
- Format Go with `gofmt`
- Format TypeScript with Prettier (80 char width)
- Commit messages: conventional commits (feat:, fix:, docs:, etc.)
- Test every feature before moving to the next phase
- When adding a new feature, update FEATURES.md and PROGRESS.md

## EverQuest-Specific Knowledge

### Log File Format
- Located at: `<EQ_DIR>/Logs/eqlog_<CharName>_pq.proj.txt`
- Each line: `[Mon Apr 13 06:00:00 2026] <message>`
- Combat: "You slash a gnoll for 150 points of damage."
- Spell cast: "You begin casting Mesmerization."
- Spell land: "A gnoll has been mesmerized."
- Spell resist: "Your target resisted the Mesmerization spell."
- Spell worn off: "Your Mesmerization spell has worn off."
- Target: not directly logged — infer from combat/spell context
- Zone: "You have entered The North Karana."

### NPC Special Abilities
The `special_abilities` field in `npc_types` is a pipe-delimited string.
Key codes to parse:
- 1 = Summon, 2 = Enrage, 3 = Rampage, 4 = Flurry
- 5 = Triple Attack, 6 = Dual Wield
- 12 = Immune to Melee, 13 = Immune to Magic
- 17 = Uncharmable, 18 = Unmezzable, 19 = Unfearable
- 20 = Immune to Slow, 24 = No Target
- 26 = See Through Invis, 28 = See Through Invis vs Undead
Full parsing logic should be in `internal/db/special_abilities.go`

### Zeal Integration
- Zeal exports inventory/spellbook as files in the EQ directory on logout
- ZealPipes provides real-time data via Windows named pipes (future feature)
- Zeal log extensions add extra info to the standard EQ log format

## Current Phase
Phase 0 — Database Setup & Exploration

## File: PROGRESS.md
Update this file after completing each task. Format:
```
## Phase 0 — Database Setup & Exploration
- [x] Task 0.1 — Docker MySQL Setup
- [ ] Task 0.2 — Database Exploration & Documentation
- [ ] Task 0.3 — Go MySQL→SQLite Converter
- [ ] Task 0.4 — Go Database Layer
```
