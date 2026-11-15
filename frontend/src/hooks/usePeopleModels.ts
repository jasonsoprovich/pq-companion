import { useEffect, useMemo, useState } from 'react'
import { getPeopleModels, listCharacters } from '../services/api'
import type { PersonModel } from '../services/api'
import { MONK_CLASS, dressFor, loadEquippedItems } from '../lib/characterDress'
import type { ModelDress } from '../lib/characterDress'
import type { MarkerModel } from '../components/maps/ZoneMap'

// Who's who, for drawing people as character models in the 3D map: each
// name's race and gender (your characters, then /who sightings), and what
// you're wearing, from your Zeal export. Looked up once per name per
// session; names nobody knows the race of are asked again a minute later,
// since a /who may have seen them since.

const known = new Map<string, PersonModel>()
const askedAt = new Map<string, number>()
const RETRY_MS = 60_000

// Your dress, by character name.
const dresses = new Map<string, Promise<ModelDress | null>>()

function selfDress(name: string): Promise<ModelDress | null> {
  const key = name.toLowerCase()
  let p = dresses.get(key)
  if (!p) {
    p = Promise.all([loadEquippedItems(name), listCharacters()])
      .then(([eq, chars]) => {
        const c = chars.characters.find((x) => x.name.toLowerCase() === key)
        if (!eq.exportedAt) return null
        return dressFor((slot) => eq.items[slot] ?? null, c?.class === MONK_CLASS)
      })
      .catch(() => null)
    dresses.set(key, p)
  }
  return p
}

/**
 * usePeopleModels returns the model to draw for each of names (lower-cased
 * keys), and self dressed in their gear. Nothing is looked up while
 * enabled is false.
 */
export function usePeopleModels(names: string[], self: string, enabled: boolean): Map<string, MarkerModel> {
  const [tick, setTick] = useState(0)
  const [dress, setDress] = useState<ModelDress | null>(null)
  const all = useMemo(() => {
    const set = new Set(names.filter(Boolean))
    if (self) set.add(self)
    return [...set].sort()
  }, [names, self])
  const key = all.join(',')

  useEffect(() => {
    if (!enabled || !key) return
    let live = true
    const ask = (): void => {
      const now = Date.now()
      const want = key.split(',').filter((n) => {
        const k = n.toLowerCase()
        const p = known.get(k)
        if (p?.known) return false
        return now - (askedAt.get(k) ?? 0) > RETRY_MS
      })
      if (want.length === 0) return
      want.forEach((n) => askedAt.set(n.toLowerCase(), now))
      getPeopleModels(want.slice(0, 120))
        .then((list) => {
          list.forEach((p) => known.set(p.name.toLowerCase(), p))
          if (live) setTick((t) => t + 1)
        })
        .catch(() => {})
    }
    ask()
    const id = setInterval(ask, RETRY_MS)
    return () => {
      live = false
      clearInterval(id)
    }
  }, [enabled, key])

  useEffect(() => {
    if (!enabled || !self) return
    let live = true
    selfDress(self).then((d) => live && setDress(d))
    return () => {
      live = false
    }
  }, [enabled, self])

  return useMemo(() => {
    const out = new Map<string, MarkerModel>()
    if (!enabled) return out
    for (const n of all) {
      const p = known.get(n.toLowerCase())
      if (!p?.known) continue
      const isSelf = n.toLowerCase() === self.toLowerCase()
      out.set(n.toLowerCase(), { race: p.race, gender: p.gender, hair: p.hair, dress: isSelf ? dress : null })
    }
    return out
    // tick: lookups landed in the module cache.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, all, self, dress, tick])
}
