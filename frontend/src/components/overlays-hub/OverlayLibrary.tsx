/**
 * OverlayLibrary — the Overlays hub's Library tab. Every overlay is a card in
 * its category: pop out / close (or show, when hidden), whether it also gets a
 * Dashboard panel, and its hotkey if one is bound. Clicking a card opens its
 * details on the right. Overlays that can't be used right now stay listed,
 * greyed, with the reason. Custom timer windows close the list under "Your
 * windows".
 *
 * Dashboard visibility is written straight to the dashboard's saved layout;
 * the Dashboard tab reads it fresh when it mounts.
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Bell, ExternalLink, Eye, Hourglass, Keyboard, LayoutDashboard, Plus, RotateCcw, Settings2, X } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import {
  CUSTOM_GROUP_INFO,
  OVERLAY_CATALOG,
  OVERLAY_CATEGORIES,
  PANEL_POPOUT,
  TRIGGER_ALERTS_INFO,
  type OverlayCatalogEntry,
} from '../../lib/overlayCatalog'
import {
  defaultGroupPanelLayout,
  loadDashboardLayout,
  loadGroupPanelLayouts,
  saveDashboardLayout,
  saveGroupPanelLayouts,
  type DashboardLayout,
  type DashboardPanelKey,
  type GroupPanelLayouts,
} from '../../services/dashboardLayout'
import { getConfig, listTimerGroups } from '../../services/api'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useItemInfoOffered } from '../../hooks/useItemDisplay'
import { NewDot, useFeatureDots } from '../../hooks/useFeatureDots'
import { WSEvent } from '../../lib/wsEvents'
import { acceleratorParts, overlayAction } from '../../lib/hotkeys'
import TimerGroupsModal from '../TimerGroupsModal'
import { useConfigDraft, type DraftSaveState } from '../../hooks/useConfigDraft'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import GlobalOverlaySettings, { AlertTextSettings } from './settings/GlobalOverlaySettings'
import { OverlayWindowControls } from './settings/OverlayWindowSettings'
import { SettingsSection } from './settings/SettingsSection'
import { OVERLAY_SETTINGS } from './settings/OverlaySpecificSettings'
import { sameSelection, type HubSelection } from './selection'
import type { TimerGroup } from '../../types/trigger'

type WindowState = 'open' | 'hidden' | undefined
type Selection = HubSelection

const POLL_MS = 1500

interface Unavailable {
  reason: string
  fix?: { label: string; to: string }
}

// ── Pieces ───────────────────────────────────────────────────────────────────

function IconTile({ icon: Icon, color, size = 32 }: { icon: LucideIcon; color: string; size?: number }): React.ReactElement {
  return (
    <span
      className="flex shrink-0 items-center justify-center rounded-md"
      style={{ width: size, height: size, backgroundColor: `${color}1f`, border: `1px solid ${color}40` }}
    >
      <Icon size={Math.round(size * 0.47)} style={{ color }} />
    </span>
  )
}

function HotkeyBadge({ accelerator, onClick }: { accelerator: string; onClick: () => void }): React.ReactElement {
  return (
    <button
      onClick={(e) => { e.stopPropagation(); onClick() }}
      className="flex shrink-0 items-center gap-0.5 rounded px-1 py-px font-mono text-[10px]"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-muted-foreground)',
      }}
      title="Hotkey · change it in Settings → Hotkeys"
    >
      {acceleratorParts(accelerator).join('+')}
    </button>
  )
}

function PopButton({
  state,
  disabled,
  onClick,
}: {
  state: WindowState
  disabled?: boolean
  onClick: () => void
}): React.ReactElement {
  const label = state === 'open' ? 'Close' : state === 'hidden' ? 'Show' : 'Pop out'
  const Icon = state === 'open' ? X : state === 'hidden' ? Eye : ExternalLink
  const primary = state !== 'open'
  return (
    <button
      onClick={(e) => { e.stopPropagation(); onClick() }}
      disabled={disabled}
      className="flex items-center gap-1 rounded px-2 py-1 text-xs"
      style={{
        backgroundColor: primary ? 'var(--color-primary)' : 'var(--color-surface-2)',
        color: primary ? 'var(--color-background)' : 'var(--color-foreground)',
        border: `1px solid ${primary ? 'var(--color-primary)' : 'var(--color-border)'}`,
        fontWeight: primary ? 600 : 400,
        opacity: disabled ? 0.4 : 1,
        cursor: disabled ? 'not-allowed' : 'pointer',
      }}
      title={
        state === 'open'
          ? 'Close this overlay window'
          : state === 'hidden'
            ? 'Hidden by Hide All or a hotkey · click to show it'
            : 'Open this overlay as a window over the game'
      }
    >
      <Icon size={11} />
      {label}
    </button>
  )
}

function DashboardSwitch({
  on,
  disabled,
  onClick,
}: {
  on: boolean
  disabled?: boolean
  onClick: () => void
}): React.ReactElement {
  return (
    <button
      onClick={(e) => { e.stopPropagation(); onClick() }}
      disabled={disabled}
      role="switch"
      aria-checked={on}
      className="flex items-center gap-1.5 text-[11px]"
      style={{
        color: on ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
        opacity: disabled ? 0.4 : 1,
        cursor: disabled ? 'not-allowed' : 'pointer',
      }}
      title={on ? 'Shown on the Dashboard tab · click to remove' : 'Add a panel for this to the Dashboard tab'}
    >
      <span
        className="relative inline-block rounded-full"
        style={{
          width: 24,
          height: 13,
          backgroundColor: on ? 'var(--color-primary)' : 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          transition: 'background-color 0.15s',
        }}
      >
        <span
          className="absolute rounded-full"
          style={{
            top: 1,
            left: on ? 12 : 1,
            width: 9,
            height: 9,
            backgroundColor: on ? 'var(--color-background)' : 'var(--color-muted-foreground)',
            transition: 'left 0.15s',
          }}
        />
      </span>
      <LayoutDashboard size={11} />
      Dashboard
    </button>
  )
}

function StatusText({ state }: { state: WindowState }): React.ReactElement | null {
  if (!state) return null
  return (
    <span
      className="text-[10px] font-semibold uppercase tracking-wide"
      style={{ color: state === 'open' ? 'var(--color-success)' : 'var(--color-muted-foreground)' }}
    >
      {state === 'open' ? 'Open' : 'Hidden'}
    </span>
  )
}

function Card({
  icon,
  color,
  title,
  blurb,
  isNew,
  selected,
  unavailable,
  hotkey,
  onHotkey,
  onSelect,
  footer,
}: {
  icon: LucideIcon
  color: string
  title: string
  blurb: string
  isNew?: boolean
  selected: boolean
  unavailable?: Unavailable | null
  hotkey?: string
  onHotkey?: () => void
  onSelect: () => void
  footer: React.ReactNode
}): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onSelect}
      onKeyDown={(e) => { if (e.key === 'Enter') onSelect() }}
      className="flex cursor-pointer flex-col gap-2 rounded-lg p-3 text-left"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${selected ? 'var(--color-primary)' : 'var(--color-border)'}`,
        boxShadow: selected ? '0 0 0 1px var(--color-primary)' : undefined,
      }}
    >
      <div className="flex items-start gap-2.5" style={{ opacity: unavailable ? 0.55 : 1 }}>
        <IconTile icon={icon} color={color} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="truncate text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {title}
            </span>
            {isNew && <NewDot />}
            <span className="flex-1" />
            {hotkey && onHotkey && <HotkeyBadge accelerator={hotkey} onClick={onHotkey} />}
          </div>
          <p className="mt-0.5 text-xs leading-snug" style={{ color: 'var(--color-muted-foreground)' }}>
            {blurb}
          </p>
        </div>
      </div>
      {unavailable && (
        <p className="text-[11px] leading-snug" style={{ color: 'var(--color-warning, #f59e0b)' }}>
          {unavailable.reason}
          {unavailable.fix && (
            <>
              {' '}
              <button
                onClick={(e) => { e.stopPropagation(); navigate(unavailable.fix!.to) }}
                className="underline"
                style={{ color: 'var(--color-primary)' }}
              >
                {unavailable.fix.label}
              </button>
            </>
          )}
        </p>
      )}
      <div className="mt-auto flex items-center gap-3 pt-1">{footer}</div>
    </div>
  )
}

function Drawer({
  icon,
  color,
  title,
  details,
  status,
  saveState,
  unavailable,
  actions,
  children,
  onClose,
}: {
  icon: LucideIcon
  color: string
  title: string
  details?: string
  status?: React.ReactNode
  saveState: DraftSaveState
  unavailable?: Unavailable | null
  actions?: React.ReactNode
  children?: React.ReactNode
  onClose: () => void
}): React.ReactElement {
  useEscapeToClose(onClose)
  return (
    <aside
      className="absolute inset-y-0 right-0 z-20 flex w-[640px] max-w-full flex-col border-l"
      style={{
        borderColor: 'var(--color-border)',
        backgroundColor: 'var(--color-background)',
        boxShadow: '-12px 0 32px rgba(0,0,0,0.45)',
      }}
    >
      <div
        className="flex shrink-0 items-center gap-2.5 border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        <IconTile icon={icon} color={color} size={36} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{title}</div>
          {status}
        </div>
        <span className="text-[11px]" style={{ color: saveState === 'error' ? 'var(--color-danger)' : 'var(--color-muted-foreground)' }}>
          {saveState === 'saving' ? 'Saving…' : saveState === 'saved' ? 'Saved' : saveState === 'error' ? 'Couldn’t save' : ''}
        </span>
        <button onClick={onClose} title="Close (Esc)" style={{ color: 'var(--color-muted-foreground)' }}>
          <X size={16} />
        </button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
        {details && (
          <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>{details}</p>
        )}
        {unavailable && (
          <p className="text-xs leading-snug" style={{ color: 'var(--color-warning)' }}>{unavailable.reason}</p>
        )}
        {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
        {children}
      </div>
    </aside>
  )
}

function SecondaryButton({
  icon: Icon,
  label,
  onClick,
  title,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  title?: string
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      className="flex items-center gap-1 rounded px-2 py-1 text-xs"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        color: 'var(--color-foreground)',
        border: '1px solid var(--color-border)',
      }}
    >
      <Icon size={11} />
      {label}
    </button>
  )
}

function SectionHeading({ label }: { label: string }): React.ReactElement {
  return (
    <h3
      className="mb-2 text-[11px] font-semibold uppercase tracking-wider"
      style={{ color: 'var(--color-muted-foreground)' }}
    >
      {label}
    </h3>
  )
}

const GRID_STYLE: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(auto-fill, minmax(250px, 1fr))',
  gap: 10,
}

// ── Library ──────────────────────────────────────────────────────────────────

export default function OverlayLibrary({
  selected,
  onSelect,
}: {
  selected: HubSelection | null
  onSelect: (s: HubSelection | null) => void
}): React.ReactElement {
  const navigate = useNavigate()
  const { overlayDot, markOverlaySeen } = useFeatureDots()
  const itemInfoOffered = useItemInfoOffered()
  const [windowStates, setWindowStates] = useState<Record<string, 'open' | 'hidden'>>({})
  const [layout, setLayout] = useState<DashboardLayout>(() => loadDashboardLayout())
  const [groupLayout, setGroupLayout] = useState<GroupPanelLayouts>(() => loadGroupPanelLayouts())
  const [bindings, setBindings] = useState<Record<string, string>>({})
  const [discordEnabled, setDiscordEnabled] = useState(false)
  const [groups, setGroups] = useState<TimerGroup[]>([])
  const [showGroupsModal, setShowGroupsModal] = useState(false)
  const setSelected = onSelect
  const { config, setConfig, saveState } = useConfigDraft()

  const refreshStates = useCallback(() => {
    window.electron?.overlay?.popoutWindowStates?.().then(setWindowStates).catch(() => {})
  }, [])

  useEffect(() => {
    refreshStates()
    const id = setInterval(refreshStates, POLL_MS)
    return () => clearInterval(id)
  }, [refreshStates])

  // Window creation is async; re-read shortly after each toggle.
  const afterToggle = useCallback(() => {
    setTimeout(refreshStates, 250)
    setTimeout(refreshStates, 900)
  }, [refreshStates])

  useEffect(() => {
    window.electron?.overlay?.hotkeysGet?.().then((s) => setBindings(s.bindings)).catch(() => {})
  }, [])

  const reloadGroups = useCallback(() => {
    listTimerGroups().then(setGroups).catch(() => {})
  }, [])
  useEffect(() => { reloadGroups() }, [reloadGroups])

  const refreshConfig = useCallback(() => {
    getConfig()
      .then((c) => setDiscordEnabled(c.preferences.discord_voice_enabled ?? false))
      .catch(() => {})
  }, [])
  useEffect(() => { refreshConfig() }, [refreshConfig])
  useWebSocket(
    useCallback(
      (msg: { type: string }) => {
        if (msg.type === WSEvent.ConfigUpdated) refreshConfig()
      },
      [refreshConfig],
    ),
  )

  const unavailableFor = useCallback(
    (panel: DashboardPanelKey): Unavailable | null => {
      if (panel === 'discordVoice' && !discordEnabled) {
        return { reason: 'Turned off.', fix: { label: 'Turn it on in its settings', to: '/overlays?panel=discordVoice' } }
      }
      if (panel === 'itemInfo' && !itemInfoOffered) {
        return { reason: 'Needs Zeal 1.5.0 or newer. Inspect any item in game to turn it on.' }
      }
      return null
    },
    [discordEnabled, itemInfoOffered],
  )

  const togglePopout = useCallback(
    (entry: OverlayCatalogEntry) => {
      markOverlaySeen(entry.panel)
      PANEL_POPOUT[entry.panel].toggle()
      afterToggle()
    },
    [afterToggle, markOverlaySeen],
  )

  const toggleDashboard = useCallback(
    (panel: DashboardPanelKey) => {
      markOverlaySeen(panel)
      setLayout((prev) => {
        const next = { ...prev, [panel]: { ...prev[panel], visible: !prev[panel].visible } }
        saveDashboardLayout(next)
        return next
      })
    },
    [markOverlaySeen],
  )

  const toggleGroupPopout = useCallback(
    (g: TimerGroup) => {
      window.electron?.overlay?.toggleCustomTimerGroup(g.id, g.name)
      afterToggle()
    },
    [afterToggle],
  )

  const toggleGroupDashboard = useCallback((id: string, index: number) => {
    setGroupLayout((prev) => {
      const cur = prev[id] ?? defaultGroupPanelLayout(index)
      const next = { ...prev, [id]: { ...cur, visible: !cur.visible } }
      saveGroupPanelLayouts(next)
      return next
    })
  }, [])

  const openHotkeys = useCallback(() => navigate('/settings?tab=hotkeys'), [navigate])

  const byCategory = useMemo(
    () => OVERLAY_CATEGORIES.map((c) => ({ ...c, entries: OVERLAY_CATALOG.filter((e) => e.category === c.id) })),
    [],
  )

  const resetPosition = (name: string): void => {
    window.electron?.overlay?.resetPosition?.(name)
  }

  // ── Settings drawer for the selection ──
  const close = (): void => setSelected(null)
  let details: React.ReactNode = null
  if (selected?.kind === 'global') {
    details = (
      <Drawer icon={Settings2} color="#c9a84c" title="Overlay settings" saveState={saveState} onClose={close}
        details="Settings for every overlay. Each overlay's own settings are on its card.">
        {config && <GlobalOverlaySettings config={config} setConfig={setConfig} focus={selected.section} />}
      </Drawer>
    )
  } else if (selected?.kind === 'overlay') {
    const entry = OVERLAY_CATALOG.find((e) => e.panel === selected.panel)
    if (entry) {
      const state = windowStates[entry.name]
      const unavailable = unavailableFor(entry.panel)
      const Specific = OVERLAY_SETTINGS[entry.panel]
      details = (
        <Drawer
          icon={entry.icon}
          color={entry.color}
          title={entry.label}
          details={entry.details}
          status={<StatusText state={state} />}
          saveState={saveState}
          unavailable={unavailable}
          onClose={close}
          actions={
            <>
              <PopButton state={state} disabled={!!unavailable && !state} onClick={() => togglePopout(entry)} />
              <SecondaryButton
                icon={RotateCcw}
                label="Reset position"
                onClick={() => resetPosition(entry.name)}
                title="Move this window to the middle of your main screen at its default size, and unlock it"
              />
              <SecondaryButton
                icon={Keyboard}
                label={bindings[overlayAction(entry.name)] ? 'Change hotkey' : 'Set a hotkey'}
                onClick={openHotkeys}
              />
            </>
          }
        >
          {config && (
            <SettingsSection title="Window">
              <OverlayWindowControls name={entry.name} label={entry.label} config={config} setConfig={setConfig} />
            </SettingsSection>
          )}
          {config && Specific && <Specific config={config} setConfig={setConfig} open={setSelected} />}
        </Drawer>
      )
    }
  } else if (selected?.kind === 'trigger') {
    details = (
      <Drawer
        icon={Bell}
        color="#c9a84c"
        title={TRIGGER_ALERTS_INFO.label}
        details={TRIGGER_ALERTS_INFO.details}
        status={<span className="text-[10px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-success)' }}>Always on</span>}
        saveState={saveState}
        onClose={close}
        actions={<SecondaryButton icon={ExternalLink} label="Open Triggers" onClick={() => navigate('/triggers')} />}
      >
        {config && <AlertTextSettings config={config} setConfig={setConfig} />}
      </Drawer>
    )
  } else if (selected?.kind === 'group') {
    const g = groups.find((x) => x.id === selected.id)
    if (g) {
      const key = `customTimer:${g.id}`
      const state = windowStates[key]
      details = (
        <Drawer
          icon={Hourglass}
          color="#38bdf8"
          title={g.name}
          details={CUSTOM_GROUP_INFO.details}
          status={<StatusText state={state} />}
          saveState={saveState}
          onClose={close}
          actions={
            <>
              <PopButton state={state} onClick={() => toggleGroupPopout(g)} />
              <SecondaryButton icon={RotateCcw} label="Reset position" onClick={() => resetPosition(key)} />
              <SecondaryButton icon={Hourglass} label="Manage timer windows" onClick={() => setShowGroupsModal(true)} />
            </>
          }
        />
      )
    }
  }

  const isSel = (s: Selection): boolean => sameSelection(selected, s)

  return (
    <div className="relative flex h-full min-h-0">
      <div className="min-w-0 flex-1 overflow-y-auto p-4">
        <div className="flex flex-col gap-6">
          {byCategory.map((cat) => (
            <section key={cat.id}>
              <SectionHeading label={cat.label} />
              <div style={GRID_STYLE}>
                {cat.entries.map((entry) => {
                  const state = windowStates[entry.name]
                  const unavailable = unavailableFor(entry.panel)
                  const sel: Selection = { kind: 'overlay', panel: entry.panel }
                  return (
                    <Card
                      key={entry.panel}
                      icon={entry.icon}
                      color={entry.color}
                      title={entry.label}
                      blurb={entry.blurb}
                      isNew={!unavailable && overlayDot(entry.panel)}
                      selected={isSel(sel)}
                      unavailable={unavailable}
                      hotkey={bindings[overlayAction(entry.name)]}
                      onHotkey={openHotkeys}
                      onSelect={() => { markOverlaySeen(entry.panel); setSelected(sel) }}
                      footer={
                        <>
                          <PopButton state={state} disabled={!!unavailable && !state} onClick={() => togglePopout(entry)} />
                          <DashboardSwitch
                            on={layout[entry.panel].visible && !unavailable}
                            disabled={!!unavailable}
                            onClick={() => toggleDashboard(entry.panel)}
                          />
                          <span className="flex-1" />
                          <StatusText state={state} />
                        </>
                      }
                    />
                  )
                })}
                {cat.id === 'alerts' && (
                  <Card
                    icon={Bell}
                    color="#c9a84c"
                    title={TRIGGER_ALERTS_INFO.label}
                    blurb={TRIGGER_ALERTS_INFO.blurb}
                    selected={isSel({ kind: 'trigger' })}
                    onSelect={() => setSelected({ kind: 'trigger' })}
                    footer={
                      <>
                        <span className="text-[10px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-success)' }}>
                          Always on
                        </span>
                        <span className="flex-1" />
                        <button
                          onClick={(e) => { e.stopPropagation(); navigate('/triggers') }}
                          className="text-[11px] underline"
                          style={{ color: 'var(--color-primary)' }}
                        >
                          Open Triggers
                        </button>
                      </>
                    }
                  />
                )}
              </div>
            </section>
          ))}

          <section>
            <SectionHeading label="Your windows" />
            <div style={GRID_STYLE}>
              {groups.map((g, i) => {
                const key = `customTimer:${g.id}`
                const state = windowStates[key]
                const sel: Selection = { kind: 'group', id: g.id }
                return (
                  <Card
                    key={g.id}
                    icon={Hourglass}
                    color="#38bdf8"
                    title={g.name}
                    blurb={CUSTOM_GROUP_INFO.blurb}
                    selected={isSel(sel)}
                    onSelect={() => setSelected(sel)}
                    footer={
                      <>
                        <PopButton state={state} onClick={() => toggleGroupPopout(g)} />
                        <DashboardSwitch
                          on={groupLayout[g.id]?.visible ?? false}
                          onClick={() => toggleGroupDashboard(g.id, i)}
                        />
                        <span className="flex-1" />
                        <StatusText state={state} />
                      </>
                    }
                  />
                )
              })}
              <button
                onClick={() => setShowGroupsModal(true)}
                className="flex min-h-[112px] flex-col items-center justify-center gap-1.5 rounded-lg p-3 text-xs"
                style={{
                  border: '1px dashed var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                }}
                title="Make a named Custom Timers window for raid or boss timers"
              >
                <Plus size={16} />
                New timer window
              </button>
            </div>
          </section>
        </div>
      </div>
      {/* Keyed by selection so switching drawers starts the new one at the top. */}
      {details && <React.Fragment key={JSON.stringify(selected)}>{details}</React.Fragment>}
      {showGroupsModal && (
        <TimerGroupsModal groups={groups} onChanged={reloadGroups} onClose={() => setShowGroupsModal(false)} />
      )}
    </div>
  )
}
