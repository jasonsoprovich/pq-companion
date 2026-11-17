---
title: "Combat History"
slug: combat-history
section: "Live & combat"
order: 20
routes: ["/combat/history"]
keywords: []
---
# Combat History

**Prerequisite:** `/log on`. Fights are saved automatically as you encounter them — no manual export step.

Every archived fight is written to a local SQLite database (`~/.pq-companion/user.db`) so the in-memory last-20 ring buffer is no longer the only record. Re-open a fight from last week to grab a parse for a guild post, browse a raid night by zone, or dig up the parse from that one Naggy attempt that mattered — the data sticks around.

### Filtering

The header has dropdowns and inputs for the four most common filters:

- **NPC name** (substring, case-insensitive) — find every fight against "Naggy", every Frenzied Ghoul, etc.
- **Character** dropdown — populated from the characters that actually appear in your saved fights, so there's no guessing at spelling.
- **Zone** dropdown — same: only zones with saved fights show up.
- **Date range** with one-click presets: *Last 24h*, *Last 7d* (default), *Last 30d*, *All*, or *Custom* with date pickers. Preset clicks apply immediately; the other filters apply on Enter or the Apply button.

Click any row to expand the full combatant and healer breakdown for that fight. Per-row delete and a clear-all button live in the header — both pop a themed confirmation before destroying anything.

### Retention

Saved fights are pruned after 30 days by default — enough to cover a typical week of raiding without the database growing without bound. The retention window is configurable in `config.yaml` under `combat.retention_days`: set a higher number to keep fights longer, or `-1` to disable pruning entirely. Pruning runs once at startup and once per hour while the app is open.

### Storage size

A fight is typically 1–10 KB. A heavy week of raiding produces a few thousand fights at most — single-digit megabytes total. SQLite's WAL journal keeps writes non-blocking even when the rest of the app is touching the same database for triggers, character data, and backups.

### Finishing blows

Instant-kill specials — finishing blow, slay, assassinate — log as an ordinary hit of about 32,000 damage, far more than the mob had left. Hits of 32,000 or more are counted as finishing blows on the attacker's row (shown in its tooltip) instead of as damage, so one of them can't double a player's total for the fight. The Threat Meter ignores them too.
