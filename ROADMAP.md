# PQ Companion — Roadmap

PQ Companion is a free, open-source desktop app built specifically for [Project Quarm](https://www.projectquarm.com/) — the EverQuest classic emulated server. It lives alongside the game and gives you the tools you wish you had built into the client.

---

## What's Done

### Foundation (Phase 0) ✅
The database engine is complete. All 160+ EverQuest game data tables — items, spells, NPCs, zones, loot tables, spawn points, skill caps, and more — have been converted from the original EQEmu MySQL format into a fast, portable SQLite database that ships inside the app. No installation, no server, no dependencies. One file, everything included.

> ~1.1 million rows of game data, fully indexed and queryable in under 60 seconds of build time.

The Go database layer is also complete. The backend can query items, spells, NPCs, and zones by ID or name search, with pagination. NPC special ability strings (summon, mez-immune, uncharmable, etc.) are fully parsed into structured data, ready for the API and overlay features.

---

## What's Coming

### Database Explorer (Next Up)
Browse the entire EverQuest game database from your desktop, without opening a browser or maintaining a server connection.

- **Item Explorer** — search every item by name, slot, class, or stat. Click any result for a full detail panel covering damage, delay, AC, resists, effects, and flags.
- **Spell Explorer** — look up any spell by name, class, or level. See duration, mana cost, resist type, and full effect descriptions.
- **NPC Explorer** — find any NPC by name or zone. See HP, level range, special abilities (summons, mez-immune, uncharmable), and what they drop.
- **Zone Explorer** — browse all zones, see their residents and spawn points on a clean layout.
- **Global Search** — hit `Cmd+K` / `Ctrl+K` from anywhere in the app to search items, spells, and NPCs simultaneously.

### Zeal Integration & Config Backup Manager
For players using [Zeal](https://github.com/iamclint/Zeal), the app will automatically read your inventory and spellbook exports on logout, letting you:

- See your full inventory from outside the game.
- Track which spells your class can learn vs. which ones you already have — the spell checklist.
- Automatically back up your EQ config files (`.ini`, keymaps, UI layouts) every time they change, with a full version history you can restore from instantly.

### Log Parser & NPC Info Overlay
The app will watch your EQ log file in real time and parse every combat, spell, zone, and chat event as it happens. The first overlay built on this will be an **NPC Info** window — a small transparent panel that appears over the game showing your current target's stats, immunities, and special abilities pulled from the database the moment you click on them.

### DPS Meter
A clean, transparent overlay showing live damage output for you, your pet, and your group. Tracks the current fight, rolling DPS, and session totals. Everything stays out of the game window until you need it.

### Spell Timer Engine
Countdown bars for every mez, stun, DoT, and debuff you have active — aware of EverQuest's server tick timing so durations are accurate. Color-coded by type, configurable layout. Never wonder how long your mez has left again.

### Audio Alerts
Configurable sound and text-to-speech alerts tied to any timer or game event. Hear when your mez is about to break. Get a voice alert when you receive a tell. Works with any audio file or your system's built-in TTS.

### Custom Trigger System
A full GINA-style trigger engine, built from scratch for Project Quarm. Write regex patterns against the log, fire any combination of sound, TTS, and on-screen text. Import and export trigger packs. Ships with a pre-built pack for enchanters and common group scenarios.

### Windows Build & Auto-Updater
One-click installer for Windows, distributed via GitHub Releases. Silent background updates so you're always on the latest version without thinking about it.

---

## Guiding Principles

- **No subscription, no account, no server.** Everything runs locally on your machine.
- **Lightweight.** The Go backend idles at a few MB of RAM. Overlays are transparent and click-through.
- **Project Quarm specific.** Features are designed around the Quarm ruleset and Zeal, not generic EQEmu.
- **Open source.** Fork it, extend it, contribute.

---

*Built by players, for players.*
