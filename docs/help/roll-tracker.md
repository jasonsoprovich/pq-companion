---
title: "Roll Tracker"
slug: roll-tracker
section: "Live & combat"
order: 160
routes: ["/rolls"]
keywords: []
---
# Roll Tracker

**Prerequisite:** `/log on`.

Watches the combat log for `/random` results and buckets them into per-range sessions. EQ logs each roll as two consecutive lines — `**A Magic Die is rolled by <Name>.` followed by `**It could have been any number from 0 to N, but this time it turned up a M.` — and the tracker pairs them, then groups by the upper bound. So when a raid calls Cowl 333, Shard 444, and Heart 666 in overlapping bidding windows, you end up with three independent leaderboards instead of one tangled list.

### Sessions and rolls

Each session is keyed by its dice range (the "333" in `0 to 333`). Rolls list the leader at the top under the current winner rule, with a trophy on the winning name. Re-rolls (a player rolling more than once in the same session) are kept in the list but struck through and tagged **reroll** — they're ignored for winner calculation, since most guilds treat the first roll as binding.

### Winner rule: Highest vs. Lowest

A header toggle picks the global rule. Flipping it instantly re-ranks every session — useful for guilds that use lowest-roll-wins for split loot. Ties on the winning value are all shown as co-winners; resolve them however you normally do.

### Manual mode vs. Timer mode

**Manual** mode is the default: sessions stay **Live** until you hit Stop, the user manually removes them, or no new rolls arrive for five minutes (after which a fresh drop with the same dice range opens a new session instead of merging).

**Timer** mode auto- stops every new session a configurable number of seconds after the first roll — useful when an officer calls a fixed bidding window like "you have 30 seconds to roll." The window is set in the header (5 – 600 s, default 45 s) and each Live session shows a countdown badge that turns amber under 5 s. Manually stopping a session before its timer expires cancels the pending auto-stop.

### Per-session and global controls

- **Stop** freezes a session immediately — late rolls won't merge in.
- **Remove** deletes a single session card without touching the others.
- **Clear** (header) wipes every session at once.

### Tiered contests

Group related sessions into a tiered contest using a configurable preset (or a custom editor for your own structure), and results render as shareable contest cards instead of raw session lists. Loot items are auto-suggested from nearby chat, and you can add a manual item label and copy a paste-ready result summary for guild chat. Name and save your own custom groupings and they stay in the Grouping list. A roll that falls outside every tier (a parchment called as 444, say) still shows as its own card, listed by time alongside the contests, newest first.

### Overlay

The Roll Tracker is available both as a dashboard panel (toggle it on from the **Overlays** toolbar) and as a frameless always-on-top pop-out window with the full set of header controls — mode toggle, duration input, winner rule, clear, and lock — so an officer can drive a roll call without leaving the overlay or alt-tabbing back to the dashboard.
