import type { Config } from '../types/config'
import type { Item, ItemSources, SearchResult } from '../types/item'
import type { NPC, NPCSpawns, NPCLootTable, NPCFaction, NPCSpells } from '../types/npc'
import type { BuffStatDelta, Spell, SpellCrossRefs, ShoppingRoute, ShoppingRouteOptions } from '../types/spell'
import type { Zone, ZoneConnection, ZoneGroundSpawn, ZoneForageItem, ZoneDropItem } from '../types/zone'
import type {
  ZealInventoryResponse,
  ZealSpellbookResponse,
  AllInventoriesResponse,
  ZealSpellsetsResponse,
  AllSpellsetsResponse,
  ZealInstallStatus,
  ZealPipeStatus,
} from '../types/zeal'
import type { KeysResponse, KeysProgressResponse } from '../types/keys'
import type {
  KeyringMasterResponse,
  KeyringCharactersResponse,
  KeyringCharacterResponse,
} from '../types/keyring'
import type {
  LockoutCharactersResponse,
  LockoutCharacterResponse,
} from '../types/lockouts'
import type { Backup, BackupsResponse } from '../types/backup'
import type { LogTailerStatus, LogFileInfo } from '../types/logEvent'
import type { TargetState } from '../types/overlay'
import type { CombatState, HistoryFacets, HistoryFilter, HistoryListResponse, StoredFight } from '../types/combat'
import type { TimerState } from '../types/timer'
import type { RespawnState } from '../types/respawn'
import type { Trigger, TriggerFired, TriggerPack, TriggerCategory, Action, TimerType, TimerAlertThreshold, TriggerSource, PipeCondition, ExtraPattern } from '../types/trigger'
import type { RollsState, RollsSettingsPatch, WinnerRule } from '../types/rolls'
import type { EnumsCatalog } from '../types/enums'
import type { RecipeSummary, RecipeDetail, RecipeTradeskillCount } from '../types/recipe'
import type { SkillsResponse } from '../types/skill'
import type {
  SandboxResult,
  SandboxSchemaResponse,
  SavedQuery,
  SavedQueryListResponse,
  SavedQueryPack,
  SavedQueryImportResponse,
} from '../types/sandbox'

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

async function del<T = void>(path: string): Promise<T> {
  const baseUrl = await getBackendBaseUrl()
  const res = await fetch(`${baseUrl}${path}`, { method: 'DELETE' })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error ?? res.statusText)
  }
  // Some endpoints (DELETE /api/app/import) return a small JSON body;
  // others (DELETE /api/backups/{id}) return 204 with no body. Try to parse,
  // fall back to undefined for the void case.
  const text = await res.text()
  if (!text) return undefined as T
  try {
    return JSON.parse(text) as T
  } catch {
    return undefined as T
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

// ── Tradeskill recipes ──────────────────────────────────────────────────────────

export interface RecipeSearchFilter {
  tradeskill?: number // -1 / undefined = any
  trivialMin?: number
  trivialMax?: number
}

export function searchRecipes(
  q: string,
  limit = 50,
  offset = 0,
  filter: RecipeSearchFilter = {},
): Promise<SearchResult<RecipeSummary>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (filter.tradeskill !== undefined && filter.tradeskill >= 0) {
    params.set('tradeskill', String(filter.tradeskill))
  }
  if (filter.trivialMin && filter.trivialMin > 0) params.set('trivial_min', String(filter.trivialMin))
  if (filter.trivialMax && filter.trivialMax > 0) params.set('trivial_max', String(filter.trivialMax))
  return get<SearchResult<RecipeSummary>>(`/api/recipes?${params}`)
}

export function getRecipe(id: number): Promise<RecipeDetail> {
  return get<RecipeDetail>(`/api/recipes/${id}`)
}

export function getRecipeTradeskills(): Promise<RecipeTradeskillCount[]> {
  return get<RecipeTradeskillCount[]>('/api/recipes/tradeskills')
}

export function getRecipeRaw(id: number): Promise<RawRow> {
  return get<RawRow>(`/api/recipes/${id}/raw`)
}

// Global (not per-character) favorite recipes.
export function listFavoriteRecipes(): Promise<RecipeSummary[]> {
  return get<RecipeSummary[]>('/api/favorite-recipes')
}

export function addFavoriteRecipe(recipeId: number): Promise<void> {
  return post<void>('/api/favorite-recipes', { recipe_id: recipeId })
}

export function removeFavoriteRecipe(recipeId: number): Promise<void> {
  return del(`/api/favorite-recipes/${recipeId}`)
}

// ── Enums catalog ──────────────────────────────────────────────────────────────

export function getEnums(): Promise<EnumsCatalog> {
  return get<EnumsCatalog>('/api/enums')
}

// ── Spells ─────────────────────────────────────────────────────────────────────

export function searchSpells(
  q: string,
  limit = 50,
  offset = 0,
  classIndex = -1,
  minLevel = 0,
  maxLevel = 0,
  goodEffectOnly = false,
): Promise<SearchResult<Spell>> {
  const params = new URLSearchParams({ q, limit: String(limit), offset: String(offset) })
  if (classIndex >= 0) params.set('class', String(classIndex))
  if (minLevel > 0) params.set('minLevel', String(minLevel))
  if (maxLevel > 0) params.set('maxLevel', String(maxLevel))
  if (goodEffectOnly) params.set('goodEffect', '1')
  return get<SearchResult<Spell>>(`/api/spells?${params}`)
}

export function getSpell(id: number): Promise<Spell> {
  return get<Spell>(`/api/spells/${id}`)
}

export function getSpellCrossRefs(id: number): Promise<SpellCrossRefs> {
  return get<SpellCrossRefs>(`/api/spells/${id}/items`)
}

export interface SpellStatDeltaEntry {
  name: string
  icon: number
  delta: BuffStatDelta
}

// Batch-resolve buff stat deltas for a list of spell IDs. Returns a map
// keyed by stringified spell ID. IDs that don't resolve to a spell are
// silently omitted. Each entry also includes the spell's name and icon so
// the raid-buff / live-buff UIs can render labels without a second fetch.
export function getSpellStatDeltas(ids: number[]): Promise<Record<string, SpellStatDeltaEntry>> {
  return post<Record<string, SpellStatDeltaEntry>>(`/api/spells/stat-deltas`, { ids })
}

// Compute an efficient shopping route covering the given spells: an ordered
// list of zones to visit (fewest-zones greedy set-cover), the spells/vendors
// at each, and any spells no vendor sells. Optionally exclude towns by
// alignment and order the stops from a starting zone. Used by the checklist.
export function getShoppingRoute(
  spellIds: number[],
  opts: ShoppingRouteOptions = {},
): Promise<ShoppingRoute> {
  return post<ShoppingRoute>(`/api/spells/shopping-route`, {
    spell_ids: spellIds,
    exclude_alignments: opts.excludeAlignments ?? [],
    start_zone: opts.startZone ?? '',
    include_pok: opts.includePoK ?? false,
    exclude_zones: opts.excludeZones ?? [],
  })
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

export function getNPCSpells(id: number): Promise<NPCSpells | null> {
  return get<NPCSpells | null>(`/api/npcs/${id}/spells`)
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

export function getZoneByShortName(shortName: string): Promise<Zone> {
  return get<Zone>(`/api/zones/short/${encodeURIComponent(shortName)}`)
}

// ── Raw row (experimental) ─────────────────────────────────────────────────────

export interface RawField {
  name: string
  value: unknown
}

export interface RawRow {
  table: string
  fields: RawField[]
}

export function getItemRaw(id: number): Promise<RawRow> {
  return get<RawRow>(`/api/items/${id}/raw`)
}

export function getSpellRaw(id: number): Promise<RawRow> {
  return get<RawRow>(`/api/spells/${id}/raw`)
}

export function getNPCRaw(id: number): Promise<RawRow> {
  return get<RawRow>(`/api/npcs/${id}/raw`)
}

export function getZoneRaw(id: number): Promise<RawRow> {
  return get<RawRow>(`/api/zones/${id}/raw`)
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

// ── Quarm client version ──────────────────────────────────────────────────────

export function getQuarmClientStatus(): Promise<import('../types/quarm').QuarmClientStatus> {
  return get<import('../types/quarm').QuarmClientStatus>('/api/quarm/client-status')
}

// ── Zeal ───────────────────────────────────────────────────────────────────────

export function detectZeal(path?: string): Promise<ZealInstallStatus> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return get<ZealInstallStatus>(`/api/zeal/detect${qs}`)
}

export function getZealPipeStatus(): Promise<ZealPipeStatus> {
  return get<ZealPipeStatus>('/api/zeal/pipe-status')
}

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

export function getZealSpellsets(character?: string): Promise<ZealSpellsetsResponse> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return get<ZealSpellsetsResponse>(`/api/zeal/spellsets${qs}`)
}

export function getAllSpellsets(): Promise<AllSpellsetsResponse> {
  return get<AllSpellsetsResponse>('/api/zeal/spellsets/all')
}

export function updateSpellsets(
  character: string,
  spellsets: { name: string; spell_ids: number[] }[],
): Promise<ZealSpellsetsResponse> {
  return put<ZealSpellsetsResponse>('/api/zeal/spellsets', { character, spellsets })
}

export function parseSpellsetsFile(path: string): Promise<ZealSpellsetsResponse> {
  return post<ZealSpellsetsResponse>('/api/zeal/spellsets/parse-file', { path })
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

// ── Keyring (per-character /keys snapshots) ───────────────────────────────────

export function getKeyringMaster(): Promise<KeyringMasterResponse> {
  return get<KeyringMasterResponse>('/api/keyring/master')
}

export function getKeyringCharacters(): Promise<KeyringCharactersResponse> {
  return get<KeyringCharactersResponse>('/api/keyring/characters')
}

export function getKeyringForCharacter(name: string): Promise<KeyringCharacterResponse> {
  return get<KeyringCharacterResponse>(`/api/keyring/characters/${encodeURIComponent(name)}`)
}

// ── Lockouts (per-character /sll loot & legacy-item lockouts) ─────────────────

export function getLockoutCharacters(): Promise<LockoutCharactersResponse> {
  return get<LockoutCharactersResponse>('/api/lockouts/characters')
}

export function getLockoutForCharacter(name: string): Promise<LockoutCharacterResponse> {
  return get<LockoutCharacterResponse>(`/api/lockouts/characters/${encodeURIComponent(name)}`)
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

// ── Players (/who sightings DB) ────────────────────────────────────────────────

export interface PlayerSearchFilters {
  search?: string
  class?: string
  zone?: string
  guild?: string
  pvp?: boolean
  limit?: number
  offset?: number
}

export function listPlayers(filters: PlayerSearchFilters = {}): Promise<import('../types/player').PlayerListResponse> {
  const params = new URLSearchParams()
  if (filters.search) params.set('search', filters.search)
  if (filters.class) params.set('class', filters.class)
  if (filters.zone) params.set('zone', filters.zone)
  if (filters.guild) params.set('guild', filters.guild)
  if (filters.pvp) params.set('pvp', '1')
  if (filters.limit) params.set('limit', String(filters.limit))
  if (filters.offset) params.set('offset', String(filters.offset))
  const qs = params.toString()
  return get<import('../types/player').PlayerListResponse>(`/api/players${qs ? '?' + qs : ''}`)
}

export function getPlayer(name: string): Promise<import('../types/player').PlayerSighting> {
  return get<import('../types/player').PlayerSighting>(`/api/players/${encodeURIComponent(name)}`)
}

export function getPlayerHistory(name: string): Promise<import('../types/player').PlayerHistoryResponse> {
  return get<import('../types/player').PlayerHistoryResponse>(`/api/players/${encodeURIComponent(name)}/history`)
}

export function updatePlayerNote(name: string, note: string, pvp: boolean): Promise<{ ok: boolean }> {
  return put<{ ok: boolean }>(`/api/players/${encodeURIComponent(name)}/note`, { note, pvp })
}

export function deletePlayer(name: string): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>(`/api/players/${encodeURIComponent(name)}`)
}

export function clearPlayers(): Promise<{ deleted: number }> {
  return post<{ deleted: number }>(`/api/players/clear`)
}

// ── Chat History (multi-channel player chat) ───────────────────────────────────

export interface ChatFilters {
  character?: string
  channel?: string
  search?: string
  from?: number // unix seconds
  to?: number
  sort?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

function chatParams(f: ChatFilters): string {
  const p = new URLSearchParams()
  if (f.character) p.set('character', f.character)
  if (f.channel) p.set('channel', f.channel)
  if (f.search) p.set('search', f.search)
  if (f.from) p.set('from', String(f.from))
  if (f.to) p.set('to', String(f.to))
  if (f.sort) p.set('sort', f.sort)
  if (f.limit) p.set('limit', String(f.limit))
  if (f.offset) p.set('offset', String(f.offset))
  const qs = p.toString()
  return qs ? '?' + qs : ''
}

export function getChatChannels(character?: string): Promise<import('../types/chat').ChatChannelsResponse> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return get<import('../types/chat').ChatChannelsResponse>(`/api/chat/channels${qs}`)
}

export function listChatConversations(f: ChatFilters = {}): Promise<import('../types/chat').ChatConversationListResponse> {
  return get<import('../types/chat').ChatConversationListResponse>(`/api/chat/conversations${chatParams(f)}`)
}

export function getChatThread(
  peer: string,
  opts: { character?: string; sort?: 'asc' | 'desc' } = {},
): Promise<import('../types/chat').ChatMessageListResponse> {
  return get<import('../types/chat').ChatMessageListResponse>(
    `/api/chat/thread/${encodeURIComponent(peer)}${chatParams(opts)}`,
  )
}

export function getChatFeed(f: ChatFilters = {}): Promise<import('../types/chat').ChatMessageListResponse> {
  return get<import('../types/chat').ChatMessageListResponse>(`/api/chat/feed${chatParams(f)}`)
}

export function deleteChatPeer(peer: string, character?: string): Promise<{ ok: boolean }> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return del<{ ok: boolean }>(`/api/chat/peer/${encodeURIComponent(peer)}${qs}`)
}

export function clearChat(character?: string, channel?: string): Promise<{ deleted: number }> {
  const p = new URLSearchParams()
  if (character) p.set('character', character)
  if (channel) p.set('channel', channel)
  const qs = p.toString()
  return post<{ deleted: number }>(`/api/chat/clear${qs ? '?' + qs : ''}`)
}

// ── Loot Tracker ───────────────────────────────────────────────────────────────

export interface LootFilters {
  character?: string
  search?: string
  player?: string
  zone?: string
  sort?: 'asc' | 'desc'
  limit?: number
  offset?: number
}

export function getLootMeta(character?: string): Promise<import('../types/loot').LootMetaResponse> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return get<import('../types/loot').LootMetaResponse>(`/api/loot/meta${qs}`)
}

export function listLoot(f: LootFilters = {}): Promise<import('../types/loot').LootListResponse> {
  const p = new URLSearchParams()
  if (f.character) p.set('character', f.character)
  if (f.search) p.set('search', f.search)
  if (f.player) p.set('player', f.player)
  if (f.zone) p.set('zone', f.zone)
  if (f.sort) p.set('sort', f.sort)
  if (f.limit) p.set('limit', String(f.limit))
  if (f.offset) p.set('offset', String(f.offset))
  const qs = p.toString()
  return get<import('../types/loot').LootListResponse>(`/api/loot${qs ? '?' + qs : ''}`)
}

export function clearLoot(character?: string): Promise<{ deleted: number }> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return post<{ deleted: number }>(`/api/loot/clear${qs}`)
}

// ── Log Backfill (retroactively populate trackers from a character's log) ──────

export interface BackfillSection {
  key: string
  label: string
}

export interface BackfillInfo {
  sections: BackfillSection[]
  characters: string[]
  active: string
}

export function getBackfillInfo(): Promise<BackfillInfo> {
  return get<BackfillInfo>('/api/backfill')
}

export function runBackfill(
  character: string,
  sections: string[],
): Promise<{ results: Record<string, number>; character: string }> {
  return post<{ results: Record<string, number>; character: string }>('/api/backfill', {
    character,
    sections,
  })
}

// ── App Backup (export/import full app state) ──────────────────────────────────

export interface AppBackupManifest {
  format_version: number
  app_version: string
  exported_at: string
  files: Array<{ name: string; size_bytes: number; sha256: string }>
  stats: { backup_count: number; total_size_bytes: number }
}

export function exportAppBackup(destinationPath: string): Promise<{ bundle_path: string; manifest: AppBackupManifest }> {
  return post<{ bundle_path: string; manifest: AppBackupManifest }>('/api/app/export', { destination_path: destinationPath })
}

export function previewAppImport(bundlePath: string): Promise<{ manifest: AppBackupManifest }> {
  return post<{ manifest: AppBackupManifest }>('/api/app/import/preview', { bundle_path: bundlePath })
}

export function stageAppImport(bundlePath: string): Promise<{ manifest: AppBackupManifest; restart_required: boolean }> {
  return post<{ manifest: AppBackupManifest; restart_required: boolean }>('/api/app/import', { bundle_path: bundlePath })
}

export function getAppImportPending(): Promise<{ pending: boolean }> {
  return get<{ pending: boolean }>('/api/app/import/pending')
}

export function cancelAppImport(): Promise<{ ok: boolean }> {
  return del<{ ok: boolean }>('/api/app/import')
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

// ── Log Browse (out-of-game viewer) ──────────────────────────────────────────

// One line from the log browser: the same shape as a live LogEvent plus the
// byte offset used as the pagination cursor. `type` is widened to string —
// browse surfaces every event type plus the 'log:raw' fallback, not just the
// live-feed subset.
export interface LogBrowseLine {
  type: string
  timestamp: string
  message: string
  data?: unknown
  offset: number
}

export interface LogBrowseResult {
  lines: LogBrowseLine[]
  // Cursor for the next (older) page, or null at the start of the file.
  next_offset: number | null
}

export function browseLog(req: {
  file: string
  q?: string
  type?: string
  beforeOffset?: number
  limit?: number
}): Promise<LogBrowseResult> {
  const p = new URLSearchParams({ file: req.file })
  if (req.q) p.set('q', req.q)
  if (req.type) p.set('type', req.type)
  if (req.beforeOffset) p.set('before_offset', String(req.beforeOffset))
  if (req.limit) p.set('limit', String(req.limit))
  return get<LogBrowseResult>(`/api/log/browse?${p.toString()}`)
}

// ── Log Replay ─────────────────────────────────────────────────────────────────

export interface ReplayFile {
  name: string
  character: string
  size_bytes: number
  modified: string
}

export interface ReplayStatus {
  state: 'idle' | 'playing' | 'paused'
  file?: string
  from?: string
  to?: string
  position?: string
  speed?: number
  lines_emitted: number
}

export function listReplayFiles(): Promise<ReplayFile[]> {
  return get<ReplayFile[]>('/api/replay/files')
}

export function getReplayInfo(file: string): Promise<{ first: string; last: string }> {
  return get<{ first: string; last: string }>(`/api/replay/info?file=${encodeURIComponent(file)}`)
}

export function getReplayStatus(): Promise<ReplayStatus> {
  return get<ReplayStatus>('/api/replay/status')
}

export function startReplay(req: {
  file: string
  from?: string
  to?: string
  speed?: number
}): Promise<ReplayStatus> {
  return post<ReplayStatus>('/api/replay/start', req)
}

export function pauseReplay(): Promise<ReplayStatus> {
  return post<ReplayStatus>('/api/replay/pause')
}

export function resumeReplay(): Promise<ReplayStatus> {
  return post<ReplayStatus>('/api/replay/resume')
}

export function stopReplay(): Promise<ReplayStatus> {
  return post<ReplayStatus>('/api/replay/stop')
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

export function clearTimers(category: 'buff' | 'detrimental' | 'custom' | 'ch_chain' | 'ch_chain_2' | 'all'): Promise<void> {
  return post<void>(`/api/overlay/timers/clear?category=${category}`)
}

export function removeTimer(id: string): Promise<void> {
  return del(`/api/overlay/timers/${encodeURIComponent(id)}`)
}

/** Start a manual countdown on the Custom Timers overlay (no trigger needed). */
export function startCustomTimer(name: string, durationSecs: number): Promise<void> {
  return post<void>('/api/overlay/timers/custom', { name, duration_secs: durationSecs })
}

export function getRespawnState(): Promise<RespawnState> {
  return get<RespawnState>('/api/overlay/respawns')
}

export function clearRespawns(): Promise<void> {
  return del('/api/overlay/respawns')
}

export function removeRespawn(id: string): Promise<void> {
  return del(`/api/overlay/respawns/${encodeURIComponent(id)}`)
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

export interface EqDiagnostics {
  eq_path: string
  has_logs: boolean
  character_count: number
  log_found: boolean
  log_enabled: boolean
  zeal_installed: boolean
  zeal_version?: string
  zeal_version_ok: boolean
  export_on_camp_found: boolean
  export_on_camp: boolean
}

export interface ValidateEQPathResponse {
  valid: boolean
  error?: string
  has_logs: boolean
  characters: DiscoveredCharacter[]
  diagnostics?: EqDiagnostics
}

export function validateEQPath(path: string): Promise<ValidateEQPathResponse> {
  return post<ValidateEQPathResponse>('/api/config/validate-eq-path', { path })
}

export function getEqDiagnostics(): Promise<EqDiagnostics> {
  return get<EqDiagnostics>('/api/config/eq-diagnostics')
}

export function setLogging(enabled: boolean): Promise<EqDiagnostics> {
  return post<EqDiagnostics>('/api/config/set-logging', { enabled })
}

export function setExportOnCamp(enabled: boolean): Promise<EqDiagnostics> {
  return post<EqDiagnostics>('/api/config/set-export-on-camp', { enabled })
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
  icon?: number // joined in by API from items.icon
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

// ── Gear Upgrade Finder ───────────────────────────────────────────────────────

// UpgradeWeights mirrors backend internal/upgrade.Weights — a per-stat scoring
// weight on an HP-equivalent scale (e.g. ac: 5 means 1 AC = 5 HP).
export interface UpgradeWeights {
  hp: number
  mana: number
  ac: number
  str: number
  sta: number
  agi: number
  dex: number
  wis: number
  int: number
  cha: number
  mr: number
  fr: number
  cr: number
  dr: number
  pr: number
  atk: number
  haste: number
  mana_regen: number
  dps: number
  focus_bonus: number
}

export interface UpgradeStatDelta {
  stat: string
  cand: number
  current: number
  effective: number
  weight: number
  weighted: number
  capped: boolean
}

export interface UpgradeCurrentItem {
  id: number
  name: string
  icon: number
  stats: Record<string, number>
  focus_effect: number
  focus_name: string
}

export interface UpgradeCandidate {
  id: number
  name: string
  icon: number
  slots: number
  nodrop: number
  req_level: number
  rec_level: number
  focus_effect: number
  focus_name: string
  score: number
  deltas: UpgradeStatDelta[]
  priority_focus: boolean
}

export interface UpgradesResponse {
  slot: string
  slot_label: string
  class: number
  level: number
  weights: UpgradeWeights
  current_items: UpgradeCurrentItem[]
  baseline_item_id: number
  candidates: UpgradeCandidate[]
  considered: number
  has_current_gear: boolean
}

export interface UpgradeWeightsResponse {
  weights: UpgradeWeights
  is_custom: boolean
  archetype: string
}

export function getCharacterUpgrades(
  id: number,
  opts: { slot: string; showAll?: boolean; showPoP?: boolean; limit?: number; weights?: UpgradeWeights },
): Promise<UpgradesResponse> {
  const p = new URLSearchParams({ slot: opts.slot })
  if (opts.showAll) p.set('show_all', '1')
  if (opts.showPoP) p.set('show_pop', '1')
  if (opts.limit) p.set('limit', String(opts.limit))
  if (opts.weights) p.set('weights', JSON.stringify(opts.weights))
  return get<UpgradesResponse>(`/api/characters/${id}/upgrades?${p.toString()}`)
}

export function getCharacterUpgradeWeights(id: number): Promise<UpgradeWeightsResponse> {
  return get<UpgradeWeightsResponse>(`/api/characters/${id}/upgrade-weights`)
}

export function setCharacterUpgradeWeights(
  id: number,
  weights: UpgradeWeights,
): Promise<UpgradeWeights> {
  return put<UpgradeWeights>(`/api/characters/${id}/upgrade-weights`, weights)
}

export function resetCharacterUpgradeWeights(id: number): Promise<UpgradeWeights> {
  return del<UpgradeWeights>(`/api/characters/${id}/upgrade-weights`)
}

export interface UpgradeOverviewSlot {
  slot: string
  slot_label: string
  current_items: UpgradeCurrentItem[]
  best: UpgradeCandidate | null
  considered: number
}

export interface UpgradesOverviewResponse {
  class: number
  level: number
  weights: UpgradeWeights
  slots: UpgradeOverviewSlot[]
  has_current_gear: boolean
}

export function getCharacterUpgradesOverview(
  id: number,
  weights?: UpgradeWeights,
  showPoP?: boolean,
): Promise<UpgradesOverviewResponse> {
  const p = new URLSearchParams()
  if (weights) p.set('weights', JSON.stringify(weights))
  if (showPoP) p.set('show_pop', '1')
  const qs = p.toString()
  return get<UpgradesOverviewResponse>(`/api/characters/${id}/upgrades/overview${qs ? `?${qs}` : ''}`)
}

export interface FocusOption {
  spell_id: number
  name: string
  count: number
}

export function getCharacterFocusOptions(id: number): Promise<FocusOption[]> {
  return get<FocusOption[]>(`/api/characters/${id}/focus-options`)
}

export function getCharacterPriorityFocus(id: number): Promise<{ spell_ids: number[] }> {
  return get<{ spell_ids: number[] }>(`/api/characters/${id}/priority-focus`)
}

export function setCharacterPriorityFocus(
  id: number,
  spellIds: number[],
): Promise<{ spell_ids: number[] }> {
  return put<{ spell_ids: number[] }>(`/api/characters/${id}/priority-focus`, { spell_ids: spellIds })
}

// ── Spell modifiers (focus extensions from items + AAs) ───────────────────────

export interface SpellModifierLimits {
  max_level?: number
  min_level?: number
  spell_type: number // -1 unset, 0 detrimental, 1 beneficial, 2 any
  min_duration_sec?: number
  exclude_effects?: number[]
  include_effects?: number[]
  include_spells?: number[]
  exclude_spells?: number[]
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
  // May be null over the wire if the backend ever emits a nil slice — always
  // null-coalesce before reading .length / .map (see ResolutionDisplay).
  applied: SpellModifier[] | null
  // True when the Permanent Illusion AA replaced the duration outright
  // (self-cast illusion, ~16h40m); the percent fields are meaningless then.
  permanent_illusion?: boolean
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

export function getCharacterSkills(id: number): Promise<SkillsResponse> {
  return get<SkillsResponse>(`/api/characters/${id}/skills`)
}

export function getZealQuarmy(character?: string): Promise<{ quarmy: QuarmyData | null }> {
  const qs = character ? `?character=${encodeURIComponent(character)}` : ''
  return get<{ quarmy: QuarmyData | null }>(`/api/zeal/quarmy${qs}`)
}

// SourceSplit attributes a stat total to its three contributing sources.
// The parts sum to the matching StatBlock field.
export interface SourceSplit {
  item: number; aa: number; buff: number
}

// StatBreakdown carries the per-source split for the stats the Stats panel
// exposes a hover breakdown on (issue #128). FT has no AA/buff source on Quarm;
// haste/dmg_shield have no AA source. For the capped stats (haste, spell_haste)
// the parts are raw source contributions and can sum above the capped total.
export interface StatBreakdown {
  mana_regen: SourceSplit
  regen: SourceSplit
  ft: SourceSplit
  attack: SourceSplit
  haste: SourceSplit
  spell_haste: SourceSplit
  dmg_shield: SourceSplit
}

export interface StatBlock {
  hp: number; mana: number; ac: number
  str: number; sta: number; agi: number; dex: number
  wis: number; int: number; cha: number
  pr: number; mr: number; dr: number; fr: number; cr: number
  attack: number; haste: number; spell_haste: number; regen: number
  mana_regen: number; ft: number; dmg_shield: number
  // atk_rating is the EQ inventory-window Attack rating (offense + to-hit),
  // distinct from `attack` (the raw worn/AA/buff +ATK bonus that feeds it).
  atk_rating: number
  breakdown: StatBreakdown
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

// DerivedStats carries one fully-derived StatBlock per display layer. Vitals
// (HP/mana/AC/resists) are computed by the backend from each layer's total
// attributes using Project Quarm's real formulas — the frontend just renders
// the block matching the active mode. preset_buff_ids/live_buff_ids are sent so
// the backend can fold those buffs into the +Buffs / Live layers.
export interface DerivedStats {
  character: string
  level: number
  class: number
  base: StatBlock
  equipped: StatBlock
  buffed: StatBlock
  live: StatBlock
}

export function getCharacterDerivedStats(
  id: number,
  presetBuffIDs: number[],
  liveBuffIDs: number[],
): Promise<DerivedStats> {
  return post<DerivedStats>(`/api/characters/${id}/derived-stats`, {
    preset_buff_ids: presetBuffIDs,
    live_buff_ids: liveBuffIDs,
  })
}

// ── Character Raid-Buff Preset ────────────────────────────────────────────────

// MAX_RAID_BUFF_SLOTS mirrors backend character.MaxRaidBuffSlots — EQ's 13
// simultaneous beneficial-buff cap.
export const MAX_RAID_BUFF_SLOTS = 13

// An empty list means the character hasn't customized their preset; the UI
// substitutes the default preset in that case.
export function getCharacterRaidBuffs(id: number): Promise<{ spell_ids: number[] }> {
  return get<{ spell_ids: number[] }>(`/api/characters/${id}/raid-buffs`)
}

export function setCharacterRaidBuffs(id: number, spellIDs: number[]): Promise<{ spell_ids: number[] }> {
  return put<{ spell_ids: number[] }>(`/api/characters/${id}/raid-buffs`, { spell_ids: spellIDs })
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

// ── Character Wishlist ─────────────────────────────────────────────────────────

import type {
  WishlistListResponse,
  WishlistEntry,
  WishlistSlotLayout,
} from '../types/wishlist'

export function listWishlist(charID: number): Promise<WishlistListResponse> {
  return get<WishlistListResponse>(`/api/characters/${charID}/wishlist`)
}

export function addWishlistEntries(
  charID: number,
  itemID: number,
  slots: string[],
): Promise<{ entries: WishlistEntry[] }> {
  return post<{ entries: WishlistEntry[] }>(`/api/characters/${charID}/wishlist`, {
    item_id: itemID,
    slots,
  })
}

export function deleteWishlistEntry(charID: number, entryID: number): Promise<void> {
  return del(`/api/characters/${charID}/wishlist/${entryID}`)
}

// Sends the character's full ordered entry-id list. Backend rejects the call
// if it doesn't match the character's current set of entries exactly — the
// frontend rebuilds the full order from its local state for every reorder.
export function reorderWishlist(charID: number, orderedIDs: number[]): Promise<void> {
  return put<void>(`/api/characters/${charID}/wishlist/reorder`, {
    ordered_ids: orderedIDs,
  })
}

export function updateWishlistSlotLayout(
  charID: number,
  layout: WishlistSlotLayout[],
): Promise<void> {
  return put<void>(`/api/characters/${charID}/wishlist/slot-layout`, { layout })
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
  /** Capture group supplying a dynamic timer duration ("" = fixed). */
  timer_duration_capture?: string
  /** Capture group naming the timer (one countdown per captured value). */
  timer_key_capture?: string
  worn_off_pattern?: string
  spell_id?: number
  display_threshold_secs?: number
  characters?: string[]
  timer_alerts?: TimerAlertThreshold[]
  exclude_patterns?: string[]
  /**
   * Additional patterns matched alongside `pattern` (any-match semantics).
   * NOTE: the backend replaces the stored list with this value on update —
   * full-update call sites must pass the trigger's current list through.
   */
  extra_patterns?: ExtraPattern[]
  /** Match source. Omitted = 'log' (backwards-compatible). */
  source?: TriggerSource
  /** Typed match definition for pipe-source triggers. */
  pipe_condition?: PipeCondition
  /**
   * Category (pack_name). Omit to leave the existing category untouched on
   * update; send '' for Uncategorized. Backend treats absent as no-change.
   */
  pack_name?: string
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

// ── Trigger categories ───────────────────────────────────────────────────────

export function listTriggerCategories(): Promise<TriggerCategory[]> {
  return get<TriggerCategory[]>('/api/triggers/categories')
}

export function createTriggerCategory(name: string): Promise<TriggerCategory> {
  return post<TriggerCategory>('/api/triggers/categories', { name })
}

export function renameTriggerCategory(name: string, newName: string): Promise<void> {
  return put<void>(`/api/triggers/categories/${encodeURIComponent(name)}`, { new_name: newName })
}

// deleteTriggerCategory removes a category. deleteTriggers=true deletes its
// triggers outright; false (default) moves them to Uncategorized.
export function deleteTriggerCategory(name: string, deleteTriggers = false): Promise<void> {
  const mode = deleteTriggers ? 'delete' : 'orphan'
  return del(`/api/triggers/categories/${encodeURIComponent(name)}?triggers=${mode}`)
}

// reorderTriggerCategories persists the display order of category sections.
export function reorderTriggerCategories(order: string[]): Promise<void> {
  return post<void>('/api/triggers/categories/order', { order })
}

// reorderTriggers persists the manual order of the given trigger IDs (their
// position in the array becomes their sort_order).
export function reorderTriggers(ids: string[]): Promise<void> {
  return post<void>('/api/triggers/order', { ids })
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
  // Resolved glow/font so the positioning card doubles as a live style
  // preview. Re-posting the same test_id restyles the card in place.
  glow_color?: string
  font_family?: string
  position?: { x: number; y: number } | null
}

export function fireTriggerTestOverlay(req: TriggerTestOverlayRequest): Promise<void> {
  return post<void>('/api/triggers/test-overlay', req)
}

export function postTriggerTestPosition(testId: string, position: { x: number; y: number }): Promise<void> {
  return post<void>('/api/triggers/test-overlay/position', { test_id: testId, position })
}

// endTriggerTestSession ends a positioning session. `cancelled` is relayed to
// the trigger editor so it can revert to the pre-session position on cancel
// (Escape) versus keeping the dragged position on confirm (Done).
export function endTriggerTestSession(testId: string, cancelled = false): Promise<void> {
  return post<void>('/api/triggers/test-overlay/end', { test_id: testId, cancelled })
}

export interface ActiveTriggerTest {
  test_id: string
  text: string
  color: string
  duration_secs: number
  font_size?: number
  glow_color?: string
  font_family?: string
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

// ── Developer SQL sandbox ──────────────────────────────────────────────────────

export function getSandboxSchema(): Promise<SandboxSchemaResponse> {
  return get<SandboxSchemaResponse>('/api/sandbox/schema')
}

export function runSandboxQuery(sql: string): Promise<SandboxResult> {
  return post<SandboxResult>('/api/sandbox/query', { sql })
}

// Saved-query CRUD + pack import/export. Backed by user.db so entries
// survive app updates (unlike the read-only quarm.db the sandbox queries
// against).

export interface SavedQueryInput {
  name: string
  description?: string
  sql: string
}

export function listSavedQueries(): Promise<SavedQueryListResponse> {
  return get<SavedQueryListResponse>('/api/sandbox/saved')
}

export function createSavedQuery(input: SavedQueryInput): Promise<SavedQuery> {
  return post<SavedQuery>('/api/sandbox/saved', input)
}

export function updateSavedQuery(id: string, input: SavedQueryInput): Promise<SavedQuery> {
  return put<SavedQuery>(`/api/sandbox/saved/${encodeURIComponent(id)}`, input)
}

export function deleteSavedQuery(id: string): Promise<void> {
  return del<void>(`/api/sandbox/saved/${encodeURIComponent(id)}`)
}

export function exportSavedQueryPack(): Promise<SavedQueryPack> {
  return get<SavedQueryPack>('/api/sandbox/saved/export')
}

export function importSavedQueryPack(pack: SavedQueryPack): Promise<SavedQueryImportResponse> {
  return post<SavedQueryImportResponse>('/api/sandbox/saved/import', pack)
}
