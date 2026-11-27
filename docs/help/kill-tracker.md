---
title: "Kill Tracker"
slug: kill-tracker
section: "History"
order: 20
routes: ["/kills"]
keywords: []
---
# Kill Tracker

**Prerequisite:** `/log on`.

Parsing → Kill Tracker records every kill your log shows and counts kills toward the milestones that drive scripted Planes of Power raids.

### Kill Log

Every kill with the time to the second, the killer and the zone. Sort by any column, filter by zone, time window or named mobs only, or switch to By mob for kill counts and how long ago each mob last died. Backfill fills in kills from your old logs.

### Kill Counters & event presets

Count kills toward milestones, with an on-screen or spoken alert as each one approaches, and follow them in game with the Kill Counters overlay (dashboard panel and pop-out). One-click presets are built from Project Quarm's own quest scripts: Bertoxxulous (trash toward each of the eight Adan kings, kings toward the final four and Bertox, and the 123-minute event clock), Coirnav, Fennin Ro, the Plane of Earth Stone Ring, Ston\`Ruak, Drornok, and Plane of Time phase 3. You can also build your own from mob names or any log line.

Counters correct themselves from the server's own announcements, such as the boss-spawn emotes, so kills your log didn't see don't throw them off. For raid timers that run on a server clock rather than a kill count, see the Raid: Scripted Event Timers trigger pack.

A counter tied to a zone keeps a separate count for each copy of that zone (read from `dbg.txt`): entering a different copy starts the count over, and coming back to the same copy picks it up again. Counters without a zone stay global.
