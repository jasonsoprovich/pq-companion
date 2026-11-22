// PoP-style "mod2" item stats, added by Project Quarm's 2026-11-23 patch
// (EQMacEmu PR #500). Mirrors db.Mod2Stats / db.Mod2Caps in the backend: the
// server sums each stat across worn gear and caps the total.

import type { Mod2Stats } from '../types/item'

export type { Mod2Stats }

export type Mod2Key = keyof Mod2Stats

export interface Mod2Def {
  key: Mod2Key
  label: string
  short: string
  // pct stats are percent chances/reductions; the rest are flat ratings.
  pct: boolean
  cap: number
  hint: string
}

export const MOD2_STATS: Mod2Def[] = [
  { key: 'accuracy', label: 'Accuracy', short: 'Acc', pct: false, cap: 150,
    hint: 'Flat bonus to your melee to-hit.' },
  { key: 'combat_effects', label: 'Combat Effects', short: 'CE', pct: false, cap: 100,
    hint: 'Raises weapon proc chance (10 = +1%).' },
  { key: 'avoidance', label: 'Avoidance', short: 'Avoid', pct: false, cap: 100,
    hint: 'Flat avoidance in the melee hit roll; not part of displayed AC.' },
  { key: 'strikethrough', label: 'Strikethrough', short: 'Strike', pct: true, cap: 35,
    hint: 'Chance to hit through a block, parry, riposte or dodge.' },
  { key: 'stun_resist', label: 'Stun Resist', short: 'StunRes', pct: true, cap: 35,
    hint: 'Chance to resist melee stuns (not spell stuns).' },
  { key: 'spell_shield', label: 'Spell Shield', short: 'SpShield', pct: true, cap: 35,
    hint: 'Reduces direct damage spells (not DoTs).' },
  { key: 'shielding', label: 'Shielding', short: 'Shield', pct: true, cap: 35,
    hint: "Reduces the damage bonus of melee hits against you." },
  { key: 'damage_shield', label: 'Damage Shield', short: 'DS', pct: false, cap: 30,
    hint: 'Adds to a damage shield spell you already have; none on its own.' },
]

export interface Mod2Entry extends Mod2Def {
  value: number
}

// mod2List returns the item's nonzero mod2 stats in display order. Fields are
// read defensively so an older backend (no mod2 columns) yields an empty list.
export function mod2List(item: Partial<Mod2Stats>): Mod2Entry[] {
  return MOD2_STATS.flatMap((d) => {
    const value = item[d.key] ?? 0
    return value !== 0 ? [{ ...d, value }] : []
  })
}

// mod2Value renders one item's stat value: "+5%" or "+15".
export function mod2Value(e: { value: number; pct: boolean }): string {
  const sign = e.value > 0 ? '+' : ''
  return `${sign}${e.value}${e.pct ? '%' : ''}`
}

// mod2TotalLabel renders a gear total against its cap: "20/35%", or
// "+5 40/35%" when gear goes past the cap (the excess is wasted), matching
// the Flowing Thought over-cap display.
export function mod2TotalLabel(d: Mod2Def, value: number, raw: number): string {
  const unit = d.pct ? '%' : ''
  return raw > d.cap
    ? `+${raw - d.cap} ${raw}/${d.cap}${unit}`
    : `${value}/${d.cap}${unit}`
}
