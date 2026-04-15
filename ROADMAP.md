# PQ Companion — Roadmap

PQ Companion is a free, open-source desktop app built specifically for [Project Quarm](https://www.projectquarm.com/) — the EverQuest classic emulated server. It lives alongside the game and gives you the tools you wish you had built into the client.

---

## What's Done

### Foundation (Phase 0) ✅
The database engine is complete. All 160+ EverQuest game data tables — items, spells, NPCs, zones, loot tables, spawn points, skill caps, and more — have been converted from the original EQEmu MySQL format into a fast, portable SQLite database that ships inside the app. No installation, no server, no dependencies. One file, everything included.

> ~1.1 million rows of game data, fully indexed and queryable in under 60 seconds of build time.

The Go database layer is also complete. The backend can query items, spells, NPCs, and zones by ID or name search, with pagination. NPC special ability strings (summon, mez-immune, uncharmable, etc.) are fully parsed into structured data, ready for the API and overlay features.

### REST API (Phase 1, Task 1.1) ✅
The Go HTTP API server is running. Items, spells, NPCs, and zones are all queryable via REST — search by name with pagination, or fetch any record directly by ID. Zones can also be looked up by their short name (e.g. `qeynos`, `crushbone`). The server is built on chi, uses structured logging, and returns clean JSON errors for 404s and bad input. Run it from `backend/` with `go run ./cmd/server`.

### WebSocket Server (Phase 1, Task 1.2) ✅
The real-time event pipeline is wired up. Connect to `ws://localhost:8080/ws` and you'll receive a stream of JSON events as the backend emits them. The hub accepts any number of concurrent clients, handles slow consumers gracefully, and keeps connections alive with ping/pong. The `ws.Event` envelope (`type` + `data`) is the contract all future features — log parser, spell timers, DPS meter — will broadcast over.

### Configuration System (Phase 1, Task 1.3) ✅
The backend now reads and writes a YAML config file at `~/.pq-companion/config.yaml`. On first run the file is created with sensible defaults (listen address, overlay opacity, combat log parsing). `GET /api/config` and `PUT /api/config` let the frontend read and update settings at runtime. The CLI `--addr` flag still overrides the config-file address for development convenience.

---

## What's Coming

### Electron + React Shell (Phase 2, Task 2.1) ✅
The desktop app is scaffolded and running. The Electron main process manages the window lifecycle, forces dark mode at the OS level, and handles the Go backend sidecar — spawning it on launch in production and killing it cleanly on quit. A preload script exposes a typed, contextBridge-safe API to the renderer. The renderer is a React 18 + Vite + TypeScript app styled with Tailwind CSS v4, using an EQ-themed dark color palette (deep blacks, gold accents). electron-builder is configured for macOS DMG (x64 + arm64) and Windows NSIS installer, with the Go binary bundled as a sidecar in the app resources.

In dev, run `go run ./cmd/server` in one terminal and `npm run dev` in another — Electron opens pointing at the Vite dev server with HMR.

### App Layout & Navigation (Phase 2, Task 2.2) ✅
The app shell is fully wired. React Router runs inside a `HashRouter` (Electron-compatible). Every screen lives under a shared `Layout` component: a slim title bar at the top, a 192px sidebar on the left, and the main content area filling the rest. The title bar is a full-width drag region — on macOS it clears the native traffic-light buttons; on Windows and Linux it renders custom Minimize / Maximize / Close controls. The sidebar lists Items, Spells, NPCs, and Zones under a "Database" header, with Settings pinned at the bottom. Active routes are highlighted in EQ gold. All placeholder pages are in place, ready to be filled in by the Database Explorer tasks.

### Item Explorer (Phase 2, Task 2.3) ✅
Browse all ~70,000 EverQuest items from the database. Type in the search box and results update as you type — the left pane shows names filtered by name with item type and level requirement inline. Click any row for the full detail panel: combat stats (DMG/DLY, AC), all stat bonuses (HP/Mana/STR/STA/…), resist values, spell effects (click/proc/worn/focus), slot and class/race restrictions, weight, price in pp/gp/sp/cp, and item flags (MAGIC, LORE, NO DROP, NO RENT).

### Spell Explorer (Phase 2, Task 2.4) ✅
Browse all EverQuest spells from the database. Search by name — results show the castable classes with their required levels and mana cost. Click any spell for the full detail panel: mana cost and cast/recast/recovery times, duration (tick-accurate, distinguishes fixed vs. level-scaling), target type, resist type, range and AoE range, every class that can cast it (with required level), active effect slots with base values, and flavor text (cast messages, fade messages). Discipline, suspendable, and no-dispell flags rendered as badge pills.

### NPC Explorer (Phase 2, Task 2.5) ✅
Browse every NPC in the Project Quarm database. Search by name — results show level and class. Click any NPC for the full detail panel: HP, mana, damage range, attack count and AC; STR/STA/DEX/AGI/INT/WIS/CHA attributes; Magic/Cold/Disease/Fire/Poison resists; special abilities parsed from the raw caret-delimited string and displayed as pill badges (Summon, Enrage, Rampage, Flurry, Unmezzable, Uncharmable, Unfearable, Immune to Slow, and more); behavior stats (aggro radius, run speed, size); and linked IDs (loot table, merchant, spells, faction). RAID TARGET and RARE SPAWN flags shown as badges.

### Zone Explorer (Phase 2, Task 2.6) ✅
Browse all ~300 EverQuest zones. Search by name — results show the zone's short name and minimum level. Click any zone for its detail panel: zone ID, min level, safe spawn coordinates, and the full resident NPC list. NPCs are loaded on demand by walking the spawn chain (`spawn2 → spawnentry → npc_types`, plus direct solo-spawn entries), deduplicated, and sorted by name. Each NPC row shows display name, class, level, and HP.

### Global Search (Phase 2, Task 2.7) ✅
Hit `Cmd+K` (macOS) or `Ctrl+K` (Windows/Linux) from anywhere in the app to open the Global Search palette. Type any name and get results from all four databases simultaneously — items, spells, NPCs, and zones — in a single grouped list. Arrow-key navigation, Enter to jump to the result's explorer page with the detail panel pre-populated, Escape to dismiss. A `⌘K` shortcut hint is shown in the sidebar for discoverability. Phase 2 is now complete.

### Zeal Export Reader (Phase 3, Task 3.1) ✅
The app now reads Zeal's inventory and spellbook exports automatically. When you log out of EverQuest with Zeal installed, it writes your inventory to `<CharName>_pq.proj-Inventory.txt` and your spellbook to `<CharName>_pq.proj-Spells.txt` in the EQ directory. The backend watches those files and re-parses them within 5 seconds of any change, broadcasting updates to all connected clients via WebSocket.

### Spell Checklist (Phase 3, Task 3.2) ✅
The **Spell Checklist** page (Zeal section in the sidebar) shows every spell your class can learn, cross-referenced against your Zeal spellbook export. Pick your class from the dropdown — Warrior through Beastlord — and see spells listed in level order with a gold checkmark for spells already in your spellbook and an empty circle for spells you don't have yet. Filter to **Known** or **Missing** to focus on what matters. Click any spell to jump to its full detail in the Spell Explorer. Your class selection is remembered between sessions.

### Inventory Tracker (Phase 3, Task 3.3) ✅
The **Inventory Tracker** page shows every item across all of your characters at once. Zeal exports are scanned from the EQ directory automatically — one tab per character, plus an **All** view. Search by item name to find anything across all bags and bank slots. Items are grouped by Equipped, Bags, Bank, and Shared Bank (shared across all characters). In the All view, a character badge appears on each item row so you always know who has what. Click the lookup icon on any row to jump to the Item Explorer.

### Key Tracker (Phase 3, Task 3.4) ✅
The **Key Tracker** page cross-references your characters' inventories against the item components required for the major Planes of Power keying quests. Six keys are currently tracked: Veeshan's Peak, Old Sebilis, Howling Stones (Charasis), Grieg's End, Grimling Forest Shackle Pens, and Katta Castellum. Expand any key card to see a component table — one column per character — with green checkmarks for items in inventory, a gold **SB** badge if the item is in the Shared Bank, and empty circles for missing pieces. Filter by All / In Progress / Complete.

### Windows Build & Distribution (Phase 6) ✅
The app ships as a one-click Windows NSIS installer distributed via GitHub Releases. The Go binary is bundled as a sidecar inside the Electron app package — no separate installation required. Silent background updates via electron-updater keep users on the latest version automatically.

### Config Backup Manager (Phase 3, Tasks 3.5–3.6) ✅
The **Config Backup Manager** (Zeal section in the sidebar) lets you snapshot and restore all of your EverQuest `.ini` configuration files — `eqclient.ini`, per-character UI settings, hotkey files, and anything else in your EQ directory with a `.ini` extension. Create a named backup in one click; every backup shows its file count, size, and creation timestamp. Restore any backup with a single click and an inline confirmation to prevent accidents. Backups are stored as zip archives in `~/.pq-companion/backups/` and their metadata is kept in `user.db`. Phase 3 is now complete.

### Log Parser & NPC Info Overlay (Phase 4)
The app will watch your EQ log file in real time and parse every combat, spell, zone, and chat event as it happens. The first overlay built on this will be an **NPC Info** window — a small transparent panel that appears over the game showing your current target's stats, immunities, and special abilities pulled from the database the moment you click on them.

### DPS Meter
A clean, transparent overlay showing live damage output for you, your pet, and your group. Tracks the current fight, rolling DPS, and session totals. Everything stays out of the game window until you need it.

### Spell Timer Engine (Phase 7)
Countdown bars for every mez, stun, DoT, and debuff you have active — aware of EverQuest's server tick timing so durations are accurate. Color-coded by type, configurable layout. Never wonder how long your mez has left again.

### Audio Alerts (Phase 8)
Configurable sound and text-to-speech alerts tied to any timer or game event. Hear when your mez is about to break. Get a voice alert when you receive a tell. Works with any audio file or your system's built-in TTS.

### Custom Trigger System (Phase 9)
A full GINA-style trigger engine, built from scratch for Project Quarm. Write regex patterns against the log, fire any combination of sound, TTS, and on-screen text. Import and export trigger packs. Ships with a pre-built pack for enchanters and common group scenarios.

### Planes of Power Flag Tracker (Phase 10, Task 10.1)
A per-character checklist for tracking Planes of Power progression flags — the access requirements for end-game zones like Plane of Time. Each character gets its own flag sheet organized by plane, with checkboxes you can tick off manually as you earn them. Designed to be filled in by hand for now (Zeal does not yet expose flag state data), but built in a way that can be wired to automatic detection later if that support is added. Think of it as a living copy of your PoP flag sheet — one tab per character, easy to scan at a glance.

### Character Todo List (Phase 10, Task 10.2)
A simple, per-character task list so you can track anything you want to get done on each of your characters — quests to complete, items to farm, skills to max, people to meet. Add a task, check it off when done, delete it when you're finished. No categories, no due dates, no complexity — just a scratch pad that lives next to your character data and persists between sessions. Kept simple intentionally; additional structure can be added based on feedback once people start using it.

### Project Website (Phase 11)
A public-facing site for the project — feature overview, download links, screenshots, and documentation. Deferred until the app is stable and feature-complete enough to be worth promoting.

### Hosted Web API (Phase 12)
A cloud-hosted version of the backend API so external tools and the project website can query EQ game data without requiring the desktop app. Lowest priority — only relevant once the app has an established user base and the data model is stable.

---

## Guiding Principles

- **No subscription, no account, no server.** Everything runs locally on your machine.
- **Lightweight.** The Go backend idles at a few MB of RAM. Overlays are transparent and click-through.
- **Project Quarm specific.** Features are designed around the Quarm ruleset and Zeal, not generic EQEmu.
- **Open source.** Fork it, extend it, contribute.

---

*Built by players, for players.*
