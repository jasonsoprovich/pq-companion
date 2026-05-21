import type { LootDrop, LootDropItem } from '../types/npc'

export function effectiveDropPct(drop: LootDrop, item: LootDropItem): number {
  return (drop.probability * item.chance) / 100
}

// cleanLootDropLabel translates the raw `lootdrop.name` value (which comes
// straight from the EQ data dump) into something a user can read at a glance.
//
// Three buckets exist in the data:
//   • Auto-generated per-NPC tables: `<id>_<NPC_Name>_MAGELO-GEN` or the
//     trailing-underscore variant. These restate the NPC's own name and add
//     no information for the player. Return null so the caller hides the
//     heading and renders the items as "main drops" without a label.
//   • Specialty splits: `<id>_<NPC>_Wear` / `_Trade` / `_Misc`. Relabel to
//     "Wearables" / "Trade goods" / "Misc".
//   • Themed/shared tables ("ruby crown table", "Rusty Weapons", etc.).
//     Strip any leading "<digits>_" and title-case if the string is all
//     lowercase; otherwise leave it alone.
export function cleanLootDropLabel(raw: string): string | null {
  const name = raw?.trim() ?? ''
  if (!name) return null

  if (/_MAGELO-GEN$/i.test(name)) return null
  if (/^\d+_.+_$/.test(name)) return null

  const wear = name.match(/^\d+_.+_(Wear|Trade|Misc)$/i)
  if (wear) {
    const kind = wear[1].toLowerCase()
    if (kind === 'wear') return 'Wearables'
    if (kind === 'trade') return 'Trade goods'
    return 'Misc'
  }

  const stripped = name.replace(/^\d+_/, '')
  if (stripped === stripped.toLowerCase()) {
    return stripped.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
  }
  return stripped.replace(/_/g, ' ')
}

// rarityColor maps an item's effective drop chance to a text color so the
// player can scan a loot table for the outliers. The scale is anchored on
// gold as the DEFAULT (typical EQ drop in the 0.5–5% range) so a loot
// table full of normal drops reads uniformly — only items that are
// notably common (cooler colors) or genuinely rare (purple) stand out.
//
// Earlier revisions made gold the rarest tier, which surprised users on
// loot tables where spell scrolls (commonly ~2%) lit up purple while the
// genuinely-rare items rendered gold. See #110.
export function rarityColor(effectivePct: number): string {
  if (effectivePct >= 50) return '#9ca3af' // grey — practically guaranteed
  if (effectivePct >= 20) return '#22c55e' // green — very common
  if (effectivePct >= 5) return '#3b82f6'  // blue — common
  if (effectivePct >= 0.5) return '#c9a84c' // gold — typical EQ drop
  return '#a855f7'                          // purple — genuinely rare
}
