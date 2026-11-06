import React, { Suspense, lazy, useEffect, useMemo, useState } from 'react'
import { useCachedState } from '../../hooks/useCachedState'
import { ChevronDown, ChevronRight, Map as MapIcon } from 'lucide-react'
import { useMapStyle } from '../../hooks/useMapStyle'
import { useMapZones } from '../../hooks/useMapZones'
import { useZoneMap } from '../../hooks/useZoneMap'
import { CATEGORY_STYLE, ZoneMap } from './ZoneMap'
import { ErrorBoundary } from '../ErrorBoundary'
import type { MapHighlight } from './ZoneMap'
import type { Pin3D } from './ZoneMap3D'
import type { MapPOICategory } from '../../types/map'
import { getNPCPatrols } from '../../services/api'
import type { NPCSpawnPoint, PatrolRoute } from '../../types/npc'

// three.js is large; it loads only when the 3D view is opened.
const ZoneMap3D = lazy(() => import('./ZoneMap3D'))

// NPCSpawnMap shows where an NPC spawns, closing the loop from the item page:
// purchased from -> vendor -> here.
//
// Expanded by default: "where is it" is one of the main reasons to open an NPC,
// and a collapsed section is easy to miss entirely. The state is remembered, so
// anyone who prefers it shut only has to close it once.

// Context layers only. The NPC's own spawn points are drawn as highlights, so
// showing every vendor and door as well would bury the thing being looked for.
const CONTEXT_LAYERS: MapPOICategory[] = ['zone_line', 'succor']

interface NPCSpawnMapProps {
  npcName: string
  npcId: number
  spawns: NPCSpawnPoint[]
  height?: number
  // collapsible adds the disclosure header. Off when this already has a tab of
  // its own, where a collapse toggle would only let you hide the tab's content.
  collapsible?: boolean
}

export function NPCSpawnMap({
  npcName,
  npcId,
  spawns,
  height = 300,
  collapsible = true,
}: NPCSpawnMapProps): React.ReactElement | null {
  const [open, setOpen] = useCachedState('npcs.mapOpen', true)

  // An NPC can spawn in several zones; each needs its own map.
  //
  // Filtered to zones we actually have a map for. Without this the picker
  // defaults to whichever zone happens to come first in the spawn list, and for
  // any NPC placed in an instanced copy — Gorenaire is in dreadlands_instanced
  // before The Dreadlands — that is a zone with no geometry, so the panel said
  // "No map available" while a perfectly good map sat one entry away.
  const { zones: mapped } = useMapZones()
  const zones = useMemo(() => {
    const haveMap = new Set(mapped.map((z) => z.zone))
    const seen = new Map<string, string>()
    for (const s of spawns) {
      if (s.zone && haveMap.has(s.zone) && !seen.has(s.zone)) {
        seen.set(s.zone, s.zone_name || s.zone)
      }
    }
    return [...seen.entries()].map(([zone, label]) => ({ zone, label }))
  }, [spawns, mapped])

  const [active, setActive] = useState<string | null>(null)
  // The map list arrives asynchronously, so the first render can legitimately
  // have no zones yet; adopt the first valid one when it does.
  useEffect(() => {
    if (zones.length === 0) return
    if (!active || !zones.some((z) => z.zone === active)) setActive(zones[0].zone)
  }, [zones, active])

  if (zones.length === 0) return null

  if (!collapsible) {
    return (
      <SpawnMapBody
        npcName={npcName}
        npcId={npcId}
        spawns={spawns}
        zones={zones}
        active={active ?? zones[0].zone}
        onZone={setActive}
        height={height}
      />
    )
  }

  return (
    <div>
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-1.5 py-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {open ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
        <MapIcon size={11} />
        Map
      </button>
      {open && (
        <SpawnMapBody
          npcName={npcName}
          npcId={npcId}
          spawns={spawns}
          zones={zones}
          active={active ?? zones[0].zone}
          onZone={setActive}
          height={height}
        />
      )}
    </div>
  )
}

function SpawnMapBody({
  npcName,
  npcId,
  spawns,
  zones,
  active,
  onZone,
  height,
}: {
  npcName: string
  npcId: number
  spawns: NPCSpawnPoint[]
  zones: { zone: string; label: string }[]
  active: string
  onZone: (z: string) => void
  height: number
}): React.ReactElement {
  // Always outline mode: this map exists to answer "where in the zone", at a
  // size where the detailed layers would be illegible anyway.
  const { zone, outline, pois, loading, error } = useZoneMap(active, 'outline')
  const { poiIgnoreZFade } = useMapStyle()

  // Spawn points arrive in game coordinates; the map works in map space, which
  // is the same negation the geometry pipeline applies.
  const highlights: MapHighlight[] = useMemo(
    () =>
      spawns
        .filter((s) => s.zone === active)
        .map((s) => ({ x: -s.x, y: -s.y, z: s.z })),
    [spawns, active],
  )

  const visible = useMemo(() => new Set(CONTEXT_LAYERS), [])

  // Patrol routes, fetched once per NPC and shown behind a toggle.
  //
  // Off by default and only offered when the NPC actually walks one: most do
  // not (about 17% of named NPCs have a grid), so a permanently visible control
  // would mostly advertise something that is not there.
  const [routes, setRoutes] = useState<PatrolRoute[]>([])
  const [showPatrol, setShowPatrol] = useCachedState('npcs.showPatrol', false)
  useEffect(() => {
    let cancelled = false
    getNPCPatrols(npcId)
      .then((r) => { if (!cancelled) setRoutes(r.routes ?? []) })
      .catch(() => { if (!cancelled) setRoutes([]) })
    return () => { cancelled = true }
  }, [npcId])

  const zoneRoutes = useMemo(
    () => routes.filter((r) => r.zone === active),
    [routes, active],
  )

  // 3D view, as on the other maps. Kept per surface like theirs.
  const [view3D, setView3D] = useCachedState('npcs.view3D', false)
  // Why 3D fell back to 2D for this zone, if it did.
  const [no3D, setNo3D] = useState<string | null>(null)
  useEffect(() => setNo3D(null), [active])
  const show3D = view3D && !no3D

  // The spawn points picked out over the context layers. A lone spawn is
  // flown to; several are left in the whole-zone view so none is off screen.
  const pins3D = useMemo((): Pin3D[] => {
    if (!show3D) return []
    const out: Pin3D[] = pois
      .filter((p) => visible.has(p.category))
      .map((p) => ({ x: p.x, y: p.y, z: p.z, color: CATEGORY_STYLE[p.category]?.color ?? '#94a3b8', label: p.label }))
    const label = npcName.replace(/_/g, ' ')
    highlights.forEach((h) =>
      out.push({ x: h.x, y: h.y, z: h.z ?? 0, color: '#ff4fd8', label, picked: true, focus: highlights.length === 1 }),
    )
    return out
  }, [show3D, pois, visible, highlights, npcName])

  if (error) {
    return (
      <p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        No map available for this zone.
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-1.5 pb-1">
      <div className="flex flex-wrap items-center gap-1">
        {zones.length > 1 && zones.map((z) => (
          <button
            key={z.zone}
            onClick={() => onZone(z.zone)}
            className="rounded border px-1.5 py-0.5 text-[10px]"
            style={{
              backgroundColor: z.zone === active ? 'var(--color-surface-2)' : 'transparent',
              borderColor: z.zone === active ? 'var(--color-primary)' : 'var(--color-border)',
              color: z.zone === active ? 'var(--color-primary)' : 'var(--color-muted)',
            }}
          >
            {z.label}
          </button>
        ))}
        <div className="ml-auto flex overflow-hidden rounded border" style={{ borderColor: 'var(--color-border)' }}>
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
      </div>
      {view3D && no3D && (
        <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>{no3D} Showing the 2D map.</span>
      )}

      {loading && !zone && (
        <p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>Loading map…</p>
      )}

      {zone && (
        <>
          <ErrorBoundary label="Spawn map">
            {show3D ? (
              <div style={{ height }}>
                <Suspense fallback={<p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>Loading 3D…</p>}>
                  <ZoneMap3D
                    key={active}
                    zone={active}
                    markers={[]}
                    pins={pins3D}
                    onUnavailable={(why) =>
                      setNo3D(
                        why === 'webgl'
                          ? '3D needs WebGL, which isn’t available here.'
                          : why === 'no-mesh'
                            ? `No ${active}.s3d in your EverQuest folder.`
                            : 'The 3D zone couldn’t be loaded.',
                      )
                    }
                  />
                </Suspense>
              </div>
            ) : (
            <ZoneMap
              zone={zone}
              geometry={null}
              outline={outline}
              mode="outline"
              pois={pois}
              visibleCategories={visible}
              poiIgnoreZFade={poiIgnoreZFade}
              highlights={highlights}
              paths={showPatrol ? zoneRoutes : undefined}
              height={height}
              showLabels={false}
            />
            )}
          </ErrorBoundary>
          {/* The 3D view has no path layer, so the toggle would do nothing. */}
          {zoneRoutes.length > 0 && !show3D && (
            <button
              onClick={() => setShowPatrol((v) => !v)}
              title="This NPC walks a fixed waypoint path. Patrol grids are server data — no downloaded map pack has them."
              className="self-start rounded border px-1.5 py-0.5 text-[10px] font-medium"
              style={{
                backgroundColor: showPatrol ? 'var(--color-surface-2)' : 'transparent',
                borderColor: showPatrol ? '#4ade80' : 'var(--color-border)',
                color: showPatrol ? '#4ade80' : 'var(--color-muted)',
              }}
            >
              {/* Naming matters: on a random grid these are places it may go,
                  not a route, and calling them a route would be a claim. */}
              {zoneRoutes.every((r) => !r.ordered) ? 'Roam area' : 'Patrol route'}
              {zoneRoutes.length > 1 ? ` (${zoneRoutes.length})` : ''}
            </button>
          )}
          <p className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
            {highlights.length === 1
              ? `Spawn point for ${npcName.replace(/_/g, ' ')}.`
              : `${highlights.length} spawn points for ${npcName.replace(/_/g, ' ')}.`}{' '}
            Marks where it spawns, not where it is now — roaming NPCs wander.
          </p>
        </>
      )}
    </div>
  )
}
