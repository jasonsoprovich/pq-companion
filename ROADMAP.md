# PQ Companion — Roadmap

PQ Companion is a free, open-source desktop app built specifically for [Project Quarm](https://www.projectquarm.com/) — the EverQuest classic emulated server. It lives alongside the game and gives you the tools you wish you had built into the client.

---

## What's Done

### Foundation (Phase 0)
The database engine is complete. All 160+ EverQuest game data tables — items, spells, NPCs, zones, loot tables, spawn points, skill caps, and more — have been converted from the original EQEmu MySQL format into a fast, portable SQLite database that ships inside the app. No installation, no server, no dependencies. One file, everything included.

> ~1.1 million rows of game data, fully indexed and queryable in under 60 seconds of build time.

The Go database layer is also complete. The backend can query items, spells, NPCs, and zones by ID or name search, with pagination. NPC special ability strings (summon, mez-immune, uncharmable, etc.) are fully parsed into structured data, ready for the API and overlay features.

### REST API & WebSocket Server (Phase 1)
The Go HTTP API server is running. Items, spells, NPCs, and zones are all queryable via REST — search by name with pagination, or fetch any record directly by ID. Zones can also be looked up by their short name (e.g. `qeynos`, `crushbone`). A real-time WebSocket event pipeline connects the backend to the frontend, broadcasting log events, combat state, spell timers, and NPC target data to all connected clients instantly.

### Configuration System (Phase 1)
The backend reads and writes a YAML config file at `~/.pq-companion/config.yaml`. On first run the file is created with sensible defaults. The Settings page in the app lets you update EQ path, character name, and preferences at runtime without restarting.

### Desktop App Shell (Phase 2)
The desktop app is scaffolded and running. The Electron main process manages the window lifecycle and the Go backend sidecar — spawning it on launch and killing it cleanly on quit. The app shell provides a persistent sidebar for navigation, a custom title bar with drag support, and a React + TypeScript + Tailwind frontend. All overlay windows (transparent, frameless, always-on-top) are managed through Electron IPC.

### Database Explorer (Phase 2)
Browse all EverQuest game data directly in the app:
- **Items** — search ~70,000 items; full detail panel with stats, effects, restrictions, and price
- **Spells** — search all spells; castable classes with required levels, timing, effects, and messages
- **NPCs** — search every NPC; combat stats, resists, attributes, and special abilities
- **Zones** — search all ~300 zones; zone info and full NPC resident list loaded on demand
- **Global Search** — `Cmd+K` / `Ctrl+K` searches all four databases simultaneously

### Zeal Integration & Character Tools (Phase 3)
The app reads Zeal's inventory and spellbook exports automatically whenever they change on disk:
- **Spell Checklist** — every spell your class can learn, cross-referenced against your Zeal spellbook. See what you're missing at a glance.
- **Inventory Tracker** — all items across all characters in one view, searchable by name. Includes bank and shared bank slots.
- **Key Tracker** — tracks the item components required for major raid keys (Veeshan's Peak, Old Sebilis, Howling Stones, and more) across all your characters.
- **Config Backup Manager** — snapshot and restore all EQ `.ini` configuration files. Named backups with timestamps, one-click restore.

### Log Parser & NPC Info Overlay (Phase 4)
The app watches your EQ log file in real time and parses every combat, spell, zone, and chat event as it happens. The **NPC Info overlay** shows your current target's level, class, race, HP, resists, attributes, and special abilities the moment you engage — pulled from the database automatically. A **Log Feed** page shows every parsed event live as it comes in.

### DPS Meter & Combat Log (Phase 5)
- **DPS Overlay** — a transparent overlay showing live damage output for you and everyone in your group. Tracks the current fight, rolling DPS, and session totals. Pop it out as a standalone overlay window that floats above the game.
- **Combat Log** — a full fight history showing every completed encounter with expandable combatant breakdowns, DPS, and damage totals.

### Windows Build & Distribution (Phase 6)
The app ships as a one-click Windows NSIS installer distributed via GitHub Releases. The Go binary is bundled as a sidecar inside the Electron package — no separate installation required. Auto-updates via `electron-updater` keep users on the latest version silently in the background.

### Spell Timer Engine (Phase 7)
Countdown bars for every buff, debuff, mez, stun, and DoT you cast — aware of EverQuest's server tick timing so durations are accurate. Two separate overlay windows: one for beneficial spells (buffs), one for detrimental spells (DoTs, mezzes, stuns, debuffs). Each can be popped out as a standalone always-on-top overlay.

---

## What's Coming

### Custom Trigger System (Phase 8)
A full GINA-style trigger engine, built from scratch for Project Quarm. Write regex patterns against the log, fire any combination of sound, TTS, and on-screen text. Import and export trigger packs. Ships with a pre-built pack for enchanters and common group scenarios.

### Audio Alerts (Phase 9)
Configurable sound and text-to-speech alerts tied to any timer or game event. Hear when your mez is about to break. Get a voice alert when you receive a tell. Works with any audio file or your system's built-in TTS.

### Character Tools (Phase 10)
A simple, per-character task list so you can track anything you want to get done on each of your characters — quests to complete, items to farm, skills to max, people to meet. Add a task, check it off when done. No categories, no due dates, no complexity — just a scratch pad that lives next to your character data and persists between sessions.

### Project Website (Phase 11)
A public-facing site for the project — feature overview, download links, screenshots, and documentation. Deferred until the app is stable and feature-complete enough to be worth promoting.

---

## Future Plans

The following features are on the long-term radar but are not scheduled for a specific phase. They'll be prioritized based on demand and feasibility once the core app is mature.

### Planes of Power Flag Tracker
A per-character checklist for tracking Planes of Power progression flags — the access requirements for end-game zones like Plane of Time. Each character gets its own flag sheet organized by plane, with checkboxes you tick off manually as you earn them. Designed to be filled in by hand for now (Zeal does not yet expose flag state data), but built in a way that can be wired to automatic detection later if that support is added.

### Hosted Web API
A cloud-hosted version of the backend API so external tools and the project website can query EQ game data without requiring the desktop app. Lowest priority — only relevant once the app has an established user base and the data model is stable.

---

## Guiding Principles

- **No subscription, no account, no server.** Everything runs locally on your machine.
- **Lightweight.** The Go backend idles at a few MB of RAM. Overlays are transparent and click-through.
- **Project Quarm specific.** Features are designed around the Quarm ruleset and Zeal, not generic EQEmu.
- **Open source.** Fork it, extend it, contribute.

---

*Built by players, for players.*
