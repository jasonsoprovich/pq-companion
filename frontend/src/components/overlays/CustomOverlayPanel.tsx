import React from 'react'
import { ExternalLink, LayoutTemplate } from 'lucide-react'
import OverlayWindow from '../OverlayWindow'
import CustomOverlayView from './CustomOverlayView'
import type { CustomOverlaysHook } from '../../hooks/useCustomOverlays'
import type { CustomOverlay } from '../../types/customOverlay'

interface CustomOverlayPanelProps {
  overlay: CustomOverlay
  // The dashboard's shared hook, so every panel uses one subscription.
  co: CustomOverlaysHook
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// Dashboard-embedded Custom Overlay panel — same live state as its popout
// window (CustomOverlayWindowPage), both fed by customoverlays:state.
export default function CustomOverlayPanel({
  overlay,
  co,
  defaultX = 16,
  defaultY = 16,
  defaultWidth = 300,
  defaultHeight = 380,
  snapGridSize,
  onLayoutChange,
}: CustomOverlayPanelProps): React.ReactElement {
  const btn: React.CSSProperties = {
    background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px',
    color: 'var(--color-muted)', display: 'flex', alignItems: 'center',
  }
  const id = overlay.def.id

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <LayoutTemplate size={13} style={{ color: '#a78bfa' }} />
          {overlay.def.name}
        </span>
      }
      headerRight={
        window.electron?.overlay ? (
          <button
            onClick={() => window.electron.overlay.toggleCustomOverlay(id, overlay.def.name)}
            title="Pop out as floating overlay"
            style={btn}
          >
            <ExternalLink size={12} />
          </button>
        ) : undefined
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
      minWidth={180}
      minHeight={100}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <CustomOverlayView
        theme="panel"
        overlay={overlay}
        skewMs={co.skewMs}
        onRemoveRow={(widget, key) => void co.act(id, { action: 'remove_row', widget, key })}
      />
    </OverlayWindow>
  )
}
