/**
 * OverlaysDashboard — customizable in-app dashboard combining the
 * draggable/resizable overlay panels. The toolbar stays compact: a global
 * "Position overlays" toggle plus a "Manage overlays" dropdown that holds the
 * per-overlay controls (show/hide the dashboard panel, pop the window out,
 * recenter it) and the bulk actions (Pop Out All, Reset Positions, Reset
 * Layout, monitor picker). The only standalone Electron overlay without an
 * in-dashboard panel is HPS (gated behind SHOW_HPS).
 *
 * Layout (positions, sizes, visibility) is persisted to localStorage and
 * restored on next mount. Drag/resize snaps to a 16px grid.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Eye, EyeOff, Monitor, MonitorPlay, RotateCcw, ExternalLink, Layers, X, Crosshair, ChevronDown, Move, ListChecks, Trash2 } from 'lucide-react'
import type { OverlayName } from '../lib/overlays'
import BuffTimerPanel from '../components/overlays/BuffTimerPanel'
import DetrimTimerPanel from '../components/overlays/DetrimTimerPanel'
import DPSPanel from '../components/overlays/DPSPanel'
import HPSPanel from '../components/overlays/HPSPanel'
import NPCPanel from '../components/overlays/NPCPanel'
import ThreatPanel from '../components/overlays/ThreatPanel'
import RollTrackerPanel from '../components/overlays/RollTrackerPanel'
import RespawnTimerPanel from '../components/overlays/RespawnTimerPanel'
import CHChainPanel from '../components/overlays/CHChainPanel'
import CHMetronomePanel from '../components/overlays/CHMetronomePanel'
import CustomTimerPanel from '../components/overlays/CustomTimerPanel'
import { ConfirmModal } from '../components/ConfirmModal'
import { clearTimers, getConfig } from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
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

// Maps each dashboard panel to its standalone popout window: the canonical
// overlay name (used for popout open-state polling and per-overlay position
// reset) and the IPC toggle that opens/closes that floating window.
const PANEL_POPOUT: Record<DashboardPanelKey, { name: OverlayName; toggle: () => void }> = {
  buff:        { name: 'buffTimer',    toggle: () => { window.electron?.overlay?.toggleBuffTimer() } },
  detrim:      { name: 'detrimTimer',  toggle: () => { window.electron?.overlay?.toggleDetrimTimer() } },
  dps:         { name: 'dps',          toggle: () => { window.electron?.overlay?.toggleDPS() } },
  npc:         { name: 'npc',          toggle: () => { window.electron?.overlay?.toggleNPC() } },
  threat:      { name: 'threat',       toggle: () => { window.electron?.overlay?.toggleThreat() } },
  hps:         { name: 'hps',          toggle: () => { window.electron?.overlay?.toggleHPS() } },
  rolls:       { name: 'rollTracker',  toggle: () => { window.electron?.overlay?.toggleRollTracker() } },
  respawn:     { name: 'respawnTimer', toggle: () => { window.electron?.overlay?.toggleRespawnTimer() } },
  chChain:     { name: 'chChain',      toggle: () => { window.electron?.overlay?.toggleCHChain() } },
  chMetronome: { name: 'chMetronome',  toggle: () => { window.electron?.overlay?.toggleCHMetronome() } },
  custom:      { name: 'customTimer',  toggle: () => { window.electron?.overlay?.toggleCustomTimer() } },
}

// Compact square icon button for the manager's per-overlay rows.
function RowIconButton({
  active,
  onClick,
  title,
  children,
}: {
  active?: boolean
  onClick: () => void
  title: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      className="flex items-center justify-center rounded"
      style={{
        width: 26,
        height: 24,
        backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface)',
        color: active ? '#fff' : 'var(--color-muted-foreground)',
        border: '1px solid var(--color-border)',
        cursor: 'pointer',
      }}
    >
      {children}
    </button>
  )
}

// OverlaysManager — the "Manage overlays ▾" dropdown. Replaces the old row of
// per-panel chips so the toolbar stays compact as overlays grow. Holds global
// actions (pop out all, reset positions/layout, monitor picker) plus a
// per-overlay grid: show/hide the dashboard panel, pop the window out, and
// recenter that window.
function OverlaysManager({
  panelKeys,
  labels,
  layout,
  onToggleVisible,
  popoutStates,
  onTogglePopout,
  onResetPanelPosition,
  placingNames,
  onMovePanel,
  anyPopoutOpen,
  onMenuOpen,
  onTogglePopouts,
  onResetPositions,
  onResetLayout,
  onClearTimers,
  displays,
  currentDisplayId,
  onDisplayChange,
}: {
  panelKeys: DashboardPanelKey[]
  labels: Record<DashboardPanelKey, string>
  layout: DashboardLayout
  onToggleVisible: (key: DashboardPanelKey) => void
  popoutStates: Record<string, boolean>
  onTogglePopout: (key: DashboardPanelKey) => void
  onResetPanelPosition: (key: DashboardPanelKey) => void
  placingNames: string[]
  onMovePanel: (key: DashboardPanelKey) => void
  anyPopoutOpen: boolean
  onMenuOpen: () => void
  onTogglePopouts: () => void
  onResetPositions: () => void
  onResetLayout: () => void
  onClearTimers: () => void
  displays: Array<{ id: number; label: string; width: number; height: number; isPrimary: boolean; isCurrent: boolean }>
  currentDisplayId?: number
  onDisplayChange: (id: number) => void
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)

  // Close on outside click. Also re-read popout state each time the menu opens
  // so the bulk-action label and per-overlay indicators are fresh on display.
  useEffect(() => {
    if (!open) return
    onMenuOpen()
    const onDown = (e: MouseEvent): void => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [open, onMenuOpen])

  const actionBtn = {
    backgroundColor: 'var(--color-surface)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
    cursor: 'pointer',
  } as const
  const headerCell = { color: 'var(--color-muted)', width: 30, textAlign: 'center' as const }

  return (
    <div ref={wrapRef} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
        style={{
          backgroundColor: open ? 'var(--color-surface-2)' : 'var(--color-surface)',
          color: 'var(--color-foreground)',
          border: '1px solid var(--color-border)',
        }}
        title="Show/hide dashboard panels, pop out windows, and reset positions"
      >
        <ListChecks size={11} />
        Manage overlays
        <ChevronDown size={11} style={{ opacity: 0.7 }} />
      </button>

      {open && (
        <div
          className="rounded-lg p-3"
          style={{
            position: 'absolute',
            right: 0,
            top: 'calc(100% + 6px)',
            zIndex: 50,
            width: 380,
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
          }}
        >
          {/* Global actions */}
          <div className="mb-3 flex flex-wrap items-center gap-2">
            {typeof window.electron?.overlay?.anyPopoutOpen === 'function' && (
              <button
                onClick={onTogglePopouts}
                className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
                style={actionBtn}
              >
                {anyPopoutOpen ? <X size={11} /> : <Layers size={11} />}
                {anyPopoutOpen ? 'Close All Popouts' : 'Pop Out All'}
              </button>
            )}
            {typeof window.electron?.overlay?.resetAllPositions === 'function' && (
              <button
                onClick={onResetPositions}
                className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
                style={actionBtn}
                title="Recenter all pop-out overlay windows on the primary monitor and unlock them"
              >
                <Crosshair size={11} /> Reset Positions
              </button>
            )}
            <button
              onClick={onResetLayout}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={actionBtn}
              title="Reset the in-app dashboard panel positions, sizes, and visibility to defaults"
            >
              <RotateCcw size={11} /> Reset Layout
            </button>
            <button
              onClick={onClearTimers}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={actionBtn}
              title="Clear every active buff, detrimental, and custom timer — use after switching characters so stale buffs from the old character don't linger"
            >
              <Trash2 size={11} /> Clear All Timers
            </button>
          </div>

          {/* Monitor picker — only meaningful with more than one display. */}
          {displays.length > 1 && (
            <div
              className="mb-3 flex items-center gap-1.5"
              title="Which monitor trigger alert text and the positioning card appear on"
            >
              <Monitor size={11} style={{ color: 'var(--color-muted-foreground)' }} />
              <select
                value={currentDisplayId ?? ''}
                onChange={(e) => onDisplayChange(Number(e.target.value))}
                className="flex-1 text-xs rounded px-1.5 py-1 outline-none"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
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

          {/* Per-overlay grid: Dashboard panel | Pop-out | Move (place) | Reset */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '1fr auto auto auto auto',
              columnGap: 8,
              alignItems: 'center',
            }}
          >
            <span className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Overlay
            </span>
            <span className="text-[10px] uppercase tracking-wide" style={headerCell}>Dash</span>
            <span className="text-[10px] uppercase tracking-wide" style={headerCell}>Pop</span>
            <span className="text-[10px] uppercase tracking-wide" style={headerCell} title="Move — drag to position">Move</span>
            <span className="text-[10px] uppercase tracking-wide" style={headerCell} title="Reset position">Res</span>
            {panelKeys.map((key) => {
              const dashOn = layout[key].visible
              const popOn = !!popoutStates[PANEL_POPOUT[key].name]
              return (
                <React.Fragment key={key}>
                  <span
                    className="truncate text-xs"
                    style={{ color: 'var(--color-foreground)', paddingTop: 5, paddingBottom: 5 }}
                  >
                    {labels[key]}
                  </span>
                  <RowIconButton
                    active={dashOn}
                    onClick={() => onToggleVisible(key)}
                    title={dashOn ? `Hide ${labels[key]} panel in the dashboard` : `Show ${labels[key]} panel in the dashboard`}
                  >
                    {dashOn ? <Eye size={12} /> : <EyeOff size={12} />}
                  </RowIconButton>
                  <RowIconButton
                    active={popOn}
                    onClick={() => onTogglePopout(key)}
                    title={popOn ? `Close the ${labels[key]} pop-out window` : `Pop out ${labels[key]} as a floating window`}
                  >
                    <ExternalLink size={12} />
                  </RowIconButton>
                  <RowIconButton
                    active={placingNames.includes(PANEL_POPOUT[key].name)}
                    onClick={() => onMovePanel(key)}
                    title={
                      placingNames.includes(PANEL_POPOUT[key].name)
                        ? `Done positioning ${labels[key]}`
                        : `Move ${labels[key]} — pops it out and lets you drag it anywhere, even a Display-only HUD`
                    }
                  >
                    <Move size={12} />
                  </RowIconButton>
                  <RowIconButton
                    onClick={() => onResetPanelPosition(key)}
                    title={`Recenter the ${labels[key]} pop-out on the primary monitor and unlock it`}
                  >
                    <Crosshair size={12} />
                  </RowIconButton>
                </React.Fragment>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

export default function OverlaysDashboard(): React.ReactElement {
  const [layout, setLayout] = useState<DashboardLayout>(() => loadDashboardLayout())
  // Bumped on Reset so panels remount with fresh defaults — OverlayWindow keeps
  // its position in local state, so we have to force a fresh mount to apply
  // new initial values.
  const [layoutVersion, setLayoutVersion] = useState(0)

  // The Threat Meter is a dev-gated experimental overlay (threat_meter_enabled).
  // Fetch the flag on mount and keep it live via config:updated so toggling it
  // in the Developer tab shows/hides the card without reloading the app.
  const [threatEnabled, setThreatEnabled] = useState(false)
  useEffect(() => {
    getConfig()
      .then((c) => setThreatEnabled(Boolean(c?.preferences?.threat_meter_enabled)))
      .catch(() => {})
  }, [])
  const handleConfigWs = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    const c = msg.data as { preferences?: { threat_meter_enabled?: boolean } }
    setThreatEnabled(Boolean(c?.preferences?.threat_meter_enabled))
  }, [])
  useWebSocket(handleConfigWs)

  // Panel keys actually offered in the UI: hps is gated by SHOW_HPS (module
  // level) and threat by the dev flag above.
  const visiblePanelKeys = useMemo(
    () => (threatEnabled ? VISIBLE_PANEL_KEYS : VISIBLE_PANEL_KEYS.filter((k) => k !== 'threat')),
    [threatEnabled],
  )

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
    (key: DashboardPanelKey) => updatePanel(key, { visible: !layout[key].visible }),
    [layout, updatePanel],
  )

  // Per-overlay popout open-state, keyed by canonical overlay name. Polled like
  // anyPopoutOpen since Electron doesn't push window-state changes.
  const [popoutStates, setPopoutStates] = useState<Record<string, boolean>>({})
  // Which overlays are currently in per-overlay "Move" (placing) mode.
  const [placingNames, setPlacingNames] = useState<string[]>([])

  useEffect(() => {
    if (!window.electron?.overlay?.popoutStates) return
    let cancelled = false
    const check = (): void => {
      window.electron.overlay
        .popoutStates()
        .then((s) => { if (!cancelled) setPopoutStates(s) })
        .catch(() => {})
      window.electron?.overlay?.placingNames?.()
        .then((n) => { if (!cancelled && n) setPlacingNames(n) })
        .catch(() => {})
    }
    check()
    const id = setInterval(check, 1500)
    return () => { cancelled = true; clearInterval(id) }
  }, [])

  const togglePopout = useCallback((key: DashboardPanelKey) => {
    PANEL_POPOUT[key].toggle()
    // Optimistic flip; the poll reconciles shortly after.
    const name = PANEL_POPOUT[key].name
    setPopoutStates((s) => ({ ...s, [name]: !s[name] }))
  }, [])

  const resetPanelPosition = useCallback((key: DashboardPanelKey) => {
    window.electron?.overlay?.resetPosition?.(PANEL_POPOUT[key].name)
  }, [])

  // Enter/leave per-overlay "Move" mode. Pops the overlay out first if it's
  // closed (placing only makes sense on an open window — the banner draws on
  // it), then flips the placing flag for that one overlay.
  const toggleMovePanel = useCallback((key: DashboardPanelKey) => {
    const name = PANEL_POPOUT[key].name
    const isPlacing = placingNames.includes(name)
    if (isPlacing) {
      window.electron?.overlay?.place?.(name, false)
      setPlacingNames((p) => p.filter((n) => n !== name))
      return
    }
    if (!popoutStates[name]) {
      PANEL_POPOUT[key].toggle()
      setPopoutStates((s) => ({ ...s, [name]: true }))
    }
    window.electron?.overlay?.place?.(name, true)
    setPlacingNames((p) => [...p, name])
  }, [placingNames, popoutStates])

  const handleReset = useCallback(() => {
    setLayout({ ...DEFAULT_DASHBOARD_LAYOUT })
    setLayoutVersion((v) => v + 1)
  }, [])

  // Recenter every popped-out overlay window on the primary monitor and unlock
  // them. This is the recovery path for an overlay that's drifted off-screen
  // (where a locked window can't reach its own unlock button). Distinct from
  // Reset Dashboard Layout above, which only affects the in-app docked panels.
  // Gated behind the shared themed ConfirmModal (not window.confirm) so it
  // matches the rest of the app's dialogs.
  const [showResetConfirm, setShowResetConfirm] = useState(false)

  const confirmResetPositions = useCallback(() => {
    setShowResetConfirm(false)
    window.electron?.overlay?.resetAllPositions?.().catch(() => {})
  }, [])

  // Clear-all-timers is destructive (wipes active buff/detrimental/custom
  // timers), so it's gated behind a confirm. Main use is after switching
  // characters, where the old character's timers would otherwise keep running.
  const [showClearTimersConfirm, setShowClearTimersConfirm] = useState(false)

  const confirmClearTimers = useCallback(() => {
    setShowClearTimersConfirm(false)
    clearTimers('all').catch(() => {})
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

  // Re-read popout open-state on demand — used when the Manage overlays menu
  // opens so the "Pop Out All / Close All Popouts" label and per-overlay Pop
  // indicators reflect reality immediately instead of waiting for the next
  // 1.5s poll. Matters right after launch, when overlays may already be open
  // (e.g. restored by "Restore overlays on launch"): the label must start on
  // "Close All Popouts" rather than briefly showing the wrong text.
  const refreshPopouts = useCallback(() => {
    const o = window.electron?.overlay
    if (!o) return
    o.anyPopoutOpen?.().then(setAnyPopoutOpen).catch(() => {})
    o.popoutStates?.().then(setPopoutStates).catch(() => {})
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
    for (const key of visiblePanelKeys) {
      const p = layout[key]
      if (!p.visible) continue
      if (p.x + p.width > maxRight) maxRight = p.x + p.width
      if (p.y + p.height > maxBottom) maxBottom = p.y + p.height
    }
    return { width: maxRight + CANVAS_PADDING, height: maxBottom + CANVAS_PADDING }
  }, [layout, visiblePanelKeys])

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
      const visiblePanels = visiblePanelKeys.filter((k) => layout[k].visible)
      o.openAllPopouts(visiblePanels).catch(() => {})
      setAnyPopoutOpen(true)
    }
  }, [anyPopoutOpen, layout, visiblePanelKeys])

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
      {/* Toolbar — compact: title left, the Manage overlays menu (per-overlay
          show/pop-out/reset + bulk actions) on the right. Each popped-out
          overlay is positioned individually (drag its window, or pick a
          monitor in Settings), so there's no global position-mode toggle. */}
      <div
        className="flex items-center gap-3 border-b px-4 py-2 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-2 shrink-0">
          <MonitorPlay size={14} style={{ color: 'var(--color-primary)' }} />
          <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Overlays
          </span>
        </div>

        <div className="min-w-0 flex-1" />

        <div className="flex items-center gap-2 shrink-0">
          <OverlaysManager
            panelKeys={visiblePanelKeys}
            labels={DASHBOARD_PANEL_LABELS}
            layout={layout}
            onToggleVisible={toggleVisible}
            popoutStates={popoutStates}
            onTogglePopout={togglePopout}
            onResetPanelPosition={resetPanelPosition}
            placingNames={placingNames}
            onMovePanel={toggleMovePanel}
            anyPopoutOpen={anyPopoutOpen}
            onMenuOpen={refreshPopouts}
            onTogglePopouts={handleTogglePopouts}
            onResetPositions={() => setShowResetConfirm(true)}
            onResetLayout={handleReset}
            onClearTimers={() => setShowClearTimersConfirm(true)}
            displays={displays}
            currentDisplayId={currentDisplayId}
            onDisplayChange={handleDisplayChange}
          />
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
        {threatEnabled && layout.threat.visible && (
          <ThreatPanel
            key={`threat-${layoutVersion}`}
            defaultX={layout.threat.x}
            defaultY={layout.threat.y}
            defaultWidth={layout.threat.width}
            defaultHeight={layout.threat.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('threat')}
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
        {layout.custom.visible && (
          <CustomTimerPanel
            key={`custom-${layoutVersion}`}
            defaultX={layout.custom.x}
            defaultY={layout.custom.y}
            defaultWidth={layout.custom.width}
            defaultHeight={layout.custom.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('custom')}
          />
        )}
      </div>

      {showResetConfirm && (
        <ConfirmModal
          title="Reset Window Positions"
          message="Recenter all pop-out overlay windows on your primary monitor and unlock them? This is the recovery path for an overlay that has drifted off-screen."
          confirmLabel="Reset Positions"
          onConfirm={confirmResetPositions}
          onCancel={() => setShowResetConfirm(false)}
        />
      )}
      {showClearTimersConfirm && (
        <ConfirmModal
          title="Clear All Timers"
          message="Clear every active buff, detrimental, and custom timer? Handy after switching characters so the old character's buffs stop showing. This can't be undone."
          confirmLabel="Clear All Timers"
          onConfirm={confirmClearTimers}
          onCancel={() => setShowClearTimersConfirm(false)}
        />
      )}
    </div>
  )
}
