---
title: "Raid Replay"
slug: raid-replay
section: "Live & combat"
order: 100
routes: ["/raid-replay"]
keywords: []
---
# Raid Replay

**Prerequisite:** [Zeal](https://github.com/CoastalRedwood/Zeal), in a raid.

While Zeal sends raid frames, PQ Companion records every raider's position in your zone once a second, along with the kills and deaths your log shows. **Parsing → Raid Replay** lists the recordings (zone, date, length, peak raiders, kills and deaths) and plays one back on the zone map.

### Playback

Raiders show as class-coloured arrows, and anyone who died shows as a cross where they fell. Play, pause, run at 1× to 60×, or drag the scrubber, which is marked with kills, deaths and a lane for CH calls. Switch between the 2D map and a 3D view built from your client files.

### Side panel & events

The side panel switches between the raider list and the raid's groups at the playhead, laid out like Raid Composition; click a raider to highlight them. A dashed line runs from the recorder to their target. The event list covers kills, deaths, CH calls, enrage, rampage and flurry (with who was hit), casts started by the target, NPC shouts, raid says and shouts, loot, and the recorder's own rez. Click an event to jump to it.

### Raid nights & summary

Recordings group into raid nights: one character's sessions with gaps of 45 minutes or less. A raid that moves through several zones, or that you leave to port out or respawn, plays back as one timeline, with the gaps skipped and shown as a banner.

A raid roster log records joins and leaves from Zeal's raid window, even while you're out of the raid's zone. Each night has a **Summary** view: when each raider joined and left, time in zone, times away, deaths, and time standing still. Joins, leaves and zoning also show on the timeline under "Joins & zoning".

### Recording & storage

Recording and how long recordings are kept (30 days by default) are set on the Raid Replay page and under Settings → Logs. A session ends when you zone or Zeal stops sending frames for two minutes, and sessions under a minute are dropped. Recordings are compact — about 1.4 MB for a four-hour, 54-raider night.
