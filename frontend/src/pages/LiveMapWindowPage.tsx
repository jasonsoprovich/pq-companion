/**
 * LiveMapWindowPage — transparent always-on-top map of the zone you are in.
 * Renders in a dedicated frameless Electron window (phase 5c).
 *
 * Deliberately leaner than the Live Map tab. An overlay sits on top of the game
 * while you play, so it answers one question — "where am I and what is near me"
 * — and everything that needs reading or typing (POI search, the inspector, the
 * layer toggles, mode switching) stays in the main window where there is room
 * for it. Layer choices made there are honoured here, since they share the same
 * cached preference.
 */
import React from 'react'
import { Navigation } from 'lucide-react'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { useCachedState } from '../hooks/useCachedState'
import OverlayLockButton from '../components/OverlayLockButton'
import { useLiveZone } from '../hooks/useLiveZone'
import { usePlayerPosition } from '../hooks/usePlayerPosition'
import { useZoneMap } from '../hooks/useZoneMap'
import { ZoneMap } from '../components/maps/ZoneMap'
import { ErrorBoundary } from '../components/ErrorBoundary'
import type { MapPOICategory } from '../types/map'

// Mirrors the main panel's default set so the overlay does not look like a
// different app, and reads the same key so a change in either follows.
const DEFAULT_LAYERS: MapPOICategory[] = [
  'vendor', 'raid_target', 'zone_line', 'trap', 'locked', 'teleport', 'switch',
]

export default function LiveMapWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('liveMap')
  const onDragMouseDown = useWindowDrag()

  const { zone: zoneName, live } = useLiveZone()
  const playerPos = usePlayerPosition()
  const [enabled] = useCachedState<MapPOICategory[]>('maps.layers', DEFAULT_LAYERS)
  // Outline mode only. At overlay size the detailed layers are illegible, and
  // this is the surface where legibility at a glance matters most.
  const { zone, outline, pois } = useZoneMap(zoneName, 'outline')
  const visible = React.useMemo(() => new Set(enabled), [enabled])
  // Not cached: an overlay should come back following you, whatever you last
  // dragged it to. Panning is for a look around, not a setting.
  const [follow, setFollow] = React.useState(true)

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        border: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
        transition: 'background-color 0.4s ease, border-color 0.4s ease',
        borderRadius: 8,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        color: 'rgba(255,255,255,0.9)',
      }}
    >
      <div
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Navigation size={11} style={{ color: live ? '#c9a84c' : 'rgba(255,255,255,0.35)' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            {zoneName ?? 'Live Map'}
          </span>
          {zoneName && !live && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              last known
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeLiveMap()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close"
          >
            ✕
          </button>
        </div>
      </div>

      <div style={{ flex: 1, minHeight: 0, position: 'relative' }}>
        {zone ? (
          <ErrorBoundary label="Live map">
            <ZoneMap
              zone={zone}
              geometry={null}
              outline={outline}
              mode="outline"
              pois={pois}
              visibleCategories={visible}
              playerPos={playerPos}
              // Follow starts on — an overlay you have to pan by hand every time
              // you move is worse than no overlay — but it is not forced. The
              // in-game map pans, so this one has to as well; dragging releases
              // follow and the reticle button puts it back.
              followPlayer={follow}
              onUserPan={() => setFollow(false)}
              onFollowRequest={() => setFollow(true)}
              followDepth
              height="fill"
              showLabels={false}
              chromeless
              transparent
            />
          </ErrorBoundary>
        ) : (
          <div
            style={{
              height: '100%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              padding: 12,
              textAlign: 'center',
              fontSize: 11,
              color: 'rgba(255,255,255,0.45)',
            }}
          >
            Waiting for your position — needs Zeal running in game.
          </div>
        )}
      </div>
    </div>
  )
}
