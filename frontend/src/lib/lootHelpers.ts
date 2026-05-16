import type { LootDrop, LootDropItem } from '../types/npc'

export function effectiveDropPct(drop: LootDrop, item: LootDropItem): number {
  return (drop.probability * item.chance) / 100
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
