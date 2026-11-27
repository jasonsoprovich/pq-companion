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

Scoring knows the difference between a stat that's already at its cap for your class and one that's still scaling, so it won't recommend a sidegrade that piles points into a stat you can't use any more of. HP, mana, and AC are scored uncapped; weapon DPS (ratio), worn ATK, and cap-aware haste are all factored in so melee offhands and haste pieces rank correctly. The PoP item stats (Accuracy, Avoidance, Strikethrough, Stun Resist, Combat Effects, Spell Shield, Shielding, Damage Shield) are summed across your worn gear and earn nothing past their server caps. A **Caps** line shows the caps the score uses (from your level and stat-cap AAs) plus Flowing Thought, worn ATK and haste, tinting any stat that's over. For the Primary slot you can lock the suggestions to Dual Wield or Two-Handed only, for classes that never switch hand style regardless of stats.

### Editable weights

The per-class stat weights are yours to tune, and they're saved per character — so a tank and a caster don't have to share one opinion of what "better" means.

### Rogue backstab

Rogues get a Backstab weight based on the server's backstab damage: it needs a one-handed piercer in Primary, scales with that weapon and your best worn Backstab skill mod, and a non-piercer in Primary shows "No backstab".

### Wanted focus effects

Pick the focus effects you want and every item carrying one (that rank or better, and not already worn) is pinned in a **Wanted focus** group above the ranked list, with its stats-only score — shown red when the item is a stat loss, so you can judge the trade. The focus picker groups foci by type and marks the ones you already wear. A focus counts as an upgrade only when it's the same kind, at least as strong, and covers at least the same spells as what you wear; AA foci stack on top, so they never make an item focus redundant.

### All-slots overview & wishlist

An all-slots overview surfaces the single best pick for every slot at once, and any suggestion stars straight onto that character's [Wishlist](help:wishlist).

### By zone

The By zone tab lists every upgrade that drops in one zone, ranked by score, with the NPCs that drop it and their drop rates. It follows your current zone or any zone you pick, and includes script-spawned bosses (like the Plane of Time gods) and zone-wide loot such as Vex Thal's. Group the list by slot, or show one list sorted by score for raid bidding. Each slot leads with its wanted-focus items.

### PoP armor molds

Ornate, Elemental and Timeless class armor shows the mold it combines from as a **From:** chip you can hover. In By zone, a mold's drops count as drops of the armor it makes, and the NO DROP filter judges that armor by whether its mold can be traded.

### Gear Upgrades overlay

The Gear Upgrades overlay is the By zone view at a glance: your active character's best upgrades by slot that drop in the zone you're in, scored with your saved weights, with the NPCs here that drop each one. Target an NPC that drops upgrades and the list narrows to its drops until you clear the target, and the popout folds to its title bar in a zone with no upgrades. Wanted-focus items are listed and badged. Turn it on from the [Overlays hub](help:overlay-dashboard).
