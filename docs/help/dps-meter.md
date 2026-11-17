---
title: "DPS Meter & Combat Log"
slug: dps-meter
section: "Live & combat"
order: 10
routes: ["/combat/log","/combat"]
keywords: []
---
# DPS Meter & Combat Log

**Prerequisite:** `/log on` in-game. The meter parses your combat log live.

Tracks per-fight and session-wide damage for you and your group. Each NPC is its own fight: a multi-mob pull archives one entry per mob with its own combatant list and DPS numbers, instead of pooling everything into one row. Pet and charm damage rolls up under the owner so a chanter's charmed pet doesn't show as a separate combatant; if a former charmed pet starts attacking you, the binding is dropped automatically.

All three damage sources Project Quarm exposes in the log are captured: melee, direct spell damage, and damage-over- time ticks. DoT ticks were added in v0.6 — necros, druids, and shamans now show their actual damage instead of just their nukes. Critical hits are tracked per combatant (visible on row hover and in clipboard exports) using Quarm's standalone "Scores a critical hit!" log line.

### Three DPS modes

The DPS panel header has a rate-mode toggle that cycles through the three metrics that actually matter when you're ranking damage. The current mode shows in the toggle button and the column header so you always know which you're reading; the choice persists across sessions and syncs to the pop-out window.

- **Personal** (default) — total damage divided by *your* first-to-last span. Fair to the individual: a wizard who joined the fight late or a necro who ran OOM early isn't punished for time they weren't engaged. Matches EQLogParser's headline column. Best for judging a single player's class performance.
- **Raid** — total damage divided by the *raid's* first-to-last span (the same denominator for every combatant in the fight). Best for ranking players against each other in one fight: it charges late joiners and OOM casters for the time they weren't contributing, so "who actually killed this boss" is the answer.
- **Encounter** — total damage divided by the fight's wall-clock duration. Useful for comparing whole fights to each other ("did we kill this boss faster than last week?"). The metric the meter has always emitted.

#### Worked example

A 3-minute boss fight with three damage dealers. The Tank swings the whole time, the Wizard arrives 30 seconds late, the Necro runs OOM at the 2-minute mark.

| Player | Damage | Personal | Raid | Encounter |
| --- | --- | --- | --- | --- |
| Tank | 100,000 | 555 | 555 | 555 |
| Wizard | 200,000 | 1,333 | 1,111 | 1,111 |
| Necro | 150,000 | 1,250 | 833 | 833 |

The Tank engaged for the full 180 s so all three numbers agree. The Wizard's Personal DPS divides by their 150 s window (200,000 ÷ 150) instead of the 180 s wall-clock — the late entry isn't held against them. Raid mode uses the 180 s raid span for everyone, so the Wizard's number matches the Encounter number here. The Necro stops casting at 120 s, so their Personal DPS is the highest of the three (150,000 ÷ 120) but their Raid contribution is third (150,000 ÷ 180).

### HPS view

A **DPS | HPS** toggle in the meter's header switches to healing. Project
Quarm only logs your own heals, so the HPS view lists who *you* healed and
how much, per target.

### Session breaks

The Combat Log and Combat History pages insert a "Session break" divider between fights separated by 120 seconds or more of inactivity. Long medding pauses, between-pull breaks, and "back from dinner" gaps become legible at a glance instead of running together. The divider is visual only — DPS calculations, fight ordering, and the data behind everything are unaffected.

### Per-class bar colors

Combatant bars on the DPS panel (and rows on the Combat History page) are colored by class — Cleric blue, Warrior red, Necro purple, and so on. Class is resolved server-side from your `/who` history, with the active character's stored class used for **You**, so pets automatically inherit their owner's color and unfamiliar combatants fall back to an Unknown swatch. The palette lives in the **DPS Meter** overlay's settings in the **Overlays** hub — hex inputs plus native color pickers per class, with per-row and global reset to defaults if you want to start over.

### Bar scaling & your own row

In a big raid fight, share-of-total bars sit nearly empty. Bars scale to the top damage dealer by default, with a header toggle to switch back to **share of total** — the % column always shows share of total. If your own row is scrolled out of view, a copy with your rank stays pinned under the table. Both work the same on the dashboard panel and the pop-out overlay.

### Sharing fights

One-click copy of any fight or session summary to the clipboard, formatted for `/gu` and `/tell`. The current DPS mode is included in the header so recipients know which metric they're reading. Click any combatant row to expand their breakdown (hit count, max hit, crits) before copying.

### Combat Log vs Combat History

The **Combat Log** page is the live, in-memory view of the current session — the last 20 fights, updated in real time. The **Combat History** page is the persistent archive of every fight you've recorded, with date-range and per-character filters. See the next section for full detail.

### The parse survives zoning and death

Zoning or dying no longer wipes your active parse. A fight only ends when the mob dies, after a configurable inactivity timeout (the DPS Meter overlay's settings in the Overlays hub; raid encounters get a higher floor), or when you end it yourself with the discard button — which clears the live meter but still archives the fight to Combat History. Return to a mob you left mid-fight and the same fight picks back up instead of starting a new one.
