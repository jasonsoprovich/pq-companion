import React, { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Check, Copy, X } from 'lucide-react'
import { useCachedState } from '../../hooks/useCachedState'
import { usePlayerPosition } from '../../hooks/usePlayerPosition'
import { useZoneMap } from '../../hooks/useZoneMap'
import { ZoneMap } from './ZoneMap'
import { ErrorBoundary } from '../ErrorBoundary'
import { mapMarkerCommand, mapShowZoneCommand } from '../../lib/zealMap'
import type { MapPOI, MapPOICategory, MapRenderMode } from '../../types/map'

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
  { key: 'succor', label: 'Succor', on: false },
  { key: 'door', label: 'Doors', on: false },
  { key: 'ground_spawn', label: 'Ground spawns', on: false },
  { key: 'tradeskill', label: 'Tradeskills', on: false },
]

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
}

export function ZoneMapPanel({
  zoneShortName,
  height = 'fill',
  showZoneButton = true,
  onJumpToZone,
}: ZoneMapPanelProps): React.ReactElement {
  const navigate = useNavigate()
  // Outline is the default: one clean line drawing, the same in every zone.
  // Detailed carries far more information but reads as a stack of overlapping
  // levels in tall zones, which is a deliberate trade rather than the everyday
  // view.
  const [mode, setMode] = useCachedState<MapRenderMode>('maps.mode', 'outline')
  const { zone, outline, geometry, detail, pois, loading, error } = useZoneMap(
    zoneShortName,
    mode,
  )
  // Layer choices persist across zones and across the two surfaces, so a player
  // who turns doors on keeps them on.
  const [enabled, setEnabled] = useCachedState<MapPOICategory[]>(
    'maps.layers',
    LAYERS.filter((l) => l.on).map((l) => l.key),
  )
  const [selected, setSelected] = useState<MapPOI | null>(null)
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
  const [followPlayer, setFollowPlayer] = useCachedState('maps.followPlayer', false)
  const [copied, setCopied] = useState<string | null>(null)

  const visible = useMemo(() => new Set(enabled), [enabled])
  const counts = useMemo(() => {
    const m: Partial<Record<MapPOICategory, number>> = {}
    for (const p of pois) m[p.category] = (m[p.category] ?? 0) + 1
    return m
  }, [pois])

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
  if (!zone) {
    return (
      <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>
        {loading ? 'Loading map…' : 'Select a zone.'}
      </p>
    )
  }

  const fill = height === 'fill'

  return (
    <div className={`flex flex-col gap-2${fill ? ' h-full' : ''}`}>
      <div className="flex shrink-0 flex-wrap items-center gap-1">
        {/* Mode first, and set apart: it changes what the other controls mean,
            so it does not belong in the run of POI toggles. */}
        <div
          className="mr-1.5 flex overflow-hidden rounded border"
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
              className="rounded border px-1.5 py-0.5 text-[10px] font-medium disabled:opacity-30"
              style={{
                backgroundColor: active ? 'var(--color-surface-2)' : 'transparent',
                borderColor: active ? 'var(--color-primary)' : 'var(--color-border)',
                color: active ? 'var(--color-primary)' : 'var(--color-muted)',
              }}
            >
              {l.label} {n > 0 && <span className="opacity-60">{n}</span>}
            </button>
          )
        })}
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
        {zone.z_span >= 40 && (
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
            style={{
              borderColor: 'var(--color-primary)',
              color: 'var(--color-primary)',
            }}
          >
            You are in {playerPos.zone}
          </button>
        )}
        {showZoneButton && (
          <button
            onClick={() => copy('showzone', mapShowZoneCommand(zone.zone))}
            title="Copy /map show_zone — preview this zone on your in-game map"
            className="ml-auto flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
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

      {/* A render fault in the canvas must not unmount the app. One did:
          a null deref in the drag handler blanked the whole window.
          min-h-0 is what lets this shrink inside the flex column instead of
          forcing the toggles and inspector off the bottom. */}
      <div className={fill ? 'min-h-0 flex-1' : undefined}>
        <ErrorBoundary label="Zone map">
          <ZoneMap
            zone={zone}
            geometry={geometry}
            detail={detail}
            showDetail={showDetail}
            outline={outline}
            mode={mode}
            colorByHeight={heightColor}
            playerPos={playerPos}
            followPlayer={followPlayer}
            pois={pois}
            visibleCategories={visible}
            highlights={selected ? [{ x: selected.x, y: selected.y, z: selected.z }] : []}
            onPOIClick={setSelected}
            height={height}
          />
        </ErrorBoundary>
      </div>

      {selected && (
        <div
          className="flex shrink-0 items-center gap-2 rounded border px-2 py-1.5 text-sm"
          style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        >
          <span
            className="shrink-0 text-[10px] uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            {selected.category.replace('_', ' ')}
          </span>
          <span className="truncate" style={{ color: 'var(--color-foreground)' }}>
            {selected.label}
          </span>
          <span className="shrink-0 font-mono text-xs" style={{ color: 'var(--color-muted)' }}>
            {/* Shown in /loc order and game sign, matching what EQ prints, so it
                can be read straight across to the game. */}
            {-selected.y}, {-selected.x}, {selected.z}
          </span>
          <div className="ml-auto flex shrink-0 items-center gap-1">
            {selected.ref_id ? (
              <button
                onClick={() =>
                  navigate(
                    // ground_spawn ref_id is the item that spawns; a locked
                    // door's is the key that opens it. Everything else is an NPC.
                    selected.category === 'ground_spawn' || selected.category === 'locked'
                      ? `/items?select=${selected.ref_id}`
                      : `/npcs?select=${selected.ref_id}`,
                  )
                }
                className="rounded border px-1.5 py-0.5 text-[10px]"
                style={{ borderColor: 'var(--color-border)', color: 'var(--color-primary)' }}
              >
                Open
              </button>
            ) : null}
            <button
              onClick={() =>
                // POIs live in map space; the Zeal command wants game
                // coordinates, which is the same negation back.
                copy('pin', mapMarkerCommand(-selected.x, -selected.y, selected.label))
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
            <button onClick={() => setSelected(null)} title="Clear selection">
              <X size={12} style={{ color: 'var(--color-muted)' }} />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
