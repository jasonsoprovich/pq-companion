import React from 'react'
import { ExternalLink, Navigation } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'
import { useLiveZone } from '../../hooks/useLiveZone'
import { useMapStyle } from '../../hooks/useMapStyle'
import { useLiveMapOverlayPrefs } from '../../hooks/useLiveMapOverlayPrefs'
import LiveMapView from '../maps/LiveMapView'
import LiveMapOverlayControls from '../maps/LiveMapOverlayControls'

interface LiveMapPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// LiveMapPanel — dashboard-docked twin of the Live Map overlay window.
//
// Kept minimal on purpose, matching the popout: follow and auto-depth on, no
// search, no inspector. It shares the popout's toggles and settings. Anything that needs reading or typing
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
  const { showGroup } = useMapStyle()
  const [prefs, setPrefs] = useLiveMapOverlayPrefs()

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Navigation size={13} style={{ color: live ? '#c9a84c' : 'rgba(255,255,255,0.35)' }} />
          {zoneName ?? 'Live Map'}
        </span>
      }
      headerRight={
        window.electron?.overlay && (
          <button
            onClick={() => window.electron.overlay.toggleLiveMap()}
            title="Pop out as floating overlay"
            style={{
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              padding: '1px 3px',
              color: 'var(--color-muted)',
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <ExternalLink size={12} />
          </button>
        )
      }
      defaultX={defaultX}
      defaultY={defaultY}
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      {zoneName ? (
        <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
          <div style={{ padding: '4px 6px', borderBottom: '1px solid rgba(255,255,255,0.08)', flexShrink: 0 }}>
            <LiveMapOverlayControls prefs={prefs} onChange={setPrefs} groupDefault={showGroup} />
          </div>
          <div style={{ flex: 1, minHeight: 0, position: 'relative' }}>
            <LiveMapView zone={zoneName} prefs={prefs} />
          </div>
        </div>
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
