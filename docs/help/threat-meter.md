---
title: "Threat Meter"
slug: threat-meter
section: "Live & combat"
order: 40
routes: []
keywords: []
---
# Threat Meter

**Prerequisite:** `/log on`.

A personal, per-mob hate estimator built entirely from your own combat log — no client hooks, no party-wide data. It watches the things you do that generate aggro and keeps a running hate total for each mob you're fighting, so you can see when you're climbing toward the top of a mob's list before you actually rip it off the tank.

### What it counts

Hate is accumulated from the damage you deal, the instant aggro of hate-generating spells, the standard hate every cast carries, healing done, and the smaller adjustments around misses — then scaled by any hate-modifying buffs you have running. Feign death and your own death reset your hate to zero the way the server does, so the estimate tracks the real fight rather than drifting.

Because it's modelled from *your* log lines, the meter only knows your own hate — it can't see what the tank or the rest of the group is generating. Treat it as a personal "how hot am I right now" gauge, not an absolute raid-wide aggro table.

### Live rate

Each mob card shows a live hate-per-second rate measured over a rolling window on a receive clock, so a burst of nukes reads as a spike you can feel in the moment and then settles as the window moves on. It's the early-warning signal that you're pulling ahead of the tank's threat generation.

### Solo vs. Raid mode

The personal (Solo) meter is always available. An experimental Raid mode, switched on under Settings › Developer › Flags, adds a Solo/Raid toggle that tries to estimate every nearby player's hate from the damage in your log. It's unreliable — out-of-range players, other players' DoTs, heals, taunts and utility casts aren't in your log, so DoT and healer classes read far too low — so treat it as an experiment. Its per-class and per-player tuning appears in Settings › Overlays once enabled. Evac, Succor, Call of the Hero and circle ports zero the departing player's hate.

### Where it lives

The Threat Meter is a first-class panel on the **Overlays** dashboard with its own toggle, and pops out as its own click-through transparent window to float over the game next to the DPS meter.
