---
title: "Privacy & safety"
slug: privacy
section: "Getting started"
order: 70
routes: []
keywords: []
---
# Privacy & safety

PQ Companion runs entirely locally. It reads two things off disk: your combat log file and your Zeal JSON exports. Triggers, character notes, and combat history are persisted to a local SQLite database at `~/.pq-companion/user.db` (combat history defaults to a 30-day retention window; everything else is yours until you delete it). Nothing is uploaded anywhere; there's no account, no telemetry beyond auto-update checks against the GitHub releases API.

The app never modifies the game client or game data. It is a read-only tool — no memory injection, no packet modification, no automation that interacts with the game client. If you're worried about server rules, the rule of thumb is: anything PQ Companion does, you could do by hand with a text editor and the in-game UI; it's just a lot faster.
