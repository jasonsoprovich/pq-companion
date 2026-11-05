import React, { Suspense, lazy, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, Check, Copy, Plus, X } from 'lucide-react'
import { useCachedState } from '../../hooks/useCachedState'
import { usePlayerPosition } from '../../hooks/usePlayerPosition'
import { useGroupPositions } from '../../hooks/useGroupPositions'
import { useRaidPositions } from '../../hooks/useRaidPositions'
import { useDPSClassColors } from '../../hooks/useDPSClassColors'
import { classColor, groupColor } from '../../lib/raidColors'
import { useMapStyle } from '../../hooks/useMapStyle'
import { useZoneMap } from '../../hooks/useZoneMap'
import { CATEGORY_STYLE, ZoneMap, type AggroRing, type MapHighlight, type RaiderMarker, type ReplayTarget, type SpawnDot } from './ZoneMap'
import type { Pin3D } from './ZoneMap3D'

// three.js is large; it loads only when the 3D view is opened.
const ZoneMap3D = lazy(() => import('./ZoneMap3D'))

// Live marker colours in 3D, matching the 2D map's player arrow and
// groupmates.
const PLAYER_3D = '#f8fafc'
const GROUP_3D = '#7dd3fc'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import type { RespawnState } from '../../types/respawn'
import { useActiveCharacterLevel } from '../../hooks/useActiveCharacterLevel'
import { CON_HEX, levelCon } from '../../lib/conColor'
import { ErrorBoundary } from '../ErrorBoundary'
import {
  mapMarkerCommand,
  mapRestoreZoneCommand,
  mapShowZoneCommand,
} from '../../lib/zealMap'
import { formatNPCName } from '../SourceNPCLink'
import {
  createMapAnnotation,
  deleteMapAnnotation,
  getMapAnnotationExport,
  getZoneNPCLocations,
  getRespawnState,
  getZoneSpawnPoints,
  listMapAnnotations,
  updateMapAnnotation,
} from '../../services/api'
import type {
  MapPOI, MapPOICategory, MapRenderMode, UserAnnotation, ZoneNPCLocation, ZoneSpawnPoint,
} from '../../types/map'

// USER_SOURCE marks a pin as the user's own, so the inspector can offer edit and
// delete on exactly those. Annotations ride through the normal MapPOI pipeline —
// drawing, hit-testing, layer toggles, depth fade, search — rather than getting a
// parallel path that would have to reimplement all of it.
const USER_SOURCE = 'user'

// Annotation categories a user can choose. Mirrors mapgen's set; the backend
// rejects anything else, so this list is convenience rather than the gate.
const ANNOTATION_CATEGORIES: { key: UserAnnotation['category']; label: string }[] = [
  { key: 'wall', label: 'Wall' },
  { key: 'hazard', label: 'Hazard' },
  { key: 'note', label: 'Note' },
]

// SearchHit is one row in the search dropdown: either a curated map POI or a
// plain NPC. Grouped by name, so an NPC with five spawn points is one row that
// highlights all five — listing each spot separately and lighting only the one
// clicked made the other four look like they did not exist.
type SearchHit =
  | { kind: 'poi'; poi: MapPOI; spots: MapPOI[] }
  | { kind: 'npc'; npc: ZoneNPCLocation; spots: ZoneNPCLocation[] }

// SearchGroup is what "Show all" selects: every spot behind every hit for one
// query, so a whole family ("trap") lights up at once.
interface SearchGroup {
  query: string
  hits: number
  spots: MapHighlight[]
}

const MAX_SEARCH_ROWS = 40

// Inspection is the selected thing reduced to what the inspector bar shows, so
// a POI and an NPC render through one piece of markup instead of two that drift.
interface Inspection {
  category: string
  label: string
  x: number
  y: number
  z: number
  // route is the database page for this thing, when it has one.
  route: string | null
  // poi is set only for map POIs, and gates the edit/delete actions that apply
  // to the user's own markers.
  poi: MapPOI | null
  // spots is every location highlighted for this selection. More than one
  // when the thing has several spawn points; x/y/z is the first of them.
  spots: MapHighlight[]
}

// Negative ids keep user annotations from colliding with map_poi ids in the same
// list, and make "is this mine" checkable without a second lookup.
function annotationToPOI(a: UserAnnotation): MapPOI {
  return {
    id: -a.id, x: a.x, y: a.y, z: a.z,
    category: a.category, label: a.label, source: USER_SOURCE,
  }
}

function toSpot(p: { x: number; y: number; z: number }): MapHighlight {
  return { x: p.x, y: p.y, z: p.z }
}

// ZoneMapPanel is the full-zone map with layer toggles and a POI inspector.
//
// Shared by the Maps page and the Zones page Map tab. Kept as one component
// because the two surfaces want identical behaviour, and this codebase has
// already been bitten once by two copies of a list view drifting apart.

// Defaults are what a player is usually hunting for. Doors, ground spawns and
// tradeskill containers start off: switching everything on at once is what
// makes dense zones unreadable.
const LAYERS: { key: MapPOICategory; label: string; on: boolean }[] = [
  { key: 'vendor', label: 'Vendors', on: true },
  { key: 'raid_target', label: 'Raid targets', on: true },
  { key: 'zone_line', label: 'Zone lines', on: true },
  { key: 'trap', label: 'Traps', on: true },
  // All three default on despite adding to the default set: they are low-count
  // by nature (173 locked, 387 teleports and 29 switches across all 178 zones),
  // so they cannot clutter a zone the way doors or ground spawns do, and each
  // answers a question you would otherwise have to leave the app for.
  { key: 'locked', label: 'Locked', on: true },
  { key: 'teleport', label: 'Teleports', on: true },
  { key: 'switch', label: 'Switches', on: true },
  // Annotation layers, shipped (source=research) and the user's own. On by
  // default: someone deliberately recorded these, which is a stronger signal
  // than anything derived, and there are few of them.
  { key: 'wall', label: 'Walls', on: true },
  { key: 'hazard', label: 'Hazards', on: true },
  { key: 'note', label: 'Notes', on: true },
  { key: 'succor', label: 'Succor', on: false },
  { key: 'door', label: 'Doors', on: false },
  { key: 'ground_spawn', label: 'Ground spawns', on: false },
  { key: 'tradeskill', label: 'Tradeskills', on: false },
]

// entityRoute maps a POI to the database page its ref_id opens, or null when
// ref_id points at nothing a user can be sent to.
//
// ref_id is a foreign key into whichever table generated the POI, and those are
// not all entities: a zone line's is a zoneidnumber, a tradeskill container's is
// a bagtype enum, and a traps-table trap's is a row id in a table with no page.
// Routing those to /npcs lands on an unrelated NPC, so they get no link at all.
function entityRoute(poi: MapPOI): string | null {
  if (!poi.ref_id) return null
  switch (poi.category) {
    // The item that spawns, and the key that opens the door.
    case 'ground_spawn':
    case 'locked':
      return `/items?select=${poi.ref_id}`
    // Traps come from two places. The spawn2-derived ones are NPCs with a real
    // page; the traps-table ones are not.
    case 'trap':
      return poi.source === 'db:spawn2-trap' ? `/npcs?select=${poi.ref_id}` : null
    case 'zone_line':
    case 'tradeskill':
      return null
    default:
      return `/npcs?select=${poi.ref_id}`
  }
}

export interface ZoneMapPanelProps {
  zoneShortName: string | null
  // 'fill' takes the parent's full height, which is what both full-page
  // surfaces want — a fixed pixel height left dead space below the map and put
  // the depth control adrift in the middle of the pane.
  height?: number | 'fill'
  // showZoneButton adds the "Show in game" clipboard action. The Maps page puts
  // it in its own header instead.
  showZoneButton?: boolean
  // onJumpToZone, when given, adds a button to switch the displayed zone to
  // whichever one the player is standing in. Only surfaces when the live
  // position says they are somewhere else, so it never appears without a reason.
  onJumpToZone?: (zone: string) => void
  // live marks the surface as "the zone I am standing in" rather than "a zone I
  // looked up". It adds POI search and defaults follow-me on, and keeps its own
  // follow preference — the right default differs by surface, and sharing one
  // key would make turning it on for the live map hijack the Zones tab too.
  live?: boolean
  // replay hides the live player, group and follow-me, for a surface that
  // draws a recording instead. raiders are its markers.
  replay?: boolean
  raiders?: RaiderMarker[]
  replayTarget?: ReplayTarget | null
}

// Rings drawn when the active character's level is unknown.
const UNKNOWN_CON_HEX = '#94a3b8'

// spawnRing turns a spawn point into its aggro ring, judged by the most
// dangerous NPC it can roll: the widest radius, the highest level, and any
// see-invis.
//
// faint follows the server's aggro check (EQMacEmu Mob::CheckWillAggro): an
// NPC below level 18 that cons green to you won't attack on sight unless it
// is undead or flagged aggro_pc. Sitting overrides that, which the map can't
// know, so these are dimmed rather than hidden.
function spawnRing(p: ZoneSpawnPoint, playerLevel: number | null): AggroRing {
  let radius = 0
  let top = 0
  let seeInvis = false
  let faint = playerLevel != null
  for (const n of p.npcs) {
    const lvl = Math.max(n.level, n.max_level ?? 0)
    radius = Math.max(radius, n.aggro_radius)
    top = Math.max(top, lvl)
    if (n.see_invis > 0 || n.see_invis_undead > 0) seeInvis = true
    const passive =
      playerLevel != null && lvl < 18 && !n.undead && !n.aggro_pc &&
      levelCon(playerLevel, lvl) === 'green'
    if (!passive) faint = false
  }
  return {
    x: p.x,
    y: p.y,
    z: p.z,
    radius,
    color: playerLevel != null ? CON_HEX[levelCon(playerLevel, top)] : UNKNOWN_CON_HEX,
    faint,
    seeInvis,
  }
}

// Respawn dot colours: on cooldown (dim, it's dead), up within
// RESPAWN_SOON_SECS, popped, and no kill seen this session.
const DOT_COOLDOWN = '#4b5563'
const DOT_SOON = '#f59e0b'
const DOT_UP = '#22c55e'
const DOT_UNKNOWN = '#94a3b8'
const RESPAWN_SOON_SECS = 120

// AggroScope is which aggro rings are drawn: none, those within the chosen
// distance of you (fading out toward it), or every spawn point's.
type AggroScope = 'off' | 'near' | 'all'
const AGGRO_SCOPES: [AggroScope, string, string][] = [
  ['off', 'Off', 'No aggro rings'],
  ['near', 'Near', 'Only rings near you, fading out with distance'],
  ['all', 'All', 'Every spawn point’s ring'],
]

// spawnNames is who can spawn at a point, for the 3D map's hover.
function spawnNames(p: ZoneSpawnPoint): string {
  const names = [...new Set(p.npcs.map((n) => n.name.replace(/_/g, ' ')))]
  return names.length > 3 ? `${names.slice(0, 3).join(', ')} +${names.length - 3}` : names.join(', ')
}

function fmtRemaining(secs: number): string {
  const s = Math.ceil(secs)
  if (s >= 3600) return `${Math.floor(s / 3600)}h${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}`
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

function DotLegend(): React.ReactElement {
  const rows: [string, string][] = [
    [DOT_COOLDOWN, 'Dead, on cooldown'],
    [DOT_SOON, 'Up within 2 min'],
    [DOT_UP, 'Popped'],
    [DOT_UNKNOWN, 'No kill seen'],
  ]
  return (
    <div className="mt-1 space-y-0.5 text-[9px]" style={{ color: 'var(--color-muted)' }}>
      {rows.map(([c, label]) => (
        <div key={label} className="flex items-center gap-1">
          <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: c }} />
          {label}
        </div>
      ))}
    </div>
  )
}

export function ZoneMapPanel({
  zoneShortName,
  height = 'fill',
  showZoneButton = true,
  onJumpToZone,
  live = false,
  replay = false,
  raiders,
  replayTarget,
}: ZoneMapPanelProps): React.ReactElement {
  const navigate = useNavigate()
  // Detailed is the default. Outline is the cleaner drawing and the same style
  // in every zone, but it is built from a walkable-surface silhouette and so
  // omits features that are not walls — Oasis of Marr's lake is absent from the
  // outline and plainly there in the detail. In-game testing across many zones
  // put detailed ahead in the large majority of them; outline remains one click
  // away for the multi-level zones where the extra lines stack up.
  // The saved default from Settings, plus whether a map pack is installed.
  // `style` is already resolved — it falls back when a stored 'external'
  // outlives the files it names.
  const { style, pack, poiIgnoreZFade, showGroup, ready } = useMapStyle()
  // Session override. Starts unset so the saved default wins; switching the
  // mode here is for a look, not a preference change, so it lasts the session
  // and Settings stays the one place a default is decided.
  const [override, setOverride] = useCachedState<MapRenderMode | null>('maps.mode', null)
  const mode = override ?? style
  const setMode = setOverride
  const { zone, outline, geometry, detail, external, pois, loading, error } =
    useZoneMap(zoneShortName, mode)
  // Layer choices persist across zones and across the two surfaces, so a player
  // who turns doors on keeps them on.
  const [enabled, setEnabled] = useCachedState<MapPOICategory[]>(
    'maps.layers',
    LAYERS.filter((l) => l.on).map((l) => l.key),
  )
  const [selected, setSelected] = useState<MapPOI | null>(null)
  // Every spot of the POI picked from search, when it has more than one (one
  // trap name repeated across a zone). Empty for a map click, which is about
  // the single pin under the cursor.
  const [selectedSpots, setSelectedSpots] = useState<MapPOI[]>([])
  // An NPC found by search, as every one of its spawn points. Held apart from
  // `selected` because it is not a POI and must not pretend to be one — it
  // belongs to no layer, has no category toggle, and is only ever on screen
  // because it was asked for.
  const [selectedNPCs, setSelectedNPCs] = useState<ZoneNPCLocation[]>([])
  const selectedNPC = selectedNPCs[0] ?? null
  // "Show all" from the search box.
  const [searchGroup, setSearchGroup] = useState<SearchGroup | null>(null)
  const [showDetail, setShowDetail] = useCachedState('maps.detail', true)
  // On by default: in a multi-level zone it is the difference between two lines
  // crossing and two lines at different heights, which a flat rendering cannot
  // express at all.
  const [heightColor, setHeightColor] = useCachedState('maps.heightColor', true)
  // Live position from Zeal. Null whenever we don't know — Zeal not running,
  // pipe stalled, not on Windows — so every consumer has one thing to check.
  const playerPos = usePlayerPosition()
  const groupMembers = useGroupPositions()
  const raidMembers = useRaidPositions()
  const palette = useDPSClassColors()
  // Group and Raid toggles, per surface. Group starts from the Settings
  // default until it's flipped here; Raid starts on. Raiders are coloured by
  // raid group or by class.
  const [groupToggle, setGroupToggle] = useCachedState<boolean | null>(
    live ? 'liveMap.showGroup' : 'maps.showGroup',
    null,
  )
  const groupOn = groupToggle ?? showGroup
  const [raidOn, setRaidOn] = useCachedState(live ? 'liveMap.showRaid' : 'maps.showRaid', true)
  const [raidColor, setRaidColor] = useCachedState<'group' | 'class'>('maps.raidColor', 'group')
  // Follow the view to the player. Off by default: it takes pan away from you,
  // which is the wrong default while browsing a map you are not standing in.
  const [followPlayer, setFollowPlayer] = useCachedState(
    live ? 'liveMap.followPlayer' : 'maps.followPlayer',
    live,
  )
  const [query, setQuery] = useState('')
  const [annotations, setAnnotations] = useState<UserAnnotation[]>([])
  // adding is the "place a marker" mode: the next click on empty map creates one.
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState<{ x: number; y: number; z: number } | null>(null)
  const [editing, setEditing] = useState<UserAnnotation | null>(null)
  const [copied, setCopied] = useState<string | null>(null)
  // Container the map's height control is portalled into, so it spans the full
  // panel width below the rail instead of being indented by it.
  const [depthHost, setDepthHost] = useState<HTMLDivElement | null>(null)

  // A selection is a set of coordinates in one zone; carried into the next
  // zone it would put crosshairs on unrelated spots.
  useEffect(() => {
    setSelected(null)
    setSelectedSpots([])
    setSelectedNPCs([])
    setSearchGroup(null)
  }, [zoneShortName])

  useEffect(() => {
    if (!zoneShortName) {
      setAnnotations([])
      return
    }
    let cancelled = false
    listMapAnnotations(zoneShortName)
      .then((r) => { if (!cancelled) setAnnotations(r.annotations ?? []) })
      // A failure here must not take the map with it: annotations are additive.
      .catch(() => { if (!cancelled) setAnnotations([]) })
    return () => { cancelled = true }
  }, [zoneShortName])

  // Every NPC in the zone, for search. Fetched only on the live surface, where
  // the search box exists — browsing the Zones tab should not pull a few
  // hundred spawn rows nobody is going to query.
  const [npcs, setNPCs] = useState<ZoneNPCLocation[]>([])
  useEffect(() => {
    if (!live || !zoneShortName) {
      setNPCs([])
      return
    }
    let cancelled = false
    getZoneNPCLocations(zoneShortName)
      .then((r) => { if (!cancelled) setNPCs(r) })
      // Search still works over POIs without this; degrade rather than fail.
      .catch(() => { if (!cancelled) setNPCs([]) })
    return () => { cancelled = true }
  }, [live, zoneShortName])

  // Aggro rings: each spawn point's aggro radius, con-coloured against the
  // active character. Off by default — a busy zone is hundreds of circles,
  // which Near me cuts to the ones around you.
  const [aggroScope, setAggroScope] = useCachedState<AggroScope>('maps.aggroScope', 'off')
  const [aggroNear, setAggroNear] = useCachedState('maps.aggroNear', 400)
  const aggroOn = aggroScope !== 'off'
  // Respawn dots: every spawn point, coloured by the respawn timer of the
  // kill placed on it. hideKilled leaves out the ones on cooldown.
  const [dotsOn, setDotsOn] = useCachedState('maps.respawnDots', false)
  const [hideKilled, setHideKilled] = useCachedState('maps.hideKilled', false)
  const playerLevel = useActiveCharacterLevel()
  const [spawnPoints, setSpawnPoints] = useState<ZoneSpawnPoint[]>([])
  const wantSpawns = aggroOn || dotsOn
  useEffect(() => {
    if (!wantSpawns || !zoneShortName) {
      setSpawnPoints([])
      return
    }
    let cancelled = false
    getZoneSpawnPoints(zoneShortName)
      .then((r) => { if (!cancelled) setSpawnPoints(r) })
      .catch(() => { if (!cancelled) setSpawnPoints([]) })
    return () => { cancelled = true }
  }, [wantSpawns, zoneShortName])
  const [respawns, setRespawns] = useState<RespawnState | null>(null)
  const wsReady = useWebSocket((msg) => {
    // Rings want it too: a spawn that's dead has no ring.
    if (wantSpawns && msg.type === WSEvent.OverlayRespawns) setRespawns(msg.data as RespawnState)
  })
  useEffect(() => {
    if (!wantSpawns || wsReady !== 'open') return
    getRespawnState().then(setRespawns).catch(() => {})
  }, [wantSpawns, wsReady])
  // Seconds until each spawn point's mob is back, by spawn id; 0 or less
  // means up.
  const respawnLeft = useMemo(() => {
    const bySpawn = new Map<number, number>()
    for (const t of respawns?.timers ?? []) {
      if (t.zone !== zoneShortName || !t.spawn_id || t.other_copy) continue
      const prev = bySpawn.get(t.spawn_id)
      if (prev === undefined || t.remaining_seconds < prev) bySpawn.set(t.spawn_id, t.remaining_seconds)
    }
    return bySpawn
  }, [respawns, zoneShortName])
  const spawnDots = useMemo((): SpawnDot[] => {
    const out: SpawnDot[] = []
    for (const p of spawnPoints) {
      const left = respawnLeft.get(p.spawn_id)
      const name = spawnNames(p)
      if (left === undefined) {
        out.push({ x: p.x, y: p.y, z: p.z, color: DOT_UNKNOWN, known: false, name })
        continue
      }
      if (hideKilled && left > RESPAWN_SOON_SECS) continue
      const color = left <= 0 ? DOT_UP : left <= RESPAWN_SOON_SECS ? DOT_SOON : DOT_COOLDOWN
      out.push({
        x: p.x, y: p.y, z: p.z, color, known: true, name,
        label: left > 0 ? fmtRemaining(left) : 'up',
      })
    }
    return out
  }, [spawnPoints, respawnLeft, hideKilled])
  // Rings leave out spawns that are dead, and under Near me the ones beyond
  // the chosen distance; the rest fade out toward it. Without a position
  // here, Near me shows them all.
  const here = playerPos && zone && playerPos.zone === zone.zone ? playerPos : null
  const aggroRings = useMemo((): AggroRing[] => {
    const out: AggroRing[] = []
    for (const p of spawnPoints) {
      const left = respawnLeft.get(p.spawn_id)
      if (left !== undefined && left > 0) continue
      const ring = spawnRing(p, playerLevel)
      if (aggroScope === 'near' && here) {
        // Height counts double, so floors above and below drop out sooner.
        const d = Math.hypot(p.x - here.x, p.y - here.y, (p.z - here.z) * 2)
        if (d > aggroNear) continue
        ring.alpha = 1 - 0.75 * (d / aggroNear) ** 2
      }
      out.push(ring)
    }
    return out
  }, [spawnPoints, respawnLeft, playerLevel, aggroScope, aggroNear, here])

  const allPois = useMemo(
    () => [...pois, ...annotations.map(annotationToPOI)],
    [pois, annotations],
  )

  // User markers are never links: they carry no ref_id, and their actions are
  // Edit and Delete rather than "go look at this".
  const inspect: Inspection | null = selected
    ? {
        category: selected.category.replace('_', ' '),
        label: selected.label,
        x: selected.x, y: selected.y, z: selected.z,
        route: selected.source !== USER_SOURCE ? entityRoute(selected) : null,
        poi: selected,
        spots: (selectedSpots.length > 0 ? selectedSpots : [selected]).map(toSpot),
      }
    : selectedNPC
      ? {
          category: selectedNPC.raid_target ? 'raid target' : 'npc',
          label: formatNPCName(selectedNPC.name),
          x: selectedNPC.x, y: selectedNPC.y, z: selectedNPC.z,
          route: `/npcs?select=${selectedNPC.npc_id}`,
          poi: null,
          spots: selectedNPCs.map(toSpot),
        }
      : null

  const clearSelection = (): void => {
    setSelected(null)
    setSelectedSpots([])
    setSelectedNPCs([])
    setSearchGroup(null)
  }

  const visible = useMemo(() => new Set(enabled), [enabled])
  const counts = useMemo(() => {
    const m: Partial<Record<MapPOICategory, number>> = {}
    for (const p of allPois) m[p.category] = (m[p.category] ?? 0) + 1
    return m
  }, [allPois])

  // Search, live surface only. Matches on the label, which is what a player
  // would type — "key", "trap", a name — rather than on category.
  //
  // POIs first, then every other NPC in the zone. Ordered that way because a
  // POI is a curated answer and an NPC row is a raw one: searching "banker"
  // should lead with the marked banker rather than a same-named guard. NPC
  // names are stored underscored, so both the match and the display go through
  // formatNPCName or "Ward Pungill" would never match "Ward_Pungill".
  //
  // Not capped: "Show all" has to reach every match, not just the rows the
  // dropdown has room for. The dropdown slices its own view.
  const matches = useMemo((): SearchHit[] => {
    const q = query.trim().toLowerCase()
    if (!live || q.length < 2) return []
    const hits: SearchHit[] = []
    const poiRow = new Map<string, Extract<SearchHit, { kind: 'poi' }>>()
    for (const poi of allPois) {
      if (!poi.label.toLowerCase().includes(q)) continue
      const key = `${poi.category}|${poi.label}`
      const row = poiRow.get(key)
      if (row) {
        row.spots.push(poi)
      } else {
        const hit = { kind: 'poi' as const, poi, spots: [poi] }
        poiRow.set(key, hit)
        hits.push(hit)
      }
    }
    // Skip NPCs already surfaced as a POI, so a vendor is not listed twice.
    const seen = new Set(allPois.map((p) => p.ref_id).filter(Boolean))
    const npcRow = new Map<string, Extract<SearchHit, { kind: 'npc' }>>()
    for (const npc of npcs) {
      if (seen.has(npc.npc_id)) continue
      if (!formatNPCName(npc.name).toLowerCase().includes(q)) continue
      // By name, not id: a named mob often has one npc_types row per spawn
      // point, and they are still the same thing to a player.
      const row = npcRow.get(npc.name)
      if (row) {
        row.spots.push(npc)
      } else {
        const hit = { kind: 'npc' as const, npc, spots: [npc] }
        npcRow.set(npc.name, hit)
        hits.push(hit)
      }
    }
    return hits
  }, [live, query, allPois, npcs])
  const matchSpots = matches.reduce((n, h) => n + h.spots.length, 0)

  const saveDraft = (category: string, label: string): void => {
    if (!draft || !zoneShortName) return
    createMapAnnotation(zoneShortName, { ...draft, category, label })
      .then((a) => {
        setAnnotations((prev) => [...prev, a])
        setDraft(null)
        // Make sure the layer it landed in is on, or it saves invisibly.
        if (!visible.has(a.category)) setEnabled((prev) => [...prev, a.category])
      })
      .catch(() => setDraft(null))
  }

  const saveEdit = (category: string, label: string): void => {
    if (!editing) return
    updateMapAnnotation(editing.id, { category, label })
      .then((a) => {
        setAnnotations((prev) => prev.map((x) => (x.id === a.id ? a : x)))
        setEditing(null)
        setSelected(null)
      })
      .catch(() => setEditing(null))
  }

  const removeAnnotation = (id: number): void => {
    deleteMapAnnotation(id)
      .then(() => {
        setAnnotations((prev) => prev.filter((x) => x.id !== id))
        setSelected(null)
      })
      .catch(() => {/* leave it on screen rather than lying about the delete */})
  }

  const exportAnnotations = (): void => {
    getMapAnnotationExport()
      .then((doc) => copy('export', JSON.stringify(doc, null, 2)))
      .catch(() => {/* nothing to copy */})
  }

  const copy = (key: string, text: string): void => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  if (error) {
    return (
      <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>
        No map available for this zone.
      </p>
    )
  }
  // Wait for the saved style before drawing. Without this the panel mounts in
  // the built-in default, then switches when the preference lands — a visible
  // flicker plus a wasted geometry fetch for a layer nobody asked for.
  // 3D view, from the client's own zone file. Kept per surface like follow-me.
  // Not offered in a replay, which has its own switch.
  const [view3D, setView3D] = useCachedState(live ? 'liveMap.view3D' : 'maps.view3D', false)
  // Why 3D fell back to 2D for this zone, if it did.
  const [no3D, setNo3D] = useState<string | null>(null)
  useEffect(() => setNo3D(null), [zoneShortName])
  const show3D = view3D && !replay && !no3D

  // The raid around you, live: everyone in your zone but you, and but your
  // group while it's shown, since those are drawn as your group.
  const liveRaiders = useMemo((): RaiderMarker[] => {
    if (replay || !raidOn || !here) return []
    const grouped = new Set(groupOn ? groupMembers.map((g) => g.name) : [])
    return raidMembers
      .filter((m) => !grouped.has(m.name))
      .map((m) => ({
        x: m.x, y: m.y, z: m.z, heading: m.heading, label: m.name,
        color: raidColor === 'class' ? classColor(palette, m.class) : groupColor(m.group),
      }))
  }, [replay, raidOn, here, groupOn, groupMembers, raidMembers, raidColor, palette])

  // You, your group and the raid as 3D markers; the camera follows you with
  // Follow me. Your group comes after the raid so it draws over it.
  const markers3D = useMemo((): RaiderMarker[] => {
    if (!show3D || !playerPos || !zone || playerPos.zone !== zone.zone) return []
    const out: RaiderMarker[] = [...liveRaiders]
    if (groupOn) {
      for (const g of groupMembers) {
        out.push({ x: g.x, y: g.y, z: g.z, heading: g.heading, color: GROUP_3D, label: g.name, mine: true })
      }
    }
    out.push({
      x: playerPos.x, y: playerPos.y, z: playerPos.z, heading: playerPos.heading,
      color: PLAYER_3D, label: 'You', focus: followPlayer, mine: true,
    })
    return out
  }, [show3D, playerPos, zone, groupMembers, groupOn, liveRaiders, followPlayer])

  // The visible pin layers and your markers, plus whatever search or a click
  // picked out, as 3D pins.
  const pins3D = useMemo((): Pin3D[] => {
    if (!show3D) return []
    const out: Pin3D[] = allPois
      .filter((p) => visible.has(p.category))
      .map((p) => ({ x: p.x, y: p.y, z: p.z, color: CATEGORY_STYLE[p.category]?.color ?? '#94a3b8', label: p.label }))
    const picked = searchGroup ? searchGroup.spots : inspect ? inspect.spots : []
    for (const h of picked) out.push({ x: h.x, y: h.y, z: h.z ?? 0, color: '#ff4fd8', label: h.label ?? '', picked: true })
    return out
  }, [show3D, allPois, visible, searchGroup, inspect])

  if (!zone || !ready) {
    return (
      <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>
        {loading ? 'Loading map…' : 'Select a zone.'}
      </p>
    )
  }

  const fill = height === 'fill'

  return (
    <div className={`flex flex-col gap-2${fill ? ' h-full' : ''}`}>
      {/* Top bar: what the map IS drawn from, and actions on the whole map.
          Kept apart from the layer toggles on the left rail, which are about
          what is drawn ON it — mixing the two in one wrapping row is what made
          this hard to scan and left "Show in game" adrift mid-row. */}
      <div className="flex shrink-0 flex-wrap items-center gap-2">
        <div
          className="flex overflow-hidden rounded border"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {(
            [
              ['outline', 'Outline', 'Clean single-line map — the same style in every zone'],
              [
                'detailed',
                'Detailed',
                'Full extracted geometry: elevation contours or every floor edge. ' +
                  'Far more information, and busier in multi-level zones.',
              ],
              // Labelled with the pack's own folder name when one is installed,
              // which is the only attribution these files carry — better to
              // credit what is on disk than to guess.
              //
              // With no pack the button stays, greyed, and leads to the setup
              // instructions instead of switching style. Hiding it outright was
              // the other option, but then a third style exists that the user
              // has no way to discover; a dead-looking button they can click to
              // find out why is the more answerable version of the confusion.
              pack?.available
                ? [
                    'external',
                    pack.name ?? 'Map pack',
                    `Drawn from the map pack in your own EQ folder (${pack.dir}), ` +
                      `in its own colours. ${pack.zones} zones. Nothing is bundled ` +
                      `with PQ Companion — these are your files, read in place.`,
                  ]
                : [
                    'external',
                    'Map pack',
                    'No map pack found in your EverQuest folder. Click to see how ' +
                      'to install one — hand-drawn maps show things the extracted ' +
                      'ones cannot, like water.',
                  ],
            ] as [MapRenderMode, string, string][]
          ).map(([key, label, title]) => {
            const unavailable = key === 'external' && !pack?.available
            return (
              <button
                key={key}
                onClick={() =>
                  unavailable ? navigate('/settings?tab=maps') : setMode(key)
                }
                title={title}
                className="px-2 py-0.5 text-[10px] font-medium"
                style={{
                  backgroundColor: mode === key ? 'var(--color-primary)' : 'transparent',
                  color: mode === key ? 'var(--color-background)' : 'var(--color-muted)',
                  opacity: unavailable ? 0.45 : 1,
                }}
              >
                {label}
              </button>
            )
          })}
        </div>

        {!replay && (
          <div className="flex overflow-hidden rounded border" style={{ borderColor: 'var(--color-border)' }}>
            {([false, true] as const).map((v) => (
              <button
                key={String(v)}
                onClick={() => {
                  setNo3D(null)
                  setView3D(v)
                }}
                title={v ? 'Solid 3D view from your own EverQuest zone files' : 'Flat map'}
                className="px-2 py-0.5 text-[10px] font-medium"
                style={{
                  backgroundColor: view3D === v ? 'var(--color-primary)' : 'transparent',
                  color: view3D === v ? 'var(--color-background)' : 'var(--color-muted)',
                }}
              >
                {v ? '3D' : '2D'}
              </button>
            ))}
          </div>
        )}
        {view3D && no3D && (
          <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>{no3D} Showing the 2D map.</span>
        )}

        {/* Also about how the map is drawn, so it belongs up here with the
            style switch rather than among the pin layers. */}
        {zone.z_span >= 40 && mode !== 'external' && (
          <button
            onClick={() => setHeightColor((v) => !v)}
            title="Tint lines by height — cool below, warm above, so stacked levels are distinguishable"
            className="rounded border px-1.5 py-0.5 text-[10px] font-medium"
            style={{
              backgroundColor: heightColor ? 'var(--color-surface-2)' : 'transparent',
              borderColor: heightColor ? 'var(--color-primary)' : 'var(--color-border)',
              color: heightColor ? 'var(--color-primary)' : 'var(--color-muted)',
            }}
          >
            Height colours
          </button>
        )}
        {mode === 'detailed' && detail && detail.count > 0 && (
          <button
            onClick={() => setShowDetail((v) => !v)}
            title="Fine boundary detail drawn under the main map"
            className="rounded border px-1.5 py-0.5 text-[10px] font-medium"
            style={{
              backgroundColor: showDetail ? 'var(--color-surface-2)' : 'transparent',
              borderColor: showDetail ? 'var(--color-primary)' : 'var(--color-border)',
              color: showDetail ? 'var(--color-primary)' : 'var(--color-muted)',
            }}
          >
            Detail <span className="opacity-60">{detail.count}</span>
          </button>
        )}

        {/* Both of these appear only with a live position, so they are absent
            entirely rather than disabled when Zeal is not running — a greyed
            control invites a question we cannot answer from here. */}
        {!replay && playerPos && playerPos.zone === zone.zone && (
          <button
            onClick={() => setFollowPlayer((v) => !v)}
            title="Keep the view centred on you as you move"
            className="rounded border px-1.5 py-0.5 text-[10px] font-medium"
            style={{
              backgroundColor: followPlayer ? 'var(--color-surface-2)' : 'transparent',
              borderColor: followPlayer ? 'var(--color-primary)' : 'var(--color-border)',
              color: followPlayer ? 'var(--color-primary)' : 'var(--color-muted)',
            }}
          >
            Follow me
          </button>
        )}
        {onJumpToZone && playerPos && playerPos.zone !== zone.zone && (
          <button
            onClick={() => onJumpToZone(playerPos.zone)}
            title="Show the zone you are standing in"
            className="rounded border px-1.5 py-0.5 text-[10px] font-medium"
            style={{ borderColor: 'var(--color-primary)', color: 'var(--color-primary)' }}
          >
            You are in {playerPos.zone}
          </button>
        )}

        {showZoneButton && (
          <>
            <button
              onClick={() => copy('showzone', mapShowZoneCommand(zone.zone))}
              title="Copy /map show_zone — preview this zone on your in-game map"
              className="ml-auto flex shrink-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor: copied === 'showzone' ? 'var(--color-primary)' : 'var(--color-border)',
                color:
                  copied === 'showzone' ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              }}
            >
              {copied === 'showzone' ? <Check size={9} /> : <Copy size={9} />}
              {copied === 'showzone' ? 'Copied' : 'Show in game'}
            </button>
            {/* The way back out. Preview mode sticks until it is cleared, so
                without this the only exit is knowing the bare command. */}
            <button
              onClick={() => copy('restorezone', mapRestoreZoneCommand())}
              title="Copy /map show_zone with no zone — put your own zone back on the in-game map"
              className="flex shrink-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor:
                  copied === 'restorezone' ? 'var(--color-primary)' : 'var(--color-border)',
                color:
                  copied === 'restorezone'
                    ? 'var(--color-primary)'
                    : 'var(--color-muted-foreground)',
              }}
            >
              {copied === 'restorezone' ? <Check size={9} /> : <Copy size={9} />}
              {copied === 'restorezone' ? 'Copied' : 'Restore'}
            </button>
          </>
        )}
      </div>

      {live && (
        <div className="relative shrink-0">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Find in this zone — an NPC, a vendor, a key, a trap…"
            className="w-full rounded border px-2 py-1 text-xs outline-none"
            style={{
              backgroundColor: 'var(--color-surface)',
              borderColor: 'var(--color-border)',
              color: 'var(--color-foreground)',
            }}
          />
          {matches.length > 0 && (
            // Absolute so the results float over the map rather than shoving it
            // down and rescaling the canvas on every keystroke.
            <div
              className="absolute inset-x-0 top-full z-10 mt-0.5 max-h-56 overflow-y-auto rounded border"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor: 'var(--color-border)',
              }}
            >
              {matchSpots > 1 && (
                // Every spot behind every row, at once — the answer to "where
                // are all the traps", which picking rows one by one never gave.
                <button
                  onClick={() => {
                    const q = query.trim()
                    setQuery('')
                    setSelected(null)
                    setSelectedSpots([])
                    setSelectedNPCs([])
                    setSearchGroup({
                      query: q,
                      hits: matches.length,
                      spots: matches.flatMap((h) =>
                        (h.spots as (MapPOI | ZoneNPCLocation)[]).map(toSpot)),
                    })
                    const layers = new Set(
                      matches.flatMap((h) => (h.kind === 'poi' ? [h.poi.category] : [])),
                    )
                    const hidden = [...layers].filter((c) => !visible.has(c))
                    if (hidden.length > 0) setEnabled((prev) => [...prev, ...hidden])
                  }}
                  className="flex w-full items-center gap-2 border-b px-2 py-1 text-left text-xs font-medium"
                  style={{ borderColor: 'var(--color-border)', color: 'var(--color-primary)' }}
                >
                  Show all {matchSpots} on the map
                </button>
              )}
              {matches.slice(0, MAX_SEARCH_ROWS).map((hit) => (
                <button
                  key={hit.kind === 'poi' ? `p${hit.poi.id}` : `n${hit.npc.npc_id}`}
                  onClick={() => {
                    // Selecting highlights it on the map and opens the inspector,
                    // which already carries "Pin in game" — so search lands in the
                    // same place a map click does rather than growing a second
                    // path to the same actions.
                    setQuery('')
                    setSearchGroup(null)
                    if (hit.kind === 'npc') {
                      setSelected(null)
                      setSelectedSpots([])
                      setSelectedNPCs(hit.spots)
                      return
                    }
                    setSelectedNPCs([])
                    setSelected(hit.poi)
                    setSelectedSpots(hit.spots)
                    // A hidden layer would highlight something invisible. NPC
                    // hits need no equivalent — they belong to no layer, and the
                    // highlight is drawn regardless of the toggles.
                    if (!visible.has(hit.poi.category)) {
                      setEnabled((prev) => [...prev, hit.poi.category])
                    }
                  }}
                  className="flex w-full items-center gap-2 border-b px-2 py-1 text-left text-xs last:border-b-0"
                  style={{ borderColor: 'var(--color-border)' }}
                >
                  <span
                    className="shrink-0 text-[9px] uppercase tracking-wider"
                    style={{ color: 'var(--color-muted)' }}
                  >
                    {hit.kind === 'poi'
                      ? hit.poi.category.replace('_', ' ')
                      : hit.npc.raid_target
                        ? 'raid'
                        : 'npc'}
                  </span>
                  <span className="truncate" style={{ color: 'var(--color-foreground)' }}>
                    {hit.kind === 'poi' ? hit.poi.label : formatNPCName(hit.npc.name)}
                  </span>
                  {(hit.spots.length > 1 || hit.kind === 'npc') && (
                    // Level disambiguates the six identically-named guards a city
                    // zone routinely has; the count says one row is several spots.
                    <span
                      className="ml-auto shrink-0 text-[9px] tabular-nums"
                      style={{ color: 'var(--color-muted)' }}
                    >
                      {hit.spots.length > 1 && `×${hit.spots.length}`}
                      {hit.spots.length > 1 && hit.kind === 'npc' && ' · '}
                      {hit.kind === 'npc' && `L${hit.npc.level}`}
                    </span>
                  )}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <div className={`flex gap-2${fill ? ' min-h-0 flex-1' : ''}`}>
        {/* Layer rail. Vertical rather than a wrapping row above the map: the
            list is fixed and long, and as a row its height changed per zone —
            which moved the map itself every time you switched. A column of the
            same width keeps the canvas anchored. */}
        <div className="flex w-32 shrink-0 flex-col gap-1 overflow-y-auto">
          {LAYERS.map((l) => {
            const n = counts[l.key] ?? 0
            const active = visible.has(l.key)
            return (
              <button
                key={l.key}
                disabled={n === 0}
                onClick={() =>
                  setEnabled((prev) =>
                    prev.includes(l.key) ? prev.filter((k) => k !== l.key) : [...prev, l.key],
                  )
                }
                className="flex items-center justify-between rounded border px-1.5 py-0.5 text-[10px] font-medium disabled:opacity-30"
                style={{
                  backgroundColor: active ? 'var(--color-surface-2)' : 'transparent',
                  borderColor: active ? 'var(--color-primary)' : 'var(--color-border)',
                  color: active ? 'var(--color-primary)' : 'var(--color-muted)',
                }}
              >
                <span>{l.label}</span>
                {n > 0 && <span className="opacity-60 tabular-nums">{n}</span>}
              </button>
            )
          })}

          {/* People: your group and the raid, live. */}
          {!replay && (
            <div className="mt-1 space-y-1 border-t pt-1.5" style={{ borderColor: 'var(--color-border)' }}>
              {(
                [
                  ['Group', groupOn, () => setGroupToggle(!groupOn), groupMembers.length,
                    'Your groupmates in this zone (Zeal)'],
                  ['Raid', raidOn, () => setRaidOn(!raidOn), liveRaiders.length,
                    'Everyone else in your raid who is in this zone (Zeal)'],
                ] as const
              ).map(([label, on, toggle, n, title]) => (
                <button
                  key={label}
                  onClick={toggle}
                  title={title}
                  className="flex w-full items-center justify-between rounded border px-1.5 py-0.5 text-[10px] font-medium"
                  style={{
                    backgroundColor: on ? 'var(--color-surface-2)' : 'transparent',
                    borderColor: on ? 'var(--color-primary)' : 'var(--color-border)',
                    color: on ? 'var(--color-primary)' : 'var(--color-muted)',
                  }}
                >
                  <span>{label}</span>
                  {on && n > 0 && <span className="opacity-60 tabular-nums">{n}</span>}
                </button>
              ))}
              {raidOn && (
                <div className="flex overflow-hidden rounded border" style={{ borderColor: 'var(--color-border)' }}>
                  {(
                    [
                      ['group', 'By group', 'Colour raiders by raid group'],
                      ['class', 'By class', 'Colour raiders by class, using your DPS meter class colours'],
                    ] as const
                  ).map(([v, label, title]) => (
                    <button
                      key={v}
                      onClick={() => setRaidColor(v)}
                      title={title}
                      className="flex-1 px-1 py-0.5 text-[10px] font-medium"
                      style={{
                        backgroundColor: raidColor === v ? 'var(--color-primary)' : 'transparent',
                        color: raidColor === v ? 'var(--color-background)' : 'var(--color-muted)',
                      }}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Rings are derived from spawn data rather than being POIs, so
              they sit apart from the layer list. */}
          <div className="mt-1 border-t pt-1.5" style={{ borderColor: 'var(--color-border)' }}>
            <div
              className="flex items-center justify-between text-[10px] font-medium"
              style={{ color: aggroOn ? 'var(--color-primary)' : 'var(--color-muted)' }}
              title={
                'Aggro radius around spawn points, coloured by how the ' +
                'toughest mob there cons to your active character. Faint: a ' +
                "green mob under level 18 that won't attack on sight (unless you " +
                'sit). The eye marks a spawn that can see invisible. Spawns ' +
                'known to be dead have no ring. Aggro also needs a hostile ' +
                'faction and line of sight.'
              }
            >
              <span>Aggro rings</span>
              {aggroOn && aggroRings.length > 0 && (
                <span className="opacity-60 tabular-nums">{aggroRings.length}</span>
              )}
            </div>
            <div className="mt-0.5 flex overflow-hidden rounded border" style={{ borderColor: 'var(--color-border)' }}>
              {AGGRO_SCOPES.map(([v, label, title]) => (
                <button
                  key={v}
                  onClick={() => setAggroScope(v)}
                  title={title}
                  className="flex-1 px-1 py-0.5 text-[10px] font-medium"
                  style={{
                    backgroundColor: aggroScope === v ? 'var(--color-primary)' : 'transparent',
                    color: aggroScope === v ? 'var(--color-background)' : 'var(--color-muted)',
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
            {aggroScope === 'near' && (
              <label className="mt-1 block text-[9px]" style={{ color: 'var(--color-muted)' }}>
                <div className="flex justify-between">
                  <span>Within</span>
                  <span className="tabular-nums">{aggroNear}</span>
                </div>
                <input
                  type="range"
                  min={100}
                  max={1500}
                  step={50}
                  value={aggroNear}
                  onChange={(e) => setAggroNear(Number(e.target.value))}
                  className="w-full"
                />
                {!here && <span>No position here, so all rings show.</span>}
              </label>
            )}
            {aggroOn && playerLevel == null && (
              <p className="mt-1 text-[9px] leading-tight" style={{ color: 'var(--color-muted)' }}>
                No character level yet, so rings aren't con-coloured.
              </p>
            )}
            <button
              onClick={() => setDotsOn((v) => !v)}
              title={
                'Every spawn point, coloured by its respawn timer. A kill is ' +
                "placed on the nearest free spawn point for that mob, judged " +
                'from its position when you had it targeted (Zeal) or yours. ' +
                'Without Zeal, only mobs with a single spawn point can be placed.'
              }
              className="mt-1 flex w-full items-center justify-between rounded border px-1.5 py-0.5 text-[10px] font-medium"
              style={{
                backgroundColor: dotsOn ? 'var(--color-surface-2)' : 'transparent',
                borderColor: dotsOn ? 'var(--color-primary)' : 'var(--color-border)',
                color: dotsOn ? 'var(--color-primary)' : 'var(--color-muted)',
              }}
            >
              <span>Respawn dots</span>
              {dotsOn && spawnDots.length > 0 && (
                <span className="opacity-60 tabular-nums">{spawnDots.length}</span>
              )}
            </button>
            {dotsOn && (
              <label
                className="mt-1 flex items-center gap-1 text-[9px]"
                style={{ color: 'var(--color-muted)' }}
                title="Leave out spawn points whose mob is dead and not back within 2 minutes"
              >
                <input type="checkbox" checked={hideKilled} onChange={(e) => setHideKilled(e.target.checked)} />
                Hide killed
              </label>
            )}
            {dotsOn && <DotLegend />}
          </div>

          {/* Set apart deliberately: everything above toggles markers that
              already exist, this one creates one. A dashed border and its own
              spacing say "different kind of thing" without needing a label to
              explain it. */}
          <div className="mt-1 border-t pt-1.5" style={{ borderColor: 'var(--color-border)' }}>
            <button
              onClick={() => { setAdding((v) => !v); setDraft(null) }}
              title={
                adding
                  ? 'Click the map to place a marker, or press again to cancel'
                  : 'Add your own marker — click here, then click the map'
              }
              className="flex w-full items-center justify-center gap-1 rounded border border-dashed px-1.5 py-1 text-[10px] font-semibold"
              style={{
                backgroundColor: adding ? 'var(--color-primary)' : 'transparent',
                borderColor: adding ? 'var(--color-primary)' : 'var(--color-muted)',
                color: adding ? 'var(--color-background)' : 'var(--color-muted-foreground)',
              }}
            >
              <Plus size={10} />
              {adding ? 'Click map…' : 'Add marker'}
            </button>
            {annotations.length > 0 && (
              <button
                onClick={exportAnnotations}
                title="Copy your markers as JSON, in the format the shipped annotation file uses — for sharing or submitting"
                className="mt-1 w-full rounded border px-1.5 py-0.5 text-[10px] font-medium"
                style={{
                  borderColor: copied === 'export' ? 'var(--color-primary)' : 'var(--color-border)',
                  color: copied === 'export' ? 'var(--color-primary)' : 'var(--color-muted)',
                }}
              >
                {copied === 'export' ? 'Copied' : `Export ${annotations.length}`}
              </button>
            )}
          </div>
        </div>

        {/* A render fault in the canvas must not unmount the app. One did:
            a null deref in the drag handler blanked the whole window.
            min-w-0/min-h-0 are what let this shrink inside the flex parents
            instead of forcing the rail and inspector off the edge. */}
        <div className={`min-w-0 flex-1${fill ? ' min-h-0' : ''}`}>
        <ErrorBoundary label="Zone map">
          {show3D && zoneShortName ? (
            <Suspense fallback={<div className="p-4 text-sm" style={{ color: 'var(--color-muted)' }}>Loading 3D…</div>}>
              <ZoneMap3D
                key={zoneShortName}
                zone={zoneShortName}
                markers={markers3D}
                pins={pins3D}
                rings={aggroOn ? aggroRings : undefined}
                spawnDots={dotsOn ? spawnDots : undefined}
                onUnavailable={(why) =>
                  setNo3D(
                    why === 'webgl'
                      ? '3D needs WebGL, which isn’t available here.'
                      : why === 'no-mesh'
                        ? `No ${zoneShortName}.s3d in your EverQuest folder.`
                        : 'The 3D zone couldn’t be loaded.',
                  )
                }
              />
            </Suspense>
          ) : (
          <ZoneMap
            zone={zone}
            geometry={geometry}
            detail={detail}
            showDetail={showDetail}
            outline={outline}
            external={external}
            mode={mode}
            colorByHeight={heightColor}
            playerPos={replay ? null : playerPos}
            groupMembers={groupOn && !replay ? groupMembers : []}
            followPlayer={followPlayer && !replay}
            raiders={replay ? raiders : liveRaiders}
            replayTarget={replayTarget}
            // Panning is a deliberate act; treat it as "I want to look
            // somewhere else" and stop following. The Follow me toggle above is
            // right there to turn it back on.
            onUserPan={() => setFollowPlayer(false)}
            onFollowRequest={() => setFollowPlayer(true)}
            pois={allPois}
            visibleCategories={visible}
            poiIgnoreZFade={poiIgnoreZFade}
            highlights={searchGroup ? searchGroup.spots : inspect ? inspect.spots : []}
            aggroRings={aggroOn ? aggroRings : undefined}
            spawnDots={dotsOn ? spawnDots : undefined}
            onPOIClick={(p) => {
              setSelectedNPCs([])
              setSelectedSpots([])
              setSearchGroup(null)
              setSelected(p)
            }}
            onEmptyClick={
              adding ? (x, y, z) => { setDraft({ x, y, z }); setAdding(false) } : undefined
            }
            height={height}
            depthSlot={depthHost}
          />
          )}
        </ErrorBoundary>
        </div>
      </div>

      {/* A pack can be installed and still not cover this zone — packs are
          published per-expansion and the zone count on the Settings tab is a
          total, not a guarantee. Without this the canvas just goes blank, which
          reads as the style being broken rather than a missing file. */}
      {mode === 'external' && !loading && external?.count === 0 && (
        <div
          className="flex shrink-0 items-start gap-2 rounded border px-2 py-1.5 text-xs"
          style={{ borderColor: 'var(--color-danger)', color: 'var(--color-muted-foreground)' }}
        >
          <AlertTriangle size={13} className="mt-px shrink-0" style={{ color: 'var(--color-danger)' }} />
          <span>
            Your map pack has no file for {zone.zone}. Switch to Outline or
            Detailed for this zone, or add{' '}
            <span className="font-mono">{zone.zone}.txt</span> to{' '}
            <span className="font-mono">{pack?.dir ?? 'the pack folder'}</span>.
          </span>
        </div>
      )}

      {/* A pack drawn for a different EverQuest. Stated rather than hidden: the
          lines render perfectly and our own pins land correctly on top, so
          nothing about a wrong map looks wrong unless you already know the
          zone. */}
      {mode === 'external' && external?.alignment?.mismatch && (
        <div
          className="flex shrink-0 items-start gap-2 rounded border px-2 py-1.5 text-xs"
          style={{ borderColor: '#f59e0b', color: 'var(--color-muted-foreground)' }}
        >
          <AlertTriangle size={13} className="mt-px shrink-0" style={{ color: '#f59e0b' }} />
          <span>
            This pack's map for {zone.zone} does not match Project Quarm's — its
            landmarks sit about {external.alignment.offset} units from where the
            server puts them. Almost certainly a zone revamped in a later
            expansion, so the pack drew a different version of it. The pins are
            still correct; the lines are not.
          </span>
        </div>
      )}

      {/* Full width, under both the rail and the canvas. Empty and zero-height
          for flat zones, where the map has no depth control to dock. */}
      <div ref={setDepthHost} className="flex shrink-0 items-center gap-2 empty:hidden" />

      {(draft || editing) && (
        <AnnotationForm
          key={editing ? `edit-${editing.id}` : 'new'}
          initial={editing ?? undefined}
          onCancel={() => { setDraft(null); setEditing(null) }}
          onSave={editing ? saveEdit : saveDraft}
        />
      )}

      {searchGroup && (
        <div
          className="flex shrink-0 items-center gap-2 rounded border px-2 py-1.5 text-sm"
          style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        >
          <span
            className="shrink-0 text-[10px] uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            search
          </span>
          <span className="truncate" style={{ color: 'var(--color-foreground)' }}>
            “{searchGroup.query}”
          </span>
          <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
            {searchGroup.spots.length} spots
            {searchGroup.hits > 1 && ` across ${searchGroup.hits} names`}
          </span>
          <button className="ml-auto" onClick={clearSelection} title="Clear selection">
            <X size={12} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>
      )}

      {!searchGroup && inspect && (
        <div
          className="flex shrink-0 items-center gap-2 rounded border px-2 py-1.5 text-sm"
          style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        >
          <span
            className="shrink-0 text-[10px] uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            {inspect.category}
          </span>
          {inspect.route ? (
            <button
              onClick={() => navigate(inspect.route as string)}
              className="truncate text-left underline decoration-dotted"
              style={{ color: 'var(--color-primary)' }}
            >
              {inspect.label}
            </button>
          ) : (
            <span className="truncate" style={{ color: 'var(--color-foreground)' }}>
              {inspect.label}
            </span>
          )}
          <span className="shrink-0 font-mono text-xs" style={{ color: 'var(--color-muted)' }}>
            {/* Shown in /loc order and game sign, matching what EQ prints, so it
                can be read straight across to the game. */}
            {-inspect.y}, {-inspect.x}, {inspect.z}
          </span>
          {inspect.spots.length > 1 && (
            <span
              className="shrink-0 text-xs"
              title="Every spawn point is highlighted; the coordinates and pin are the first"
              style={{ color: 'var(--color-muted)' }}
            >
              +{inspect.spots.length - 1} more
            </span>
          )}
          <div className="ml-auto flex shrink-0 items-center gap-1">
            {inspect.poi && inspect.poi.source === USER_SOURCE ? (
              <>
                <button
                  onClick={() => {
                    const a = annotations.find((x) => x.id === -(inspect.poi as MapPOI).id)
                    if (a) setEditing(a)
                  }}
                  className="rounded border px-1.5 py-0.5 text-[10px]"
                  style={{ borderColor: 'var(--color-border)', color: 'var(--color-primary)' }}
                >
                  Edit
                </button>
                <button
                  onClick={() => removeAnnotation(-(inspect.poi as MapPOI).id)}
                  className="rounded border px-1.5 py-0.5 text-[10px]"
                  style={{ borderColor: 'var(--color-border)', color: '#f87171' }}
                >
                  Delete
                </button>
              </>
            ) : null}
            <button
              onClick={() =>
                // Map space back to game coordinates, which is the same negation.
                copy('pin', mapMarkerCommand(-inspect.x, -inspect.y, inspect.label))
              }
              className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{
                borderColor: copied === 'pin' ? 'var(--color-primary)' : 'var(--color-border)',
                color: copied === 'pin' ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              }}
            >
              {copied === 'pin' ? <Check size={9} /> : <Copy size={9} />}
              {copied === 'pin' ? 'Copied' : 'Pin in game'}
            </button>
            <button onClick={clearSelection} title="Clear selection">
              <X size={12} style={{ color: 'var(--color-muted)' }} />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// AnnotationForm edits a new or existing user marker.
//
// Inline rather than a modal: the map stays visible, so you can see the pin you
// are naming. A modal would cover the one thing that gives the label meaning.
function AnnotationForm({
  initial,
  onSave,
  onCancel,
}: {
  initial?: UserAnnotation
  onSave: (category: string, label: string) => void
  onCancel: () => void
}): React.ReactElement {
  const [category, setCategory] = useState<string>(initial?.category ?? 'note')
  const [label, setLabel] = useState(initial?.label ?? '')

  return (
    <div
      className="flex shrink-0 items-center gap-2 rounded border px-2 py-1.5"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-primary)' }}
    >
      <span
        className="shrink-0 text-[10px] uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {initial ? 'Edit marker' : 'New marker'}
      </span>
      <select
        value={category}
        onChange={(e) => setCategory(e.target.value)}
        className="rounded border px-1 py-0.5 text-xs"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          borderColor: 'var(--color-border)',
          color: 'var(--color-foreground)',
        }}
      >
        {ANNOTATION_CATEGORIES.map((c) => (
          <option key={c.key} value={c.key}>{c.label}</option>
        ))}
      </select>
      <input
        autoFocus
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        // Enter saves, Escape cancels — this sits between a map click and the
        // next one, so reaching for the mouse again is the wrong rhythm.
        onKeyDown={(e) => {
          if (e.key === 'Enter' && label.trim()) onSave(category, label.trim())
          if (e.key === 'Escape') onCancel()
        }}
        placeholder="What is here? e.g. Fake wall — walk through to the ledge"
        className="flex-1 rounded border px-2 py-0.5 text-xs outline-none"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          borderColor: 'var(--color-border)',
          color: 'var(--color-foreground)',
        }}
      />
      <button
        onClick={() => label.trim() && onSave(category, label.trim())}
        disabled={!label.trim()}
        className="rounded px-2 py-0.5 text-[10px] font-medium disabled:opacity-40"
        style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
      >
        Save
      </button>
      <button
        onClick={onCancel}
        className="text-[10px]"
        style={{ color: 'var(--color-muted)' }}
      >
        Cancel
      </button>
    </div>
  )
}
