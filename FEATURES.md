# PQ Companion — Features

## Phase 0 — Database Setup & Exploration ✅
- MySQL 8 Docker environment for EQEmu dump exploration
- Go CLI tool (`dbconvert`): MySQL dump → SQLite converter
  - `--from-dump` mode: parses `.sql` dump files directly, no MySQL required
  - `--from-mysql` mode: reads from a live MySQL connection
  - Handles all MySQL→SQLite type mapping, index conversion, and data migration
  - Converts ~1.1 million rows in under 60 seconds
- Documented schema for all key tables (items, spells, NPCs, zones, loot, spawns)
- Go database layer (`internal/db`): typed read-only access to quarm.db
  - `Get` and `Search` functions for items, spells, NPCs, and zones
  - Paginated search results with total count
  - `ParseSpecialAbilities`: parses NPC caret-delimited special ability strings
  - All queries use parameterized statements; tested against real quarm.db

## Phase 1 — Go Backend API
- REST API: items, spells, NPCs, zones with search and filtering
- WebSocket server for real-time event broadcasting to all connected clients
- YAML configuration system (EQ install path, active character, preferences)

## Phase 2 — Database Explorer (Frontend)
- **Item Explorer**: search by name/slot/class/stat, detail panel with all item fields
- **Spell Explorer**: search by name/class/level, duration and resist type display
- **NPC Explorer**: search by name/zone, special ability parsing (summon, mez-immune, etc.), loot table view
- **Zone Explorer**: browse zones, list resident NPCs
- **Global Search**: cross-database search via `Ctrl+K` / `Cmd+K` keyboard shortcut

## Phase 3 — Zeal Integration & Backup Manager
- Parse Zeal inventory export files (on logout) to track carried items
- Spell checklist: compare spellbook export against all available class spells by level
- INI/config file watcher: automatic versioned backups when EQ config files change
- Backup timeline UI: browse and restore any previous config version

## Phase 4 — Log Parsing & NPC Info Overlay
- Real-time EQ log file tailer (reads new lines as they appear)
- Combat, spell, zone, and chat event parsing from log format
- Event broadcast to all WebSocket clients
- NPC info overlay: transparent always-on-top window showing current target's stats and special abilities

## Phase 5 — Combat Tracking & DPS Meter
- Per-entity damage tracking (you, pet, group members, NPCs)
- DPS calculations: current fight, rolling average, session total
- Live DPS overlay: transparent always-on-top meter with group breakdown
- Combat log history: browse past fights with detailed damage breakdowns

## Phase 6 — Spell Timer Engine
- Countdown tracking for mez, stuns, DoTs, buffs
- Server-tick-aware duration calculations
- Timer overlay: color-coded bars grouped by type (mez / DoT / buff / debuff)
- Buff window enhancement: self-buff tracking with exact remaining durations

## Phase 7 — Audio Alerts
- System audio integration via Web Audio API
- Configurable alerts when timers expire (sound file or TTS)
- TTS notifications for game events (tells, death, zone messages)
- Per-trigger volume and voice settings

## Phase 8 — Custom Trigger System
- Regex-based trigger engine: match log lines → fire actions
- Actions: play sound, speak TTS, display overlay text, log to history
- Trigger Manager UI: create/edit/delete triggers, import/export packs
- Pre-built trigger packs (enchanter mez breaks, resist spam, named spawns)
- Text overlay window for trigger output display

## Phase 9 — Build & Distribution
- Windows `.exe` installer via electron-builder + GitHub Actions CI
- Auto-updater: silent background updates via electron-updater + GitHub Releases
- Optional hosted web API on Cloudflare Workers (same Go API, cloud DB)
- Project website on Cloudflare Pages
