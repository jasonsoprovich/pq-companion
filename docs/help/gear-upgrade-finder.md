---
title: "Gear Upgrade Finder"
slug: gear-upgrade-finder
section: "Progress & goals"
order: 70
routes: ["/characters/upgrades"]
keywords: []
---
# Gear Upgrade Finder

**Prerequisite:** a Zeal character export, so the app knows what you currently have equipped.

A per-character, per-slot upgrade scanner. It reads your worn gear and ranks the database for real upgrades in each slot, scoring candidates against the stats that actually help your class.

### Cap-aware scoring

Scoring knows the difference between a stat that's already at its cap for your class and one that's still scaling, so it won't recommend a sidegrade that piles points into a stat you can't use any more of. HP, mana, and AC are scored uncapped; weapon DPS (ratio), worn ATK, and cap-aware haste are all factored in so melee offhands and haste pieces rank correctly. For the Primary slot you can lock the suggestions to Dual Wield or Two-Handed only, for classes that never switch hand style regardless of stats.

### Editable weights

The per-class stat weights are yours to tune, and they're saved per character — so a tank and a caster don't have to share one opinion of what "better" means.

### Priority focus effects

Mark the focus effects you care about and matching items get a score boost and a badge, so the right clicky or focus piece wins the slot instead of a marginally-higher stat stick.

### All-slots overview & wishlist

An all-slots overview surfaces the single best pick for every slot at once, and any suggestion stars straight onto that character's [Wishlist](help:wishlist).

### By zone

The By zone tab lists every upgrade that drops in one zone, ranked by score, with the NPCs that drop it and their drop rates. It follows your current zone or any zone you pick, and includes script-spawned bosses (like the Plane of Time gods) and zone-wide loot such as Vex Thal's. Group the list by slot, or show one list sorted by score for raid bidding.
