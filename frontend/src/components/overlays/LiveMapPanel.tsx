import React, { useMemo } from 'react'
import { Navigation } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'
import { useCachedState } from '../../hooks/useCachedState'
import { useLiveZone } from '../../hooks/useLiveZone'
import { usePlayerPosition } from '../../hooks/usePlayerPosition'
import { useZoneMap } from '../../hooks/useZoneMap'
import { ZoneMap } from '../maps/ZoneMap'
import { ErrorBoundary } from '../ErrorBoundary'
import type { MapPOICategory } from '../../types/map'

interface LiveMapPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// Same default set as the full map panel, read from the same key, so layer
// choices follow between surfaces instead of each one having its own idea.
const DEFAULT_LAYERS: MapPOICategory[] = [
  'vendor', 'raid_target', 'zone_line', 'trap', 'locked', 'teleport', 'switch',
]

// LiveMapPanel — dashboard-docked twin of the Live Map overlay window.
//
// Kept minimal on purpose, matching the popout: follow and auto-depth on, no
// search, no inspector, no mode switch. Anything that needs reading or typing
// belongs on the Live Map tab, which has room for it. This exists so the overlay
// appears in "Manage overlays" with working Dash/Pop/Move/Reset like every other
// one (feedback_overlay_dashboard_pattern) — and because the dashboard is where
// you arrange overlays before committing them to the screen.
export default function LiveMapPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 384,
  defaultHeight = 384,
  snapGridSize,
  onLayoutChange,
}: LiveMapPanelProps): React.ReactElement {
  const { zone: zoneName, live } = useLiveZone()
  const playerPos = usePlayerPosition()
  const [enabled] = useCachedState<MapPOICategory[]>('maps.layers', DEFAULT_LAYERS)
  const { zone, outline, pois } = useZoneMap(zoneName, 'outline')
  const visible = useMemo(() => new Set(enabled), [enabled])

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Navigation size={13} style={{ color: live ? '#c9a84c' : 'rgba(255,255,255,0.35)' }} />
          {zoneName ?? 'Live Map'}
        </span>
      }
      defaultX={defaultX}
      defaultY={defaultY}
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
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
            followPlayer
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
    </OverlayWindow>
  )
}
