import type { Item, SearchResult } from '../types/item'
import type { Spell } from '../types/spell'

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
