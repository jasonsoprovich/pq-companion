import { useEffect, useState } from 'react'
import { fetchSpellCached, getCachedSpell } from '../lib/spellCache'
import { effectSpellRef } from '../lib/spellHelpers'

interface EffectSlots {
  effect_ids: number[]
  effect_base_values: number[]
}

const EMPTY: number[] = []

/**
 * Resolves spell IDs referenced by a spell's effect slots (SPA 85 Add Proc —
 * the base value is the proc spell's ID) to display names, for
 * effectDescription's `spellNames` map. Names are read synchronously from the
 * session spell cache when available, so a prefetched hover card never
 * flashes the "Spell #N" fallback; cache misses fetch in the background and
 * re-render when they land.
 */
export function useSpellRefNames(spell: EffectSlots | null): ReadonlyMap<number, string> {
  const [, bump] = useState(0)

  const ids = spell
    ? spell.effect_ids
        .map((id, i) => effectSpellRef(id, spell.effect_base_values[i] ?? 0))
        .filter((v): v is number => v !== null)
    : EMPTY
  const idsKey = ids.join(',')

  const names = new Map<number, string>()
  for (const id of ids) {
    const cached = getCachedSpell(id)
    if (cached) names.set(id, cached.name)
  }

  useEffect(() => {
    const missing = idsKey === '' ? [] : idsKey.split(',').map(Number).filter((id) => !getCachedSpell(id))
    if (missing.length === 0) return
    let cancelled = false
    Promise.all(missing.map((id) => fetchSpellCached(id).catch(() => null)))
      .then(() => { if (!cancelled) bump((v) => v + 1) })
    return () => { cancelled = true }
  }, [idsKey])

  return names
}
