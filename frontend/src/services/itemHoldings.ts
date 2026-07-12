import { getAllInventories } from './api'
import type { AllInventoriesResponse } from '../types/zeal'

export interface ItemHolding {
  character: string // '' = shared bank
  location: string
  count: number
}

// Zeal exports only change on camp/logout, so a short-lived cache keeps the
// item detail views from re-scanning every export file on each item selection.
const TTL_MS = 60_000

let cachedAt = 0
let cached: Promise<AllInventoriesResponse> | null = null

function allInventoriesCached(): Promise<AllInventoriesResponse> {
  const now = Date.now()
  if (cached && now - cachedAt < TTL_MS) return cached
  cachedAt = now
  cached = getAllInventories().catch((err) => {
    cached = null
    throw err
  })
  return cached
}

// findItemHoldings returns every place the item appears across all characters'
// Zeal inventory exports — imported or not, so "do I have this somewhere?"
// always gets a complete answer — plus the shared bank.
export async function findItemHoldings(itemId: number): Promise<ItemHolding[]> {
  if (itemId <= 0) return []
  const res = await allInventoriesCached()
  const holdings: ItemHolding[] = []
  for (const inv of res.characters) {
    for (const e of inv.entries) {
      if (e.id === itemId) {
        holdings.push({ character: inv.character, location: e.location, count: e.count })
      }
    }
  }
  for (const e of res.shared_bank) {
    if (e.id === itemId) holdings.push({ character: '', location: e.location, count: e.count })
  }
  holdings.sort(
    (a, b) => a.character.localeCompare(b.character) || a.location.localeCompare(b.location),
  )
  return holdings
}

export interface ItemHoldingsByItem {
  // False when no character has ever written a Zeal inventory export — an
  // empty map in that case means "unknown," not "confirmed missing."
  configured: boolean
  map: Map<number, ItemHolding[]>
}

// findHoldingsForItems batches findItemHoldings across many items in a single
// pass over the cached inventory snapshot, so a recipe with a dozen
// components costs one scan instead of one per row.
export async function findHoldingsForItems(itemIds: number[]): Promise<ItemHoldingsByItem> {
  const ids = new Set(itemIds.filter((id) => id > 0))
  const map = new Map<number, ItemHolding[]>()
  if (ids.size === 0) return { configured: true, map }
  const res = await allInventoriesCached()
  const add = (id: number, h: ItemHolding) => {
    const arr = map.get(id)
    if (arr) arr.push(h)
    else map.set(id, [h])
  }
  for (const inv of res.characters) {
    for (const e of inv.entries) {
      if (ids.has(e.id)) add(e.id, { character: inv.character, location: e.location, count: e.count })
    }
  }
  for (const e of res.shared_bank) {
    if (ids.has(e.id)) add(e.id, { character: '', location: e.location, count: e.count })
  }
  for (const arr of map.values()) {
    arr.sort((a, b) => a.character.localeCompare(b.character) || a.location.localeCompare(b.location))
  }
  return { configured: res.configured, map }
}
