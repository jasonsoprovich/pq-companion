import React, { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Check, Copy, Plus, X } from 'lucide-react'
import { useCachedState } from '../../hooks/useCachedState'
import { usePlayerPosition } from '../../hooks/usePlayerPosition'
import { useMapStyle } from '../../hooks/useMapStyle'
import { useZoneMap } from '../../hooks/useZoneMap'
import { ZoneMap } from './ZoneMap'
import { ErrorBoundary } from '../ErrorBoundary'
import { mapMarkerCommand, mapShowZoneCommand } from '../../lib/zealMap'
import { formatNPCName } from '../SourceNPCLink'
import {
  createMapAnnotation,
  deleteMapAnnotation,
  getMapAnnotationExport,
  getZoneNPCLocations,
  listMapAnnotations,
  updateMapAnnotation,
} from '../../services/api'
import type {
  MapPOI, MapPOICategory, MapRenderMode, UserAnnotation, ZoneNPCLocation,
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
// plain NPC spawn point.
type SearchHit =
  | { kind: 'poi'; poi: MapPOI }
  | { kind: 'npc'; npc: ZoneNPCLocation }

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
}

// Negative ids keep user annotations from colliding with map_poi ids in the same
// list, and make "is this mine" checkable without a second lookup.
function annotationToPOI(a: UserAnnotation): MapPOI {
  return {
    id: -a.id, x: a.x, y: a.y, z: a.z,
    category: a.category, label: a.label, source: USER_SOURCE,
  }
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
}

export function ZoneMapPanel({
  zoneShortName,
  height = 'fill',
  showZoneButton = true,
  onJumpToZone,
  live = false,
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
  const { style, pack, ready } = useMapStyle()
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
  // An NPC found by search. Held apart from `selected` because it is not a POI
  // and must not pretend to be one — it belongs to no layer, has no category
  // toggle, and is only ever on screen because it was asked for.
  const [selectedNPC, setSelectedNPC] = useState<ZoneNPCLocation | null>(null)
  const [showDetail, setShowDetail] = useCachedState('maps.detail', true)
  // On by default: in a multi-level zone it is the difference between two lines
  // crossing and two lines at different heights, which a flat rendering cannot
  // express at all.
  const [heightColor, setHeightColor] = useCachedState('maps.heightColor', true)
  // Live position from Zeal. Null whenever we don't know — Zeal not running,
  // pipe stalled, not on Windows — so every consumer has one thing to check.
  const playerPos = usePlayerPosition()
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
      }
    : selectedNPC
      ? {
          category: selectedNPC.raid_target ? 'raid target' : 'npc',
          label: formatNPCName(selectedNPC.name),
          x: selectedNPC.x, y: selectedNPC.y, z: selectedNPC.z,
          route: `/npcs?select=${selectedNPC.npc_id}`,
          poi: null,
        }
      : null

  const clearSelection = (): void => { setSelected(null); setSelectedNPC(null) }

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
  const matches = useMemo((): SearchHit[] => {
    const q = query.trim().toLowerCase()
    if (!live || q.length < 2) return []
    const hits: SearchHit[] = allPois
      .filter((p) => p.label.toLowerCase().includes(q))
      .map((poi) => ({ kind: 'poi', poi }))
    // Skip NPCs already surfaced as a POI, so a vendor is not listed twice.
    const seen = new Set(allPois.map((p) => p.ref_id).filter(Boolean))
    for (const npc of npcs) {
      if (hits.length >= 40) break
      if (seen.has(npc.npc_id)) continue
      if (formatNPCName(npc.name).toLowerCase().includes(q)) {
        hits.push({ kind: 'npc', npc })
      }
    }
    return hits.slice(0, 40)
  }, [live, query, allPois, npcs])

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
              // Only offered when a pack is actually installed. Labelled with
              // the pack's own folder name, which is the only attribution these
              // files carry — better to credit what is on disk than to guess.
              ...(pack?.available
                ? ([[
                    'external',
                    pack.name ?? 'Map pack',
                    `Drawn from the map pack in your own EQ folder (${pack.dir}), ` +
                      `in its own colours. ${pack.zones} zones. Nothing is bundled ` +
                      `with PQ Companion — these are your files, read in place.`,
                  ]] as [MapRenderMode, string, string][])
                : []),
            ] as [MapRenderMode, string, string][]
          ).map(([key, label, title]) => (
            <button
              key={key}
              onClick={() => setMode(key)}
              title={title}
              className="px-2 py-0.5 text-[10px] font-medium"
              style={{
                backgroundColor: mode === key ? 'var(--color-primary)' : 'transparent',
                color: mode === key ? 'var(--color-background)' : 'var(--color-muted)',
              }}
            >
              {label}
            </button>
          ))}
        </div>

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
        {playerPos && playerPos.zone === zone.zone && (
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
          <button
            onClick={() => copy('showzone', mapShowZoneCommand(zone.zone))}
            title="Copy /map show_zone — preview this zone on your in-game map"
            className="ml-auto flex shrink-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
            style={{
              backgroundColor: 'var(--color-surface)',
              borderColor: copied === 'showzone' ? 'var(--color-primary)' : 'var(--color-border)',
              color: copied === 'showzone' ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
            }}
          >
            {copied === 'showzone' ? <Check size={9} /> : <Copy size={9} />}
            {copied === 'showzone' ? 'Copied' : 'Show in game'}
          </button>
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
              {matches.map((hit) => (
                <button
                  key={hit.kind === 'poi' ? `p${hit.poi.id}` : `n${hit.npc.npc_id}`}
                  onClick={() => {
                    // Selecting highlights it on the map and opens the inspector,
                    // which already carries "Pin in game" — so search lands in the
                    // same place a map click does rather than growing a second
                    // path to the same actions.
                    setQuery('')
                    if (hit.kind === 'npc') {
                      setSelected(null)
                      setSelectedNPC(hit.npc)
                      return
                    }
                    setSelectedNPC(null)
                    setSelected(hit.poi)
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
                  {hit.kind === 'npc' && (
                    // Level disambiguates the six identically-named guards a city
                    // zone routinely has.
                    <span
                      className="ml-auto shrink-0 text-[9px] tabular-nums"
                      style={{ color: 'var(--color-muted)' }}
                    >
                      L{hit.npc.level}
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
          <ZoneMap
            zone={zone}
            geometry={geometry}
            detail={detail}
            showDetail={showDetail}
            outline={outline}
            external={external}
            mode={mode}
            colorByHeight={heightColor}
            playerPos={playerPos}
            followPlayer={followPlayer}
            // Panning is a deliberate act; treat it as "I want to look
            // somewhere else" and stop following. The Follow me toggle above is
            // right there to turn it back on.
            onUserPan={() => setFollowPlayer(false)}
            onFollowRequest={() => setFollowPlayer(true)}
            pois={allPois}
            visibleCategories={visible}
            highlights={inspect ? [{ x: inspect.x, y: inspect.y, z: inspect.z }] : []}
            onPOIClick={(p) => { setSelectedNPC(null); setSelected(p) }}
            onEmptyClick={
              adding ? (x, y, z) => { setDraft({ x, y, z }); setAdding(false) } : undefined
            }
            height={height}
            depthSlot={depthHost}
          />
        </ErrorBoundary>
        </div>
      </div>

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

      {inspect && (
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
