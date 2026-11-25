/**
 * OverlayLibrary — the Overlays hub's Library tab. Every overlay is a card in
 * its category: pop out / close (or show, when hidden), whether it also gets a
 * Dashboard panel, and its hotkey if one is bound. Clicking a card opens its
 * details on the right. Overlays that can't be used right now stay listed,
 * greyed, with the reason. Custom timer windows and stacks (several overlays
 * as tabs in one window) close the list under "Your windows". A stacked
 * overlay's Move and Reset act on its stack. Custom overlays (user-built
 * widget windows driven by log rules) are listed there too.
 *
 * Dashboard visibility is written straight to the dashboard's saved layout;
 * the Dashboard tab reads it fresh when it mounts.
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Bell, Eraser, ExternalLink, Eye, Hourglass, Keyboard, Layers, LayoutDashboard, LayoutTemplate, MessagesSquare, Move, Pause, Pencil, Play, Plus, RotateCcw, Settings2, SquareArrowOutUpRight, Trash2, X } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import {
  CUSTOM_GROUP_INFO,
  OVERLAY_CATALOG,
  OVERLAY_CATEGORIES,
  PANEL_POPOUT,
  TRIGGER_ALERTS_INFO,
  overlayWindowLabel,
  type OverlayCatalogEntry,
} from '../../lib/overlayCatalog'
import { useOverlayLayouts } from '../../hooks/useOverlayLayouts'
import type { OverlayStackSummary } from '../../types/electron'
import type { OverlayName } from '../../lib/overlays'
import {
  defaultGroupPanelLayout,
  updateDashboardGroups,
  updateDashboardPanels,
  useDashboardLayouts,
  type DashboardPanelKey,
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
import { IDLE_COLLAPSE_HINT, IdleCollapseSelect, OverlayWindowControls } from './settings/OverlayWindowSettings'
import { SettingsSection } from './settings/SettingsSection'
import { OVERLAY_SETTINGS } from './settings/OverlaySpecificSettings'
import { sameSelection, type HubSelection } from './selection'
import type { TimerGroup } from '../../types/trigger'
import type { CustomOverlay } from '../../types/customOverlay'
import { useCustomOverlays } from '../../hooks/useCustomOverlays'
import { customOverlayWindowKey } from '../../lib/customOverlay'
import { deleteCustomOverlay } from '../../services/api'
import { ConfirmModal } from '../ConfirmModal'
import {
  CUSTOM_OVERLAY_INFO,
  CustomOverlayRulesSection,
  ImportCustomOverlayModal,
  NewCustomOverlayModal,
  ResetContentsButton,
  ShareButtons,
} from './CustomOverlayHub'
import CustomOverlayEditor from './customOverlayEditor/CustomOverlayEditor'
import ChatWindowEditor from './chatWindowEditor/ChatWindowEditor'
import { CHAT_WINDOW_INFO, ChatShareButtons, ImportChatWindowsModal, NewChatWindowModal } from './ChatWindowHub'
import { useChatWindows } from '../../hooks/useChatWindows'
import { CHAT_WINDOW_COLOR, chatWindowBlurb, chatWindowKey } from '../../lib/chatWindow'
import { deleteChatWindow } from '../../services/api'
import { CHAT_WINDOWS_LOCK_ACTION } from '../../lib/hotkeys'
import type { ChatWindow } from '../../types/chatWindow'

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
            ? 'Hidden by a hotkey · click to show it'
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
  active,
}: {
  icon: LucideIcon
  label: string
  onClick: () => void
  title?: string
  active?: boolean
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      className="flex items-center gap-1 rounded px-2 py-1 text-xs"
      style={{
        backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
        color: active ? '#fff' : 'var(--color-foreground)',
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

const STACK_INFO = {
  blurb: 'Several overlays as tabs in one window.',
  details:
    'Drag an overlay by its title bar onto another one\u2019s title bar to stack them. ' +
    'Right-click a tab, or use the buttons below, to take one back out. The stack has its ' +
    'own position, lock, opacity and fade; each tab keeps its zoom and its own settings.',
}

const CUSTOM_OVERLAY_COLOR = '#a78bfa'

function customOverlayBlurb(o: CustomOverlay): string {
  if (!o.def.enabled) return 'Rules paused.'
  const w = o.def.widgets.length
  const r = o.def.rules.length
  return `${w} ${w === 1 ? 'widget' : 'widgets'} · ${r} ${r === 1 ? 'rule' : 'rules'}`
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
  // Windows in Move mode (a drag banner over the window, even a locked one).
  const [placing, setPlacing] = useState<string[]>([])
  const { panels: layout, groups: groupLayout } = useDashboardLayouts()
  const [bindings, setBindings] = useState<Record<string, string>>({})
  const [discordEnabled, setDiscordEnabled] = useState(false)
  const [groups, setGroups] = useState<TimerGroup[]>([])
  const [showGroupsModal, setShowGroupsModal] = useState(false)
  const customOverlays = useCustomOverlays()
  const [showNewCustom, setShowNewCustom] = useState(false)
  const [showImport, setShowImport] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<CustomOverlay | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const editing = customOverlays.overlays.find((o) => o.def.id === editingId)
  const coNames = useMemo(
    () => customOverlays.overlays.map((o) => ({ id: o.def.id, name: o.def.name })),
    [customOverlays.overlays],
  )
  const chatWindows = useChatWindows()
  const [showNewChat, setShowNewChat] = useState(false)
  const [showChatImport, setShowChatImport] = useState(false)
  const [confirmDeleteChat, setConfirmDeleteChat] = useState<ChatWindow | null>(null)
  const [chatEditing, setChatEditing] = useState<{ id: string; suggest?: { seq: number; mode: 'hide' | 'keep' } } | null>(null)
  const chatEditingWindow = chatWindows.windows.find((w) => w.def.id === chatEditing?.id)
  const setSelected = onSelect

  // Chat windows have no drawer: selecting one (a card click, or a deep
  // link from its popout's pencil or "lines like this") opens its editor,
  // and the selection is dropped.
  useEffect(() => {
    if (selected?.kind !== 'chat') return
    setChatEditing({ id: selected.id, suggest: selected.suggest })
    onSelect(null)
  }, [selected, onSelect])
  const { config, setConfig, saveState } = useConfigDraft()
  const { state: layoutState } = useOverlayLayouts()
  const stacks: OverlayStackSummary[] = useMemo(
    () => layoutState?.layouts.find((l) => l.id === layoutState.activeId)?.stacks ?? [],
    [layoutState],
  )
  const stackOf = (key: string): OverlayStackSummary | undefined => stacks.find((s) => s.tabs.includes(key))
  const tabName = (s: OverlayStackSummary, tab: string): string =>
    tab.startsWith('customTimer:')
      ? s.groupNames[tab] || overlayWindowLabel(tab, groups)
      : overlayWindowLabel(tab, groups, coNames)
  const stackTitle = (s: OverlayStackSummary): string => s.tabs.map((t) => tabName(s, t)).join(' + ')

  const refreshStates = useCallback(() => {
    window.electron?.overlay?.popoutWindowStates?.().then(setWindowStates).catch(() => {})
    window.electron?.overlay?.placingNames?.().then((n) => setPlacing(n ?? [])).catch(() => {})
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
      updateDashboardPanels((prev) => ({ ...prev, [panel]: { ...prev[panel], visible: !prev[panel].visible } }))
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
    updateDashboardGroups((prev) => {
      const cur = prev[id] ?? defaultGroupPanelLayout(index)
      return { ...prev, [id]: { ...cur, visible: !cur.visible } }
    })
  }, [])

  const toggleCustomPopout = useCallback(
    (o: CustomOverlay) => {
      window.electron?.overlay?.toggleCustomOverlay(o.def.id, o.def.name)
      afterToggle()
    },
    [afterToggle],
  )

  // Custom overlays share the dashboard's group panel map under their window
  // key; place them after the timer windows when first shown.
  const toggleCustomDashboard = useCallback((o: CustomOverlay, index: number) => {
    const key = customOverlayWindowKey(o.def.id)
    updateDashboardGroups((prev) => {
      const cur = prev[key] ?? defaultGroupPanelLayout(index)
      return { ...prev, [key]: { ...cur, visible: !cur.visible } }
    })
  }, [])

  const doDelete = (o: CustomOverlay): void => {
    window.electron?.overlay?.closeCustomOverlay(o.def.id)
    deleteCustomOverlay(o.def.id)
      .then(() => {
        updateDashboardGroups((prev) => {
          const next = { ...prev }
          delete next[customOverlayWindowKey(o.def.id)]
          return next
        })
        if (selected?.kind === 'custom' && selected.id === o.def.id) setSelected(null)
      })
      .catch(() => {})
    setConfirmDelete(null)
  }

  const openHotkeys = useCallback(() => navigate('/settings?tab=hotkeys'), [navigate])

  const toggleChatPopout = useCallback(
    (w: ChatWindow) => {
      window.electron?.overlay?.toggleChatWindow(w.def.id, w.def.name)
      afterToggle()
    },
    [afterToggle],
  )

  const doDeleteChat = (w: ChatWindow): void => {
    window.electron?.overlay?.closeChatWindow(w.def.id)
    deleteChatWindow(w.def.id).catch(() => {})
    setConfirmDeleteChat(null)
    setChatEditing(null)
  }

  const byCategory = useMemo(
    () => OVERLAY_CATEGORIES.map((c) => ({ ...c, entries: OVERLAY_CATALOG.filter((e) => e.category === c.id) })),
    [],
  )

  const resetPosition = (name: string): void => {
    window.electron?.overlay?.resetPosition?.(name)
  }

  // Move mode needs an open window to draw its banner on, so a closed one
  // pops out first.
  const toggleMove = (name: string, openWindow: () => void): void => {
    const on = !placing.includes(name)
    if (on && !windowStates[name]) openWindow()
    window.electron?.overlay?.place?.(name, on)
    setPlacing((p) => (on ? [...p, name] : p.filter((n) => n !== name)))
    afterToggle()
  }
  const moveButton = (name: string, label: string, openWindow: () => void): React.ReactNode => {
    if (typeof window.electron?.overlay?.place !== 'function') return null
    const on = placing.includes(name)
    const stacked = !name.startsWith('stack:') && !!stackOf(name)
    return (
      <SecondaryButton
        icon={Move}
        label={on ? 'Done moving' : stacked ? 'Move stack' : 'Move'}
        active={on}
        onClick={() => toggleMove(name, openWindow)}
        title={
          on
            ? `Done positioning ${label}`
            : stacked
              ? `Drag ${label}'s stack anywhere, even when it's locked`
              : `Pop out ${label} and drag it anywhere, even when it's locked`
        }
      />
    )
  }
  const resetButton = (name: string, label: string): React.ReactNode => {
    const stacked = !name.startsWith('stack:') && !!stackOf(name)
    return (
      <SecondaryButton
        icon={RotateCcw}
        label={stacked ? 'Reset stack position' : 'Reset position'}
        onClick={() => resetPosition(name)}
        title={`Move ${stacked ? `${label}'s stack` : 'this window'} to the middle of your main screen at its default size, and unlock it`}
      />
    )
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
              {!unavailable && moveButton(entry.name, entry.label, () => togglePopout(entry))}
              {resetButton(entry.name, entry.label)}
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
              {moveButton(key, g.name, () => toggleGroupPopout(g))}
              {resetButton(key, g.name)}
              <SecondaryButton icon={Hourglass} label="Manage timer windows" onClick={() => setShowGroupsModal(true)} />
            </>
          }
        >
          {config && (
            <SettingsSection title="Window">
              <div className="flex items-center justify-between gap-3 py-1.5">
                <div className="min-w-0">
                  <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>Collapse when idle</p>
                  <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{IDLE_COLLAPSE_HINT}</p>
                </div>
                <div className="shrink-0 text-xs">
                  <IdleCollapseSelect windowKey={key} label={g.name} config={config} setConfig={setConfig} />
                </div>
              </div>
            </SettingsSection>
          )}
        </Drawer>
      )
    }
  }

  if (selected?.kind === 'custom') {
    const o = customOverlays.overlays.find((x) => x.def.id === selected.id)
    if (o) {
      const key = customOverlayWindowKey(o.def.id)
      const state = windowStates[key]
      details = (
        <Drawer
          icon={LayoutTemplate}
          color={CUSTOM_OVERLAY_COLOR}
          title={o.def.name}
          details={CUSTOM_OVERLAY_INFO.details}
          status={<StatusText state={state} />}
          saveState={saveState}
          onClose={close}
          actions={
            <>
              <PopButton state={state} onClick={() => toggleCustomPopout(o)} />
              {moveButton(key, o.def.name, () => toggleCustomPopout(o))}
              {resetButton(key, o.def.name)}
              <SecondaryButton icon={Pencil} label="Edit overlay" onClick={() => setEditingId(o.def.id)} />
              <ResetContentsButton onReset={() => customOverlays.act(o.def.id, { action: 'reset' })} />
              <SecondaryButton
                icon={Trash2}
                label="Delete"
                onClick={() => setConfirmDelete(o)}
                title="Delete this overlay and its contents"
              />
            </>
          }
        >
          <SettingsSection title="Rules">
            <CustomOverlayRulesSection overlay={o} />
          </SettingsSection>
          <SettingsSection title="Share">
            <p className="pb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Share this overlay’s widgets and rules with your guild. Its contents and character
              filter stay here.
            </p>
            <ShareButtons overlay={o} />
          </SettingsSection>
          {config && (
            <SettingsSection title="Window">
              <p className="pb-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Lock, opacity, fade and zoom are shared by every custom overlay.
              </p>
              <OverlayWindowControls
                name="customOverlay"
                windowKey={key}
                label={o.def.name}
                config={config}
                setConfig={setConfig}
              />
            </SettingsSection>
          )}
        </Drawer>
      )
    }
  }

  if (selected?.kind === 'stack') {
    const s = stacks.find((x) => x.key === selected.key)
    if (s) {
      const state = windowStates[s.key]
      const title = stackTitle(s)
      details = (
        <Drawer
          icon={Layers}
          color="#c9a84c"
          title={title}
          details={STACK_INFO.details}
          status={<StatusText state={state} />}
          saveState={saveState}
          onClose={close}
          actions={
            <>
              <PopButton state={state} onClick={() => { window.electron?.overlay?.stackToggle(s.key); afterToggle() }} />
              {moveButton(s.key, 'the stack', () => window.electron?.overlay?.stackToggle(s.key))}
              {resetButton(s.key, 'the stack')}
            </>
          }
        >
          <SettingsSection title="Tabs">
            <div className="flex flex-col">
              {s.tabs.map((tab) => (
                <div key={tab} className="flex items-center justify-between gap-3 py-1.5">
                  <span className="truncate text-sm" style={{ color: 'var(--color-foreground)' }}>{tabName(s, tab)}</span>
                  <span className="flex shrink-0 gap-1.5">
                    <SecondaryButton
                      icon={SquareArrowOutUpRight}
                      label="Unstack"
                      onClick={() => { window.electron?.overlay?.stackRemove(tab, true); afterToggle() }}
                      title={`Move ${tabName(s, tab)} back into its own window`}
                    />
                    <SecondaryButton
                      icon={X}
                      label="Close"
                      onClick={() => { window.electron?.overlay?.stackRemove(tab, false); afterToggle() }}
                      title={`Close ${tabName(s, tab)} and take it out of the stack`}
                    />
                  </span>
                </div>
              ))}
            </div>
          </SettingsSection>
          {config && (
            <SettingsSection title="Window">
              <OverlayWindowControls name={s.key as OverlayName} label="stack" config={config} setConfig={setConfig} />
            </SettingsSection>
          )}
        </Drawer>
      )
    }
  }

  const isSel = (s: Selection): boolean => sameSelection(selected, s)

  // The chat editor's Window tab: what the other cards keep in their
  // drawer. These act at once, outside the editor's Save.
  const chatWindowTab = (w: ChatWindow): React.ReactNode => {
    const key = chatWindowKey(w.def.id)
    const state = windowStates[key]
    return (
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <PopButton state={state} onClick={() => toggleChatPopout(w)} />
          {moveButton(key, w.def.name, () => toggleChatPopout(w))}
          {resetButton(key, w.def.name)}
          <SecondaryButton
            icon={w.def.enabled ? Pause : Play}
            label={w.def.enabled ? 'Pause' : 'Resume'}
            onClick={() => chatWindows.act(w.def.id, { action: 'enable', enabled: !w.def.enabled })}
            title={w.def.enabled ? 'Stop adding new lines to this window' : 'Start adding new lines again'}
          />
          <SecondaryButton
            icon={Eraser}
            label="Clear"
            onClick={() => chatWindows.act(w.def.id, { action: 'clear' })}
            title="Empty this window’s lines and pins"
          />
          <SecondaryButton
            icon={Keyboard}
            label={bindings[CHAT_WINDOWS_LOCK_ACTION] ? 'Change lock hotkey' : 'Set a lock hotkey'}
            onClick={openHotkeys}
            title="One hotkey locks or unlocks every chat window"
          />
          <SecondaryButton icon={Trash2} label="Delete" onClick={() => setConfirmDeleteChat(w)} />
          <span className="flex-1" />
          <StatusText state={state} />
        </div>
        <SettingsSection title="Share">
          <p className="pb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Share this window’s channels, rules and looks, or move them to another PC. Its lines and
            character filter stay here. Unsaved edits aren’t included.
          </p>
          <ChatShareButtons ids={[w.def.id]} name={w.def.name} />
          {chatWindows.windows.length > 1 && (
            <div className="pt-2">
              <p className="pb-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                All {chatWindows.windows.length} chat windows as one file:
              </p>
              <ChatShareButtons ids={[]} name="all" />
            </div>
          )}
        </SettingsSection>
        {config && (
          <SettingsSection title="Window">
            <p className="pb-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Lock and fade are shared by every chat window. Font size and background are on each
              window (Appearance tab, or Ctrl+scroll over it).
              {saveState === 'saving' ? ' Saving…' : saveState === 'error' ? ' Couldn’t save.' : ''}
            </p>
            <OverlayWindowControls
              name="chatWindow"
              windowKey={key}
              label={w.def.name}
              config={config}
              setConfig={setConfig}
              hideOpacity
            />
          </SettingsSection>
        )}
      </div>
    )
  }

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
              {customOverlays.overlays.map((o, i) => {
                const key = customOverlayWindowKey(o.def.id)
                const state = windowStates[key]
                const sel: Selection = { kind: 'custom', id: o.def.id }
                return (
                  <Card
                    key={key}
                    icon={LayoutTemplate}
                    color={CUSTOM_OVERLAY_COLOR}
                    title={o.def.name}
                    blurb={customOverlayBlurb(o)}
                    selected={isSel(sel)}
                    onSelect={() => setSelected(sel)}
                    footer={
                      <>
                        <PopButton state={state} onClick={() => toggleCustomPopout(o)} />
                        <DashboardSwitch
                          on={groupLayout[key]?.visible ?? false}
                          onClick={() => toggleCustomDashboard(o, groups.length + i)}
                        />
                        <span className="flex-1" />
                        <StatusText state={state} />
                      </>
                    }
                  />
                )
              })}
              {chatWindows.windows.map((w) => {
                const key = chatWindowKey(w.def.id)
                const state = windowStates[key]
                return (
                  <Card
                    key={key}
                    icon={MessagesSquare}
                    color={CHAT_WINDOW_COLOR}
                    title={w.def.name}
                    blurb={chatWindowBlurb(w)}
                    selected={chatEditing?.id === w.def.id}
                    onSelect={() => setChatEditing({ id: w.def.id })}
                    footer={
                      <>
                        <PopButton state={state} onClick={() => toggleChatPopout(w)} />
                        <span className="flex-1" />
                        <StatusText state={state} />
                      </>
                    }
                  />
                )
              })}
              {stacks.map((s) => {
                const state = windowStates[s.key]
                const sel: Selection = { kind: 'stack', key: s.key }
                return (
                  <Card
                    key={s.key}
                    icon={Layers}
                    color="#c9a84c"
                    title={stackTitle(s)}
                    blurb={STACK_INFO.blurb}
                    selected={isSel(sel)}
                    onSelect={() => setSelected(sel)}
                    footer={
                      <>
                        <PopButton state={state} onClick={() => { window.electron?.overlay?.stackToggle(s.key); afterToggle() }} />
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
              <button
                onClick={() => setShowNewCustom(true)}
                className="flex min-h-[112px] flex-col items-center justify-center gap-1.5 rounded-lg p-3 text-xs"
                style={{
                  border: '1px dashed var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                }}
                title={CUSTOM_OVERLAY_INFO.blurb}
              >
                <Plus size={16} />
                New custom overlay
              </button>
              <button
                onClick={() => setShowNewChat(true)}
                className="flex min-h-[112px] flex-col items-center justify-center gap-1.5 rounded-lg p-3 text-xs"
                style={{
                  border: '1px dashed var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                }}
                title={CHAT_WINDOW_INFO.blurb}
              >
                <Plus size={16} />
                New chat window
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
      {showNewCustom && (
        <NewCustomOverlayModal
          onClose={() => setShowNewCustom(false)}
          onImport={() => { setShowNewCustom(false); setShowImport(true) }}
          onCreated={(o) => {
            setShowNewCustom(false)
            setSelected({ kind: 'custom', id: o.def.id })
            setEditingId(o.def.id)
          }}
        />
      )}
      {showImport && (
        <ImportCustomOverlayModal
          existingNames={customOverlays.overlays.map((o) => o.def.name)}
          onClose={() => setShowImport(false)}
          onImported={(o) => {
            setShowImport(false)
            setSelected({ kind: 'custom', id: o.def.id })
          }}
        />
      )}
      {editing && (
        <CustomOverlayEditor
          key={editing.def.id}
          overlay={editing}
          skewMs={customOverlays.skewMs}
          onClose={() => setEditingId(null)}
        />
      )}
      {showNewChat && (
        <NewChatWindowModal
          onClose={() => setShowNewChat(false)}
          onImport={() => { setShowNewChat(false); setShowChatImport(true) }}
          onCreated={(def) => {
            setShowNewChat(false)
            setChatEditing({ id: def.id })
            window.electron?.overlay?.openChatWindow(def.id, def.name)
            afterToggle()
          }}
        />
      )}
      {showChatImport && (
        <ImportChatWindowsModal
          onClose={() => setShowChatImport(false)}
          onImported={(defs) => {
            setShowChatImport(false)
            if (defs[0]?.id) setChatEditing({ id: defs[0].id })
          }}
        />
      )}
      {chatEditing && chatEditingWindow && (
        <ChatWindowEditor
          key={chatEditing.id}
          window={chatEditingWindow}
          nowMs={Date.now() + chatWindows.skewMs}
          suggest={chatEditing.suggest}
          windowTab={chatWindowTab(chatEditingWindow)}
          onClose={() => setChatEditing(null)}
        />
      )}
      {confirmDeleteChat && (
        <ConfirmModal
          title="Delete chat window?"
          message={`“${confirmDeleteChat.def.name}” will be deleted. This can’t be undone.`}
          confirmLabel="Delete"
          tone="danger"
          onConfirm={() => doDeleteChat(confirmDeleteChat)}
          onCancel={() => setConfirmDeleteChat(null)}
        />
      )}
      {confirmDelete && (
        <ConfirmModal
          title="Delete custom overlay?"
          message={`“${confirmDelete.def.name}” and everything in it will be deleted. This can’t be undone.`}
          confirmLabel="Delete"
          tone="danger"
          onConfirm={() => doDelete(confirmDelete)}
          onCancel={() => setConfirmDelete(null)}
        />
      )}
    </div>
  )
}
