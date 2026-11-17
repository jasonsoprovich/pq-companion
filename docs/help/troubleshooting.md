---
title: "Troubleshooting"
slug: troubleshooting
section: "Getting started"
order: 60
routes: []
keywords: []
---
# Troubleshooting

### The DPS meter / spell timers / NPC info aren't doing anything

All three read from your combat log. Check that you ran `/log on` in-game (it's per-character and per-session), and that Settings points to the right Project Quarm folder. The log file lives in your Project Quarm folder as `eqlog_<Character>_pq.proj.txt`.

### The Spell Checklist / Inventory / AAs are empty

Those features need [Zeal](https://github.com/iamclint/Zeal) exports. Run Zeal at least once on the character to populate the JSON, then switch to that character in PQ Companion.

### I changed a config and the game broke

Open **Backups** and restore the most recent snapshot. If you had auto-backups enabled (recommended), there's almost always a clean copy from before the change.

### The app warns that another copy is running

An older copy of PQ Companion in a different folder — often an unzipped or copied one — is already running and blocking launch. The warning names its version and location, with a **Show in Explorer** button so you can close or delete it. Portable or copied folders offer updates as a manual download instead of installing a second copy into the default location.

### Running on Linux (Wine / Proton)

The installer doesn't work under Wine, so every release also ships a portable `.zip`: unzip it and run `PQ Companion.exe` directly. It doesn't auto-update, so grab the new zip from the releases page each time. Under Wine the main window uses the native title bar and display scaling is fixed at 1× so clicks land where you point; `PQC_NATIVE_FRAME=0` and `PQC_SCALE_FACTOR` (a number, or `auto`) override these. Overlays and text-to-speech don't work on Linux — use sound-file alerts instead.

### Still stuck?

Open an issue on [GitHub](https://github.com/jasonsoprovich/pq-companion/issues) with what you tried and any console errors.
