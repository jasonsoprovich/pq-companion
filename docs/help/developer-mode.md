---
title: "Developer mode"
slug: developer-mode
section: "Getting started"
order: 50
routes: []
keywords: []
---
# Developer mode

A hidden Settings tab unlocked with <kbd>Ctrl + Shift + D</kbd>. Aimed at contributors and curious power users — not required for any normal feature.

Once unlocked, the Developer tab exposes a read-only SQL sandbox over the Project Quarm database (the same database the explorer reads from) with ten curated starter queries, an interactive schema graph showing how the tables relate, and a Mermaid ER diagrams panel for the most-asked-about subsystems (items, NPCs, loot, spells). Queries run through a guarded backend endpoint that enforces read-only access — no write or schema statements are accepted.

The [Spell Emote Customizer](help:spell-emote-customizer) has its own Spell Emotes sub-tab here, and a Flags sub-tab hosts features still being proven out — currently the experimental raid-wide [Threat Meter](help:threat-meter) mode. Once a feature has had enough real-world use, its toggle graduates out of Developer mode and it becomes a normal always-on feature — the [Faction Tracker](help:faction-tracker), [PoP Flagging Tracker](help:pop-flagging), and [Raid Composition](help:raid-composition) all started here.

You can save queries you reuse, give them names, and export a set as a query pack to share or re-import on another machine. Each tab keeps its own query history, and wide result sets scroll horizontally so big SELECTs stay readable.
