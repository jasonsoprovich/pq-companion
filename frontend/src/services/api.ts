import type { Item, SearchResult } from '../types/item'
import type { NPC } from '../types/npc'
import type { Spell } from '../types/spell'
import type { Zone } from '../types/zone'
import type { ZealInventoryResponse, ZealSpellbookResponse, AllInventoriesResponse } from '../types/zeal'

export interface GlobalSearchResult {
  items: Item[]
  spells: Spell[]
  npcs: NPC[]
  zones: Zone[]
}

const BASE_URL = 'http://localhost:8080'

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  return res.json() as Promise<T>
}

// ── Items ──────────────────────────────────────────────────────────────────────

export function searchItems(
  q: string,
  limit = 50,
  offset = 0,
): Promise<SearchResult<Item>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  return get<SearchResult<Item>>(`/api/items?${params}`)
}

export function getItem(id: number): Promise<Item> {
  return get<Item>(`/api/items/${id}`)
}

// ── Spells ─────────────────────────────────────────────────────────────────────

export function searchSpells(
  q: string,
  limit = 50,
  offset = 0,
): Promise<SearchResult<Spell>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  return get<SearchResult<Spell>>(`/api/spells?${params}`)
}

export function getSpell(id: number): Promise<Spell> {
  return get<Spell>(`/api/spells/${id}`)
}

// ── NPCs ───────────────────────────────────────────────────────────────────────

export function searchNPCs(
  q: string,
  limit = 50,
  offset = 0,
): Promise<SearchResult<NPC>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  return get<SearchResult<NPC>>(`/api/npcs?${params}`)
}

export function getNPC(id: number): Promise<NPC> {
  return get<NPC>(`/api/npcs/${id}`)
}

// ── Zones ──────────────────────────────────────────────────────────────────────

export function searchZones(
  q: string,
  limit = 50,
  offset = 0,
): Promise<SearchResult<Zone>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  return get<SearchResult<Zone>>(`/api/zones?${params}`)
}

export function getZone(id: number): Promise<Zone> {
  return get<Zone>(`/api/zones/${id}`)
}

// ── Global Search ──────────────────────────────────────────────────────────────

export function globalSearch(q: string, limit = 5): Promise<GlobalSearchResult> {
  const params = new URLSearchParams({ q, limit: String(limit) })
  return get<GlobalSearchResult>(`/api/search?${params}`)
}

export function getNPCsByZone(
  shortName: string,
  limit = 200,
  offset = 0,
): Promise<SearchResult<NPC>> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  return get<SearchResult<NPC>>(`/api/zones/short/${encodeURIComponent(shortName)}/npcs?${params}`)
}

// ── Zeal ───────────────────────────────────────────────────────────────────────

export function getZealInventory(): Promise<ZealInventoryResponse> {
  return get<ZealInventoryResponse>('/api/zeal/inventory')
}

export function getZealSpellbook(): Promise<ZealSpellbookResponse> {
  return get<ZealSpellbookResponse>('/api/zeal/spells')
}

export function getAllInventories(): Promise<AllInventoriesResponse> {
  return get<AllInventoriesResponse>('/api/zeal/all-inventories')
}

// ── Spell Checklist ────────────────────────────────────────────────────────────

export function getSpellsByClass(
  classIndex: number,
  limit = 500,
  offset = 0,
): Promise<SearchResult<Spell>> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  return get<SearchResult<Spell>>(`/api/spells/class/${classIndex}?${params}`)
}
