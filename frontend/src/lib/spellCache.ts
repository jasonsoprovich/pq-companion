/**
 * Module-level session cache for spell fetches, shared by SpellHoverCard and
 * useSpellRefNames so hovering an item effect and resolving its proc spell
 * name hit the network once per spell id.
 */
import { getSpell } from '../services/api'
import { loadEnums } from './enumsCache'
import type { Spell } from '../types/spell'

const spellCache = new Map<number, Spell>()
const inFlight = new Map<number, Promise<Spell>>()

/** Synchronous cache read — undefined when the spell hasn't been fetched yet. */
export function getCachedSpell(id: number): Spell | undefined {
  return spellCache.get(id)
}

export function fetchSpellCached(id: number): Promise<Spell> {
  const hit = spellCache.get(id)
  if (hit) return Promise.resolve(hit)
  let p = inFlight.get(id)
  if (!p) {
    // Enum labels (target/resist/skill) read a module-level catalog loaded at
    // app boot; await it alongside the spell so a cold hover never renders
    // "Unknown (n)" placeholders.
    p = Promise.all([getSpell(id), loadEnums()])
      .then(([s]) => {
        spellCache.set(id, s)
        inFlight.delete(id)
        return s
      })
      .catch((err) => {
        inFlight.delete(id)
        throw err
      })
    inFlight.set(id, p)
  }
  return p
}
