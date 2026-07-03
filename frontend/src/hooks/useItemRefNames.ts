import { useEffect, useState } from 'react'
import { fetchItemCached, getCachedItem } from '../lib/itemCache'
import { effectItemRef } from '../lib/spellHelpers'

interface EffectSlots {
  effect_ids: number[]
  effect_base_values: number[]
}

const EMPTY: number[] = []

/**
 * Resolves item IDs referenced by a spell's effect slots (SPA 32 Summon Item /
 * SPA 109 Summon Item Into Bag — the base value is the summoned item's ID) to
 * display names, for effectDescription's `itemNames` map. Mirrors
 * useSpellRefNames: names are read synchronously from the session item cache
 * when available; cache misses fetch in the background and re-render when they
 * land.
 */
export function useItemRefNames(spell: EffectSlots | null): ReadonlyMap<number, string> {
  const [, bump] = useState(0)

  const ids = spell
    ? spell.effect_ids
        .map((id, i) => effectItemRef(id, spell.effect_base_values[i] ?? 0))
        .filter((v): v is number => v !== null)
    : EMPTY
  const idsKey = ids.join(',')

  const names = new Map<number, string>()
  for (const id of ids) {
    const cached = getCachedItem(id)
    if (cached) names.set(id, cached.name)
  }

  useEffect(() => {
    const missing = idsKey === '' ? [] : idsKey.split(',').map(Number).filter((id) => !getCachedItem(id))
    if (missing.length === 0) return
    let cancelled = false
    Promise.all(missing.map((id) => fetchItemCached(id).catch(() => null)))
      .then(() => { if (!cancelled) bump((v) => v + 1) })
    return () => { cancelled = true }
  }, [idsKey])

  return names
}
