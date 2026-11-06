/**
 * LiveMapWindowPage — transparent always-on-top map of the zone you are in.
 * Renders in a dedicated frameless Electron window (phase 5c).
 *
 * Deliberately leaner than the Live Map tab. An overlay sits on top of the game
 * while you play, so it answers one question — "where am I and what is near me"
 * — and everything that needs reading or typing (POI search, the inspector)
 * stays in the main window where there is room for it. What it draws (2D or
 * 3D, group, raid, rings, spawns, names) is set from a row of toggles shown
 * while unlocked, or from Settings → Maps once it's locked.
 */
import React from 'react'
import { Navigation } from 'lucide-react'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { useLiveZone } from '../hooks/useLiveZone'
import { useMapStyle } from '../hooks/useMapStyle'
import { useLiveMapOverlayPrefs } from '../hooks/useLiveMapOverlayPrefs'
import LiveMapView from '../components/maps/LiveMapView'
import LiveMapOverlayControls from '../components/maps/LiveMapOverlayControls'

export default function LiveMapWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('liveMap')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('liveMap')
  const chrome = useOverlayChromeFade('liveMap', mode === 'display-only')
  const onDragMouseDown = useWindowDrag()

  const { zone: zoneName, live } = useLiveZone()
  const { showGroup } = useMapStyle()
  const [prefs, setPrefs] = useLiveMapOverlayPrefs()

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100%',
        height: '100%',
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

      {/* The toggles, while unlocked, fading with the header. They keep
          their row so the map doesn't jump on every hover, and sit off the
          map so they never cover its follow button. Locked, clicks go
          through to the game, so the same toggles are in Settings → Maps. */}
      {!locked && (
        <div
          className="no-drag"
          style={{
            padding: '4px 6px',
            borderBottom: '1px solid rgba(255,255,255,0.08)',
            flexShrink: 0,
            opacity: chrome ? 1 : 0,
            pointerEvents: chrome ? 'auto' : 'none',
            transition: 'opacity 0.4s ease',
          }}
        >
          <LiveMapOverlayControls prefs={prefs} onChange={setPrefs} groupDefault={showGroup} />
        </div>
      )}

      <div style={{ flex: 1, minHeight: 0, position: 'relative' }}>
        {zoneName ? (
          <LiveMapView zone={zoneName} prefs={prefs} />
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
