---
title: "Quick Start"
slug: quickstart
section: "Getting started"
order: 5
routes: []
keywords: ["setup", "install", "first run", "new", "begin", "log on", "zeal"]
---
# Quick Start

Five steps get PQ Companion running. The setup wizard walks you through
the first three on first launch; you can rerun it any time from
**Help → Tours → Re-run setup wizard**.

### 1. Point it at your Project Quarm folder

Choose the folder that contains `eqgame.exe`. PQ Companion finds your log
file and `.ini` files from there. Pick your character, or leave it on
auto-detect and the app follows whoever logged in most recently.

### 2. Turn on the combat log in-game

In Project Quarm, type `/log on`. The game then writes your log to disk,
and the DPS meter, spell timers, NPC info overlay and triggers all read
from it. No log means no live data. You don't need the game open for setup:
the wizard's **Turn on** button sets `Log=TRUE` in `eqclient.ini` so logging
starts on every login (close EverQuest first, since it rewrites that file on
exit).

### 3. (Recommended) Install Zeal

[Zeal](https://github.com/iamclint/Zeal) is a community client add-on that
exports your spellbook, inventory, AAs and stats, and streams live target,
HP and position data. Without it the app still works; the Spell Checklist,
Inventory, Character pages and some overlay details just have less data.
Install it from **Settings → General → Zeal Integration**.

### 4. Set up your overlays

The overlay wizard picks a starting set of overlays for your class and
places them on your game monitor. Afterwards, the [Overlays hub](help:overlay-dashboard)
is where you add more, arrange them, and change their settings.

### 5. Add some triggers

Open **Triggers → Packs** and install a pack for your class, or press
**New trigger** and pick a template: speak or show a message when a line
appears, start a spell timer, or warn on a mob emote. See
[Custom Triggers & Alerts](help:triggers).

### Where next

- Press <kbd>Ctrl</kbd> + <kbd>K</kbd> from anywhere to search items, spells,
  NPCs and zones, and to jump to any page, setting or help article.
- The **?** button at the top of each page opens the help for that page.
- Snapshot your EverQuest `.ini` files under **Settings → Backup &
  Restore** before changing your UI or hotkeys.
- Not seeing data? Check **Feature health** at the top of
  **Settings → General**, then [Troubleshooting](help:troubleshooting).
