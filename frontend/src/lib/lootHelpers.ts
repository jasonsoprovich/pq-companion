import type { LootDrop, LootDropItem } from '../types/npc'

export function effectiveDropPct(drop: LootDrop, item: LootDropItem): number {
  return (drop.probability * item.chance) / 100
}

export function rarityColor(effectivePct: number): string {
  if (effectivePct >= 50) return '#9ca3af'
  if (effectivePct >= 20) return '#22c55e'
  if (effectivePct >= 5) return '#3b82f6'
  if (effectivePct >= 1) return '#a855f7'
  return '#c9a84c'
}
