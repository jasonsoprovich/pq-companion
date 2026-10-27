/**
 * ItemInfoWindowPage — transparent always-on-top overlay showing the full
 * item detail tabs (Overview, sources, Tradeskills, Quests, Characters) for
 * the item last opened in game, fed by the proposed Zeal item pipe message.
 * Links inside open in the main window rather than navigating this one.
 */
import React from 'react'
import { Info } from 'lucide-react'
import ItemInfoView from '../components/overlays/ItemInfoView'
import { useItemDisplay } from '../hooks/useItemDisplay'
import { AppNavigateContext } from '../hooks/useAppNavigate'
import { openRouteInMain } from '../lib/overlayNav'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'

export default function ItemInfoWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('itemInfo')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('itemInfo')
  const chrome = useOverlayChromeFade('itemInfo', mode === 'display-only')
  const onDragMouseDown = useWindowDrag()
  const display = useItemDisplay()

  const headerBtn: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    padding: '1px 5px',
    borderRadius: 3,
    border: '1px solid rgba(255,255,255,0.1)',
    backgroundColor: 'transparent',
    color: 'rgba(255,255,255,0.4)',
    cursor: 'pointer',
    lineHeight: 1,
  }

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
      {/* ── Drag handle / title bar ─────────────────────────────────────── */}
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
          <Info size={11} style={{ color: '#38bdf8' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>Item Info</span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeItemInfo()}
            style={{ ...headerBtn, fontSize: 11 }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      <AppNavigateContext.Provider value={openRouteInMain}>
        <div className="flex min-h-0 flex-1 flex-col">
          <ItemInfoView {...display} />
        </div>
      </AppNavigateContext.Provider>
    </div>
  )
}
