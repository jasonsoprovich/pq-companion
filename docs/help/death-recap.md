---
title: "Death Recap"
slug: death-recap
section: "Live & combat"
order: 30
routes: []
keywords: []
---
# Death Recap

**Prerequisite:** `/log on` and PQ Companion v0.33.1 or later.

Every death records the 20 seconds before it, so after a wipe you can see exactly what happened instead of scrolling back through the log.

### What a recap shows

- **Damage taken** — every hit and miss on you, with the biggest hit called out.
- **Healing received** — heals that landed, including Complete Heals, and the time since your last CH.
- **Killer** — the mob named in “You have been slain by …!”, or the last thing that hit you when the log only says “You died.”

### Where to find it

The **Combat Log** page lists deaths from the last 30 days, grouped by raid night (6am to 6am). Click a death to expand its recap. The **Death Recap** overlay shows your latest death as a dashboard card with a pop-out window. Recaps follow your combat history retention, and deaths aren't saved during log replay.

### No recaps in v0.33.0

v0.33.0 only recognised a death line ending in a period, but Project Quarm writes “You have been slain by …!”, so most deaths were never recorded and the Combat Log and overlay stayed empty. v0.33.1 fixes this. Deaths from before the update can't be added afterwards, since replaying old logs doesn't save deaths. The same fix means your buff timers clear, the Threat Meter resets your hate and the NPC overlay drops its target when you die.
