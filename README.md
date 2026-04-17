# PQ Companion

A desktop companion app for [Project Quarm](https://www.projectquarm.com/) — the EverQuest classic emulated server. It sits alongside the game and gives you the tools you wish were built into the client.

> **Status:** Active development. Phases 0–8 complete. See [ROADMAP.md](ROADMAP.md) for what's built and what's coming.

---

## What It Does

- **Database Explorer** — Search every item, spell, NPC, and zone in the Project Quarm database. Full stat panels, resist info, class restrictions, spell effects, NPC special abilities, and zone NPC rosters.
- **Global Search** — Press `Cmd+K` / `Ctrl+K` to search items, spells, NPCs, and zones simultaneously from anywhere in the app.
- **NPC Info Overlay** — See your current target's level, class, HP, resists, and special abilities (Summon, Unmezzable, Uncharmable, etc.) the moment you engage — pulled from the database automatically via your combat log.
- **DPS Meter** — A transparent overlay showing live damage output for you and your group, with fight duration and session totals. Floats above the game as a standalone window.
- **Combat Log** — Full fight history with expandable per-combatant breakdowns.
- **Spell Timer Engine** — Countdown bars for every buff, debuff, mez, stun, and DoT. Tick-accurate durations. Separate overlay windows for buffs and detrimentals.
- **Log Feed** — Real-time feed of every parsed combat, spell, and zone event from your EQ log.
- **Spell Checklist** — See every spell your class can learn, cross-referenced against your Zeal spellbook, so you always know what you're missing.
- **Inventory Tracker** — All items across all your characters in one searchable view, including bank and shared bank.
- **Key Tracker** — Tracks item components for major raid keys (Veeshan's Peak, Old Sebilis, Howling Stones, and more) across all your characters.
- **Config Backup Manager** — Snapshot and restore all your EQ `.ini` config files with one click.
- **Custom Triggers** — A GINA-style regex trigger engine. Write patterns that match any log line and fire configurable on-screen text alerts. Ships with pre-built packs for enchanters (mez breaks, charm breaks, resists) and group awareness (tells, deaths). Import and export trigger packs as JSON. Live history feed and a standalone transparent overlay window.
- **Settings** — Point the app at your EQ folder and character name; everything else is automatic.

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

If you want to build from source or contribute, see the [developer setup guide](docs/01_stack.md) and [PROGRESS.md](PROGRESS.md) for the current implementation status.

Quick start:

```bash
# Terminal 1 — Go backend
cd backend
go run ./cmd/server

# Terminal 2 — Electron + React frontend
npm run dev
```

Requires Go 1.22+ and Node.js 20+. The Vite dev server starts on port 5173; Electron opens pointing at it. The Go backend runs on port 8080.

See [FEATURES.md](FEATURES.md) for detailed implementation notes on every completed task, and [ROADMAP.md](ROADMAP.md) for the full feature plan.

---

## Support

<a href="https://www.buymeacoffee.com/jasonsoprovich"><img src="https://img.buymeacoffee.com/button-api/?text=Buy me a coffee&emoji=&slug=jasonsoprovich&button_colour=FFDD00&font_colour=000000&font_family=Cookie&outline_colour=000000&coffee_colour=ffffff" /></a>

---

## Credits

Created by Osui \<Seekers of Souls\> on Project Quarm

---

## License

*Built by players, for players.*
