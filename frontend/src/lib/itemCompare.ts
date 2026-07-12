import type { Item } from '../types/item'
import { baneRatioLabel, weaponRatioLabel } from './itemHelpers'

// Canonical ordered stat rows for the compare table — one source of truth for
// both the multi-item (Phase 1) and vs-equipped (Phase 2) views, so they never
// drift out of sync on which stats are shown or in what order.
export interface CompareStatRow {
  key: string
  label: string
  get: (item: Item) => number
  // Higher is always better for these rows (true for every stat here — EQ has
  // no item stat where lower is preferable).
  format?: (value: number) => string
}

export const COMPARE_STAT_ROWS: CompareStatRow[] = [
  { key: 'ac', label: 'AC', get: (i) => i.ac },
  { key: 'hp', label: 'HP', get: (i) => i.hp },
  { key: 'mana', label: 'Mana', get: (i) => i.mana },
  { key: 'str', label: 'STR', get: (i) => i.str },
  { key: 'sta', label: 'STA', get: (i) => i.sta },
  { key: 'agi', label: 'AGI', get: (i) => i.agi },
  { key: 'dex', label: 'DEX', get: (i) => i.dex },
  { key: 'wis', label: 'WIS', get: (i) => i.wis },
  { key: 'int', label: 'INT', get: (i) => i.int },
  { key: 'cha', label: 'CHA', get: (i) => i.cha },
  { key: 'mr', label: 'Magic Resist', get: (i) => i.mr },
  { key: 'cr', label: 'Cold Resist', get: (i) => i.cr },
  { key: 'dr', label: 'Disease Resist', get: (i) => i.dr },
  { key: 'fr', label: 'Fire Resist', get: (i) => i.fr },
  { key: 'pr', label: 'Poison Resist', get: (i) => i.pr },
]

/** Weapon damage/delay ratio, shown as its own row since it's not a flat stat. */
export function weaponRatio(item: Item): number {
  if (item.delay <= 0) return 0
  return item.damage / item.delay
}

export function hasCombatRow(item: Item): boolean {
  return item.damage > 0 && item.delay > 0
}

export function combatRowLabel(item: Item): string {
  if (item.bane_amt > 0) return baneRatioLabel(item.damage, item.delay, item.bane_amt)
  return weaponRatioLabel(item.damage, item.delay)
}

/** Rows with a non-zero value on at least one of the given items. */
export function activeStatRows(items: Item[]): CompareStatRow[] {
  return COMPARE_STAT_ROWS.filter((row) => items.some((item) => row.get(item) !== 0))
}

// Green/red/muted delta coloring — matches the Gear Upgrade Finder's DeltaChips.
export const DELTA_UP_COLOR = '#22c55e'
export const DELTA_DOWN_COLOR = '#ef4444'

export function deltaColor(delta: number): string {
  if (delta === 0) return 'var(--color-muted)'
  return delta > 0 ? DELTA_UP_COLOR : DELTA_DOWN_COLOR
}

export function deltaLabel(delta: number): string {
  if (delta === 0) return '—'
  return delta > 0 ? `+${delta}` : `${delta}`
}
