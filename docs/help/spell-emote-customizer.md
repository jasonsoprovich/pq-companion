---
title: "Spell Emote Customizer"
slug: spell-emote-customizer
section: "Database & tools"
order: 30
routes: []
keywords: []
---
# Spell Emote Customizer

**Prerequisite:** [Developer mode](help:developer-mode) — the customizer is available once Developer mode is unlocked.

Edit the client-visible chat emotes in your `spells_en.txt` file from Settings › Developer › Spell Emotes, or inline on the Spells page. Some spells share an identical emote — every slow in the game says " yawns." on landing — which makes it impossible to tell from a bystander message alone which spell actually landed. The customizer lets you rewrite one spell's emote to disambiguate it, or add an emote to a spell that ships with none (Divine Aura has no cast-on-other text by default).

### Survives server patches

Your edits are stored structurally in the app's own database, not as a raw diff of the file. When Project Quarm patches and republishes `spells_en.txt`, a watcher detects the external change and offers to re-apply your customizations on top of the new file, with a per-spell field-level diff so you can see exactly what changed.

### Import existing hand-edits

If you'd already been hand-editing `spells_en.txt` before trying this feature, the app detects those changes the first time it runs and offers to import them — review each one with a red/green diff, import all at once, or dismiss the prompt for now. Nothing is silently adopted or overwritten.

### Linked trigger warnings

Editing an emote can break a trigger whose regex pattern was written against the old text — the built-in Caustic Mist / Putrefy Flesh alert is a real example, since both spells share the same landed-on-other line. The customizer scans your triggers for any pattern that references the emote you're changing and proposes a diff to apply, one match at a time — it never rewrites a pattern automatically, since a trigger deliberately matching several spells that share an emote should stay untouched.
