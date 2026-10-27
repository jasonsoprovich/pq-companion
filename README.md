# PQ Companion

A desktop companion app for [Project Quarm](https://www.projectquarm.com/) — the EverQuest classic emulated server. It sits alongside the game and gives you the tools you wish were built into the client.

> **Status:** Feature-complete and in active maintenance — recently through a full stability, performance, and security-hardening pass (atomic settings saves, coalesced raid-time meter updates, sandboxed windows, and a batch of correctness fixes). See [FEATURES.md](FEATURES.md) for the full list of what's implemented and the latest changes.

**🌐 Website: [pq-companion.com](https://pq-companion.com)** — the best place to start: feature tour, screenshots, setup walkthrough, quick-start guide, and the full changelog.

Join the community on [Discord](https://discord.gg/Srj4FXcRaz) for help, feedback, and release announcements.

---

## What It Does

- **Database Explorer** — Search every item, spell, NPC, and zone in the Project Quarm database. Full stat panels, resist info, class restrictions, spell effects, NPC special abilities, the zones each NPC spawns in (shown on NPC pages, in search, and on item drop sources), and zone NPC rosters with sortable tabs and each NPC's spawn chance. Compare items side-by-side, or against whatever a character currently has equipped, and filter spells by target type, resist, school and effect.
- **Global Search** — Press `Cmd+K` / `Ctrl+K` to search items, spells, NPCs, and zones simultaneously from anywhere in the app.
- **Recipe Browser** — Browse tradeskill recipes in the database section, and on any item see every recipe it's an ingredient in via the Tradeskills tab. Pin frequently-used combines as favorites, and see at a glance which components you already have versus what's missing. See your success and failure odds on any combine from your skill and crafting gear, and follow a curated **Recommended leveling path** (or build your own **Custom path**) to raise any tradeskill to your target.
- **Quests** — Self-contained quest walkthroughs built into the app: a Quests section in the database explorer, a Quests tab on any item showing its rewards and turn-ins, and full quest-chain to-do lists for multi-step keys.
- **NPC Info Overlay** — See your current target's level, class, HP, resists, swing timer, aggro/assist radius, and special abilities (Summon, Unmezzable, Uncharmable, etc.) the moment you engage — pulled from the database automatically via your combat log. With Zeal connected, the overlay also shows a live HP bar and a pet-owner badge, plus an on-demand loot table with drop rates. With a Zeal build that reports target info (the release after 1.4.7), it shows the spawn's real level, tells duplicate-named NPCs apart (Vex Thal bosses, Shissar Revenant, Plane of Fear) so only the right loot table shows, and adds a range-coloured distance-to-target readout with markers on enemy spells that can reach you. Customize it from its own Settings tab with a live preview: recolor and reorder ability tags and sections, choose what shows on the dashboard vs. the popout, and add outline rules that frame the overlay for targets you care about (unmezzable, rampage, casters…). Cast and ranged range are worked out from your gear.
- **Live Maps** — Rendered zone maps parsed directly from your EQ client files, with Outline and Detailed display modes, live player and groupmate position tracking, a pop-out overlay window, and user-authored annotations that write back to the in-game `/map`. NPC spawn points show with patrol routes and trap markers, and a user-installed Brewall-style map pack can be rendered as a third display mode. Map search groups each NPC or POI into one result and can highlight every spawn point at once.
- **DPS Meter** — A transparent overlay showing live damage output for you and your group, with fight duration, session totals, and per-class bar colors. Bars scale to the top damage dealer (or share of total), and your own row stays pinned when it's scrolled out of view. Floats above the game as a standalone window.
- **Combat Log** — Persistent fight history with Personal / Raid / Encounter DPS metrics, expandable per-combatant breakdowns, date-range filters, and event-based session grouping.
- **Threat Meter** — A personal, per-mob hate estimator built from your own combat log: damage, spell aggro, hate-modifying buffs, heals, and feign resets, with a live rolling-window hate-per-second readout. Dashboard card and pop-out overlay.
- **Roll Tracker** — Parses `/random` rolls into sessions, with tiered grouping profiles (Pick/Upgrade/Alt, Need/Greed, or your own saved custom groupings), an optional timer that auto-stops collection, a dashboard panel, and a pop-out overlay window.
- **Kill Tracker** — A Kill Log of every kill your log shows, with the time to the second, killer and zone: sort and filter it, or switch to a By mob view of kill counts and time since each mob last died, and backfill it from old logs. Kill Counters count kills toward event milestones with on-screen or spoken alerts, correct themselves from the server's own announcements, and show in a Kill Counters overlay. One-click presets built from Project Quarm's quest scripts cover Bertoxxulous, Coirnav, Fennin Ro, the Stone Ring, Ston`Ruak, Drornok and Plane of Time phase 3.
- **Spell Timer Engine** — Countdown bars for every buff, debuff, mez, stun, and DoT. Tick-accurate durations, with Zeal-pipe corroboration when available. Separate overlay windows for buffs and detrimentals, plus a Filtered Timers overlay you filter by spell, player or mob, which can list group or raid members missing a buff. Expired timers can be kept until refreshed, with adjustable linger times per type and per spell.
- **NPC Respawn Timers** — A transparent overlay that tracks NPC deaths and counts down to respawn, with Project Quarm's fast-respawn reduction applied automatically. A manual Fast/Normal toggle switches to raw (unreduced) timers for the PvP instance, the one place the server skips the reduction (the open world and ordinary guild instances both use it). Optional audio or text-to-speech alerts fire when a respawn is ready, scoped to your current zone. Pin a camp/farm priority timer to sort it first, keep it alive past the normal pop grace window, and give it a short click-to-edit label; an optional setting carries the pin to the next kill at the same spot.
- **CH Chain Overlay** — Tracks raid Complete Heal chains in real time from heal callouts: cast-to-land bars, live measured cadence, and a stall indicator, plus a personal CH metronome overlay that paces your slot. Both float as standalone dashboard panels.
- **Loot Tracker** — A dedicated page that logs drops as they happen, with clickable items (full detail popup) and zones (jump straight to the Zone browser).
- **Chat History** — A multi-channel log of your tells and conversations, with per-character tabs and chat-style threads so you can scroll back through who said what.
- **Log Feed** — Real-time feed of every parsed combat, spell, and zone event from your EQ log, plus a read-only Browse mode for reading logs out of game and a Replay mode that plays historical log segments back through the live pipeline to test triggers and overlays.
- **Spell Checklist** — See every spell your class can learn, cross-referenced against your Zeal spellbook, so you always know what you're missing — with a "Where to get it" button on each spell that surfaces where it can be obtained, a name-search filter, and a shopping-route planner that maps an efficient vendor run for the spells you select. Pass on spells you'll never buy — they leave the missing count and the route, and a Passed filter lets you reverse it. Spells you already own as a scroll but haven't scribed are flagged automatically, checked across every character's bags and bank, with a hover tooltip showing exactly where each one was found.
- **Spellsets** — Read and edit your Zeal `_spellsets.ini` exports inside the app. Add and remove sets, rename inline, and import another character's spellsets — with off-class and ineligible-spell blocking so you never load something you can't cast.
- **Bandolier & Macros** — Read and edit your Zeal weapon-swap bandolier sets and your in-game social macros inside the app, with guards that keep you to gear you own and can equip. Includes a `/cancelbuff` macro builder that turns a buff search into cancel-by-name lines, and copies macros from one character to another, and lets you view and edit your Page 10 social keybinds.
- **Inventory Tracker** — All items across all your characters in one searchable view, including bank and shared bank, with a dedicated Rechargeable Items section and remaining charge counts on limited-charge clickies.
- **Trader Tracker** — Infers your Bazaar sales by comparing your Trader's Satchel between inventory exports, so you can see what sold while you were AFK.
- **Key Tracker** — Tracks item components for major raid keys (Veeshan's Peak, Old Sebilis, Howling Stones, and more) across all your characters.
- **Keyring** — Live per-character `/keys` snapshot, with an inventory fallback for keys that aren't in the keyring yet (Plane of Time, Vex Thal, Grieg's End).
- **Gear Upgrade Finder** — A per-character, per-slot upgrade scanner that ranks gear with cap-aware stat scoring (it knows when a stat is already maxed vs. still scaling), editable per-class weights, an all-slots overview, a By zone view of every upgrade that drops in your current (or any) zone, weapon/ATK/haste scoring, and priority focus effects. Star upgrades straight into your wishlist.
- **Resist Calculator** — Estimates a spell's land chance against any NPC, with a resist-debuff section and a searchable NPC target picker.
- **Charm Pet Finder** — Lists charmable NPCs per zone, class, and spell, ranked by DPS with land-chance odds and level-cap warnings.
- **Wishlist** — A per-character wishlist with drag-to-reorder. Star any item from the database — or from the Gear Upgrade Finder — to add or remove it, and get alerted when a wishlisted item shows up in your combat log.
- **Players Tracker** — A searchable database of every player you've seen via `/who`, `/guildstat`, and — with Zeal — your raid roster: name, class, guild, zone, level history, first-seen / last-seen. Sortable, filterable by guild and class, across all your characters. Add per-player notes and a PVP flag, with a sound + on-screen warning when a PVP-flagged player shows up in `/who`, plus automatic tracking of tells and group joins.
- **Lockouts Tracker** — Live `/sll` countdowns for loot and legacy lockouts, tracked per character so you always know when an instance or flag is available again, plus a Plane of Time section synced from `#timelockout` and a per-zone Loot Lockouts sub-tab showing every character's status for that zone at a glance. A dedicated Zone Lockouts overlay (dashboard panel and pop-out window) surfaces the same per-zone status without leaving the game.
- **Faction Tracker** — Per-character tracking of every faction you've ever killed toward or `/con`'d, with a 9-segment `/con` disposition bucket bar and best-effort point estimates from resolved kills and quest turn-ins. Persists across restarts; log backfill seeds a baseline from existing history.
- **Raid Composition** — A role taxonomy, an encounter knowledge base, and a live MIN/REC composition checker that compares your raid's roles against the current Zeal raid roster (or a manual roster). Includes a Raid Summary dashboard tab with per-group cards and saved roster snapshots, a Raid Readiness overlay, and pack import/export for sharing encounter data.
- **PoP Flags** — A per-character checklist for the Planes of Power planar-progression flags, with prerequisite locking and any-of grouping for equivalent quest paths. Syncs from the Seer Mal Nae`Shi guided meditation or Quarm's `#popflags`/`#timelockout` commands, in addition to manual toggles and live kill detection. A tiered Flow chart view lays out the same flags zone-by-zone with unlock arrows, alongside the Checklist, and an All characters tab shows every character's progress at once with a JSON export and a copy-as-text summary.
- **Progression Recap** — Rolling 7/30/90-day level, AA, spell, and skill progression for any character, or ranked most-active-first across all of them, with total coin/change and a scoped loot panel. A tab on Character Info.
- **Spell Modifiers** — A tab on Character Info listing every spell focus your gear and AAs give you, plus the focus types your class could use but you have nothing equipped for. Click any of them to browse the items your class and level can wear, best first, with upgrades and items you already own on any character marked, and try what-if gear and AA loadouts in Test Resolution to see how a spell would resolve before you swap.
- **Stat Snapshots** — Capture Zeal's `/mystats` output from your log and compare two captures side-by-side, with a plain-English label on every stat and the change between them. A tab on Character Info, handy for A/B-testing a gear or bandolier swap.
- **Blocked Buffs** — A Characters page for the `#blockbuff` list: keep the buffs you want refused, see which are actually applied from the server's replies in your log, and copy the exact commands that bring the server in line.
- **Config Backup Manager** — Snapshot and restore all your EQ `.ini` config files with one click.
- **App Backup & Restore** — Export your full app state (settings, triggers, trigger packs) as a single bundle and restore it on another machine or after a reinstall.
- **Discord Voice Overlay** — A pop-out overlay embedding Discord's own StreamKit Overlay page, showing a live voice-channel roster with a speaking highlight. Supports multiple named links and adjustable content-area transparency.
- **Custom Triggers** — A GINA-style regex trigger engine. Each trigger can hold multiple regex patterns (with per-row toggles) and use `{c}`/`{target}` tokens for GINA pattern compatibility; matches fire configurable on-screen text and audio alerts (including an optional locally-installed Piper neural voice for text-to-speech), and alerts can play on a chosen speaker or headset instead of the Windows default. Triggers can also capture a target's name into the alert and its timer. Organize triggers into custom categories with drag-and-drop reordering, and customize the alert text style (color, glow, font, size). Ships with built-in packs for every class plus ten general-purpose community packs including class crowd-control break alerts, raid slows tracking, and countdown timers for scripted raid events. A unified import wizard detects and previews GINA, EQNag, EQLogParser, and PQ Companion trigger packs before committing them; export your own packs as JSON, or export a single category as its own shareable pack. Per-trigger refire and repeat-audio cooldowns tame bursts of duplicate alerts, and a repeat count window with `{repeated}` shows how many mobs a PBAE hit in one alert, and a "Copy to Clipboard" action can fire on a match. Live history feed and a standalone transparent overlay window. A Tester tab lets you paste log lines and see exactly which triggers would match (or why not) before they run for real, and the trigger editor has an inline sample-line test as you write a pattern.
- **Spell Emote Customizer** — Edit the client-visible chat emotes in your `spells_en.txt` to disambiguate spells that share identical text (e.g. slows' " yawns.") or add one to a spell that ships with none. Overrides survive server patches that republish the file, with a per-spell diff view and detection of any trigger whose pattern was written against the old text. Gated behind Settings > Developer > Flags, off by default.
- **Custom Timers** — Manual countdown timers, including durations pulled from trigger capture groups, and timers that repeat on expiry for waves on a fixed cadence. Organize triggers into named Timer Groups, each with its own dedicated overlay window — pop out several at once, pin individual timers to stay on top, and close windows from their own title bar.
- **Zeal Pipes Integration** — When Zeal is running, the app talks to it over Windows named pipes for real-time target, pet, cast, and buff data — no waiting on log lines or file exports.
- **Settings** — Point the app at your EQ folder and character name; everything else is automatic, and settings save as you change them. Includes app-wide zoom and a high-contrast text mode for readability, per-overlay opacity, fade and lock-behaviour controls with a "Display only" HUD mode and a global Position-overlays / Manage-overlays workflow, a Navigation tab to hide and reorder sidebar tabs (with pinnable favorites at the top) and pop out every overlay at once, unified Log Backfill with status diagnostics, and an About tab with project links.

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

### Getting `quarm.db`

The EQ game database (`backend/data/quarm.db`) is **not** committed to the repo — it's ~85&nbsp;MB and regenerated from MySQL dumps by the `data-release` workflow. A fresh clone won't have it, and the backend won't start without it. Download the latest copy from the `data-latest` release into `backend/data/`:

```bash
# with the GitHub CLI
mkdir -p backend/data
gh release download data-latest --repo jasonsoprovich/pq-companion -p quarm.db -D backend/data

# or with curl
curl -L -o backend/data/quarm.db \
  https://github.com/jasonsoprovich/pq-companion/releases/download/data-latest/quarm.db
```

(Linux source-build users: this is the file to grab here — you don't need a Windows install to copy it from.)

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
