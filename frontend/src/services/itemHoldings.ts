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
