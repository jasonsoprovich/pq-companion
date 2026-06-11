/**
 * OverlaysDashboard — customizable in-app dashboard combining the four
 * draggable/resizable overlay panels. The toolbar exposes per-panel show/hide,
 * a Pop Out All toggle, and a Reset button. The only standalone Electron overlay
 * without an in-dashboard panel is HPS (gated behind SHOW_HPS).
 *
 * Layout (positions, sizes, visibility) is persisted to localStorage and
 * restored on next mount. Drag/resize snaps to a 16px grid.
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Eye, EyeOff, Monitor, MonitorPlay, RotateCcw, HeartPulse, Hourglass, ExternalLink, Layers, X } from 'lucide-react'
import BuffTimerPanel from '../components/overlays/BuffTimerPanel'
import DetrimTimerPanel from '../components/overlays/DetrimTimerPanel'
import DPSPanel from '../components/overlays/DPSPanel'
import HPSPanel from '../components/overlays/HPSPanel'
import NPCPanel from '../components/overlays/NPCPanel'
import RollTrackerPanel from '../components/overlays/RollTrackerPanel'
import RespawnTimerPanel from '../components/overlays/RespawnTimerPanel'
import CHChainPanel from '../components/overlays/CHChainPanel'
import CHMetronomePanel from '../components/overlays/CHMetronomePanel'
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
// Extra empty space kept below/right of the lowest/rightmost visible panel
// so panels never sit flush against the scroll container edge and users can
// still grab the bottom resize handle once scrolled to the end.
const CANVAS_PADDING = 120

// HPS tracking is wired up end-to-end (panel, dashboard layout, popout window)
// but no log-parsing pipeline currently produces healer stats, so the UI is
// hidden. Flip this flag to true once the backend emits real heal data.
const SHOW_HPS = false

const VISIBLE_PANEL_KEYS: DashboardPanelKey[] = SHOW_HPS
  ? DASHBOARD_PANEL_KEYS
  : DASHBOARD_PANEL_KEYS.filter((k) => k !== 'hps')

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

  // Tracks whether any standalone popout window is currently open. Polled
  // because Electron doesn't push window-state changes to this renderer.
  const [anyPopoutOpen, setAnyPopoutOpen] = useState(false)

  useEffect(() => {
    if (!window.electron?.overlay?.anyPopoutOpen) return
    let cancelled = false
    const check = (): void => {
      window.electron.overlay
        .anyPopoutOpen()
        .then((v) => { if (!cancelled) setAnyPopoutOpen(v) })
        .catch(() => {})
    }
    check()
    const id = setInterval(check, 1500)
    return () => { cancelled = true; clearInterval(id) }
  }, [])

  // Multi-monitor: pick which monitor trigger alert text (and the positioning
  // card) appears on. The trigger overlay covers exactly that one monitor —
  // spanning the whole desktop is unreliable across mixed-DPI screens. The
  // picker is hidden on single-monitor setups (nothing to choose).
  const [displays, setDisplays] = useState<
    Array<{ id: number; label: string; width: number; height: number; isPrimary: boolean; isCurrent: boolean }>
  >([])

  const refreshDisplays = useCallback(() => {
    window.electron?.screen?.listDisplays?.()
      .then((list) => setDisplays(list ?? []))
      .catch(() => {})
  }, [])

  useEffect(() => { refreshDisplays() }, [refreshDisplays])

  const currentDisplayId = displays.find((d) => d.isCurrent)?.id

  const handleDisplayChange = useCallback(
    (id: number) => {
      window.electron?.overlay?.setDisplay?.(id)
        .then(() => refreshDisplays())
        .catch(() => {})
    },
    [refreshDisplays],
  )

  const sizerExtent = useMemo(() => {
    let maxRight = 0
    let maxBottom = 0
    for (const key of VISIBLE_PANEL_KEYS) {
      const p = layout[key]
      if (!p.visible) continue
      if (p.x + p.width > maxRight) maxRight = p.x + p.width
      if (p.y + p.height > maxBottom) maxBottom = p.y + p.height
    }
    return { width: maxRight + CANVAS_PADDING, height: maxBottom + CANVAS_PADDING }
  }, [layout])

  const handleTogglePopouts = useCallback(() => {
    const o = window.electron?.overlay
    if (!o) return
    if (anyPopoutOpen) {
      o.closeAllPopouts().catch(() => {})
      setAnyPopoutOpen(false)
    } else {
      // Only pop out overlays the user has toggled visible in the dashboard —
      // a panel hidden here shouldn't open as a floating window. Trigger Alerts
      // has no dashboard toggle and is always included by the main process.
      const visiblePanels = VISIBLE_PANEL_KEYS.filter((k) => layout[k].visible)
      o.openAllPopouts(visiblePanels).catch(() => {})
      setAnyPopoutOpen(true)
    }
  }, [anyPopoutOpen, layout])

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
      {/* Toolbar — title pinned left, panel toggles flow in the middle (wrapping
          to new rows as the window narrows), action buttons pinned right so they
          never wrap away into a disconnected row. */}
      <div
        className="flex items-start gap-3 border-b px-4 py-2 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-2 shrink-0 h-6">
          <MonitorPlay size={14} style={{ color: 'var(--color-primary)' }} />
          <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Overlays
          </span>
        </div>

        {/* Per-panel show/hide — grows to fill, wraps within its own area */}
        <div className="flex items-center gap-1.5 flex-wrap flex-1 min-w-0">
          {VISIBLE_PANEL_KEYS.map((key) => (
            <PanelToggle
              key={key}
              label={DASHBOARD_PANEL_LABELS[key]}
              visible={layout[key].visible}
              onToggle={toggleVisible(key)}
            />
          ))}
        </div>

        <div className="flex items-center gap-2 shrink-0">
          {/* Overlay monitor picker — only meaningful with more than one
              display. Trigger alert text and the positioning card are pinned
              to the chosen monitor. */}
          {displays.length > 1 && (
            <div
              className="flex items-center gap-1.5"
              title="Which monitor trigger alert text and the positioning card appear on"
            >
              <Monitor size={11} style={{ color: 'var(--color-muted-foreground)' }} />
              <select
                value={currentDisplayId ?? ''}
                onChange={(e) => handleDisplayChange(Number(e.target.value))}
                className="text-xs rounded px-1.5 py-1 outline-none"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                  cursor: 'pointer',
                }}
              >
                {displays.map((d) => (
                  <option key={d.id} value={d.id}>
                    {`${d.label} (${d.width}×${d.height})${d.isPrimary ? ' • Primary' : ''}`}
                  </option>
                ))}
              </select>
            </div>
          )}
          {/* HPS has no in-dashboard panel — pop it out as a floating window. */}
          {SHOW_HPS && (
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
          )}
          {/* Custom Timers has no in-dashboard panel — pop it out as a
              floating window. Generic countdowns (manual or trigger-driven
              with timer type "custom") live there. */}
          <button
            onClick={() => window.electron?.overlay?.toggleCustomTimer()}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Toggle the Custom Timers overlay window — generic countdowns with a quick-add form"
          >
            <Hourglass size={11} />
            Custom Timers
            <ExternalLink size={9} style={{ opacity: 0.6 }} />
          </button>
          {typeof window.electron?.overlay?.anyPopoutOpen === 'function' && (
            <button
              onClick={handleTogglePopouts}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: anyPopoutOpen ? 'var(--color-surface-2)' : 'var(--color-surface)',
                color: anyPopoutOpen ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
              title={
                anyPopoutOpen
                  ? 'Close all standalone overlay windows'
                  : 'Pop out the overlays you have toggled visible above (plus Trigger Alerts)'
              }
            >
              {anyPopoutOpen ? <X size={11} /> : <Layers size={11} />}
              {anyPopoutOpen ? 'Close All Popouts' : 'Pop Out All'}
            </button>
          )}
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
        {/* Invisible sizer forces the scroll container to extend past the
            lowest/rightmost panel so panels aren't tight against the edge
            when scrolled to the end. */}
        <div
          aria-hidden
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: sizerExtent.width,
            height: sizerExtent.height,
            pointerEvents: 'none',
          }}
        />
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
        {SHOW_HPS && layout.hps.visible && (
          <HPSPanel
            key={`hps-${layoutVersion}`}
            defaultX={layout.hps.x}
            defaultY={layout.hps.y}
            defaultWidth={layout.hps.width}
            defaultHeight={layout.hps.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('hps')}
          />
        )}
        {layout.rolls.visible && (
          <RollTrackerPanel
            key={`rolls-${layoutVersion}`}
            defaultX={layout.rolls.x}
            defaultY={layout.rolls.y}
            defaultWidth={layout.rolls.width}
            defaultHeight={layout.rolls.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('rolls')}
          />
        )}
        {layout.respawn.visible && (
          <RespawnTimerPanel
            key={`respawn-${layoutVersion}`}
            defaultX={layout.respawn.x}
            defaultY={layout.respawn.y}
            defaultWidth={layout.respawn.width}
            defaultHeight={layout.respawn.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('respawn')}
          />
        )}
        {layout.chChain.visible && (
          <CHChainPanel
            key={`chChain-${layoutVersion}`}
            defaultX={layout.chChain.x}
            defaultY={layout.chChain.y}
            defaultWidth={layout.chChain.width}
            defaultHeight={layout.chChain.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('chChain')}
          />
        )}
        {layout.chMetronome.visible && (
          <CHMetronomePanel
            key={`chMetronome-${layoutVersion}`}
            defaultX={layout.chMetronome.x}
            defaultY={layout.chMetronome.y}
            defaultWidth={layout.chMetronome.width}
            defaultHeight={layout.chMetronome.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('chMetronome')}
          />
        )}
      </div>
    </div>
  )
}
