// Shared charm-finder helpers for the Charm Pet Finder page and the Pet
// overlay's finder mode.
import type { CharmSpellOption } from '../services/api'
import type { Spellbook } from '../types/zeal'

// The charm-capable classes, in the spells_new / eqstat 0-based ordering.
// Magician's only charm is the level-65 PoP spell Call of the Arch Mage
// (summoned/elemental targets only).
export const CHARM_CLASSES = [
  { idx: 13, name: 'Enchanter' },
  { idx: 10, name: 'Necromancer' },
  { idx: 5, name: 'Druid' },
  { idx: 7, name: 'Bard' },
  { idx: 12, name: 'Magician' },
]
export const ENCHANTER = 13

export function isCharmClass(classIdx: number): boolean {
  return CHARM_CLASSES.some((c) => c.idx === classIdx)
}

// pickDefaultSpell returns the strongest charm the class can cast at the given
// level (highest req level ≤ level), falling back to the lowest-tier charm so
// the dropdown always has a sensible selection.
export function pickDefaultSpell(opts: CharmSpellOption[], level: number): number | null {
  if (opts.length === 0) return null
  const castable = opts.filter((o) => o.req_level <= level)
  if (castable.length > 0) {
    return castable.reduce((best, o) => (o.req_level > best.req_level ? o : best)).spell_id
  }
  // None castable yet — show the earliest charm in the line.
  return opts.reduce((low, o) => (o.req_level < low.req_level ? o : low)).spell_id
}

// pickScribedSpell returns the strongest castable charm the character has in
// their Zeal spellbook export, matching by id and falling back to name. Both
// can differ for the same spell: Zeal exports Boltran's as 1705 "Boltran`s
// Agacerie" while the charm catalog lists 1706 "Boltran's Agacerie", so names
// compare with backticks folded to apostrophes. Null when no export exists or
// none of the line is scribed — callers then fall back to pickDefaultSpell.
export function pickScribedSpell(
  opts: CharmSpellOption[],
  level: number,
  book: Spellbook | null,
): number | null {
  if (!book) return null
  const fold = (n: string): string => n.replace(/`/g, "'").toLowerCase()
  const ids = new Set(book.spell_ids)
  const names = new Set((book.names ?? []).map(fold))
  const scribed = opts.filter(
    (o) => o.req_level <= level && (ids.has(o.spell_id) || names.has(fold(o.name))),
  )
  if (scribed.length === 0) return null
  return scribed.reduce((best, o) => (o.req_level > best.req_level ? o : best)).spell_id
}

// landColor colours a 0–1 charm land chance the way the Finder table does.
export function landColor(chance: number): string {
  return chance >= 0.9 ? '#4ade80' : chance >= 0.6 ? '#facc15' : '#f87171'
}
