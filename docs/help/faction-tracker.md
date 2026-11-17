---
title: "Faction Tracker"
slug: faction-tracker
section: "Progress & goals"
order: 40
routes: ["/characters/factions"]
keywords: []
---
# Faction Tracker

**Prerequisite:** `/log on`.

EverQuest never logs a faction's absolute value or the exact point amount a change moved it by, so this tracker builds the closest thing to one from what actually is logged: every "Your faction standing with X got better/worse" line, plus every `/con` result. It uses the same "record everything encountered" approach as the Lockout and Player trackers — no need to pin a faction before it starts tracking; every faction you've ever killed toward or considered gets a persisted tally automatically.

### Estimates, not exact values

The running better/worse tally gets a best-effort point estimate whenever a change correlates to a resolved kill (matched against the database's faction hit tables), and an exact numeric delta when it correlates to a quest turn-in (parsed from the quest script's own dialogue). Everything else stays direction-only. A faction with no recorded `/con` shows "Needs /con" — that's the prompt to set a baseline.

### /con disposition bar

Each tracked faction shows a 9-segment bar spanning the full disposition spectrum (scowls → ally) built from your most recent `/con` reading. A reading taken while you had an illusion active is flagged suspect, since illusions can shift how an NPC considers you.

### Pinning & search

Pinning a faction (search + star) is purely a display favorite — unpinning never deletes its tracked history. Searching surfaces any faction's known data whether pinned or not, even for a character other than the active one.

### Log Backfill

Faction Tracker is included in **Settings → Logs → Log Backfill**, so existing `/con` history already in a character's log can seed a starting baseline. Backfill only scans `/con` lines and keeps the most recent reading per faction — it doesn't replay kill or quest history into the better/worse counters, since a static log replay can't reconstruct the buff-timer state (like an active illusion) needed to flag a backfilled reading suspect. Re-running backfill is always safe.
