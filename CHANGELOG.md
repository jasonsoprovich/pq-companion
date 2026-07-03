# Changelog

User-facing release notes for PQ Companion. This file is the single source of
truth for the website changelog page and the Discord `#changelog` channel.
The internal commit history lives in git; only changes users would notice
appear here.

Newest first. To add a new release, prepend a new `## vX.Y.Z — YYYY-MM-DD`
section at the top — the `discord-notify` workflow picks up the topmost
section automatically.

## v0.15.2 — 2026-07-02

Lockout unlock times at a glance, plus overlay and trigger-audio fixes.

### Highlights
- **Lockouts** — each active lockout now shows the absolute unlock date and
  time in your local timezone (e.g. `07/04 3:25 PM`) right next to the live
  countdown, so you can see exactly when something frees up without doing the
  math. Hover for the full date/time.

### Fixes
- Idle spell-timer overlays now reliably fade their placeholder text, with a
  self-healing pass that clears overlays that previously stayed stuck.
- Fixed trigger sounds occasionally duplicating by adding a single-owner
  playback gate.

## v0.15.1 — 2026-07-02

A broad stability, performance, and security hardening pass, plus trigger
action templates, bulk editing, and non-destructive pack updates.

### Highlights
- **Trigger action templates & bulk editing** — apply a saved action template
  to many triggers at once, convert TTS alerts to sounds in bulk (including
  timer alerts), and pick exactly which triggers a bulk edit touches.
- **Non-destructive pack updates** — when a built-in trigger pack ships
  changes, a review panel on the Packs tab shows a per-field diff and lets you
  preserve your customizations or reset to the new defaults, install by
  install.
- **Spell quick-view tooltips** — hover any spell-effect link (and the focus
  badges on gear upgrades) for an instant summary without leaving the page.
- **Per-monitor default overlay text** — the Default Overlay Text Position
  setting now includes a monitor selector for multi-display setups.

### Fixes
- Gear upgrade scores and character-info stats no longer count Ammo-slot
  items, and the Ammo slot explains why it isn't scored.
- Add Proc effects now resolve to real spell names, and the Player Size
  wording is corrected.
- The loot list no longer flickers on reload, drops a stray tab-bar
  scrollbar, and caps how many rows it renders.
- Item search clears a stuck "Searching…" state on short queries or reopen,
  and search terms escape all LIKE wildcards so special characters match
  literally.
- Zone NPC lists no longer pull in collision/placeholder NPCs, and NPC
  locations correlate zone and coordinates correctly.
- Combat-log fights get unique keys so history navigation tracks the right
  fight, and deleting a character now removes its child records.
- The overlay-text position picker starts at the global default, and a
  recovery dialog now appears if the backend fails to start.
- Numerous performance and reliability improvements to the combat meter,
  overlays, database pools, config writes, and backup handling.

## v0.15.0 — 2026-07-01

Text-to-speech gains an adjustable voice rate, and Settings is reorganized
around a grouped side navigation.

### Highlights
- **Text-to-Speech Voice Rate** — a new global speed control under Settings ›
  Overlays speeds up (or slows down) every spoken alert, with a Test button
  that previews the rate live before you save.
- **Redesigned Settings** — the crowded top tab bar is replaced by a grouped
  side navigation, and a new Accessibility section gathers high-contrast mode,
  app zoom, and per-overlay font/zoom sliders so each popped-out overlay can be
  sized independently.
- **Clickable links across overlays and the database** — items and spells in
  the NPC overlay, spawn-group members, lockout rows, and spell-modifier
  source/focus spells are now clickable and open directly in the database
  explorer, gated by a new "Clickable Links in Overlays" toggle.
- **DPS parse survives zoning and death** — your parse no longer resets when
  you zone or die; it ends only on a kill, after a configurable inactivity
  timeout, or when you discard it manually. A new discard button clears the
  meter while archiving the fight to Combat History.
- **Roll Tracker tiered grouping** — /random contests can be grouped with
  configurable presets and a custom editor, results render as contest cards,
  loot items are auto-suggested from chat, and you can add a manual item label
  and copy a paste-ready result summary.
- **Spell Modifiers — full focus coverage** — the Spell Modifiers tab now
  surfaces every worn focus effect (spell damage, healing, resists, and more),
  not just the ones Resolve applies.
- **Raid threat estimation on by default** — the Threat Meter's raid-aware
  mode ships enabled, with its settings moved into Settings › Overlays.
- **Fractional trigger timers** — timer durations and refire cooldowns now
  accept fractional-second values.

### Fixes
- Spell Modifiers focus limit numbering is corrected (SPA 139/141/142), and
  haste focuses now honour the minimum-cast-time limit (SPA 143).
- The Roll Tracker buckets rolls by their full Min–Max range instead of the
  maximum alone, so overlapping ranges no longer merge.
- Duplicate trigger alert sounds that fired from the same log line are
  collapsed to a single play.
- Open overlays are snapshotted before shutdown, so "restore overlays on
  launch" works even when quitting from the window's close button.

## v0.14.1 — 2026-06-28

A Planes of Power flagging tracker arrives, and the Threat Meter gains a
raid-aware estimation mode.

### Highlights
- **PoP Flagging Tracker** — a new tool that tracks your Planes of Power
  progression as a step-by-step checklist with an interactive dependency
  graph. Each flag is classified and colour-coded by step type (kill, turn-in,
  zone, etc.), locked steps stay locked until their prerequisites are met, and
  progress can be seeded from a Seer paste-in or detected live from your log as
  kills happen.
- **Threat Meter — Raid mode** — a new Solo/Raid overlay toggle that models
  taunt in raid settings (drops the default solo tank boost), registers
  hate-modifying buffs cast on you by other players, and detects successful
  taunt emotes.

### Fixes
- Charm-pet damage now attributes correctly to its owner on the DPS meter —
  leading-article casing is canonicalized, Zeal's target-pet-owner label is
  honoured, and self-healed druid pets bind properly.
- Threat estimates are more accurate: spell-cast hate now applies on
  land/resist rather than cast-begin, stun-nukes add standard hate, and feign
  death no longer leaves residual hate on raid mobs.
- Mob name casing no longer splits hate tracking across solo and raid mobs.
- PoP flag checklist no longer resets checked flags during an async load race,
  blocks manually completing a locked flag out of order, and handles locked
  flags gracefully without a full-page error.
- Navigation to graph-heavy (xyflow) pages no longer lags — routing is now
  synchronous.

## v0.14.0 — 2026-06-26

Four new tools arrive — Threat Meter, Trader Tracker, Resist Calculator, and
Charm Pet Finder — and triggers gain multi-format import.

### Highlights
- **Threat Meter** — a personal, per-mob hate estimator built from your own
  log lines (damage, spell aggro, hate-modifying buffs, heals, and feign
  resets), with a live rolling-window hate-per-second readout. Surfaced as an
  overlay with a dashboard card and pop-out window.
- **Trader Tracker** — infers your Bazaar sales by diffing your Trader's
  Satchel between inventory exports, so you can see what sold while you were
  away.
- **Resist Calculator** — estimates a spell's land chance against any NPC,
  including a resist-debuff section and a searchable NPC target picker.
- **Charm Pet Finder** — lists charmable NPCs per zone, class, and spell,
  ranked by DPS with land-chance and level-cap warnings.
- **Multi-format trigger import** — a unified Import Triggers wizard that
  detects and previews GINA, EQNag, EQLogParser, and PQ Companion trigger
  packs before committing them into a category.
- **Per-trigger refire cooldown** — an anti-spam lockout, plus a separate
  repeat-audio cooldown, to tame bursts of duplicate alerts.
- **Trigger "Copy to Clipboard" action** — fire a trigger that copies text
  straight to your clipboard.
- **Log Feed** — right-click any line to "Play from this point", an opt-in
  "Raw lines" toggle so live search finds any line, and replay file/date/time
  selections now persist as you navigate.
- **Custom timers & overlays** — per-trigger timer bar colors with a global
  appearance setting, a bar-color picker on quick-add, optional overdue
  reminders for expired timers, NPC overlay target pin/lock, and overlays now
  restore on launch.
- **Items** — item details show weapon damage ratio; character stats now
  include natural (level/race) HP regen.

### Fixes
- The main window now keeps its size and position on a secondary or
  mixed-DPI monitor on Windows.
- Mez/charm/root break triggers no longer play multiple stacked sounds for a
  single break, and trigger sounds no longer cut off mid-play.
- GM-only items (e.g. the Red Glowing Robe) are hidden from item queries and
  gear upgrade suggestions.
- Flowing Thought totals now count flavor-named worn FT effects.
- Log Feed browse search is fast and cancellable on large logs.
- Spell-detail effect ranges and resist magnitudes now scale correctly with
  level and respect the PoP era cap.

## v0.13.7 — 2026-06-21

Respawn timers now start for DoT and swarm-kited kills.

### Fixes
- Respawn timers now start when a mob dies with no melee killing blow — e.g.
  a bard DoT-kiting a swarm. Project Quarm logs these as "<name> died.", which
  the previous build didn't recognise, so those kills started no timer. Both
  "<name> died." and "<name> has died." are now counted as kills.

## v0.13.6 — 2026-06-20

Richer Players tracking, per-overlay positioning, and a batch of combat/loot fixes.

### Highlights
- **Players page — manual identity & guild overrides** — fill in class, level, and
  race for anonymous players yourself, and set a player's guild from a dropdown of
  guilds you've already seen, so your roster stays accurate even when /who hides details.
- **Players page — class-colour accents & "active within" filter** — each row now
  carries a class-coloured accent line, and a new filter narrows the list to players
  seen within a chosen time window.
- **Overlays — "Move" mode** — every overlay gets a dedicated move mode for dragging it
  into position, separate from resizing.
- **Overlays — per-overlay monitor picker & aligned settings** — pick which monitor each
  overlay lives on, with a tidied, aligned settings grid across all overlays.
- **Roll tracker — settings persist** — the winner rule, mode, and timer now carry over
  between sessions instead of resetting each launch.

### Fixes
- DoT and swarm-pet kills now start respawn timers — deaths logged as "X has died."
  are counted as kills.
- The Chat History feed no longer flashes or jumps to the top when new lines arrive.
- Class titles (Phantasmist, Hierophant, …) are coloured correctly on the Players page.
- Loot pools specific to an NPC now rank above shared spell/cash pools.
- The gear browser's No Drop filter and badge no longer read inverted.
- Per-overlay Reset buttons are now labelled instead of an ambiguous crosshair icon.
- Overlay settings panels fit their content, and the gear paper-doll neck row is fixed.

## v0.13.5 — 2026-06-18

Reliable drag-and-drop everywhere, finer-grained gear upgrade targets, and a portable download for Wine/Proton users.

### Highlights
- **Drag-and-drop rebuilt** — reordering wishlist items, triggers, and character tasks now uses a pointer-based engine that works reliably on Windows, where the old drag handles were flaky or didn't work at all.
- **Gear Upgrade Finder — per-slot ear/wrist/finger targets** — each of your two ear, wrist, and finger slots is now scored separately, so an upgrade for one slot no longer hides behind what's already in the other.
- **Gear Upgrade Finder — clearer weights editor** — editing per-class stat weights now offers importance presets and shows save feedback, and proc/click effects are called out with their own badge.
- **CH metronome — Main/Secondary chain switch** — the Complete Heal chain overlay can be flipped between your main and secondary rotation right from the Overlays dashboard.
- **Portable Windows download** — every release now ships a `.zip` alongside the installer, for users who run PQ Companion under Wine/Proton on Linux (where the installer fails) or on locked-down machines. Unzip and run — note auto-update doesn't reach the zip, so re-download to upgrade.

### Fixes
- Only one copy of PQ Companion can run at a time now; launching it again focuses the existing window instead of starting a duplicate.
- Spell checklist now matches Zeal spellbook exports correctly when two spells share a name.
- AoE mez timers track each target separately and clear one break at a time instead of all at once.
- Alert sounds use a single 750ms de-duplication window, so a burst of matching lines no longer replays the same sound repeatedly.
- Tidied the priority-focus picker wording and the gear paper-doll layout after the charm slot's removal.

## v0.13.4 — 2026-06-16

A small fixes release tidying up the volume sliders and the equipment slot list.

### Fixes
- Volume sliders now use the same gold fill everywhere. The per-trigger volume controls previously fell back to each operating system's default color, so they looked different on Windows and Mac and didn't match the Master Volume slider in Settings.
- Removed the Charm equipment slot from the Gear Upgrade Finder and the Character Info gear view. Project Quarm's client has no charm slot, so it was never usable.

## v0.13.3 — 2026-06-16

The Gear Upgrade Finder stops suggesting items you can't actually get, plus inventory stack totals and a round of overlay and trigger refinements.

### Highlights
- **Gear Upgrade Finder — unobtainable gear filtered out** — NO RENT temporaries no longer appear as upgrades, and GM-only items (Sunset Home GM-zone vendor gear, the Scepter of Al`Kabor) are excluded. A new **NO DROP** toggle lets you hide items you'd have to farm yourself.
- **Inventory — stack totals** — searching a stackable item (e.g. Othmir Fur) now shows a per-character total and a grand total across all characters, above the individual stacks.
- **Overlays — Clear All Timers** — a new action in the Manage overlays dropdown wipes every active buff, detrimental, and custom timer at once, handy after switching characters so the old character's buffs stop lingering.
- **Triggers — Pet Spell Worn Off** — pet buffs wearing off now fire their own trigger instead of the player's "Spell Worn Off", so they're easy to tell apart. Existing Spell Breaks installs pick it up automatically.
- **Gear weights — ATK & Mana Regen notes** — both now carry a caveat note (ATK is soft-capped around 250, worn Mana Regen / Flowing Thought is item-capped at 15) and sit alongside the other situational weights.

### Fixes
- Self-targeted buffs that carry an HP cost — such as Ancient: Master of Death — now show in the buff overlay instead of the detrimental overlay.
- Respawn alert text-to-speech now pronounces "respawned" correctly; saved alerts using the old wording are migrated automatically.
- Scrollbars are wider and easier to grab.
- The app title is now properly centered in the title bar instead of sitting slightly to the left.

## v0.13.2 — 2026-06-15

Respawn and custom-timer alerts arrive, alongside a batch of Gear Upgrade Finder refinements.

### Highlights
- **Respawn alerts** — get an audio or text-to-speech heads-up the moment a tracked mob's respawn timer is ready, scoped to the zone you're currently in so you only hear what's relevant.
- **Custom timer alerts** — add a per-timer alert bell right from the Custom Timers quick-add form, with default alert behavior configurable in Settings.
- **Gear Upgrade Finder — hide crafted gear** — a new toggle (on by default) keeps tradeskill-made items out of upgrade suggestions.
- **Gear Upgrade Finder — Planes of Power gear** — turning on "Show PoP gear" now correctly surfaces level-65 Planes drops (e.g. Plane of Time loot) that were previously filtered out.

### Fixes
- Gear Upgrade Finder no longer suggests a LORE item you already wear for a second slot, or recommends unobtainable GM-event items (Soul Devourer, The Prime Healers Bulwark).
- Fixed a black screen when expanding a gear suggestion's sources, and tightened item-name alignment when the column is narrow.
- Spell timers now track clicky buffs whose "lands on you" text collides with another clicky.
- The "Group Member Died" trigger no longer fires when a pet kills an NPC.
- The triggers search/filter bar now stays pinned above the scrolling list.
- CH chain accepts DCH/RAMP heal tokens; the "Ramp" button is renamed "Secondary."
- The last log line is now flushed when the game goes idle, so the final event isn't held back.
- Hid the stray HPS Meter row in Settings → Overlays.

## v0.13.1 — 2026-06-15

A performance release: much faster startup, plus the Minimize to Tray
option now works.

### Highlights
- **Faster startup** — the app launches significantly quicker and no
  longer shows a multi-second black screen while loading; a brief
  loading splash appears instead.
- **Minimize to Tray** — the setting now does what it says: closing the
  window hides PQ Companion to the system tray (with a tray icon to
  restore it or quit) instead of quitting.

### Fixes
- The Items page no longer comes up blank on a fresh launch (it used to
  stay empty until you clicked another tab).
- Database browsing (items, NPCs, drops, vendors) is a little snappier.

## v0.13.0 — 2026-06-15

A large feature release: a new Gear Upgrade Finder, built-in quest
walkthroughs, a reworked trigger/regex system, custom timers, log replay,
and a rebuilt navigation layout.

### Highlights
- **Gear Upgrade Finder** — a new per-character, per-slot upgrade scanner
  that ranks gear with cap-aware stat scoring (it knows when a stat is
  already maxed vs. still scaling), editable per-class weights, an all-slots
  overview, and weapon/ATK/haste scoring. Pairs with a wishlist you can star
  upgrades into.
- **Quests** — self-contained quest walkthroughs are now built into the app:
  a Quests section in the database explorer, a Quests tab on item pages
  (rewards + turn-ins), and full quest-chain to-do lists for multi-step keys.
- **Trigger & regex rework** — triggers now support multiple regex patterns
  each (with per-row toggles), GINA-style `{c}`/`{target}` token
  compatibility, custom categories with drag-and-drop reordering, target-name
  capture into timers and alerts, and per-pattern timer overrides. Seven
  community trigger packs ship built-in, including class CC-break alerts.
- **Custom timers** — create manual countdown timers with their own dedicated
  overlay, including durations pulled from capture groups.
- **Log replay & browse** — replay historical log segments through the live
  pipeline to test triggers and overlays, plus a read-only Browse mode for
  viewing logs out of game.
- **Navigation rebuild** — the sidebar now has collapsible sections with
  character pages nested under each character, and smoother scrolling.
- **Overlay controls** — a global "Position overlays" mode and "Manage
  overlays" menu, a "Display only" click-through HUD lock mode, one-click
  reset to recover off-screen overlays, customizable trigger alert text style
  (color, glow, font, size), and optional fade-out of overlay chrome.
- **Player tracker** — per-player notes and a PVP flag, an audible + on-screen
  warning when a PVP-flagged player appears in `/who`, and automatic tracking
  of tells and group joins.
- **Combat meter** — a Combined (pooled) view, a "Last 20 mobs" rolling-average
  scope, expanded per-pet damage, and spell/melee crit counting.
- **Inventory** — hide empty bags by default, scope the tracker to imported
  characters, flat cross-character search, and an item Characters tab showing
  who holds an item.
- **NPC overlay** — a player info + timers tab when you target another character.
- **Settings** — settings now autosave as you change them (no more
  Save/Discard), plus a new About tab. Donations have moved to Ko-fi.
- **Planes of Power preview** — an optional `pop_enabled` toggle that gates the
  PoP-era level cap and content for testing ahead of the era launch.

### Fixes
- Drag-and-drop (reordering triggers, moving them between categories, and
  reordering the wishlist) now works on Windows — it was silently broken by the
  title-bar/sidebar drag regions.
- Buff-duration modeling corrected: SPA 137/141 focus limits enforced, AA
  duration extensions now apply to off-class clickies, and the Permanent
  Illusion override is honored.
- CH chain now matches your own shout/OOC casts; outdated pinned chain patterns
  are migrated.
- Combat meter now counts spell crits and the player's own melee crits.
- Spell timer overlays (detrimental, buff, CH chain) scroll again when popped out.
- Smaller fixes: spellset edit-state alignment, a log-replayer crash on stop,
  out-of-order quest search results, and a black screen when switching to a
  character with an empty gear slot.

## v0.12.3 — 2026-06-08

Trigger alert overlays now behave reliably on multi-monitor setups, with a
new monitor picker.

### Highlights
- **Overlay monitor** — on multi-monitor setups, a new picker in the Overlays
  tab chooses which monitor trigger alert text (and the positioning card)
  appears on. Single-monitor users see no change.

### Fixes
- Trigger overlay text positioning now works reliably across multiple monitors.
  The previous release stopped the app from freezing, but the draggable card
  could still vanish when dragged toward another screen, or never appear at all
  if the app was started on certain monitors — a side effect of the alert
  overlay stretching across every display, which doesn't map cleanly across
  monitors that use different scaling. The overlay is now confined to a single
  chosen monitor, so the card stays visible and lands where you drop it.

## v0.12.2 — 2026-06-06

Two overlay fixes: a multi-monitor positioning lockout and CH chain ordering.

### Fixes
- Setting a trigger's on-screen text position no longer freezes the app on
  multi-monitor setups. While positioning, the alert overlay spans every monitor
  and captures all mouse input, so if the draggable card appeared on a screen you
  weren't looking at (or behind a fullscreen game) every click was swallowed and
  the app seemed frozen. The card now opens centered on the monitor the main
  window is on, can no longer land in a gap between displays, and pressing Escape
  always cancels positioning no matter which window has focus.
- The CH Chain overlay now lists heals in the order they were cast instead of
  alphabetically. Chains that use letter-based call numbers were being sorted by
  name; bars are now ordered by cast time.

## v0.12.1 — 2026-06-05

A fix for NPC level display: range-spawning NPCs now show their full level range.

### Fixes
- NPCs that spawn within a level range (e.g. a Shissar Revenant at 50–54)
  previously displayed only their lowest possible level, which could
  mislead on level-gated mechanics like charm caps. They now show the
  full range (e.g. "50-54") everywhere a level appears: the NPC overlays
  (dashboard and pop-out), the Database Explorer NPC detail and list, the
  Zones NPC list, and global search.

## v0.12.0 — 2026-06-05

A raid-utility release: Complete Heal chain overlays, a Loot Tracker, a
multi-channel Chat History, and an automatic spell-shopping route planner.

### Highlights
- **CH Chain & Metronome** — track raid Complete Heal chains in real time:
  cast-to-land bars, live measured cadence, and a stall indicator, plus a
  personal metronome overlay. Both are first-class dashboard panels with
  their own Overlays toggles
- **Loot Tracker** — a dedicated page that logs drops as they happen, with
  clickable items (detail popup) and zones (jump to the Zone browser) (#135)
- **Chat History** — the Tell Tracker grew into a full multi-channel chat
  log with per-character tabs and chat-style conversation threads (#136)
- **Shopping route planner** — pick spells on the checklist and the app plans
  an efficient vendor shopping run: a greedy set-cover solver, distance-aware
  sourcing, Druid/Wizard teleports modeled as a Nexus hub, a Plane of
  Knowledge toggle, town exclusion, and alignment/start-zone ordering
- **NPC caster summary** — NPC pages and the NPC overlay now headline an
  NPC's class, key procs, and signature spells, with clickable proc spells;
  the overlay also gains an optional faction section
- **Rechargeable Items** — the inventory tracker adds a Rechargeable Items
  section, and limited-charge clicky items show their remaining charge count
- **Log Backfill** — unified into the Logs tab and now runs in the background
  with a bottom progress bar, plus a 30-day log trim and wizard-driven
  backfill
- **Navigation settings** — a new Settings → Navigation tab lets you hide and
  reorder sidebar tabs
- **Spell checklist** — added a spell-name search filter and per-spell
  selection for shopping runs
- **Gear layouts** — swap gear-display layouts between the Gear tab and the
  Inventory tab

### Fixes
- Ear, Ring, and Wrist slots no longer show empty for Zeal `_pq.proj` exports
  (format-1 equipment slot names are now normalized) (#137)
- Corrected Offense/weapon skill IDs used for the ATK rating
- Opening Chat History no longer black-screens with no chat rows, double-loads
  its spinner, or shifts layout on load
- Fixed a Primal Avatar crash from an empty buff-modifier resolution, and
  hardened the Spell Modifiers panel against bad resolutions
- Ambiguous Shissar/Brood self-lands now resolve to the correct targeted timer
- Duplicate-named bosses headline the strongest matching NPC and collapse the
  rest
- Switching between items/spells/NPCs/zones no longer flickers the detail panel

## v0.11.0 — 2026-06-02

A tradeskills release: a new Recipes browser, accessibility options, richer
character stat breakdowns, and a wave of trigger and overlay fixes.

### Highlights
- **Recipes browser** — new Recipes page in the database section, with full
  tradeskill recipe lookups, named combine-station containers, and a global
  favorite-recipes store; item pages gain a Tradeskills tab listing every
  ingredient
- **Accessibility** — app-wide zoom and a high-contrast text mode in Settings
  for readability (#130)
- **Spell checklist** — a "Where to get it" sources button shows where each
  spell can be acquired
- **Trigger captures** — regex capture groups can now be substituted into
  trigger action text (#132)
- **Character breakdowns** — hover popovers detail the sources of Haste, Spell
  Haste, Damage Shield, ATK rating, and HP/mana regen, replacing the laggy
  native tooltip (#128)
- **Per-overlay lock behaviour** — each overlay's locked-mode behaviour is now
  configurable in the Overlays tab

### Fixes
- Buff durations now use the EQMacEmu formulas, fixing Forlorn Deeds and other
  spells that showed incorrect durations (#131)
- Zeal `/outputfile` exports are recognized in both naming formats (#133)
- Fletching Mastery is no longer offered as an AA to non-ranger classes (#134)
- The trigger overlay no longer steals game focus, Set Position is recoverable
  on multi-monitor setups, and Escape reliably closes modals
- Instant-clicky spell timers no longer collide via item lookup
- Corrected NPC run-speed percentages
- Back navigation preserves search and drill-down state and steps through item
  selections across explorers
- Self spell timers clear correctly on your own death
- Duplicate-name items collapse in spell cross-references

## v0.10.0 — 2026-05-30

A big feature release: a new Lockouts tracker, character stats now computed from real Project Quarm formulas, NPC respawn timers, and multi-monitor overlay support.

### Highlights
- **Lockouts tracker** — new Lockouts page with live `/sll` countdowns, parsing loot and legacy lockouts per character
- **Character stats** — stats are now derived on the backend from real Project Quarm formulas (HP/mana/AC/resists), including AA passive stat bonuses resolved from the game data, instead of frontend approximations
- **NPC respawn timers** — new death/respawn timer overlay, with Quarm's fast-respawn reduction applied to death timers
- **Multi-monitor overlays** — the trigger overlay can span all monitors, and overlay windows can be dragged across monitors
- **Combat tab** — Combat Log and History merged into one sidebar tab with sub-tabs; game-generated pet names are now attributed to their owner in DPS
- **NPC overlay** — same-name NPCs disambiguated by zone and player position; wishlisted drops are highlighted in the loot section
- **Database explorer** — duplicate-name items and spells collapse to a single canonical row with links to the variants
- **Key tracker** — shows the bag/bank location of held key components
- **Window state** — the main window remembers its size, position, and maximized state between launches
- **Overlays** — locked overlays become interactive on hover for scrolling and per-row actions

### Fixes
- Overlay positions are preserved across auto-updates, and popout windows no longer flicker on open
- Fixed duplicate WebSocket connections racing on startup
- Charm spell timers are handled correctly — cleared on charm-break, kept when an unrelated mob dies, and no longer spawn phantom duplicates
- Other players' clickies and NPC self-buffs no longer flood the buff overlay
- Detrimental timers drop correctly when targeting a Zeal corpse
- Character Info resist order now matches the NPC overlay (MR/CR/FR/DR/PR), and the Defense skill uses the correct skill ID for AC
- Debuff trigger patterns broadened and bard song durations corrected across all class packs
- NPC run speed percentage and level-scaled movement spell range fixed; run speed now shows on the popped-out overlay
- Clarified confusing "Scheduled" labels in the config backup list

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
