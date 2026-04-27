/**
 * OverlaysDashboard — unified in-app dashboard combining all four overlay
 * panels (Buff Timers, Detrimental Timers, DPS Meter, NPC Overlay) on a
 * single canvas. Each panel is a draggable/resizable OverlayWindow that
 * also exposes a pop-out button to launch as a standalone Electron overlay.
 */
import React from 'react'
import BuffTimerPanel from '../components/overlays/BuffTimerPanel'
import DetrimTimerPanel from '../components/overlays/DetrimTimerPanel'
import DPSPanel from '../components/overlays/DPSPanel'
import NPCPanel from '../components/overlays/NPCPanel'

export default function OverlaysDashboard(): React.ReactElement {
  return (
    <div
      style={{
        position: 'relative',
        height: '100%',
        overflow: 'auto',
        backgroundColor: 'var(--color-background)',
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          pointerEvents: 'none',
          userSelect: 'none',
        }}
      >
        <p style={{ fontSize: 12, color: 'var(--color-muted)', opacity: 0.4 }}>
          Drag title bars to move · Drag edges/corners to resize · Pop out any panel as a floating overlay
        </p>
      </div>

      <BuffTimerPanel defaultX={24} defaultY={24} defaultWidth={300} defaultHeight={380} />
      <DetrimTimerPanel defaultX={344} defaultY={24} defaultWidth={300} defaultHeight={380} />
      <DPSPanel defaultX={24} defaultY={424} defaultWidth={620} defaultHeight={380} />
      <NPCPanel defaultX={664} defaultY={24} defaultWidth={400} defaultHeight={780} />
    </div>
  )
}
