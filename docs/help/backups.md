---
title: "Backup & Restore"
slug: backups
section: "Getting started"
order: 40
routes: []
keywords: []
---
# Backup & Restore

**Settings → Backup & Restore** has one tab per job: Character files, Snapshots, Move to another PC, and Reset.

### Character files

Each EQ settings file keeps its own last 30 versions: per character, spellsets, bandolier, hotkeys and macros, UI layout and bazaar, plus the shared `eqclient.ini`, `zeal.ini` and `quarmclient.ini`. A version is captured within a minute of a change and stored compressed in the app's database, so it also travels in the app export. Pick a character, open any file's history, and view or restore one version on its own. Before restoring, the current file is saved as a version, so a restore can be undone. History is on by default, with the toggle and the per-file version count on the same tab.

### Move to another PC

Exports your entire PQ Companion state — settings, triggers, character data, EQ config snapshots — into a single `.pqcb` bundle. Drop the bundle on another machine, import, and you're exactly where you left off. The panel is always visible under **Settings → Backup & Restore** so it's there when you need to move to a new PC. It carries your blocked buffs, snapshots and file history too.

The bundle carries a manifest with a version and schema check; a forward-incompatible bundle is refused at import instead of silently corrupting the destination. On import, the existing data is renamed in place rather than deleted — if anything goes wrong you have a clean rollback path.

### Snapshots

One-click backup and restore for every Project Quarm `.ini` file — UI, hotkeys, socials, the lot. Snapshot before a risky UI overhaul, roll back instantly if something breaks. A new snapshot can include your log files, and a restore lets you pick which files to put back (logs are unticked by default).

### Auto-backups

Configure auto-backups on a custom schedule (e.g. once a day) or on file change (every time the game writes the file). Browse, label, and restore previous backups from a single list. The Backup Manager header has an **Open folder** button that jumps straight to the on-disk backup directory — useful if you want to copy a snapshot somewhere else or archive it manually.

Backup files live under your home directory (e.g. `~/.pq-companion/eq-config-backups/`) so they're independent of the install location. Older installs that wrote backups beside the app are migrated to this directory automatically on first launch.
