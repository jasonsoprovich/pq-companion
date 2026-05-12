import type { Config } from '../types/config'
import type { Item, ItemSources, SearchResult } from '../types/item'
import type { NPC, NPCSpawns, NPCLootTable, NPCFaction } from '../types/npc'
import type { Spell, SpellCrossRefs } from '../types/spell'
import type { Zone, ZoneConnection, ZoneGroundSpawn, ZoneForageItem, ZoneDropItem } from '../types/zone'
import type { ZealInventoryResponse, ZealSpellbookResponse, AllInventoriesResponse } from '../types/zeal'
import type { KeysResponse, KeysProgressResponse } from '../types/keys'
import type { Backup, BackupsResponse } from '../types/backup'
import type { LogTailerStatus, LogFileInfo } from '../types/logEvent'
import type { TargetState } from '../types/overlay'
import type { CombatState, HistoryFacets, HistoryFilter, HistoryListResponse, StoredFight } from '../types/combat'
import type { TimerState } from '../types/timer'
import type { Trigger, TriggerFired, TriggerPack, Action, TimerType, TimerAlertThreshold } from '../types/trigger'
import type { RollsState, RollsSettingsPatch, WinnerRule } from '../types/rolls'

export interface GlobalSearchResult {
  items: Item[]
  spells: Spell[]
  npcs: NPC[]
  zones: Zone[]
}

import { getBackendBaseUrl } from './backendUrl'

async function get<T>(path: string): Promise<T> {
  const baseUrl = await getBackendBaseUrl()
  const res = await fetch(`${baseUrl}${path}`)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  return res.json() as Promise<T>
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const baseUrl = await getBackendBaseUrl()
  const res = await fetch(`${baseUrl}${path}`, {
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
  const baseUrl = await getBackendBaseUrl()
  const res = await fetch(`${baseUrl}${path}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

async function del(path: string): Promise<void> {
  const baseUrl = await getBackendBaseUrl()
  const res = await fetch(`${baseUrl}${path}`, { method: 'DELETE' })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
}

// ── Items ──────────────────────────────────────────────────────────────────────

export interface ItemSearchFilter {
  baneBody?: number
  race?: number
  class?: number
  minLevel?: number
  maxLevel?: number
  slot?: number
  itemType?: number // -1 = any
  minSTR?: number
  minSTA?: number
  minAGI?: number
  minDEX?: number
  minWIS?: number
  minINT?: number
  minCHA?: number
  minHP?: number
  minMana?: number
  minAC?: number
  minMR?: number
  minCR?: number
  minDR?: number
  minFR?: number
  minPR?: number
}

export function searchItems(
  q: string,
  limit = 50,
  offset = 0,
  filter: ItemSearchFilter = {},
): Promise<SearchResult<Item>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (filter.baneBody && filter.baneBody > 0) params.set('bane_body', String(filter.baneBody))
  if (filter.race && filter.race > 0) params.set('race', String(filter.race))
  if (filter.class && filter.class > 0) params.set('class', String(filter.class))
  if (filter.minLevel && filter.minLevel > 0) params.set('min_level', String(filter.minLevel))
  if (filter.maxLevel && filter.maxLevel > 0) params.set('max_level', String(filter.maxLevel))
  if (filter.slot && filter.slot > 0) params.set('slot', String(filter.slot))
  if (filter.itemType !== undefined && filter.itemType >= 0) params.set('item_type', String(filter.itemType))
  if (filter.minSTR && filter.minSTR > 0) params.set('min_str', String(filter.minSTR))
  if (filter.minSTA && filter.minSTA > 0) params.set('min_sta', String(filter.minSTA))
  if (filter.minAGI && filter.minAGI > 0) params.set('min_agi', String(filter.minAGI))
  if (filter.minDEX && filter.minDEX > 0) params.set('min_dex', String(filter.minDEX))
  if (filter.minWIS && filter.minWIS > 0) params.set('min_wis', String(filter.minWIS))
  if (filter.minINT && filter.minINT > 0) params.set('min_int', String(filter.minINT))
  if (filter.minCHA && filter.minCHA > 0) params.set('min_cha', String(filter.minCHA))
  if (filter.minHP && filter.minHP > 0) params.set('min_hp', String(filter.minHP))
  if (filter.minMana && filter.minMana > 0) params.set('min_mana', String(filter.minMana))
  if (filter.minAC && filter.minAC > 0) params.set('min_ac', String(filter.minAC))
  if (filter.minMR && filter.minMR > 0) params.set('min_mr', String(filter.minMR))
  if (filter.minCR && filter.minCR > 0) params.set('min_cr', String(filter.minCR))
  if (filter.minDR && filter.minDR > 0) params.set('min_dr', String(filter.minDR))
  if (filter.minFR && filter.minFR > 0) params.set('min_fr', String(filter.minFR))
  if (filter.minPR && filter.minPR > 0) params.set('min_pr', String(filter.minPR))
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
  classIndex = -1,
  minLevel = 0,
  maxLevel = 0,
): Promise<SearchResult<Spell>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (classIndex >= 0) params.set('class', String(classIndex))
  if (minLevel > 0) params.set('minLevel', String(minLevel))
  if (maxLevel > 0) params.set('maxLevel', String(maxLevel))
  return get<SearchResult<Spell>>(`/api/spells?${params}`)
}

export function getSpell(id: number): Promise<Spell> {
  return get<Spell>(`/api/spells/${id}`)
}

export function getSpellCrossRefs(id: number): Promise<SpellCrossRefs> {
  return get<SpellCrossRefs>(`/api/spells/${id}/items`)
}

// ── NPCs ───────────────────────────────────────────────────────────────────────

export function searchNPCs(
  q: string,
  limit = 50,
  offset = 0,
  showPlaceholders = false,
): Promise<SearchResult<NPC>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (showPlaceholders) params.set('show_placeholders', '1')
  return get<SearchResult<NPC>>(`/api/npcs?${params}`)
}

export function getNPC(id: number): Promise<NPC> {
  return get<NPC>(`/api/npcs/${id}`)
}

export function getNPCSpawns(id: number): Promise<NPCSpawns> {
  return get<NPCSpawns>(`/api/npcs/${id}/spawns`)
}

export function getNPCLoot(id: number): Promise<NPCLootTable> {
  return get<NPCLootTable>(`/api/npcs/${id}/loot`)
}

export function getNPCFaction(id: number): Promise<NPCFaction | null> {
  return get<NPCFaction | null>(`/api/npcs/${id}/faction`)
}

// ── Zones ──────────────────────────────────────────────────────────────────────

export interface ZoneSearchFilters {
  expansion?: number
}

export function searchZones(
  q: string,
  filters: ZoneSearchFilters = {},
  limit = 50,
  offset = 0,
): Promise<SearchResult<Zone>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (filters.expansion !== undefined) {
    params.set('expansion', String(filters.expansion))
  }
  return get<SearchResult<Zone>>(`/api/zones?${params}`)
}

export function getZoneExpansions(): Promise<number[]> {
  return get<number[]>('/api/zones/expansions')
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

export function getZoneConnections(shortName: string): Promise<ZoneConnection[]> {
  return get<ZoneConnection[]>(`/api/zones/short/${encodeURIComponent(shortName)}/connections`)
}

export function getZoneGroundSpawns(shortName: string): Promise<ZoneGroundSpawn[]> {
  return get<ZoneGroundSpawn[]>(`/api/zones/short/${encodeURIComponent(shortName)}/ground-spawns`)
}

export function getZoneForage(shortName: string): Promise<ZoneForageItem[]> {
  return get<ZoneForageItem[]>(`/api/zones/short/${encodeURIComponent(shortName)}/forage`)
}

export function getZoneDrops(shortName: string): Promise<ZoneDropItem[]> {
  return get<ZoneDropItem[]>(`/api/zones/short/${encodeURIComponent(shortName)}/drops`)
}

// ── Zeal ───────────────────────────────────────────────────────────────────────

export function getZealInventory(): Promise<ZealInventoryResponse> {
  return get<ZealInventoryResponse>('/api/zeal/inventory')
}

export function getZealSpellbook(character?: string): Promise<ZealSpellbookResponse> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return get<ZealSpellbookResponse>(`/api/zeal/spells${qs}`)
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

// ── Combat history (persisted) ─────────────────────────────────────────────

export function listCombatHistory(filter: HistoryFilter = {}): Promise<HistoryListResponse> {
  const params = new URLSearchParams()
  if (filter.start) params.set('start', filter.start)
  if (filter.end) params.set('end', filter.end)
  if (filter.npc) params.set('npc', filter.npc)
  if (filter.character) params.set('character', filter.character)
  if (filter.zone) params.set('zone', filter.zone)
  if (filter.limit !== undefined) params.set('limit', String(filter.limit))
  if (filter.offset !== undefined) params.set('offset', String(filter.offset))
  const qs = params.toString()
  return get<HistoryListResponse>(`/api/combat/history${qs ? `?${qs}` : ''}`)
}

export function getCombatHistoryFight(id: number): Promise<StoredFight> {
  return get<StoredFight>(`/api/combat/history/${id}`)
}

export function getCombatHistoryFacets(): Promise<HistoryFacets> {
  return get<HistoryFacets>('/api/combat/history/facets')
}

export function deleteCombatHistoryFight(id: number): Promise<void> {
  return del(`/api/combat/history/${id}`)
}

export async function clearCombatHistory(): Promise<{ removed: number }> {
  // del() returns void; use a raw fetch here so we can read the body.
  const baseUrl = await getBackendBaseUrl()
  const r = await fetch(`${baseUrl}/api/combat/history`, { method: 'DELETE' })
  if (!r.ok) throw new Error(`clear history: ${r.status}`)
  return r.json() as Promise<{ removed: number }>
}

export function getTimerState(): Promise<TimerState> {
  return get<TimerState>('/api/overlay/timers')
}

export function clearTimers(category: 'buff' | 'detrimental' | 'all'): Promise<void> {
  return post<void>(`/api/overlay/timers/clear?category=${category}`)
}

export function removeTimer(id: string): Promise<void> {
  return del(`/api/overlay/timers/${encodeURIComponent(id)}`)
}

// ── Config ─────────────────────────────────────────────────────────────────────

export function getConfig(): Promise<Config> {
  return get<Config>('/api/config')
}

export function updateConfig(cfg: Config): Promise<Config> {
  return put<Config>('/api/config', cfg)
}

export interface DiscoveredCharacter {
  name: string
  mod_time: number
}

export interface ValidateEQPathResponse {
  valid: boolean
  error?: string
  has_logs: boolean
  characters: DiscoveredCharacter[]
}

export function validateEQPath(path: string): Promise<ValidateEQPathResponse> {
  return post<ValidateEQPathResponse>('/api/config/validate-eq-path', { path })
}

// ── Characters ─────────────────────────────────────────────────────────────────

export interface Character {
  id: number
  name: string
  class: number  // -1=not set, 0-14=EQ class index
  race: number   // -1=not set, EQ race id
  level: number
  base_str: number
  base_sta: number
  base_cha: number
  base_dex: number
  base_int: number
  base_agi: number
  base_wis: number
}

export interface CharacterAA {
  aa_id: number
  rank: number
  name?: string
}

// AAInfo describes a single AA in the catalog returned from /characters/{id}/aas.
// aa_id is altadv_vars.eqmacid (the EQ client AA index used by the Zeal export).
// type: 1=General, 2=Archetype, 3=Class, 4=PoP Advance, 5=PoP Ability.
export interface AAInfo {
  aa_id: number
  name: string
  cost: number
  cost_inc: number
  max_level: number
  type: number
  description?: string
}

export interface CharacterAAsResponse {
  trained: CharacterAA[]
  available: AAInfo[]
}

export interface QuarmyStats {
  base_str: number
  base_sta: number
  base_cha: number
  base_dex: number
  base_int: number
  base_agi: number
  base_wis: number
}

export interface QuarmyInventoryEntry {
  location: string
  name: string
  id: number
  count: number
  slots: number
}

export interface QuarmyData {
  character: string
  exported_at: string
  stats: QuarmyStats
  inventory: QuarmyInventoryEntry[]
  aas: CharacterAA[]
}

export interface CharactersResponse {
  characters: Character[]
  active: string
  manual: boolean
  // detected is what auto-mode would resolve to right now (most-recently
  // modified eqlog). Populated regardless of manual mode.
  detected: string
}

export interface CharacterRequest {
  name: string
  class: number
  race: number
  level: number
}

export function listCharacters(): Promise<CharactersResponse> {
  return get<CharactersResponse>('/api/characters')
}

export function createCharacter(req: CharacterRequest): Promise<Character> {
  return post<Character>('/api/characters', req)
}

export function deleteCharacter(id: number): Promise<void> {
  return del(`/api/characters/${id}`)
}

export function discoverCharacters(): Promise<{ names: string[] }> {
  return get<{ names: string[] }>('/api/characters/discover')
}

export function getCharacterAAs(id: number): Promise<CharacterAAsResponse> {
  return get<CharacterAAsResponse>(`/api/characters/${id}/aas`)
}

// ── Spell modifiers (focus extensions from items + AAs) ───────────────────────

export interface SpellModifierLimits {
  max_level?: number
  min_level?: number
  spell_type: number // -1 unset, 0 detrimental, 1 beneficial, 2 any
  min_duration_sec?: number
  exclude_effects?: number[]
  include_spells?: number[]
  target_types?: number[]
}

export interface SpellModifier {
  source: 'item' | 'aa'
  source_item_id?: number
  source_item_name?: string
  source_item_slot?: string
  source_aa_id?: number
  source_aa_name?: string
  source_aa_rank?: number
  focus_spell_id?: number
  focus_spell_name?: string
  spa: number // 127 cast time, 128 duration
  percent: number
  limits: SpellModifierLimits
}

export interface SpellModifierResolution {
  spell_id: number
  spell_name: string
  spell_type: number
  spell_level: number
  caster_level: number
  base_duration_sec: number
  extended_duration_sec: number
  duration_aa_percent: number
  duration_item_percent: number
  duration_percent: number
  cast_time_percent: number
  applied: SpellModifier[]
}

export interface SpellModifiersResponse {
  character: string
  contributors: SpellModifier[]
  resolution?: SpellModifierResolution
}

export function getCharacterSpellModifiers(
  id: number,
  spellID?: number,
): Promise<SpellModifiersResponse> {
  const qs = spellID ? `?spell_id=${spellID}` : ''
  return get<SpellModifiersResponse>(`/api/characters/${id}/spell-modifiers${qs}`)
}

export function getZealQuarmy(character?: string): Promise<{ quarmy: QuarmyData | null }> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return get<{ quarmy: QuarmyData | null }>(`/api/zeal/quarmy${qs}`)
}

export interface StatBlock {
  hp: number; mana: number; ac: number
  str: number; sta: number; agi: number; dex: number
  wis: number; int: number; cha: number
  pr: number; mr: number; dr: number; fr: number; cr: number
  attack: number; haste: number; spell_haste: number; regen: number
  mana_regen: number; ft: number; dmg_shield: number
}

export interface EquippedStats {
  character: string
  level: number
  class: number
  base: StatBlock
  equipment: StatBlock
}

export function getCharacterEquippedStats(id: number): Promise<EquippedStats> {
  return get<EquippedStats>(`/api/characters/${id}/equipped-stats`)
}

// ── Character Tasks ────────────────────────────────────────────────────────────

export interface Subtask {
  id: number
  task_id: number
  name: string
  completed: boolean
  position: number
}

export interface CharacterTask {
  id: number
  character_id: number
  name: string
  description: string
  position: number
  completed: boolean
  created_at: number
  subtasks: Subtask[]
}

export interface TaskRequest {
  name: string
  description: string
  completed: boolean
}

export interface SubtaskRequest {
  name: string
  completed: boolean
}

export function listCharacterTasks(charID: number): Promise<{ tasks: CharacterTask[] }> {
  return get<{ tasks: CharacterTask[] }>(`/api/characters/${charID}/tasks`)
}

export function createCharacterTask(charID: number, req: TaskRequest): Promise<CharacterTask> {
  return post<CharacterTask>(`/api/characters/${charID}/tasks`, req)
}

export function updateCharacterTask(charID: number, taskID: number, req: TaskRequest): Promise<void> {
  return put<void>(`/api/characters/${charID}/tasks/${taskID}`, req)
}

export function deleteCharacterTask(charID: number, taskID: number): Promise<void> {
  return del(`/api/characters/${charID}/tasks/${taskID}`)
}

export function reorderCharacterTasks(charID: number, orderedIDs: number[]): Promise<void> {
  return put<void>(`/api/characters/${charID}/tasks/reorder`, { ordered_ids: orderedIDs })
}

export function createCharacterSubtask(charID: number, taskID: number, req: SubtaskRequest): Promise<Subtask> {
  return post<Subtask>(`/api/characters/${charID}/tasks/${taskID}/subtasks`, req)
}

export function updateCharacterSubtask(charID: number, taskID: number, subtaskID: number, req: SubtaskRequest): Promise<void> {
  return put<void>(`/api/characters/${charID}/tasks/${taskID}/subtasks/${subtaskID}`, req)
}

export function deleteCharacterSubtask(charID: number, taskID: number, subtaskID: number): Promise<void> {
  return del(`/api/characters/${charID}/tasks/${taskID}/subtasks/${subtaskID}`)
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
  timer_type?: TimerType
  timer_duration_secs?: number
  worn_off_pattern?: string
  spell_id?: number
  display_threshold_secs?: number
  characters?: string[]
  timer_alerts?: TimerAlertThreshold[]
  exclude_patterns?: string[]
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

export function clearAllTriggers(): Promise<void> {
  return del('/api/triggers')
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

export function removeTriggerPack(packName: string): Promise<void> {
  return del(`/api/triggers/packs/${encodeURIComponent(packName)}`)
}

export function importTriggerPack(pack: TriggerPack): Promise<{ status: string; pack_name: string }> {
  return post<{ status: string; pack_name: string }>('/api/triggers/import', pack)
}

export function exportTriggerPack(): Promise<TriggerPack> {
  return get<TriggerPack>('/api/triggers/export')
}

export interface TriggerTestOverlayRequest {
  test_id: string
  text: string
  color: string
  duration_secs: number
  font_size?: number
  position?: { x: number; y: number } | null
}

export function fireTriggerTestOverlay(req: TriggerTestOverlayRequest): Promise<void> {
  return post<void>('/api/triggers/test-overlay', req)
}

export function postTriggerTestPosition(testId: string, position: { x: number; y: number }): Promise<void> {
  return post<void>('/api/triggers/test-overlay/position', { test_id: testId, position })
}

export function endTriggerTestSession(testId: string): Promise<void> {
  return post<void>('/api/triggers/test-overlay/end', { test_id: testId })
}

export interface ActiveTriggerTest {
  test_id: string
  text: string
  color: string
  duration_secs: number
  font_size?: number
  position?: { x: number; y: number } | null
}

export function getActiveTriggerTest(): Promise<ActiveTriggerTest | null> {
  return get<ActiveTriggerTest | null>('/api/triggers/test-overlay/active')
}

// ── Backend server info / port testing ───────────────────────────────────────

export interface ServerInfo {
  actual_port: number
  preferred_addr: string
}

export function getServerInfo(): Promise<ServerInfo> {
  return get<ServerInfo>('/api/config/server-info')
}

export interface TestPortResult {
  available: boolean
  error?: string
  in_use_by?: string
}

export function testPortAvailability(port: number): Promise<TestPortResult> {
  return get<TestPortResult>(`/api/config/test-port?port=${port}`)
}

export async function importGINAxml(
  xml: string,
  packName: string,
): Promise<{ status: string; pack_name: string; imported: number }> {
  const baseUrl = await getBackendBaseUrl()
  const url = `${baseUrl}/api/triggers/import-gina?pack_name=${encodeURIComponent(packName)}`
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/xml' },
    body: xml,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  return res.json()
}

// ── Roll Tracker ───────────────────────────────────────────────────────────────

export function getRolls(): Promise<RollsState> {
  return get<RollsState>('/api/rolls')
}

export function stopRollSession(id: number): Promise<RollsState> {
  return post<RollsState>(`/api/rolls/sessions/${id}/stop`)
}

export async function removeRollSession(id: number): Promise<void> {
  await del(`/api/rolls/sessions/${id}`)
}

export function clearRolls(): Promise<void> {
  return del('/api/rolls')
}

export function setRollWinnerRule(rule: WinnerRule): Promise<RollsState> {
  return put<RollsState>('/api/rolls/settings', { winner_rule: rule })
}

export function updateRollsSettings(patch: RollsSettingsPatch): Promise<RollsState> {
  return put<RollsState>('/api/rolls/settings', patch)
}
