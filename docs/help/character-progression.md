---
title: "Character Progression"
slug: character-progression
section: "Character"
order: 10
routes: ["/characters/overview","/characters/progress"]
keywords: []
---
# Character Progression

**Prerequisite:** [Zeal](https://github.com/iamclint/Zeal) installed and at least one export run on this character.

A full per-character workspace driven by Zeal exports. Switch characters with the global character switcher — every progression sub-tab follows the active character.

- **Stats** — base HP/Mana, stat caps, worn effects, and softcap AC. Base attributes show their base value and green AA bonus separately, with a tooltip explaining the source. When a piece of gear pushes a stat past your level cap, the panel shows exactly how far over cap you are instead of silently clamping the displayed number. The Planes of Power AAs that raise your stat caps (Planar Power for every stat, Innate Enlightenment for INT and WIS) lift the cap shown per stat, and Flowing Thought shows how far your gear is over its cap of 15. A Tanking view splits avoidance from mitigation and folds in the mitigation softcap, melee hit chance, and tanking AAs. With Zeal, measured HP and mana regen per tick sit next to the computed values: the median of your last four clean ticks, where nothing hit you, you hit nothing, no heal landed and you didn't cast, zone or die.
- **Gear** — paper-doll view with bag and bank cards. Click any item for the full database entry.
- **AAs** — split into trained and available, with descriptions and cost-aware planning, plus your unspent (bankable) point pool. The unspent count comes from Zeal while the game is running (or a “you have gained an ability point” log line), shown with an “as of” time since spending points isn't logged.
- **Stat Snapshots** — with logging on, running Zeal's `/mystats` in game is captured automatically as a timestamped snapshot. Pick any two and this tab lays them out side-by-side with a plain-English label on every stat (mitigation, avoidance, offense, to-hit, per-weapon DPS…) and the change between them — handy for A/B-testing a gear or bandolier swap. The numbers are Zeal's own client-side estimate.
- **Recap** — a rolling 7/30/90-day journal of level, AA, spell and skill milestones for this character (or ranked most-active-first across all of them), with coin change and a “Camped in …” line showing the zone you last left this character in.
- **Spell Modifiers** — every focus and worn effect that's modifying your spells, in one tab, plus the focus types your class could use but has nothing equipped for (Spell Haste, Affliction Efficiency, Extended Range, and so on). Click a missing type — or the Upgrades button on any worn focus — to browse the items your class and level can wear for it, best first, with your current item, real upgrades, and anything you already own on another character marked. Damage foci are split by resist school, and a focus's max spell level counts as a hard limit, so a level-65 caster is shown Spell Haste IV rather than a level-60 III with a higher percent. Mana, damage, healing, duration and reagent foci roll a random 1% to their listed maximum on each cast; spell haste and range are fixed. An **Equipped focus types** strip lists what you're wearing next to what you're missing, each opening its focus card with the slot each item goes in. In **Test Resolution**, build a what-if loadout — swap worn items slot by slot, empty slots, or change the ranks of the AAs that feed focus effects — and see a spell resolve against it next to your real gear. Loadouts save per character, and mocked items can be added to your wishlist.
- **Live Buffs** — the Stats page's buffs panel is now driven by the spell timer engine. Swap between raid-buff presets per character, confirm-on-edit, and remove individual buffs to keep the panel honest if you suspect a buff fell off off-screen.
- **To-dos** — per-character checklist with subtasks for goals (epic pieces, key quests, AA targets).
- **Blocked Buffs** — keeps the list of other players' buffs you want refused with `#blockbuff`. The server stores that list itself, so the page reads its replies from your log to track what is actually applied and shows the exact commands needed to close the gap.
