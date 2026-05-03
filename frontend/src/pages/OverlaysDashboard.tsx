/**
 * OverlaysDashboard — customizable in-app dashboard combining the four
 * draggable/resizable overlay panels. The toolbar exposes per-panel show/hide,
 * a Reset button, and pop-out toggles for the standalone Electron overlay
 * windows that don't have an in-dashboard panel (Trigger Alerts, HPS).
 *
 * Layout (positions, sizes, visibility) is persisted to localStorage and
 * restored on next mount. Drag/resize snaps to a 16px grid.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Eye, EyeOff, MonitorPlay, RotateCcw, Zap, HeartPulse, ExternalLink } from 'lucide-react'
import BuffTimerPanel from '../components/overlays/BuffTimerPanel'
import DetrimTimerPanel from '../components/overlays/DetrimTimerPanel'
import DPSPanel from '../components/overlays/DPSPanel'
import NPCPanel from '../components/overlays/NPCPanel'
import {
  DASHBOARD_PANEL_KEYS,
  DASHBOARD_PANEL_LABELS,
  DEFAULT_DASHBOARD_LAYOUT,
  loadDashboardLayout,
  saveDashboardLayout,
  type DashboardLayout,
  type DashboardPanelKey,
} from '../services/dashboardLayout'

const SNAP_GRID = 16

function PanelToggle({
  label,
  visible,
  onToggle,
}: {
  label: string
  visible: boolean
  onToggle: () => void
}): React.ReactElement {
  const Icon = visible ? Eye : EyeOff
  return (
    <button
      onClick={onToggle}
      className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
      style={{
        backgroundColor: visible ? 'var(--color-surface-2)' : 'var(--color-surface)',
        color: visible ? 'var(--color-foreground)' : 'var(--color-muted)',
        border: '1px solid var(--color-border)',
      }}
      title={visible ? `Hide ${label}` : `Show ${label}`}
    >
      <Icon size={11} />
      {label}
    </button>
  )
}

export default function OverlaysDashboard(): React.ReactElement {
  const [layout, setLayout] = useState<DashboardLayout>(() => loadDashboardLayout())
  // Bumped on Reset so panels remount with fresh defaults — OverlayWindow keeps
  // its position in local state, so we have to force a fresh mount to apply
  // new initial values.
  const [layoutVersion, setLayoutVersion] = useState(0)

  useEffect(() => {
    saveDashboardLayout(layout)
  }, [layout])

  const updatePanel = useCallback(
    (key: DashboardPanelKey, patch: Partial<DashboardLayout[DashboardPanelKey]>) => {
      setLayout((prev) => ({ ...prev, [key]: { ...prev[key], ...patch } }))
    },
    [],
  )

  const handleLayoutChange = useCallback(
    (key: DashboardPanelKey) =>
      (b: { x: number; y: number; width: number; height: number }) => {
        updatePanel(key, { x: b.x, y: b.y, width: b.width, height: b.height })
      },
    [updatePanel],
  )

  const toggleVisible = useCallback(
    (key: DashboardPanelKey) => () => updatePanel(key, { visible: !layout[key].visible }),
    [layout, updatePanel],
  )

  const handleReset = useCallback(() => {
    setLayout({ ...DEFAULT_DASHBOARD_LAYOUT })
    setLayoutVersion((v) => v + 1)
  }, [])

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
      {/* Toolbar */}
      <div
        className="flex items-center gap-2 border-b px-4 py-2 shrink-0 flex-wrap"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <MonitorPlay size={14} style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs font-semibold mr-2" style={{ color: 'var(--color-foreground)' }}>
          Overlays
        </span>

        {/* Per-panel show/hide */}
        <div className="flex items-center gap-1.5 flex-wrap">
          {DASHBOARD_PANEL_KEYS.map((key) => (
            <PanelToggle
              key={key}
              label={DASHBOARD_PANEL_LABELS[key]}
              visible={layout[key].visible}
              onToggle={toggleVisible(key)}
            />
          ))}
        </div>

        <div className="ml-auto flex items-center gap-2">
          {/* Standalone overlays — these don't have in-dashboard panels but the
              user can pop them out as floating Electron windows. */}
          <button
            onClick={() => window.electron?.overlay?.toggleHPS()}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Toggle the HPS meter overlay window"
          >
            <HeartPulse size={11} />
            HPS Meter
            <ExternalLink size={9} style={{ opacity: 0.6 }} />
          </button>
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
            <ExternalLink size={9} style={{ opacity: 0.6 }} />
          </button>
          <button
            onClick={handleReset}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Reset all panel positions, sizes, and visibility to defaults"
          >
            <RotateCcw size={11} />
            Reset Layout
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
            Drag title bars to move · Drag edges/corners to resize · Snaps to {SNAP_GRID}px grid
          </p>
        </div>

        {layout.buff.visible && (
          <BuffTimerPanel
            key={`buff-${layoutVersion}`}
            defaultX={layout.buff.x}
            defaultY={layout.buff.y}
            defaultWidth={layout.buff.width}
            defaultHeight={layout.buff.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('buff')}
          />
        )}
        {layout.detrim.visible && (
          <DetrimTimerPanel
            key={`detrim-${layoutVersion}`}
            defaultX={layout.detrim.x}
            defaultY={layout.detrim.y}
            defaultWidth={layout.detrim.width}
            defaultHeight={layout.detrim.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('detrim')}
          />
        )}
        {layout.dps.visible && (
          <DPSPanel
            key={`dps-${layoutVersion}`}
            defaultX={layout.dps.x}
            defaultY={layout.dps.y}
            defaultWidth={layout.dps.width}
            defaultHeight={layout.dps.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('dps')}
          />
        )}
        {layout.npc.visible && (
          <NPCPanel
            key={`npc-${layoutVersion}`}
            defaultX={layout.npc.x}
            defaultY={layout.npc.y}
            defaultWidth={layout.npc.width}
            defaultHeight={layout.npc.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('npc')}
          />
        )}
      </div>
    </div>
  )
}
