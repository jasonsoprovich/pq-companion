/**
 * OverlaysDashboard — unified in-app dashboard combining all four overlay
 * panels (Buff Timers, Detrimental Timers, DPS Meter, NPC Overlay) on a
 * single canvas. Each panel is a draggable/resizable OverlayWindow that
 * also exposes a pop-out button to launch as a standalone Electron overlay.
 */
import React from 'react'
import { MonitorPlay, Zap } from 'lucide-react'
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
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: 'var(--color-background)',
      }}
    >
      {/* Toolbar — pop-out toggles for standalone overlay windows */}
      <div
        className="flex items-center gap-3 border-b px-4 py-2 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <MonitorPlay size={14} style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Overlays
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => window.electron?.overlay?.toggleTrigger()}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Toggle the trigger alerts overlay window — shows on-screen alert text from custom triggers"
          >
            <Zap size={11} />
            Trigger Alerts
          </button>
        </div>
      </div>

      {/* Dashboard canvas */}
      <div style={{ position: 'relative', flex: 1, overflow: 'auto' }}>
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
    </div>
  )
}
