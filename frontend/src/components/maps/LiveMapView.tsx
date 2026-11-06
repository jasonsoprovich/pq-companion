import React, { Suspense, lazy, useEffect, useMemo, useState } from 'react'
import { useCachedState } from '../../hooks/useCachedState'
import { useMapStyle } from '../../hooks/useMapStyle'
import { usePlayerPosition } from '../../hooks/usePlayerPosition'
import { useGroupPositions } from '../../hooks/useGroupPositions'
import { useRaidPositions } from '../../hooks/useRaidPositions'
import { useTargetPosition } from '../../hooks/useTargetPosition'
import { useDPSClassColors } from '../../hooks/useDPSClassColors'
import { useZoneMap } from '../../hooks/useZoneMap'
import type { LiveMapOverlayPrefs } from '../../hooks/useLiveMapOverlayPrefs'
import { classColor, groupColor } from '../../lib/raidColors'
import { CATEGORY_STYLE, ZoneMap, type RaiderMarker, type ReplayTarget } from './ZoneMap'
import type { Pin3D } from './ZoneMap3D'
import { TARGET_RING_COLOR, useSpawnOverlays } from './spawnOverlays'
import { ErrorBoundary } from '../ErrorBoundary'
import type { MapPOICategory } from '../../types/map'

// three.js is large; it loads only when the 3D view is picked.
const ZoneMap3D = lazy(() => import('./ZoneMap3D'))

// Mirrors the map page's default layers and reads the same key.
const DEFAULT_LAYERS: MapPOICategory[] = [
  'vendor', 'raid_target', 'zone_line', 'trap', 'locked', 'teleport', 'switch',
]

// The 3D view's colours for you and your group, as on the map page.
const PLAYER_3D = '#f8fafc'
const GROUP_3D = '#7dd3fc'

interface LiveMapViewProps {
  // zone is the short name of the zone you're in, or were last in.
  zone: string
  prefs: LiveMapOverlayPrefs
}

// LiveMapView is the live map as the overlay draws it: the zone you're in,
// following you, with your group, the raid, aggro rings and respawn dots
// as prefs say, in 2D or 3D. No search or inspector; those need the room
// the Live Map page has.
export default function LiveMapView({ zone: zoneName, prefs }: LiveMapViewProps): React.ReactElement {
  const playerPos = usePlayerPosition()
  const groupMembers = useGroupPositions()
  const raidMembers = useRaidPositions()
  const targetPos = useTargetPosition()
  const palette = useDPSClassColors()
  const { poiIgnoreZFade, showGroup } = useMapStyle()
  const [enabled] = useCachedState<MapPOICategory[]>('maps.layers', DEFAULT_LAYERS)
  // Outline only in 2D: at overlay size the detailed layers are illegible.
  const { zone, outline, pois } = useZoneMap(zoneName, 'outline')
  const visible = useMemo(() => new Set(enabled), [enabled])
  // Not saved: an overlay should come back following you, whatever you last
  // dragged it to. Panning is for a look around, not a setting.
  const [follow, setFollow] = useState(true)
  // Why 3D fell back to 2D, if it did.
  const [no3D, setNo3D] = useState(false)
  useEffect(() => setNo3D(false), [zoneName])
  const show3D = prefs.view3D && !no3D

  const here = playerPos && zone && playerPos.zone === zone.zone ? playerPos : null
  const groupOn = prefs.group ?? showGroup
  const liveTarget = prefs.target && here && targetPos?.zone === here.zone ? targetPos : null
  const { aggroRings, spawnDots, targetRing } = useSpawnOverlays(zoneName, {
    aggroScope: prefs.aggro,
    aggroNear: prefs.aggroNear,
    dots: prefs.dots,
    hideKilled: prefs.hideKilled,
    here,
    target: liveTarget,
  })

  // The raid but you, and but your group while it's drawn as your group.
  const raiders = useMemo((): RaiderMarker[] => {
    if (!prefs.raid || !here) return []
    const grouped = new Set(groupOn ? groupMembers.map((g) => g.name) : [])
    return raidMembers
      .filter((m) => !grouped.has(m.name))
      .map((m) => ({
        x: m.x, y: m.y, z: m.z, heading: m.heading, label: m.name,
        color: prefs.raidColor === 'class' ? classColor(palette, m.class) : groupColor(m.group),
      }))
  }, [prefs.raid, prefs.raidColor, here, groupOn, groupMembers, raidMembers, palette])

  // The line to your target: onto a raider's own arrow, or to a ring and
  // name for anything else.
  const targetLine = useMemo((): ReplayTarget | null => {
    if (!liveTarget || !here) return null
    const drawn = new Set([...(groupOn ? groupMembers.map((g) => g.name) : []), ...raiders.map((r) => r.label)])
    return {
      from: { x: here.x, y: here.y, z: here.z },
      to: { x: liveTarget.x, y: liveTarget.y, z: liveTarget.z },
      label: liveTarget.name,
      onRaider: drawn.has(liveTarget.name),
    }
  }, [liveTarget, here, groupOn, groupMembers, raiders])
  const rings = useMemo(
    () => [...(prefs.aggro !== 'off' ? aggroRings : []), ...(targetRing ? [targetRing] : [])],
    [prefs.aggro, aggroRings, targetRing],
  )

  const markers3D = useMemo((): RaiderMarker[] => {
    if (!show3D || !here) return []
    const out = [...raiders]
    if (groupOn) {
      for (const g of groupMembers) {
        out.push({ x: g.x, y: g.y, z: g.z, heading: g.heading, color: GROUP_3D, label: g.name, mine: true })
      }
    }
    out.push({
      x: here.x, y: here.y, z: here.z, heading: here.heading,
      color: PLAYER_3D, label: 'You', focus: true, mine: true,
    })
    return out
  }, [show3D, here, raiders, groupOn, groupMembers])

  const pins3D = useMemo((): Pin3D[] => {
    if (!show3D) return []
    const out: Pin3D[] = pois
      .filter((p) => visible.has(p.category))
      .map((p) => ({ x: p.x, y: p.y, z: p.z, color: CATEGORY_STYLE[p.category]?.color ?? '#94a3b8', label: p.label }))
    if (targetLine && !targetLine.onRaider) {
      out.push({ ...targetLine.to, color: TARGET_RING_COLOR, label: targetLine.label, picked: true })
    }
    return out
  }, [show3D, pois, visible, targetLine])

  if (!zone) return <div style={{ height: '100%' }} />

  return (
    <ErrorBoundary label="Live map">
      {show3D ? (
        <Suspense fallback={<div style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.45)' }}>Loading 3D…</div>}>
          <ZoneMap3D
            key={zoneName}
            zone={zoneName}
            markers={markers3D}
            pins={pins3D}
            target={targetLine}
            rings={rings}
            spawnDots={prefs.dots ? spawnDots : undefined}
            labels={prefs.labels}
            chromeless
            transparent
            onUnavailable={() => setNo3D(true)}
          />
        </Suspense>
      ) : (
        <ZoneMap
          zone={zone}
          geometry={null}
          outline={outline}
          mode="outline"
          pois={pois}
          visibleCategories={visible}
          poiIgnoreZFade={poiIgnoreZFade}
          playerPos={playerPos}
          groupMembers={groupOn ? groupMembers : []}
          raiders={raiders}
          replayTarget={targetLine}
          aggroRings={rings}
          spawnDots={prefs.dots ? spawnDots : undefined}
          // Follow starts on — an overlay you have to pan by hand every time
          // you move is worse than no overlay — but it is not forced. The
          // in-game map pans, so this one has to as well; dragging releases
          // follow and the reticle button puts it back.
          followPlayer={follow}
          onUserPan={() => setFollow(false)}
          onFollowRequest={() => setFollow(true)}
          followDepth
          height="fill"
          // 2D names cover POIs as well as people, so only under All.
          showLabels={prefs.labels === 'all'}
          chromeless
          transparent
        />
      )}
    </ErrorBoundary>
  )
}
