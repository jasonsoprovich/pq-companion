---
title: "Database Explorer"
slug: database-explorer
section: "Database & tools"
order: 10
routes: ["/items","/spells","/npcs","/zones"]
keywords: []
---
# Database Explorer

A searchable database of every item, spell, NPC, and zone in Project Quarm. Item and spell icons render inline; cross- references between scrolls and the spells they teach, NPC drops and source mobs, and zones and their NPC rosters are all click-through.

### Global search

Press <kbd>Ctrl + K</kbd> from anywhere in the app for global search. Results group by type (items, spells, NPCs, zones) so you can jump straight to what you want.

### Filters

- **Items:** race, class, level, slot, type, stat thresholds (HP, mana, AC, resists, etc.), and a hide-NO-DROP toggle for when you're only after tradeable gear.
- **Spells:** by class and level range. NPC-only spells are hidden by default. A collapsible **More filters** section narrows results by beneficial/detrimental, target type (including Any PB AE and Any Targeted AE groups), resist type, spell school, and whether the spell has a given effect in any slot.
- **Zones:** tabs for connected zones, NPCs, drops, ground spawns, and forage, each sortable and filterable. The NPCs tab shows each NPC's chance to spawn and hides NPCs that can't currently spawn on Quarm behind a Show inactive toggle. The Drops tab shows the real drop rate (matching the item page) and pages past its 500-item cap with a Show more button instead of truncating.

### Item Compare

From an item's detail view, open Item Compare to line it up side-by-side against other items — or against whatever's currently equipped in that slot on your active character — so you can see exactly which stats change before you decide it's an upgrade.

### PoP armor molds

Planes of Power molds and patterns show what they make: a mold's page lists the class armor it combines into, and each piece of that armor names the mold and recipe behind it. The same appears in NPC loot tables, the Item Info overlay (per class) and Gear Upgrades, so you know what you're rolling on.

### NPC zones, scrolls & spell components

Every NPC shows the zones it spawns in right under its name, in NPC search results (including **Ctrl + K**), and on item drop and vendor sources. NPCs spawned by events or scripts, such as the Plane of Time bosses, show the zone their NPC id belongs to, marked with \* (hover for details).

Scroll, song, tome and ancient items have a Teaches row linking to the spell, and the spell page links back to its scroll. The spell page also lists the reagents a spell consumes, the focus items it needs, and the instrument type for bard songs.
