---
title: "Players Database"
slug: players
section: "History"
order: 30
routes: ["/players"]
keywords: []
---
# Players Database

**Prerequisite:** `/log on` — every `/who` and `/guildstat` you run gets captured automatically.

A searchable, sortable database of every player you've seen via `/who` and `/guildstat` — name, class, race, guild, last-seen zone, last-seen level, plus a per-character level history. Captures `/who`, `/who all`, `/who guild`, and `/guildstat` output. Filter by class, guild, or zone; click any row for the full level progression.

### Raid roster (with Zeal)

While you're in a raid, Zeal reports the full roster over the pipe, so every raid member's name, level and class is added to the database without anyone running `/who`. The same data also feeds class attribution in the raid threat estimate.

### Anonymous-aware

Anonymous and roleplay sightings don't overwrite class and level data you already had from a non-anonymous sighting — the row just gets a new last-seen timestamp, so going /anon doesn't erase a known player from your database.

### Class title aliases

EQ titles like "Lord", "Necromancer Lord", or "Crusader" collapse into the right base class automatically, so a 50+ player isn't filed under a different class from their under-50 self.

### Notes & PVP flag

Add your own free-text note to any player, and flag the ones you'd rather steer clear of. When a flagged player turns up in a `/who`, you get a sound and an on-screen warning so you notice before they notice you. A global toggle disables the PVP warning if you don't want it. Notes and flags are kept separate from sighting data, so they survive a Clear-all.

### Automatic interaction tracking

Tells and group joins are logged as interactions automatically, so the people you actually play with surface alongside the players you've only seen in `/who`.
