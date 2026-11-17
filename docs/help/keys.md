---
title: "Keys & Keyring"
slug: keys
section: "Progress & goals"
order: 20
routes: ["/characters/keys"]
keywords: []
---
# Keys & Keyring

**Prerequisite:** Zeal — the Keyring tab needs `/keys` or inventory exports; the Key Tracker tab needs an inventory export.

Two complementary tabs sit side-by-side on the Keys page, with Keyring as the default since it's the one you'll check before pulling a key-gated zone.

### Keyring (live /keys snapshot)

The Keyring tab mirrors the in-game keyring for each character. The data refreshes the moment a new `/keys` export lands. For keys that aren't in the keyring yet (Plane of Time fragments, Vex Thal Shroud, and similar one-off items), inventory is used as a fallback. Upgraded key stages — e.g. a finished Vex Thal Shroud — are automatically inferred as owning all the prerequisite pieces, so you don't see ghost "missing" entries after a combine.

### Key Tracker (component quests)

The Key Tracker walks the component quest for every major raid key and ticks off the pieces you already own. Each key is modelled from the canonical quest path: Howling Stones uses the full Key to Charasis quest (with Hand of Glory as a separate internal-doors key), Arx Seru uses the 4-shard Praesertum quest, Veeshan's Peak uses the full Key of Veeshan quest, and Sleeper's Tomb accepts any one of the four Velious talismans. Sebilis is tracked from the Trakanon Idol quest. Vex Thal sub-shards use the correct Lucid Shard zone mappings so you know which zone to camp for the missing piece.

Intermediate combines are detected as you progress, so the list updates without manual ticking. Each held component shows the bag or bank slot it's sitting in, so you don't have to dig through inventory to find it.
