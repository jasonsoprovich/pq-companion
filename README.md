# PQ Companion

A desktop companion app for [Project Quarm](https://www.projectquarm.com/) — the EverQuest classic emulated server. It sits alongside the game and gives you the tools you wish were built into the client.

> **Status:** Feature-complete and in active maintenance. See [FEATURES.md](FEATURES.md) for the full list of what's implemented.

**🌐 Website: [pq-companion.com](https://pq-companion.com)** — the best place to start: feature tour, screenshots, setup walkthrough, quick-start guide, and the full changelog.

Join the community on [Discord](https://discord.gg/Srj4FXcRaz) for help, feedback, and release announcements.

---

## What It Does

- **Database Explorer** — Search every item, spell, NPC, and zone in the Project Quarm database. Full stat panels, resist info, class restrictions, spell effects, NPC special abilities, and zone NPC rosters.
- **Global Search** — Press `Cmd+K` / `Ctrl+K` to search items, spells, NPCs, and zones simultaneously from anywhere in the app.
- **Recipe Browser** — Browse tradeskill recipes in the database section, and on any item see every recipe it's an ingredient in via the Tradeskills tab. Pin frequently-used combines as favorites.
- **Quests** — Self-contained quest walkthroughs built into the app: a Quests section in the database explorer, a Quests tab on any item showing its rewards and turn-ins, and full quest-chain to-do lists for multi-step keys.
- **NPC Info Overlay** — See your current target's level, class, HP, resists, and special abilities (Summon, Unmezzable, Uncharmable, etc.) the moment you engage — pulled from the database automatically via your combat log. With Zeal connected, the overlay also shows a live HP bar and a pet-owner badge, plus an on-demand loot table with drop rates.
- **DPS Meter** — A transparent overlay showing live damage output for you and your group, with fight duration, session totals, and per-class bar colors. Floats above the game as a standalone window.
- **Combat Log** — Persistent fight history with Personal / Raid / Encounter DPS metrics, expandable per-combatant breakdowns, date-range filters, and event-based session grouping.
- **Roll Tracker** — Parses `/random` rolls into sessions, with an optional timer that auto-stops collection, a dashboard panel, and a pop-out overlay window.
- **Spell Timer Engine** — Countdown bars for every buff, debuff, mez, stun, and DoT. Tick-accurate durations, with Zeal-pipe corroboration when available. Separate overlay windows for buffs and detrimentals.
- **NPC Respawn Timers** — A transparent overlay that tracks NPC deaths and counts down to respawn, with Project Quarm's fast-respawn reduction applied automatically. Optional audio or text-to-speech alerts fire when a respawn is ready, scoped to your current zone.
- **CH Chain Overlay** — Tracks raid Complete Heal chains in real time from heal callouts: cast-to-land bars, live measured cadence, and a stall indicator, plus a personal CH metronome overlay that paces your slot. Both float as standalone dashboard panels.
- **Loot Tracker** — A dedicated page that logs drops as they happen, with clickable items (full detail popup) and zones (jump straight to the Zone browser).
- **Chat History** — A multi-channel log of your tells and conversations, with per-character tabs and chat-style threads so you can scroll back through who said what.
- **Log Feed** — Real-time feed of every parsed combat, spell, and zone event from your EQ log, plus a read-only Browse mode for reading logs out of game and a Replay mode that plays historical log segments back through the live pipeline to test triggers and overlays.
- **Spell Checklist** — See every spell your class can learn, cross-referenced against your Zeal spellbook, so you always know what you're missing — with a "Where to get it" button on each spell that surfaces where it can be obtained, a name-search filter, and a shopping-route planner that maps an efficient vendor run for the spells you select.
- **Spellsets** — Read and edit your Zeal `_spellsets.ini` exports inside the app. Add and remove sets, rename inline, and import another character's spellsets — with off-class and ineligible-spell blocking so you never load something you can't cast.
- **Inventory Tracker** — All items across all your characters in one searchable view, including bank and shared bank, with a dedicated Rechargeable Items section and remaining charge counts on limited-charge clickies.
- **Key Tracker** — Tracks item components for major raid keys (Veeshan's Peak, Old Sebilis, Howling Stones, and more) across all your characters.
- **Keyring** — Live per-character `/keys` snapshot, with an inventory fallback for keys that aren't in the keyring yet (Plane of Time, Vex Thal, Grieg's End).
- **Gear Upgrade Finder** — A per-character, per-slot upgrade scanner that ranks gear with cap-aware stat scoring (it knows when a stat is already maxed vs. still scaling), editable per-class weights, an all-slots overview, weapon/ATK/haste scoring, and priority focus effects. Star upgrades straight into your wishlist.
- **Wishlist** — A per-character wishlist with drag-to-reorder. Star any item from the database — or from the Gear Upgrade Finder — to add or remove it.
- **Players Tracker** — A searchable database of every player you've seen via `/who` and `/guildstat`: name, class, guild, zone, level history, first-seen / last-seen. Sortable, filterable by guild and class, across all your characters. Add per-player notes and a PVP flag, with a sound + on-screen warning when a PVP-flagged player shows up in `/who`, plus automatic tracking of tells and group joins.
- **Lockouts Tracker** — Live `/sll` countdowns for loot and legacy lockouts, tracked per character so you always know when an instance or flag is available again.
- **Config Backup Manager** — Snapshot and restore all your EQ `.ini` config files with one click.
- **App Backup & Restore** — Export your full app state (settings, triggers, trigger packs) as a single bundle and restore it on another machine or after a reinstall.
- **Custom Triggers** — A GINA-style regex trigger engine. Each trigger can hold multiple regex patterns (with per-row toggles) and use `{c}`/`{target}` tokens for GINA pattern compatibility; matches fire configurable on-screen text and audio alerts, and can capture a target's name into the alert and its timer. Organize triggers into custom categories with drag-and-drop reordering, and customize the alert text style (color, glow, font, size). Ships with seven built-in community packs including class crowd-control break alerts. Import and export trigger packs as JSON. Live history feed and a standalone transparent overlay window.
- **Custom Timers** — Manual countdown timers with their own dedicated overlay, including durations pulled from trigger capture groups.
- **Zeal Pipes Integration** — When Zeal is running, the app talks to it over Windows named pipes for real-time target, pet, cast, and buff data — no waiting on log lines or file exports.
- **Settings** — Point the app at your EQ folder and character name; everything else is automatic, and settings save as you change them. Includes app-wide zoom and a high-contrast text mode for readability, per-overlay lock-behaviour controls with a "Display only" HUD mode and a global Position-overlays / Manage-overlays workflow, a Navigation tab to hide and reorder sidebar tabs, unified Log Backfill with status diagnostics, and an About tab with project links.

---

## Download & Install

1. Go to the [Releases page](../../releases) and download the latest installer:
   - **Windows** — `PQ-Companion-Setup-x.x.x.exe` (NSIS installer)

2. Run the installer and launch **PQ Companion**.

3. Open **Settings** (bottom of the sidebar) and set:
   - **EverQuest Path** — the folder where EverQuest is installed (e.g. `C:\EverQuest`)
   - **Character Name** — your character's name exactly as it appears in-game

4. That's it. The app will find your log file and Zeal exports automatically.

> **Note:** PQ Companion includes everything it needs. No Go, Node.js, or Docker required to run the app.

---

## Requirements

- Windows 10/11
- [Zeal](https://github.com/iamclint/Zeal) installed (recommended) — enables the Spell Checklist, Inventory Tracker, and Key Tracker
- EverQuest log file enabled — in EQ, type `/log on` to start writing the combat log

---

## Getting Started with Overlays

The overlay windows (NPC Info, DPS Meter, Spell Timers) float above the game window as transparent, click-through panels.

1. Make sure **Parse Combat Log** is enabled in Settings.
2. Make sure your EQ log file is active (`/log on` in-game).
3. Navigate to the overlay tab in the app (e.g. **DPS Overlay**, **Spell Timers**).
4. Click the pop-out button (⤢) to launch the overlay as a standalone window above the game.

The app connects to your live EQ log file. As you play, overlays update in real time — no manual refresh needed.

---

## Auto-Updates

PQ Companion updates itself automatically. When a new version is available it downloads in the background and prompts you to restart. No action needed other than approving the restart.

---

## For Developers

If you want to build from source or contribute, see the [developer setup guide](docs/01_stack.md). [FEATURES.md](FEATURES.md) documents what's implemented under the hood.

Quick start:

```bash
# Terminal 1 — Go backend
cd backend
go run ./cmd/server

# Terminal 2 — Electron + React frontend
npm run dev
```

Requires Go 1.22+ and Node.js 20+. The Vite dev server starts on port 5173; Electron opens pointing at it. The Go backend runs on port 8080.

See [FEATURES.md](FEATURES.md) for detailed implementation notes on every shipped feature.

---

## Donate

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/jasonsoprovich)

---

## Credits

Created by Osui \<Seekers of Souls\> on Project Quarm

---

## License

*Built by players, for players.*
