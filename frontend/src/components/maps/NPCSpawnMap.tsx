import React, { useMemo, useState } from 'react'
import { useCachedState } from '../../hooks/useCachedState'
import { ChevronDown, ChevronRight, Map as MapIcon } from 'lucide-react'
import { useZoneMap } from '../../hooks/useZoneMap'
import { ZoneMap } from './ZoneMap'
import { ErrorBoundary } from '../ErrorBoundary'
import type { MapHighlight } from './ZoneMap'
import type { MapPOICategory } from '../../types/map'
import type { NPCSpawnPoint } from '../../types/npc'

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
  spawns: NPCSpawnPoint[]
}

export function NPCSpawnMap({ npcName, spawns }: NPCSpawnMapProps): React.ReactElement | null {
  const [open, setOpen] = useCachedState('npcs.mapOpen', true)

  // An NPC can spawn in several zones; each needs its own map.
  const zones = useMemo(() => {
    const seen = new Map<string, string>()
    for (const s of spawns) {
      if (s.zone && !seen.has(s.zone)) seen.set(s.zone, s.zone_name || s.zone)
    }
    return [...seen.entries()].map(([zone, label]) => ({ zone, label }))
  }, [spawns])

  const [active, setActive] = useState(zones[0]?.zone ?? null)
  if (zones.length === 0) return null

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
          spawns={spawns}
          zones={zones}
          active={active ?? zones[0].zone}
          onZone={setActive}
        />
      )}
    </div>
  )
}

function SpawnMapBody({
  npcName,
  spawns,
  zones,
  active,
  onZone,
}: {
  npcName: string
  spawns: NPCSpawnPoint[]
  zones: { zone: string; label: string }[]
  active: string
  onZone: (z: string) => void
}): React.ReactElement {
  // Always outline mode: this map is 300px tall and exists to answer "where in
  // the zone", so legibility at a glance beats detail that would be illegible
  // at this size anyway.
  const { zone, outline, pois, loading, error } = useZoneMap(active, 'outline')

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

  if (error) {
    return (
      <p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        No map available for this zone.
      </p>
    )
  }

  return (
    <div className="flex flex-col gap-1.5 pb-1">
      {zones.length > 1 && (
        <div className="flex flex-wrap gap-1">
          {zones.map((z) => (
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
        </div>
      )}

      {loading && !zone && (
        <p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>Loading map…</p>
      )}

      {zone && (
        <>
          <ErrorBoundary label="Spawn map">
            <ZoneMap
              zone={zone}
              geometry={null}
              outline={outline}
              mode="outline"
              pois={pois}
              visibleCategories={visible}
              highlights={highlights}
              height={300}
              showLabels={false}
            />
          </ErrorBoundary>
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
