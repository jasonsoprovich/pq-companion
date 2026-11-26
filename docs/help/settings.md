---
title: "Settings & data sources"
slug: settings
section: "Getting started"
order: 20
routes: ["/settings"]
keywords: []
---
# Settings & data sources

Open **Settings** from the sidebar. You'll set up two paths and a name:

- **Project Quarm folder** — the directory containing `eqgame.exe`. The app finds your combat log and `.ini` files from here.
- **Character name** — used to pick the right log file (`eqlog_<Character>_pq.proj.txt`) and to tag Zeal exports per character.
- **Zeal folder** (optional) — only needed if Zeal writes its exports somewhere other than the default. The app will detect this automatically in most cases.

The character switcher in the top-right lets you flip between any character with a Zeal export. The whole app — progression, inventory, spell checklist, to-dos — follows the active character.

Settings save automatically as you change them — there are no Save or Discard buttons to remember.

### Feature health

The top of **Settings → General** lists everything PQ Companion reads from the game and whether it's working, in three groups:

- **Setup** — EverQuest folder, character, log file and when the log was last written. Grey log activity just means EverQuest is closed.
- **Logging** — the app's log parsing switch, **EverQuest logging** (sets `Log=TRUE` in `eqclient.ini` so the game logs on every login) and **Zeal output on camp** (Zeal exports inventory, spellbook and stats when you /camp). The two ini switches need EverQuest closed, since the client rewrites its files on exit; a backup is saved first.
- **Zeal** — whether Zeal is installed and current, the live pipe connection, and Zeal's in-game settings PQ Companion benefits from, each with the command to turn it on and a copy button.

Click any row to see what it does, which features use it and how to turn it on. The Getting Started checklist uses the same checks.

### Master Volume

Settings has a master volume slider that scales every trigger sound, TTS utterance, event alert, and timer alert on top of each action's per-trigger volume — dampen everything together without losing the relative balance between individual triggers.

### Alert output device

**Settings → Audio → Alert Output Device** sends trigger sounds and spoken alerts to a speaker or headset you choose, separate from the Windows default — for example alerts in your headset alongside Discord while the game stays on speakers. A Test button plays a sample, and if the chosen device is unplugged, alerts fall back to the default device.

### Accessibility

Settings is organized around a grouped side navigation, and an Accessibility section gathers high-contrast text mode, app-wide zoom, and the per-overlay font/zoom sliders (see the Overlay Dashboard section) in one place.

### EQ client version

A status panel under Settings shows the `eqgame.exe` version detected at your configured Project Quarm path. Useful for confirming you patched after a server-side update; the panel uses the file's FileVersion as the primary signal so patch-day MD5 churn doesn't flip the status to "unknown".

### Zeal integration

The **Zeal integration** card shows whether Zeal is installed, its version, and a soft warning when a newer release is out or you're below the minimum supported version. The live pipe, output on camp and Zeal's in-game settings are in Feature health above it.

### Navigation

A **Settings → Navigation** tab lets you hide and reorder the sidebar tabs, so you can trim the app down to just the features you use and put them in the order that suits how you play. You can also pin frequently-used nav items so they stay pinned to the top of the sidebar, above the rest of your tabs.

### Log Backfill

Log Backfill lives in the **Logs** tab and walks your existing combat logs to populate history — combat, chat, loot, and the rest — for events that happened before you installed the app. It runs in the background with a progress bar along the bottom so you can keep using the app while it catches up. The Logs tab also surfaces log and Zeal status toggles, wizard diagnostics, and notices when an expected log file is missing.

### Archive & Trim

EverQuest log files grow forever. **Archive & Trim** on the Logs tab zips up the old part of a log and keeps either the last N days or the newest N MB, so the app (and the game) aren't working with a multi-gigabyte file. Running it twice in one day keeps both archives.

When a log gets large, an orange dot appears on the sidebar **Settings** button. Clicking it opens Settings → Logs, where the Logs tab carries a matching dot. The warning size can be changed or turned off on the same tab.

### Danger zone: app reset

**Settings → Backup & Restore** has a "Danger zone" for starting over. **Reset app data** clears your tracked data — characters, chat, loot, triggers, tasks, notes, flags, combat history — but keeps your settings and EQ folder path. **Factory reset** also clears settings and reopens first-time setup. Both require typing `RESET` to confirm, and your old data is set aside for recovery rather than deleted outright. Your EverQuest game files and logs are never touched.

### Moving to a new PC

Exporting your setup for another machine now imports in one "Import & restart" step instead of a multi-stage prompt. See the Backup & Restore section for the export side.

### About

A **Settings → About** tab shows the app version and links out to the community Discord, the GitHub repository, the website, and the project's Ko-fi page if you'd like to support development.

### Changelog & "What's New"

After an auto-update, a one-time popup lists every release since the version you last had open, with a "don't show this again" checkbox if you'd rather not see it. The full version history is always available in **Settings → Changelog** — the ten most recent releases are shown by default, with a "Show all" to page back through the rest.
