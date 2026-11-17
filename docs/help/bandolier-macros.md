---
title: "Bandolier & Macro Editors"
slug: bandolier-macros
section: "Character"
order: 40
routes: ["/characters/bandolier","/characters/macros"]
keywords: []
---
# Bandolier & Macro Editors

**Prerequisite:** Zeal-exported inventory for the bandolier editor; your live in-game `.ini` for the macro editor.

Two editors for the files Zeal already reads, right inside the app instead of hand-editing an .ini in a text editor.

### Bandolier editor

Edit your weapon-swap bandolier sets with a guard that keeps you to gear you actually own and can equip — checked against class, race, and level — with item hover cards and a preferred storage bag per character.

### Macro editor

The in-game macro grid mirrors EverQuest's real 2-column social layout, with drag-to-rearrange slots and a one-click reset back to the page 1 defaults. Edits write directly to your character's live `[Socials]` section without touching the rest of the file. A built-in `/cancelbuff` builder turns a buff-name search into ready-made cancel-by-name macro lines, and macros can be copied from one character to another. Moving or copying a macro between pages uses an explicit picker instead of cross-page drag-and-drop — pick the destination page and slot, then choose **Swap** or **Replace** if that slot is already occupied.

### Page 10 keybinds

The Macros tab shows each Page 10 social slot's keybind as a badge, and you can edit its primary and alternate keys there. Binds are read from and saved to `eqclient.ini`, per-character keybinds from Zeal are respected, your config is backed up before saving, and you're warned if a key is already used by another bind.

### Hiding mule tabs

The character ribbon here (and on every other per-character page, plus the sidebar switcher) can be trimmed of mules and alts you never play. The show/hide control lives on the Active Characters page — toggling a character there removes it everywhere. A character that only has a macro file on disk and no log yet still shows up there so its tab can be hidden.
