/**
 * OverlaysDashboard — the Overlays hub's Dashboard tab: the overlay panels
 * inside the app, draggable and resizable, for a second monitor. Each overlay
 * layout keeps its own arrangement (see services/dashboardLayout.ts), so
 * switching layouts swaps the dashboard too. The toolbar only picks which
 * panels show and resets the arrangement; popouts are managed from the
 * Library tab.
 * Drag/resize snaps to a 16px grid.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CheckSquare, ChevronDown, ListChecks, RotateCcw, Square } from 'lucide-react'
import BuffTimerPanel from '../components/overlays/BuffTimerPanel'
import FilteredTimerPanel from '../components/overlays/FilteredTimerPanel'
import KillCounterPanel from '../components/overlays/KillCounterPanel'
import DeathRecapPanel from '../components/overlays/DeathRecapPanel'
import ItemInfoPanel from '../components/overlays/ItemInfoPanel'
import PetPanel from '../components/overlays/PetPanel'
import DetrimTimerPanel from '../components/overlays/DetrimTimerPanel'
import DPSPanel from '../components/overlays/DPSPanel'
import NPCPanel from '../components/overlays/NPCPanel'
import ThreatPanel from '../components/overlays/ThreatPanel'
import RollTrackerPanel from '../components/overlays/RollTrackerPanel'
import RespawnTimerPanel from '../components/overlays/RespawnTimerPanel'
import ZoneLockoutsPanel from '../components/overlays/ZoneLockoutsPanel'
import RaidReadinessPanel from '../components/overlays/RaidReadinessPanel'
import CHChainPanel from '../components/overlays/CHChainPanel'
import CHMetronomePanel from '../components/overlays/CHMetronomePanel'
import CustomTimerPanel from '../components/overlays/CustomTimerPanel'
import DiscordVoicePanel from '../components/overlays/DiscordVoicePanel'
import LiveMapPanel from '../components/overlays/LiveMapPanel'
import { ConfirmModal } from '../components/ConfirmModal'
import { listTimerGroups, getConfig } from '../services/api'
import CustomOverlayPanel from '../components/overlays/CustomOverlayPanel'
import { useCustomOverlays } from '../hooks/useCustomOverlays'
import { customOverlayWindowKey } from '../lib/customOverlay'
import { useWebSocket } from '../hooks/useWebSocket'
import { useItemInfoOffered } from '../hooks/useItemDisplay'
import { WSEvent } from '../lib/wsEvents'
import type { TimerGroup } from '../types/trigger'
import {
  DASHBOARD_PANEL_LABELS,
  DASHBOARD_PANEL_KEYS,
  defaultGroupPanelLayout,
  resetDashboard,
  updateDashboardGroups,
  updateDashboardPanels,
  useDashboardLayouts,
  type DashboardLayout,
  type DashboardPanelKey,
  type GroupPanelLayouts,
  type PanelLayout,
} from '../services/dashboardLayout'
import { NewDot, useFeatureDots } from '../hooks/useFeatureDots'

const SNAP_GRID = 16
// Extra empty space kept below/right of the lowest/rightmost visible panel
// so panels never sit flush against the scroll container edge and users can
// still grab the bottom resize handle once scrolled to the end.
const CANVAS_PADDING = 120

// PanelsPicker — "Panels ▾": which panels show on this layout's dashboard.
// Popping out, positioning and hotkeys live on the Library cards.
function PanelsPicker({
  panelKeys,
  labels,
  lockedHints,
  layout,
  onToggleVisible,
  groups,
  groupLayout,
  onToggleGroupVisible,
}: {
  panelKeys: DashboardPanelKey[]
  labels: Record<DashboardPanelKey, string>
  // Panels listed but not usable yet: greyed, with this hint (e.g. Item Info
  // before Zeal support is detected).
  lockedHints: Partial<Record<DashboardPanelKey, string>>
  layout: DashboardLayout
  onToggleVisible: (key: DashboardPanelKey) => void
  // "Your windows": named timer windows (keyed by group id) and custom
  // overlays (keyed customOverlay:<id>), each with a groupLayout entry.
  groups: Array<{ id: string; name: string }>
  groupLayout: GroupPanelLayouts
  onToggleGroupVisible: (id: string) => void
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)
  const { overlayDot, markOverlaySeen } = useFeatureDots()
  const anyNew = panelKeys.some((k) => !lockedHints[k] && overlayDot(k))
  const shown = panelKeys.filter((k) => !lockedHints[k] && layout[k].visible).length
    + groups.filter((g) => groupLayout[g.id]?.visible).length

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent): void => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const row = (
    key: string,
    label: string,
    on: boolean,
    onClick: () => void,
    extra?: React.ReactNode,
  ): React.ReactElement => (
    <button
      key={key}
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs hover:bg-[var(--color-surface-2)]"
      style={{ color: 'var(--color-foreground)' }}
    >
      {on
        ? <CheckSquare size={12} style={{ color: 'var(--color-primary)' }} />
        : <Square size={12} style={{ color: 'var(--color-muted-foreground)' }} />}
      <span className="truncate">{label}</span>
      {extra}
    </button>
  )

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
        title="Choose which panels show on this layout's dashboard"
      >
        <ListChecks size={11} />
        Panels
        <span style={{ color: 'var(--color-muted-foreground)' }}>{shown}</span>
        <ChevronDown size={11} style={{ opacity: 0.7 }} />
        {anyNew && <NewDot title="New overlays inside" />}
      </button>

      {open && (
        <div
          className="rounded-lg p-1.5"
          style={{
            position: 'absolute',
            right: 0,
            top: 'calc(100% + 6px)',
            zIndex: 50,
            width: 240,
            maxHeight: '70vh',
            overflowY: 'auto',
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
          }}
        >
          {panelKeys.map((key) => {
            const hint = lockedHints[key]
            if (hint) {
              return (
                <div key={key} className="px-2 py-1" style={{ opacity: 0.6 }} title={hint}>
                  <span className="block truncate text-xs" style={{ color: 'var(--color-muted)' }}>
                    {labels[key]}
                  </span>
                  <span className="block text-[10px] leading-tight" style={{ color: 'var(--color-muted)' }}>
                    {hint}
                  </span>
                </div>
              )
            }
            return row(
              key,
              labels[key],
              layout[key].visible,
              () => { markOverlaySeen(key); onToggleVisible(key) },
              overlayDot(key) && <NewDot title="New since your last update" />,
            )
          })}
          {groups.length > 0 && (
            <div
              className="mt-1 border-t px-2 pb-0.5 pt-1.5 text-[10px] uppercase tracking-wide"
              style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
            >
              Your windows
            </div>
          )}
          {groups.map((g) =>
            row(g.id, g.name, groupLayout[g.id]?.visible ?? false, () => onToggleGroupVisible(g.id)),
          )}
        </div>
      )}
    </div>
  )
}

export default function OverlaysDashboard(): React.ReactElement {
  // The active overlay layout's arrangement. version bumps when it's replaced
  // (layout switch, reset, first load); panels are keyed on it to remount,
  // since OverlayWindow keeps its position in local state.
  const {
    ready,
    panels: layout,
    groups: groupLayout,
    version: layoutVersion,
  } = useDashboardLayouts()

  // Discord Voice only makes sense to offer here once the user has actually
  // turned it on in its Overlays hub settings — unlike the other panels, showing it
  // with nothing configured would just be dead weight in the menu. Read live
  // since it's a runtime preference the user can flip at any time.
  const [discordVoiceEnabled, setDiscordVoiceEnabled] = useState(false)
  const refreshDiscordVoiceEnabled = useCallback(() => {
    getConfig()
      .then((c) => setDiscordVoiceEnabled(c.preferences.discord_voice_enabled ?? false))
      .catch(() => {})
  }, [])
  useEffect(() => { refreshDiscordVoiceEnabled() }, [refreshDiscordVoiceEnabled])
  useWebSocket(
    useCallback(
      (msg: { type: string }) => {
        if (msg.type === WSEvent.ConfigUpdated) refreshDiscordVoiceEnabled()
      },
      [refreshDiscordVoiceEnabled],
    ),
  )

  // Item Info stays out of the dashboard until Zeal has sent an item display
  // event (or Developer Mode is on) — see itemInfoOffered.
  const itemInfoOffered = useItemInfoOffered()

  // Panel keys actually offered in the UI: discordVoice is gated by the
  // setting above; itemInfo by Zeal support; the rest (including Raid
  // Readiness, the Threat Meter and Pet) are always available.
  const visiblePanelKeys = useMemo(() => {
    let keys = DASHBOARD_PANEL_KEYS
    if (!discordVoiceEnabled) keys = keys.filter((k) => k !== 'discordVoice')
    if (!itemInfoOffered) keys = keys.filter((k) => k !== 'itemInfo')
    return keys
  }, [discordVoiceEnabled, itemInfoOffered])

  // The Panels picker still lists Item Info before Zeal support is
  // detected, greyed with a hint, so users know the feature exists and how
  // to unlock it.
  const menuPanelKeys = useMemo(
    () => DASHBOARD_PANEL_KEYS.filter(
      (k) => k !== 'discordVoice' || discordVoiceEnabled,
    ),
    [discordVoiceEnabled],
  )
  const menuLockedHints = useMemo<Partial<Record<DashboardPanelKey, string>>>(
    () => (itemInfoOffered ? {} : {
      itemInfo: 'Needs Zeal 1.5.0+ — open any item in game to enable',
    }),
    [itemInfoOffered],
  )

  const updatePanel = useCallback(
    (key: DashboardPanelKey, patch: Partial<DashboardLayout[DashboardPanelKey]>) => {
      updateDashboardPanels((prev) => ({ ...prev, [key]: { ...prev[key], ...patch } }))
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

  // Reset dashboard: the active layout's panels go back to the default
  // arrangement. Asks first, since a hand-built dashboard can't be undone.
  const [showResetConfirm, setShowResetConfirm] = useState(false)
  const confirmReset = useCallback(() => {
    setShowResetConfirm(false)
    resetDashboard()
  }, [])

  // ── Named timer-group dashboard panels ──────────────────────────────────
  // Mirrors the fixed-panel state/handlers above, but keyed by TimerGroup.id
  // instead of a compile-time DashboardPanelKey — see dashboardLayout.ts.
  const [timerGroups, setTimerGroups] = useState<TimerGroup[]>([])

  const reloadTimerGroups = useCallback(() => {
    listTimerGroups().then(setTimerGroups).catch(() => {})
  }, [])

  useEffect(() => { reloadTimerGroups() }, [reloadTimerGroups])

  // Custom overlays share the group panel map, keyed customOverlay:<id> so
  // they can't collide with a timer group's id.
  const co = useCustomOverlays()
  const yourWindows = useMemo(
    () => [
      ...timerGroups.map((g) => ({ id: g.id, name: g.name })),
      ...co.overlays.map((o) => ({ id: customOverlayWindowKey(o.def.id), name: o.def.name })),
    ],
    [timerGroups, co.overlays],
  )

  // A group the dashboard hasn't seen before (just created, new to this
  // layout, or after a reset) gets a default hidden panel layout.
  useEffect(() => {
    if (!ready) return
    updateDashboardGroups((prev) => {
      let changed = false
      const next = { ...prev }
      yourWindows.forEach((g, i) => {
        if (!next[g.id]) {
          next[g.id] = defaultGroupPanelLayout(i)
          changed = true
        }
      })
      return changed ? next : prev
    })
  }, [yourWindows, ready, groupLayout])

  const updateGroupPanel = useCallback((id: string, patch: Partial<PanelLayout>) => {
    updateDashboardGroups((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? defaultGroupPanelLayout(0)), ...patch },
    }))
  }, [])

  const handleGroupLayoutChange = useCallback(
    (id: string) =>
      (b: { x: number; y: number; width: number; height: number }) => {
        updateGroupPanel(id, { x: b.x, y: b.y, width: b.width, height: b.height })
      },
    [updateGroupPanel],
  )

  const toggleGroupVisible = useCallback(
    (id: string) => updateGroupPanel(id, { visible: !(groupLayout[id]?.visible ?? false) }),
    [groupLayout, updateGroupPanel],
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
    for (const g of yourWindows) {
      const p = groupLayout[g.id]
      if (!p?.visible) continue
      if (p.x + p.width > maxRight) maxRight = p.x + p.width
      if (p.y + p.height > maxBottom) maxBottom = p.y + p.height
    }
    return { width: maxRight + CANVAS_PADDING, height: maxBottom + CANVAS_PADDING }
  }, [layout, visiblePanelKeys, yourWindows, groupLayout])

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
      {/* Toolbar: which panels show on this layout's dashboard, and a reset.
          Popouts, positions, hotkeys and timer windows are on the Library
          tab; actions on every overlay are in the hub's ⋯ menu. */}
      <div
        className="flex items-center gap-2 border-b px-4 py-2 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="min-w-0 flex-1" />
        <button
          onClick={() => setShowResetConfirm(true)}
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
          title="Put this layout's dashboard panels back to the default arrangement"
        >
          <RotateCcw size={11} />
          Reset dashboard
        </button>
        <PanelsPicker
          panelKeys={menuPanelKeys}
          lockedHints={menuLockedHints}
          labels={DASHBOARD_PANEL_LABELS}
          layout={layout}
          onToggleVisible={toggleVisible}
          groups={yourWindows}
          groupLayout={groupLayout}
          onToggleGroupVisible={toggleGroupVisible}
        />
      </div>

      {/* Dashboard canvas. Hidden until the layout's arrangement loads, so
          panels don't flash at their default spots first. */}
      <div style={{ position: 'relative', flex: 1, overflow: 'auto', visibility: ready ? undefined : 'hidden' }}>
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
        {layout.filtered.visible && (
          <FilteredTimerPanel
            key={`filtered-${layoutVersion}`}
            defaultX={layout.filtered.x}
            defaultY={layout.filtered.y}
            defaultWidth={layout.filtered.width}
            defaultHeight={layout.filtered.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('filtered')}
          />
        )}
        {layout.kills.visible && (
          <KillCounterPanel
            key={`kills-${layoutVersion}`}
            defaultX={layout.kills.x}
            defaultY={layout.kills.y}
            defaultWidth={layout.kills.width}
            defaultHeight={layout.kills.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('kills')}
          />
        )}
        {layout.itemInfo.visible && itemInfoOffered && (
          <ItemInfoPanel
            key={`itemInfo-${layoutVersion}`}
            defaultX={layout.itemInfo.x}
            defaultY={layout.itemInfo.y}
            defaultWidth={layout.itemInfo.width}
            defaultHeight={layout.itemInfo.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('itemInfo')}
          />
        )}
        {layout.pet.visible && (
          <PetPanel
            key={`pet-${layoutVersion}`}
            defaultX={layout.pet.x}
            defaultY={layout.pet.y}
            defaultWidth={layout.pet.width}
            defaultHeight={layout.pet.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('pet')}
          />
        )}
        {layout.deathRecap.visible && (
          <DeathRecapPanel
            key={`deathRecap-${layoutVersion}`}
            defaultX={layout.deathRecap.x}
            defaultY={layout.deathRecap.y}
            defaultWidth={layout.deathRecap.width}
            defaultHeight={layout.deathRecap.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('deathRecap')}
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
        {layout.threat.visible && (
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
        {layout.zoneLockouts.visible && (
          <ZoneLockoutsPanel
            key={`zoneLockouts-${layoutVersion}`}
            defaultX={layout.zoneLockouts.x}
            defaultY={layout.zoneLockouts.y}
            defaultWidth={layout.zoneLockouts.width}
            defaultHeight={layout.zoneLockouts.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('zoneLockouts')}
          />
        )}
        {layout.raidReadiness.visible && (
          <RaidReadinessPanel
            key={`raidReadiness-${layoutVersion}`}
            defaultX={layout.raidReadiness.x}
            defaultY={layout.raidReadiness.y}
            defaultWidth={layout.raidReadiness.width}
            defaultHeight={layout.raidReadiness.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('raidReadiness')}
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
        {discordVoiceEnabled && layout.discordVoice.visible && (
          <DiscordVoicePanel
            key={`discordVoice-${layoutVersion}`}
            defaultX={layout.discordVoice.x}
            defaultY={layout.discordVoice.y}
            defaultWidth={layout.discordVoice.width}
            defaultHeight={layout.discordVoice.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('discordVoice')}
          />
        )}
        {layout.liveMap.visible && (
          <LiveMapPanel
            key={`liveMap-${layoutVersion}`}
            defaultX={layout.liveMap.x}
            defaultY={layout.liveMap.y}
            defaultWidth={layout.liveMap.width}
            defaultHeight={layout.liveMap.height}
            snapGridSize={SNAP_GRID}
            onLayoutChange={handleLayoutChange('liveMap')}
          />
        )}
        {timerGroups.map((g) => {
          const p = groupLayout[g.id]
          if (!p?.visible) return null
          return (
            <CustomTimerPanel
              key={`group-${g.id}-${layoutVersion}`}
              groupId={g.id}
              groupName={g.name}
              defaultX={p.x}
              defaultY={p.y}
              defaultWidth={p.width}
              defaultHeight={p.height}
              snapGridSize={SNAP_GRID}
              onLayoutChange={handleGroupLayoutChange(g.id)}
            />
          )
        })}
        {co.overlays.map((o) => {
          const key = customOverlayWindowKey(o.def.id)
          const p = groupLayout[key]
          if (!p?.visible) return null
          return (
            <CustomOverlayPanel
              key={`${key}-${layoutVersion}`}
              overlay={o}
              co={co}
              defaultX={p.x}
              defaultY={p.y}
              defaultWidth={p.width}
              defaultHeight={p.height}
              snapGridSize={SNAP_GRID}
              onLayoutChange={handleGroupLayoutChange(key)}
            />
          )
        })}
      </div>

      {showResetConfirm && (
        <ConfirmModal
          title="Reset Dashboard"
          message="Put this layout's dashboard panels back where they start, at their default sizes, with the default panels shown? Pop-out windows aren't affected."
          confirmLabel="Reset Dashboard"
          onConfirm={confirmReset}
          onCancel={() => setShowResetConfirm(false)}
        />
      )}
    </div>
  )
}
