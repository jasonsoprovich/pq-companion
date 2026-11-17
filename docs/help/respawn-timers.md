---
title: "NPC Respawn Timers"
slug: respawn-timers
section: "Live & combat"
order: 110
routes: []
keywords: []
---
# NPC Respawn Timers

**Prerequisite:** `/log on`.

A transparent overlay that watches NPC deaths in your combat log and counts down to respawn, so you know exactly when a named or a contested camp is due back up. The countdown starts from the kill line the moment a mob dies — no manual timers to start or stop.

### Quarm respawn rates

Project Quarm uses accelerated respawn rates compared with classic EverQuest, and the overlay applies that reduction automatically. The countdown reflects the timer on the server you're actually playing on, so you're not mentally subtracting from a classic respawn window.

### Ready alerts

An optional audio or text-to-speech alert fires the moment a tracked mob's respawn timer is up, scoped to the zone you're currently in so you only hear about camps that matter where you are. DoT and swarm-kited kills count too — a mob that dies with no melee killing blow (a bard kiting a swarm, say) still starts its respawn timer.

### Pinning a camp timer

Click the pin on any row to flag it as your priority: it sorts to the top, gets a gold marker, and stays visible at **POP** instead of clearing after the normal grace window, so you can tell which timer is the one you're camping amid a pile of trash kills. A pinned row also takes a short click-to-edit label such as “named PH”. A “pinned only” filter and an unpin-all button keep the list tidy. Zeal users can pin from the keyboard with `/pipe respawn pin`, `/pipe respawn unpin` and `/pipe respawn clearpins`.

By default a pin stays on the timer you set it on. Turn on **Auto-hand off pinned respawns** (**Overlays → Overlay settings → Timers**) and a popped pin moves to the next kill at the same spot (matched by your position when Zeal is connected, otherwise by name). It's a guess — spawn ids rotate and placeholders spawn different mobs — so it's off unless you want it. Pins and timers don't survive an app restart.

### Fast vs. normal timers

Zones flagged for reduced respawns use Quarm's fast timers in the open world and in ordinary guild instances, and the overlay applies them automatically. The one exception is the PvP instance, which uses the raw database timer. The app can't tell which kind of instance you're in, so the shield button in the overlay header lets you switch new kills to normal (unreduced) timers when you're there. Raid-target spawn points are never shortened in the open world, so they always show their real timer.

### Zone copies

The app follows the client's `dbg.txt` to learn which copy of a zone you're in. Kills are tagged with that copy, so timers from another copy of the current zone are marked, sort after yours, don't hold spawn points, and don't colour the map's respawn dots.

### Where it lives

The respawn overlay sits alongside the rest of the Overlay Dashboard and pops out as its own click-through transparent window, so you can float it over the game next to the DPS meter and spell timers.
