# PQ Companion — Claude Code Project Instructions

## Project Overview
PQ Companion is a desktop companion app for the EverQuest emulated server
"Project Quarm." The app is feature-complete and currently in a
fine-tuning, bug-fixing, and maintenance phase — most ongoing work is
polish, regressions, and incorporating periodic Project Quarm database
updates.

Capabilities (see `FEATURES.md` for the full implementation log):
- Database explorer (items, spells, NPCs, zones) with global Cmd/Ctrl+K search
- Combat log parser, DPS/HPS meter, and combat history with per-combatant breakdowns
- Spell timer engine with separate buff and detrimental overlays
- NPC info overlay (level, class, HP, resists, special abilities) keyed off the active target
- Spell checklist cross-referenced against the Zeal spellbook export
- Inventory tracker and key tracker (raid key components) across all characters
- Character info pages (stats, AAs, spell modifiers)
- Config backup manager for EQ `.ini` files
- GINA-style custom trigger system with regex patterns, on-screen alerts, audio alerts, and importable trigger packs
- Auto-updating Windows installer

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
- **Marketing site** (separate repo `pq-companion-site/`): Astro + Tailwind
  static site that mirrors the app's theme tokens. Lives outside this repo.

## Tech Stack
- Go 1.22+ with `modernc.org/sqlite`, `go-chi/chi`, `gorilla/websocket`
- Node.js 20+ with Electron 33, React 18, TypeScript, Vite (electron-vite), Tailwind CSS v4
- SQLite for all data storage (no external DB dependency for end users)
- electron-builder for packaging, electron-updater for auto-updates

## Key Conventions

### Go Backend
- Use standard library where possible, minimize dependencies
- Database queries live under `internal/db/` (and feature-specific subpackages where appropriate)
- Models with JSON struct tags
- API handlers in `internal/api/` — one file per resource
- Feature packages under `internal/`: `logparser`, `combat`, `spelltimer`, `trigger`, `buffmod`, `character`, `backup`, `keys`, `overlay`, `zeal`, `converter`, `config`, `ws`
- Use structured logging (slog)
- Error handling: wrap errors with context, never panic in library code
- Tests: table-driven tests, test against real SQLite DB with test fixtures from `testdata/`

### Frontend (React + TypeScript)
- Functional components only, hooks for state management
- Use shadcn/ui components where applicable
- WebSocket hook in `hooks/useWebSocket.ts` — singleton connection
- API client in `services/api.ts` — typed fetch wrappers
- Types mirror Go structs in `types/`
- Dark theme only
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
- `quarm.db` is regenerated from MySQL dumps via the `data-release` GitHub
  Actions workflow — never hand-edit the file

### General
- Format Go with `gofmt`
- Format TypeScript with Prettier (80 char width)
- Commit messages: conventional commits (`feat:`, `fix:`, `docs:`, `chore:`, etc.)
- Test changes locally before committing
- `FEATURES.md` and `README.md` are only updated at release time (when running
  `/newrelease`). For ordinary fixes and tweaks, just commit the code change.
- Per-task progress tracking files (`PROGRESS.md`, `ROADMAP.md`) have been
  removed — the app is past the phase-by-phase build-out. Track ongoing work
  through GitHub issues and the git log instead.

### Branching
- All development, fixes, and testing happen directly on `main`. There is no
  long-lived `develop` branch.
- Short-lived topic branches are fine when useful, but they merge straight
  back into `main` — not into an integration branch.

### Releases
- Run `/newrelease` to bump the version, build the Windows installer, tag,
  and publish a GitHub release. Update `FEATURES.md` and `README.md` as part
  of that flow if user-visible behavior changed.

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
The `special_abilities` field in `npc_types` is a caret-delimited (`^`) string.
Format: `code,value^code,value^...` (e.g., `"1,1^18,1^19,1"`).

Codes match the `SpecialAbility` namespace in Project Quarm's EQMacEmu fork
(`common/emu_constants.h`) — they differ from modern EQEmu master numbering.
Common codes:
- 1 = Summon, 2 = Enrage, 3 = Rampage, 4 = Area Rampage
- 5 = Flurry, 6 = Triple Attack, 7 = Dual Wield
- 9 = Bane Attack, 10 = Magical Attack, 11 = Ranged Attack
- 12 = Immune to Slow, 13 = Mez, 14 = Charm, 15 = Stun
- 16 = Snare, 17 = Fear, 18 = Dispel, 19 = Melee, 20 = Magic
- 31 = Immune to Pacify, 35 = Immune to Harm from Client
- See `SCHEMA.md` for the full code 1–54 table.

See-invis flags are stored on dedicated `npc_types.see_invis` and
`see_invis_undead` columns, not in this string. The label table lives in
`internal/db/special_abilities.go` and is mirrored in
`frontend/src/lib/npcHelpers.ts`.

### Zeal Integration
- Zeal exports inventory/spellbook as files in the EQ directory on logout
- ZealPipes provides real-time data via Windows named pipes (future feature)
- Zeal log extensions add extra info to the standard EQ log format
