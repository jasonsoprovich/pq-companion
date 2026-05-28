# Changelog

User-facing release notes for PQ Companion. This file is the single source of
truth for the website changelog page and the Discord `#changelog` channel.
The internal commit history lives in git; only changes users would notice
appear here.

Newest first. To add a new release, prepend a new `## vX.Y.Z — YYYY-MM-DD`
section at the top — the `discord-notify` workflow picks up the topmost
section automatically.

## v0.9.0 — 2026-05-28

A foundational update: the desktop runtime under the app has been refreshed to the latest stable version, closing out every known security advisory.

### Highlights
- **Runtime upgrade** — PQ Companion now runs on the latest stable version of Electron (the desktop framework). This brings a modern Chromium engine and Node.js runtime under the hood, which gives the app better rendering performance and access to newer web platform features. No outward behavior changes — everything should work exactly the way it did before, just on a more current foundation.
- **Security** — the runtime upgrade resolves 18 published Electron security advisories. As of this release, the app has zero known vulnerabilities across its entire dependency tree.

## v0.8.3 — 2026-05-26

SQL sandbox grows saved queries with import/export, bard song timers stop misbehaving, and item detail tabs hide empty source categories.

### Highlights
- **SQL sandbox** — saved queries with import/export of query packs; horizontal scroll for wide result sets; query persistence and per-tab history; new Quarm-slot example query
- **NPC overlay** — per-surface section visibility, so you can hide individual sections on the live overlay, the NPC detail page, or both
- **Item details** — Drops From / Purchased From / Foraged From / Ground Spawns / Tradeskills tabs hide themselves when an item has no entries in that category, instead of opening to an empty list
- **Installer** — uninstaller clears the auto-updater cache and optionally removes your user data (`~/.pq-companion`) on uninstall

### Fixes
- Bard song timers now use the correct base duration and apply item clicky timing rules consistently
- Log parser recognizes "You begin singing" for bard songs so timers start when the song actually starts
- Removed the stray Katta Castellum entry from the key tracker

## v0.8.2 — 2026-05-25

Developer mode lands with a guarded SQL sandbox, key tracker gets a sweeping quest rewrite, and the NPC overlay grows mana plus color-coded resists.

### Highlights
- **Developer mode** — hidden Settings tab unlocked by Ctrl+Shift+D, with a read-only SQL sandbox (guarded `/api/sandbox` endpoint), 10 curated starter queries, an interactive schema graph, and a curated Mermaid ER diagrams panel
- **Key tracker rewrites** — Howling Stones now uses the full Key to Charasis quest; Arx Seru uses the 4-shard Praesertum quest; Veeshan's Peak uses the full Key of Veeshan quest; Sleeper's Tomb accepts any one Velious talisman; Sebilis swaps Trakanon's Tooth for the Trakanon Idol quest. Hand of Glory (Charasis internal doors) added. Grieg's End and Grimling pens dropped from the tracker
- **Keyring** — tab is first and default; live-refreshes on `/keys` and infers upgraded stages as owned; corrected Lucid Shard zone mappings for the Vex Thal key
- **NPC overlay** — NPC max mana shown beside HP when > 0, resists reordered to MR/CR/FR/DR/PR with EQ-convention colour-coding, multi-field `special_abilities` entries (e.g. Rampage range) now parse correctly
- **NPC detail page** — new **Spells & Procs** section; long cast-spell lists collapse with a show-more toggle; Escape and outside-click dismiss ability popovers and modals
- **Zeal** — soft update notice and an `ExportOnCamp` warning when the setting is disabled
- **Settings** — EQ client version status panel (drops the unused `eqw.dll` row)
- **Backup Manager** — Open folder button in the header
- **Spell timers** — mez timer defers rendering until the spell actually lands; NPC names are normalized on kill match and unmatched kills are logged

### Fixes
- Quarm client-status detection uses `FileVersion` (not MD5) as the primary signal, so patch-day MD5 churn no longer flips users to "unknown"
- Triggers: positioning button is passive while a session is active, and the positioning card is reliably draggable
- Triggers: removed duplicate generic resist/interrupt rules from the Enchanter pack
- Spells: class spell list excludes disciplines and entries above level 60
- Installer pinned to per-user install so it stops defaulting to Program Files
- EQ-config backups directory moved to the user home with a migration from the legacy location
- `quarm.db` DSN now includes `immutable=1` so Program Files installs work without write access

## v0.8.1 — 2026-05-22

Wishlist gets collapsible sections, drag-reorderable cards, and a flat All Items view.

### Highlights
- **Wishlist sections** — each slot is now a card you can collapse via `+`/`−` or by clicking its header; drag the grip on the left to reorder cards. The layout (order + collapse state) persists per character.
- **Expand all / Collapse all** in the Wishlist toolbar — handy after a kill for quickly checking what's still on the list.
- **All Items view** in Wishlist — flat list of everything wishlisted across slots, with free-form cross-slot drag for prioritizing upgrades. Toggle Category / All items from the toolbar.
- One global ordering now backs both views, so reorders in either are reflected in the other.

### Fixes
- Character race displays "Iksar" / "Vah Shir" / "Froglok" instead of `Race 128` and the other post-Kunark race IDs.

## v0.8.0 — 2026-05-22

Wishlist, Keyring tracker, Live Buffs, and a sweeping rewrite of game-data enum labels.

### Highlights
- **Wishlist** — per-character wishlist tab with drag-reorder; star button on Items DB rows adds or removes entries
- **Keyring tracker** — per-character `/keys` snapshot with inventory fallback for keys not yet added to a keyring (Plane of Time, Vex Thal, Grieg's End)
- **Live Buffs** in Character Stats — buffs panel is now driven by the spell timer engine with swappable raid-buff presets per character; confirm-on-edit and remove-buff actions
- **Canonical enum catalog** — item types, NPC classes/races, zone types, slot/race/class bitmasks, bane types, body types, expansions, special abilities, tradeskills, and spell target types all flow through a single Go catalog served by `/api/enums`, with a new `enum-audit` CLI for verification. Many previously-wrong labels are corrected (NPC body types, post-PoP race/class hiding, Quarm-specific SPAs 160 + 500–504, spell target types 0/9/17)
- **Vex Thal zone-wide loot overlay** — zone-wide drops now show on every NPC in the zone, targeted by lootdrop id with pool names
- **Trigger packs** — full discipline coverage across all melee classes; new `dedup_key` field lets shared triggers across packs fire once; pack-grouped UI with collapsibles, pack filter, sort, and shared badge; Global Alerts folded into General Triggers; Gift of Brilliance added to Enchanter pack
- **Zeal version warning** — detects installed Zeal via crash-handler anchor and warns when below the minimum supported version
- **NPC overlay** resolves corpse targets to the underlying NPC and humanizes loot-table headings
- **Inventory** — hide-empty-bags toggle
- **Zones** — bard swarm/warp limits shown on zone overview
- **Items** — modal title links back to the items page
- **Logging** — backend slog mirrors to `~/.pq-companion/logs/server.log`; Electron + sidecar stdio mirrors to `electron.log` (3-session rotation on both)

### Fixes
- Trigger fires deduplicated so a single log line no longer triggers multiple alerts
- Mez TTS pronunciation corrected
- Atol's Spectral Shackles wear-off labeled as snare rather than root
- Spell-modal prefill matches both self and other landed text
- NPC overlay shows "default" instead of "1" when `attack_count = -1`
- Invisible-man placeholder rows hidden from NPC queries
- Tradeskill ID → name mapping corrected on items
- Wishlist: null-guarded source lists and items array; clicking a filled star prompts to remove the entry
- DPS post-fight panel stays visible when broadcasts batch
- v1 worn haste computed from item wornlevel + spell formula
- Zeal shared bank slots capped at 10 on Project Quarm
- Overlays: grid-snap defaults, bottom/right canvas padding, drag autoscroll
- Keys: separate tabs for Key Tracker + Keyring; Grieg's End zone added; intermediate-state check shown in green

## v0.7.2 — 2026-05-18

Character-switch and Zeal-pipe log hygiene.

### Fixes
- Active character now follows in-game camp/login transitions even when a manual character override is set — the main button and Characters tab stay pinned to who you're actually playing
- Spell timer engine deduplicates Zeal-pipe buff-slot divergence logs so a long-running buff (KEI, Aegolism) no longer floods the console with hundreds of identical lines per minute

## v0.7.1 — 2026-05-17

Per-class DPS bar colors with a Settings palette.

### Highlights
- **Per-class bar colors** on the DPS meter and combat history rows — class is resolved server-side from the `/who` tracker (and the active character's stored class for "You"), so pets inherit their owner's color automatically
- **Settings → DPS Class Colors** exposes hex inputs plus color pickers per class with an Unknown fallback, with per-row and global reset to defaults

## v0.7.0 — 2026-05-17

Zeal integration, melee discipline cooldowns, Players page, app backup, Spellsets, and a settings overhaul.

### Highlights
- **Zeal integration** — the app now talks to the Zeal mod over its local pipe for authoritative live data. Unlocks live target HP bar and pet-owner badge in the NPC overlay, better pet damage attribution in the combat meter, confirmed casts in the spell timer engine, and a brand-new pipe-source trigger type (fire on target name, target HP%, buff land/fade, or your in-game `/pipe <text>`).
- **Melee discipline cooldowns** — discipline triggers now spawn a second timer tracking the reuse cooldown with a TTS "ready" alert. Covers every warrior, monk, rogue, paladin, SK, and ranger discipline plus Divine Intervention/Aura, Dictate, Avatar, Voice of Terris, Unholy Aura, Harmshield, Ferocity, Lay on Hands, and Harvest.
- **Players page** — a `/who`-driven database that remembers every player you've seen with class, race, guild, last zone, and level history. Searchable, sortable, filterable. Anonymous sightings preserve previously known info.
- **App Backup & Restore** — export your entire app state (settings, triggers, characters, EQ config backups) into one `.pqcb` bundle, then import on another PC.
- **Spellsets** — new tab to view/rename/add/remove spellsets via Zeal's `_spellsets.ini`, plus import another character's spellsets with ineligible spells blocked at import.
- **Settings reorganized** into 6 category tabs (General, Overlays, Spell Timers, Logs, Backups, Advanced). Renamed "Backup Manager" → "EQ Config Backups" and "Backup & Purge" → "Archive & Trim Log File" so the two backup features stop looking like the same thing.
- **NPC overlay** — loot table toggle with drop rates, tighter layout in the popped-out window, class labels for trainers/bankers/merchants.
- **Raw Data modal** on every item/spell/NPC/zone detail view — see every column from the underlying DB row.

### Fixes
- Combat meter shows charm pet damage when the pet name matches the current target
- VT Lucid Shard correctly attributed to Ssraeshza Temple (was Grieg's End)
- Loot rarity color tiers remapped — gold is now the default
- Log parser recognizes the "looks at you" (dubious faction) con message
- Item type 45 labeled "Hand to Hand"; item-type labels aligned with EQMacEmu enum
- Spell raw-data fields flow column-major to match pqdi.cc layout

## v0.6.4 — 2026-05-13

Backend port discovery and CSP fixes.

### Fixes
- Frontend targets 127.0.0.1 instead of localhost for more reliable backend dial
- CSP allows dynamic backend ports on localhost and 127.0.0.1
- Settings recovers from backend-unreachable errors
- Backend port now discovered via file in dev mode

## v0.6.3 — 2026-05-12

Settings port-edit polish and a build pipeline fix.

### Highlights
- Unsaved-change banner and inline Save for backend port edits in Settings

### Fixes
- Server binds 127.0.0.1 explicitly for reliable conflict detection
- `npm run dist:win` now rebuilds the Go backend as part of the dist pipeline

## v0.6.2 — 2026-05-12

Backend network controls in Settings.

### Highlights
- New **Backend Network** section in Settings — display, test, and reset the backend port
- Backend tries your preferred port and falls back to an OS-assigned one on conflict, then plumbs the actual port to the renderer

## v0.6.1 — 2026-05-12

Spell-modifier accuracy: bard exemption, 50% spell-haste cap, item-clicky gating.

### Highlights
- Bards exempted from spell duration extensions (and locked in for in-class spells too)
- Spell haste capped at 50% and now surfaced on the character stats page
- Item-clicky duration extensions only apply when the spell is actually castable by your class
- Item, spell, and NPC lists paginate with "Show more"
- App detects a missing `quarm.db` on startup and guides you through manual repair

### Fixes
- Item in-game links use the correct Mac-era format for Project Quarm

## v0.6.0 — 2026-05-12

Combat History page, roll tracker, and three DPS metrics.

### Highlights
- **Combat History** — every fight is archived to SQLite with retention pruning. Browse, filter, and group by NPC, character, date presets, or facet dropdowns. Toggle pets, me-only, and DPS-mode on the fly. Session-grouping option detects fight breaks.
- **Roll Tracker** — watches `/random` in the combat log, buckets rolls into per-range sessions (so Cowl 333 / Shard 444 / Heart 666 in overlapping windows stay separate). Per-session Stop/Remove/Clear, optional Timer mode with 5–600s auto-stop, dashboard panel and pop-out overlay.
- **Three DPS metrics** — Personal, Raid, Encounter — switch on the fly across every aggregate display
- Session-break dividers in combat with a 120s gap rule
- DoT ticks and PQ-format crit hits now captured by the combat parser
- Per-combatant bar colors on the DPS meter
- Curated expansion grouping in the zones browser, with graveyard pop-out info
- Overlays: lock toggles click-through, only the header stays clickable

### Fixes
- Orphan detrimental timers clear when the target NPC dies
- Combat parser handles single-word bosses, charmed pets, Eye scouts, and 0-damage rows
- DPS mode toggle now updates every aggregate display

## v0.5.3 — 2026-05-08

Faster zones browser.

### Fixes
- Zone list shows immediately on mount instead of waiting for a 300ms debounce
- NPC name search treats spaces as underscores

## v0.5.2 — 2026-05-08

Master volume slider.

### Highlights
- Master volume slider added to Settings

## v0.5.1 — 2026-05-07

Trigger polish and charm-pet damage fixes.

### Highlights
- Multiple trigger edit forms can be open simultaneously
- Per-trigger **exclude patterns** to filter pet/merchant lines
- Keyed one-time additive default updates for built-in packs — new defaults land cleanly without overwriting your edits

### Fixes
- Charmed pet damage stays attributed to you when you tagged the mob first
- Unpositioned text overlays default to screen center instead of (0,0)
- Global event alerts default to off
- Per-trigger threshold no longer lost when the trigger fires before spell-land
- Per-row remove now works and shows in popped-out timer overlays

## v0.5.0 — 2026-05-07

Active-time DPS, trigger-only timer mode, and overlay quality-of-life.

### Highlights
- Toggle between **fight-duration DPS and active-time DPS** — fairer for DoT classes and downtime-heavy roles
- **Spell Timer tracking mode**: Auto (every recognised spell) or Triggers-only (only spells you've curated)
- **Overlay hover-disable** — hovering anywhere in a locked overlay temporarily disables passthrough so you can interact
- Per-timer **X button** to dismiss individual timers
- Trigger-driven timers now capture the spell icon
- Settings: stay on page after save/discard with an inline notice

### Fixes
- Combat parser recognises passive-form kills ("X has been slain by Y!")
- Logparser handles YOU all-caps and multi-word NPC actor names
- `pq-audio://` scheme now uses `protocol.handle` with explicit MIME types so paths with spaces and special characters work

## v0.4.3 — 2026-05-05

Trigger and spell-timer accuracy fixes.

### Highlights
- **Class filter for spell timers**, with detrimentals exempt from buff scope
- Drag the alert card directly when positioning instead of fiddling with chrome
- Buff timer **sort-mode toggle**
- Dashboard toolbar gets a **Pop Out All / Close All Popouts** toggle

### Fixes
- Spell duration formulas 8, 9, and 10 corrected; halved Pacify; matched generic charm-fade
- Trigger Clear-All now does one DELETE plus one engine reload
- Audio: TTS no longer cancels prior speech; autoplay forced; failures surfaced
- Log feed persists events across tab navigation
- DPS rolls "You" up with character row so charmed-pet damage attributes correctly

## v0.4.2 — 2026-05-05

Spell timer persistence and CC break alerts.

### Highlights
- Spell timers **persist across zone**, scope by caster, and drop on victim death
- **CC break alerts** (overlay + TTS) added across class packs
- Default **fading-soon / expiring** TTS alerts added to every built-in class pack

### Fixes
- user.db busy timeout bumped from 5s to 30s to avoid SQLITE_BUSY
- Icons load correctly in the packaged app via relative paths

## v0.4.1 — 2026-05-04

All 14 class trigger packs.

### Highlights
- Built-in trigger packs for every class: Warrior, Cleric, Paladin, Ranger, Shadowknight, Druid, Monk, Bard, Rogue, Shaman, Necromancer, Wizard, Magician, Beastlord — plus Enchanter expanded with charm/root/pacify and Boltran's
- **Per-character chips** on trigger edit form; pack imports default to matching-class characters only
- **Search + class filter** on the trigger list
- Triggers page renamed **Triggers / Timers**
- Quarmy-driven persona refresh for all characters
- Zone browser switches to a PQDI-based allowlist (181 zones) with expansion filter
- **Clear All** button with confirmation modal on triggers

### Fixes
- NPC special-ability codes corrected to match Project Quarm's EQMacEmu fork
- Spell SPAs 145–220 labeled so disciplines show effect names
- Native form controls render in dark mode
- Class-pack triggers disabled when no matching character exists

## v0.4.0 — 2026-05-03

Overlay dashboard with snap grid, icons everywhere, and a character stats rework.

### Highlights
- **Customizable Overlay Dashboard** with snap-grid drag-and-drop layout that persists across sessions; pop any panel out as its own transparent click-through window
- **Item and spell icons** throughout — database explorer, inventory, spell checklist, timer overlays, modals, NPC loot, cross-refs
- **Character Stats rework**: base HP/Mana, stat caps, side buff list, worn effects, softcap AC, INT/WIS-driven mana
- **AA descriptions** shown for each Alternate Advancement
- **Per-character sub-tabs** across every tracker page
- Persona now **driven by Zeal** — no more manual edit
- Trigger packs: **Test/Position** button for overlay-text alerts; play/test buttons for sound/TTS; native file picker for sound paths
- Per-action position and font size added to the Action model
- Audio: `pq-audio://` scheme for local sound files
- HPS dashboard panel wired in (currently hidden behind dev flag)

### Fixes
- Boss fights no longer fragment into many entries
- Pet/charm damage rolls up under owner
- Enemy title shows on NPC info; stop attributing fights to players

## v0.3.1 — 2026-04-28

Spell Modifiers tab and overlay consolidation.

### Highlights
- **Spell Modifiers tab** on Character Info shows every focus and worn effect modifying your spells
- All overlays consolidated into a single dashboard tab

### Fixes
- NPC-only spell filter; NPC-only spells excluded from All Classes
- Spell SPA labels rebuilt from canonical EQEmu `spdat.h`
- Cross-class trained AAs dropped from character AA list
- Intermediate combine marks complete when final key is owned
- Power toggle replaced with **Clear button** on timer overlays
- Lock toggle added to overlay popout windows
- Duplicate timer creates de-duped; timers cancel when spell fails to hold
- EQ log timestamps parsed as local time, not UTC

## v0.3.0 — 2026-04-26

Character Progression, first-launch wizard, paper-doll inventory, and to-do lists.

### Highlights
- **Character Progression** — full per-character workspace with stat tracking, gear view, and AA tracking
- **First-launch onboarding wizard**
- **Per-character to-do list** with subtasks and reordering
- **AA trained/available split** with in-place item detail modal
- **Paper-doll inventory layout** with collapsible bag/bank cards
- **Advanced item filter modal** — race, class, level, slot, type, stat thresholds
- **Class and level filters** on Spells tab and Spell Checklist
- NPC, DPS, and Spell Timer overlays consolidated under Overlays nav
- Character-related nav items grouped under Characters with sub-tabs
- Active character auto-detected in the nav-bar selector
- Backup Manager moved from nav bar into Settings tab
- Forward/back navigation buttons near global search
- Persisted overlay window size/position across sessions
- Trigger-driven spell timers, GINA import, DB-entry shortcuts
- **Click-through** enabled on overlay windows so game input passes through

### Fixes
- Placeholder NPCs filtered from search by default
- Independent scroll regions for header, nav, and main pane
- Vex Thal intermediate combine detection
- DPS list excludes NPCs attacking group members not the player
- Audio/alert hooks only run in the main window — no more duplicate TTS

## v0.2.2 — 2026-04-19

Multi-character support and a flood of DB-explorer cross-references.

### Highlights
- **Multi-character support** with Characters tab and CRUD; auto-discover characters from EQ logs
- Item pages gain **Drops From, Purchased From, Foraged From, Ground Spawns, and Tradeskills** tabs
- Zone pages gain **connected zones, drops, ground spawns, forage, and NPC** tabs
- Spell detail gains **Taught by** and **Items with this effect**
- NPC detail gains **loot table, spawn tables, respawn timers, faction name, kill hits**
- **Copy In-Game Link** button on item detail
- **Copy DPS/HPS fight summary** to clipboard
- Clickable popups for special abilities and spell effect links
- Item source (drop/merchant) shown on item detail

## v0.2.1 — 2026-04-19

Installer/updater fix.

### Fixes
- Sidecar killed reliably on Windows so the installer and auto-updater can replace files

## v0.2.0 — 2026-04-18

Auto-updater, code signing, key tracker additions, and DPS overhaul.

### Highlights
- **Auto-updater** with silent install to same path and a restart countdown
- **Windows code signing** via electron-builder secrets
- **Ring of the Shissar** and **Scepter of Shadows** tracked in Key Tracker
- **Character switcher** under global search in sidebar
- **Log backup & purge** with 75 MB size warning
- Active character **auto-detected from log file activity**
- Clickable NPC rows in Zones tab that link to NPC detail

### Fixes
- Non-melee spell damage parsed for caster DPS tracking
- Fight duration correction; DPS overlay persists 30s after a fight ends
- Zone name no longer leaks into the NPC overlay target
- Default overlay opacity set to 25%
- Re-select item/spell/NPC/zone from global search when already on that page
- Spell detail popup modal added to Spell Checklist

## v0.1.2 — 2026-04-17

Maintenance release.

## v0.1.1 — 2026-04-15

Maintenance release.

## v0.1.0 — 2026-04-15

Initial release.
