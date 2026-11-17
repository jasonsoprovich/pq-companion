---
title: "Spell Checklist"
slug: spell-checklist
section: "Character"
order: 20
routes: ["/characters/spells"]
keywords: []
---
# Spell Checklist

**Prerequisite:** Zeal spellbook export.

Cross-references every spell your class can learn against your Zeal-exported spellbook so you always know exactly which spells you still need to scribe. Filter by class and level range, narrow the list with the spell-name search box, and click any spell to jump to its full database entry. NPC-only and unobtainable spells are hidden by default, and a "Where to get it" button on each spell surfaces where it can be obtained.

### Shopping-route planner

Tick the spells you want to pick up and hit **Plan shopping route**: the app works out an efficient vendor run that covers every selected spell with the fewest stops. It's a greedy set-cover over the merchants that sell each spell, made distance-aware so nearby vendors are preferred, with Druid and Wizard ports modelled as a Nexus hub. Route controls let you exclude specific spells, filter towns (handy for alignment-restricted characters), and set the start zone so the route begins where you actually are. Whatever the start zone sells is bought there first, so starting in Plane of Knowledge keeps everything PoK carries in the first stop instead of spreading it across other towns.

### Passing on a spell

Spellbook full, or a scroll your class will never use (the cleric Imbue spells)? Click the eye icon on a missing spell to **pass** on it: it drops out of the missing count, your route selection, and the shopping route. Passed spells are remembered per character and collected under the **Passed** filter, where the same icon un-passes one if you change your mind.
