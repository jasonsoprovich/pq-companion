/**
 * Module-level session cache for item fetches, mirroring spellCache. Used by
 * useItemRefNames to resolve item IDs referenced by spell effect base values
 * (SPA 32 Summon Item / SPA 109 Summon Item Into Bag) to display names,
 * hitting the network once per item id.
 */
import { getItem } from '../services/api'
import type { Item } from '../types/item'

const itemCache = new Map<number, Item>()
const inFlight = new Map<number, Promise<Item>>()

/** Synchronous cache read — undefined when the item hasn't been fetched yet. */
export function getCachedItem(id: number): Item | undefined {
  return itemCache.get(id)
}

export function fetchItemCached(id: number): Promise<Item> {
  const hit = itemCache.get(id)
  if (hit) return Promise.resolve(hit)
  let p = inFlight.get(id)
  if (!p) {
    p = getItem(id)
      .then((it) => {
        itemCache.set(id, it)
        inFlight.delete(id)
        return it
      })
      .catch((err) => {
        inFlight.delete(id)
        throw err
      })
    inFlight.set(id, p)
  }
  return p
}
