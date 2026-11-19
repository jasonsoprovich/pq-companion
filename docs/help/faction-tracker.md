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

Until a faction has a standing estimate, it shows a 9-segment bar spanning the full disposition spectrum (scowls → ally) built from your most recent `/con` reading. A reading taken while you were illusioned, invisible, feigned, engaged with the NPC or under a faction buff is flagged suspect, since the server answers those with something other than your standing.

### Standing estimate

Your faction is a hidden personal value, usually −2000 to +2000, plus a fixed modifier for your race, class and deity. `/con` shows the sum, so some factions run past ±2000: Claws of Veeshan sits 800 lower for everyone, so a character at the bottom cons at −2800 and needs to climb about 2300 points to reach Dubious.

The tracker starts an estimate when either of these appears:

- **"Your faction standing with X could not possibly get any worse/better."** This line only appears when you're already at the cap, so it pins the value exactly.
- **A clean `/con`**, which narrows you to that bucket's range.

From then on the bar is drawn to value, with a band showing where you must be and how far it is to the next bucket. Kills the tracker can match move the band by their database value. Changes with no kill line in view (common on raids) take their size from your recent kills of the same faction, shown as "inferred". Anything else widens the band. The next clean `/con` narrows it again, and replaces it outright if the two disagree.

XP potions multiply faction gains by an amount the client never sees, so with one running you're likely further along than the estimate shows. `/con` now and then to correct it. The race, class and deity modifier comes from your Zeal export; until one exists, factions with those modifiers show "approx. modifier".

### Pinning & search

Pinning a faction (search + star) is purely a display favorite — unpinning never deletes its tracked history. Searching surfaces any faction's known data whether pinned or not, even for a character other than the active one.

### Log Backfill

Faction Tracker is included in **Settings → Logs → Log Backfill**. It replays a character's logs, archives included, to recover each faction's most recent `/con` reading and a starting standing estimate: the last "could not possibly get any…" line or `/con`, walked through every faction change after it. It doesn't fill in the better/worse counters, which only build up live.

A replay can't tell whether a past `/con` was taken while illusioned, engaged, or under a faction buff like Alliance, so a backfilled reading is never flagged suspect and can start an estimate in the wrong place. Your next clean `/con` corrects it. Re-running backfill is always safe: an estimate the live tracker anchored more recently is kept.
