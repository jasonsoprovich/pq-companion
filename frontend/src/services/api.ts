import type { Config } from '../types/config'
import type { Item, ItemSources, SearchResult } from '../types/item'
import type { NPC, NPCSpawns } from '../types/npc'
import type { Spell } from '../types/spell'
import type { Zone } from '../types/zone'
import type { ZealInventoryResponse, ZealSpellbookResponse, AllInventoriesResponse } from '../types/zeal'
import type { KeysResponse, KeysProgressResponse } from '../types/keys'
import type { Backup, BackupsResponse } from '../types/backup'
import type { LogTailerStatus, LogFileInfo } from '../types/logEvent'
import type { TargetState } from '../types/overlay'
import type { CombatState } from '../types/combat'
import type { TimerState } from '../types/timer'
import type { Trigger, TriggerFired, TriggerPack, Action } from '../types/trigger'

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

async function post<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

async function put<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  return res.json() as Promise<T>
}

async function del(path: string): Promise<void> {
  const res = await fetch(`${BASE_URL}${path}`, { method: 'DELETE' })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
}

// ── Items ──────────────────────────────────────────────────────────────────────

export function searchItems(
  q: string,
  limit = 50,
  offset = 0,
  baneBody = 0,
): Promise<SearchResult<Item>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (baneBody > 0) params.set('bane_body', String(baneBody))
  return get<SearchResult<Item>>(`/api/items?${params}`)
}

export function getItem(id: number): Promise<Item> {
  return get<Item>(`/api/items/${id}`)
}

export function getItemSources(id: number): Promise<ItemSources> {
  return get<ItemSources>(`/api/items/${id}/sources`)
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

export function getNPCSpawns(id: number): Promise<NPCSpawns> {
  return get<NPCSpawns>(`/api/npcs/${id}/spawns`)
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

// ── Keys ───────────────────────────────────────────────────────────────────────

export function getKeys(): Promise<KeysResponse> {
  return get<KeysResponse>('/api/keys')
}

export function getKeysProgress(): Promise<KeysProgressResponse> {
  return get<KeysProgressResponse>('/api/keys/progress')
}

// ── Backups ────────────────────────────────────────────────────────────────────

export function listBackups(): Promise<BackupsResponse> {
  return get<BackupsResponse>('/api/backups')
}

export function createBackup(name: string, notes: string): Promise<Backup> {
  return post<Backup>('/api/backups', { name, notes })
}

export function deleteBackup(id: string): Promise<void> {
  return del(`/api/backups/${encodeURIComponent(id)}`)
}

export function restoreBackup(id: string): Promise<void> {
  return post<void>(`/api/backups/${encodeURIComponent(id)}/restore`)
}

export function lockBackup(id: string): Promise<void> {
  return put<void>(`/api/backups/${encodeURIComponent(id)}/lock`, {})
}

export function unlockBackup(id: string): Promise<void> {
  return put<void>(`/api/backups/${encodeURIComponent(id)}/unlock`, {})
}

export function pruneBackups(maxBackups: number): Promise<{ deleted: number }> {
  return post<{ deleted: number }>('/api/backups/prune', { max_backups: maxBackups })
}

// ── Log Parser ─────────────────────────────────────────────────────────────────

export function getLogStatus(): Promise<LogTailerStatus> {
  return get<LogTailerStatus>('/api/log/status')
}

export function getLogFileInfo(): Promise<LogFileInfo> {
  return get<LogFileInfo>('/api/log/info')
}

export function cleanupLog(): Promise<{ backup_path: string }> {
  return post<{ backup_path: string }>('/api/log/cleanup', {})
}

// ── Overlay ────────────────────────────────────────────────────────────────────

export function getOverlayNPCTarget(): Promise<TargetState> {
  return get<TargetState>('/api/overlay/npc/target')
}

export function getCombatState(): Promise<CombatState> {
  return get<CombatState>('/api/overlay/combat')
}

export function resetCombatState(): Promise<void> {
  return post<void>('/api/combat/reset')
}

export function getTimerState(): Promise<TimerState> {
  return get<TimerState>('/api/overlay/timers')
}

// ── Config ─────────────────────────────────────────────────────────────────────

export function getConfig(): Promise<Config> {
  return get<Config>('/api/config')
}

export function updateConfig(cfg: Config): Promise<Config> {
  return put<Config>('/api/config', cfg)
}

// ── Characters ─────────────────────────────────────────────────────────────────

export interface DiscoveredCharacter {
  name: string
  mod_time: number
}

export interface CharactersResponse {
  characters: DiscoveredCharacter[]
  active: string
  manual: boolean
}

export function listCharacters(): Promise<CharactersResponse> {
  return get<CharactersResponse>('/api/characters')
}

// ── Triggers ───────────────────────────────────────────────────────────────────

export function listTriggers(): Promise<Trigger[]> {
  return get<Trigger[]>('/api/triggers')
}

export interface CreateTriggerRequest {
  name: string
  enabled: boolean
  pattern: string
  actions: Action[]
}

export function createTrigger(req: CreateTriggerRequest): Promise<Trigger> {
  return post<Trigger>('/api/triggers', req)
}

export function updateTrigger(id: string, req: CreateTriggerRequest): Promise<Trigger> {
  return put<Trigger>(`/api/triggers/${encodeURIComponent(id)}`, req)
}

export function deleteTrigger(id: string): Promise<void> {
  return del(`/api/triggers/${encodeURIComponent(id)}`)
}

export function getTriggerHistory(): Promise<TriggerFired[]> {
  return get<TriggerFired[]>('/api/triggers/history')
}

export function getBuiltinPacks(): Promise<TriggerPack[]> {
  return get<TriggerPack[]>('/api/triggers/packs')
}

export function installBuiltinPack(packName: string): Promise<{ status: string; pack_name: string }> {
  return post<{ status: string; pack_name: string }>(`/api/triggers/packs/${encodeURIComponent(packName)}`)
}

export function importTriggerPack(pack: TriggerPack): Promise<{ status: string; pack_name: string }> {
  return post<{ status: string; pack_name: string }>('/api/triggers/import', pack)
}

export function exportTriggerPack(): Promise<TriggerPack> {
  return get<TriggerPack>('/api/triggers/export')
}
