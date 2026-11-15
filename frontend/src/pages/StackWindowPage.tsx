/**
 * StackWindowPage — several overlays as tabs in one popout window (route
 * stack-window?id=<id>). Main keeps the stack in the active layout and tells
 * this page its tabs and which one is showing.
 *
 * The stack owns the window: one title bar with the tabs, its own lock,
 * opacity, fade and collapse (its settings fall back to the first tab's until
 * set for the stack). Each tab is the overlay's normal page rendered inside
 * OverlayStackContext, which hides the page's title bar and points its window
 * hooks at the stack. Every tab stays mounted, hidden when not showing, so
 * its data keeps flowing; each keeps its own zoom.
 *
 * Tabs report whether they have something to show. A tab that isn't showing
 * gets a dot when its activity starts, until it's picked. The stack folds
 * once every tab is idle (tabs with no idle state never count as idle),
 * after its own idle delay, or by default the longest delay set on its tabs.
 * A tab's right-click menu unstacks it into its own window or closes it.
 *
 * A new stack starts with its first tab's lock mode, opacity and fade, copied
 * once so the hub shows them as the stack's own.
 */
import React, { Suspense, createRef, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Layers } from 'lucide-react'
import { ErrorBoundary } from '../components/ErrorBoundary'
import OverlayLockButton from '../components/OverlayLockButton'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { useIdleCollapse } from '../hooks/useIdleCollapse'
import { idleCollapseSecs, supportsIdleCollapse } from '../lib/idleCollapse'
import type { Config } from '../types/config'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { OverlayStackContext, type OverlayStackValue } from '../hooks/overlayStackContext'
import { setStackPrefFallback, type OverlayName } from '../lib/overlays'
import { WSEvent } from '../lib/wsEvents'
import { getConfig, saveConfigChanges } from '../services/api'
import type { StackInfo } from '../types/electron'
import { instanceId, instanceKind, isInstanceKey } from '../lib/instanceKeys'

const PAGES: Record<string, React.LazyExoticComponent<React.ComponentType<{ groupId?: string }>>> = {
  dps: lazy(() => import('./DPSOverlayWindowPage')),
  buffTimer: lazy(() => import('./BuffTimerWindowPage')),
  filteredTimer: lazy(() => import('./FilteredTimerWindowPage')),
  killCounter: lazy(() => import('./KillCounterWindowPage')),
  pet: lazy(() => import('./PetWindowPage')),
  deathRecap: lazy(() => import('./DeathRecapWindowPage')),
  gearUpgrades: lazy(() => import('./GearUpgradesWindowPage')),
  itemInfo: lazy(() => import('./ItemInfoWindowPage')),
  detrimTimer: lazy(() => import('./DetrimTimerWindowPage')),
  customTimer: lazy(() => import('./CustomTimerWindowPage')),
  customOverlay: lazy(() => import('./CustomOverlayWindowPage')),
  npc: lazy(() => import('./NPCOverlayWindowPage')),
  threat: lazy(() => import('./ThreatOverlayWindowPage')),
  rollTracker: lazy(() => import('./RollTrackerWindowPage')),
  respawnTimer: lazy(() => import('./RespawnTimerWindowPage')),
  zoneLockouts: lazy(() => import('./ZoneLockoutsWindowPage')),
  raidReadiness: lazy(() => import('./RaidReadinessWindowPage')),
  liveMap: lazy(() => import('./LiveMapWindowPage')),
  chChain: lazy(() => import('./CHChainOverlayWindowPage')),
  chMetronome: lazy(() => import('./CHMetronomeOverlayWindowPage')),
}

// Short tab names, so a few tabs fit in the title bar.
const TAB_LABELS: Record<string, string> = {
  dps: 'DPS',
  buffTimer: 'Buffs',
  filteredTimer: 'Filtered',
  killCounter: 'Kills',
  pet: 'Pet',
  deathRecap: 'Death',
  gearUpgrades: 'Upgrades',
  itemInfo: 'Item',
  detrimTimer: 'Detrims',
  customTimer: 'Timers',
  npc: 'NPC',
  threat: 'Threat',
  rollTracker: 'Rolls',
  respawnTimer: 'Respawns',
  zoneLockouts: 'Lockouts',
  raidReadiness: 'Raid',
  liveMap: 'Map',
  chChain: 'CH Chain',
  chMetronome: 'Metronome',
}

// Instance tabs (`customTimer:<id>`, `customOverlay:<id>`) render their
// kind's page with the id as its groupId prop.
function baseKey(tab: string): string {
  return instanceKind(tab) ?? tab
}

function tabLabel(tab: string, info: StackInfo): string {
  const kind = instanceKind(tab)
  if (kind) return info.groupNames[tab] || (kind === 'customTimer' ? 'Timers' : 'Overlay')
  return TAB_LABELS[tab] ?? tab
}

const noop = (): void => {}

// Copies the first tab's window settings to a stack that has none yet.
async function seedStackSettings(stackKey: string, firstTab: string): Promise<void> {
  const base = await getConfig()
  const p = base.preferences
  const maps = ['overlay_locked_modes', 'overlay_opacities', 'overlay_fade_modes'] as const
  const next = { ...p }
  let changed = false
  for (const m of maps) {
    const cur = (p[m] ?? {}) as Record<string, unknown>
    if (cur[stackKey] !== undefined || cur[firstTab] === undefined) continue
    ;(next as Record<string, unknown>)[m] = { ...cur, [stackKey]: cur[firstTab] }
    changed = true
  }
  if (changed) await saveConfigChanges({ ...base, preferences: next }, base)
}

// The app config (tab zooms, idle collapse delays), kept current.
function useConfig(): Config | null {
  const [config, setConfig] = useState<Config | null>(null)
  const load = useCallback(() => {
    getConfig().then(setConfig).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  useWebSocket(useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ConfigUpdated) load()
  }, [load]))
  return config
}

type TabMenu = { tab: string; x: number; y: number }

const MENU_WIDTH = 170

function TabMenuPopup({
  menu,
  label,
  onClose,
}: {
  menu: TabMenu
  label: string
  onClose: () => void
}): React.ReactElement {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const onDown = (e: MouseEvent): void => {
      if (!ref.current?.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    window.addEventListener('blur', onClose)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('blur', onClose)
    }
  }, [onClose])
  const run = (popOut: boolean): void => {
    onClose()
    window.electron?.overlay?.stackRemove(menu.tab, popOut)
  }
  const item: React.CSSProperties = {
    display: 'block',
    width: '100%',
    textAlign: 'left',
    padding: '5px 10px',
    fontSize: 11,
    background: 'none',
    border: 'none',
    color: 'rgba(255,255,255,0.85)',
    cursor: 'pointer',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  }
  return (
    <div
      ref={ref}
      className="no-drag"
      role="menu"
      style={{
        position: 'fixed',
        top: Math.min(menu.y, window.innerHeight - 64),
        left: Math.max(4, Math.min(menu.x, window.innerWidth - MENU_WIDTH - 4)),
        width: MENU_WIDTH,
        zIndex: 1000,
        padding: 3,
        borderRadius: 6,
        backgroundColor: 'rgba(16,16,20,0.97)',
        border: '1px solid rgba(255,255,255,0.15)',
        boxShadow: '0 6px 18px rgba(0,0,0,0.5)',
      }}
    >
      <button role="menuitem" style={item} onClick={() => run(true)} title={`Move ${label} back into its own window`}>
        Unstack {label}
      </button>
      <button role="menuitem" style={item} onClick={() => run(false)} title={`Close ${label} and take it out of the stack`}>
        Close {label}
      </button>
    </div>
  )
}

function StackWindow({ info }: { info: StackInfo }): React.ReactElement {
  // Settings fall back to the first tab's until set for the stack. Registered
  // before the hooks below first read them.
  setStackPrefFallback(info.key, info.tabs[0] ?? '')
  const key = info.key as OverlayName
  const opacity = useOverlayOpacity(key)
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock(key)
  const chromeFade = useOverlayChromeFade(key, mode === 'display-only')
  const { collapsed, toggleCollapsed, setCollapsed, headerRef, ready: collapseReady } =
    useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const config = useConfig()
  const zooms = config?.preferences.overlay_zoom_factors ?? {}
  const [menu, setMenu] = useState<TabMenu | null>(null)
  const closeMenu = useCallback(() => setMenu(null), [])

  // Tab activity, and dots on tabs whose activity started while hidden.
  const activeTabRef = useRef(info.active)
  activeTabRef.current = info.active
  const activityRef = useRef<Record<string, boolean>>({})
  const [activity, setActivity] = useState<Record<string, boolean>>({})
  const [dots, setDots] = useState<Record<string, boolean>>({})
  const reportActive = useCallback((tab: string, active: boolean) => {
    const was = activityRef.current[tab]
    if (was === active) return
    activityRef.current = { ...activityRef.current, [tab]: active }
    setActivity(activityRef.current)
    // A tab's first report is its state on load, not new activity.
    if (active && was !== undefined && tab !== activeTabRef.current) {
      setDots((d) => (d[tab] ? d : { ...d, [tab]: true }))
    }
  }, [])
  useEffect(() => {
    setDots((d) => {
      if (!d[info.active]) return d
      const next = { ...d }
      delete next[info.active]
      return next
    })
  }, [info.active])

  const firstTab = info.tabs[0] ?? ''
  useEffect(() => {
    if (firstTab) seedStackSettings(info.key, firstTab).catch(() => {})
  }, [info.key, firstTab])

  // The stack folds once every tab is idle: after its own delay when set
  // (-1 = off), else the longest one set on its tabs.
  const own = config?.preferences.overlay_idle_collapse?.[info.key]
  const tabDelays = info.tabs.map((t) => idleCollapseSecs(config, t)).filter((v) => v > 0)
  const delay = own !== undefined ? Math.max(own, 0) : tabDelays.length > 0 ? Math.max(...tabDelays) : 0
  useIdleCollapse({
    key: info.key,
    active: info.tabs.some((t) => (supportsIdleCollapse(t) ? !!activity[t] : true)),
    collapsed,
    ready: collapseReady && config !== null,
    setCollapsed,
    allowed: mode !== 'display-only',
    delaySecs: delay,
  })

  const inertCollapse = useMemo<OverlayStackValue['collapse']>(
    () => ({ collapsed: false, toggleCollapsed: noop, setCollapsed: noop, headerRef: createRef(), ready: false }),
    [],
  )
  const ctx = useMemo<OverlayStackValue>(
    () => ({
      stackKey: info.key,
      lock: { locked, mode, toggleLocked, rowsInteractive },
      opacity,
      chrome,
      reportActive,
      collapse: inertCollapse,
    }),
    [info.key, locked, mode, toggleLocked, rowsInteractive, opacity, chrome, reportActive, inertCollapse],
  )

  const select = (tab: string): void => {
    if (tab !== info.active) window.electron?.overlay?.stackSelect(tab)
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
      <div
        ref={headerRef}
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: '3px 8px 3px 6px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <Layers size={11} style={{ color: '#c9a84c', flexShrink: 0 }} />
        <div
          className="no-drag"
          role="tablist"
          style={{ display: 'flex', alignItems: 'center', gap: 2, minWidth: 0, flex: 1, overflow: 'hidden' }}
        >
          {info.tabs.map((tab) => {
            const on = tab === info.active
            return (
              <button
                key={tab}
                role="tab"
                aria-selected={on}
                onClick={() => select(tab)}
                onContextMenu={(e) => {
                  e.preventDefault()
                  setMenu({ tab, x: e.clientX, y: e.clientY })
                }}
                title={`${tabLabel(tab, info)}${dots[tab] ? ' · new activity' : ''} — right-click to unstack`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 4,
                  flexShrink: 1,
                  minWidth: 0,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  padding: '2px 6px',
                  fontSize: 11,
                  fontWeight: on ? 700 : 500,
                  color: on ? 'rgba(255,255,255,0.9)' : 'rgba(255,255,255,0.45)',
                  background: 'none',
                  border: 'none',
                  borderBottom: `2px solid ${on ? '#c9a84c' : 'transparent'}`,
                  cursor: 'pointer',
                }}
              >
                <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{tabLabel(tab, info)}</span>
                {dots[tab] && (
                  <span
                    aria-label="new activity"
                    style={{ width: 5, height: 5, borderRadius: '50%', backgroundColor: '#c9a84c', flexShrink: 0 }}
                  />
                )}
              </button>
            )
          })}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} />
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.stackClose()}
            style={{
              display: 'flex',
              alignItems: 'center',
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
              lineHeight: 1,
              fontSize: 11,
            }}
            title="Close this stack (all its tabs)"
          >
            ×
          </button>
        </div>
      </div>

      {menu && <TabMenuPopup menu={menu} label={tabLabel(menu.tab, info)} onClose={closeMenu} />}

      <OverlayStackContext.Provider value={ctx}>
        {info.tabs.map((tab) => {
          const Page = PAGES[baseKey(tab)]
          if (!Page) return null
          const zoom = zooms[tab] && zooms[tab] > 0 ? zooms[tab] : 1
          return (
            <div
              key={tab}
              className="overlay-stack-tab"
              style={{
                display: tab === info.active ? 'block' : 'none',
                flex: '1 1 0',
                minHeight: 0,
                overflow: 'hidden',
                ...(Math.abs(zoom - 1) > 0.001 ? { zoom } : {}),
              }}
            >
              <ErrorBoundary label={`${tabLabel(tab, info)} tab`}>
                <Suspense fallback={null}>
                  <Page {...(isInstanceKey(tab) ? { groupId: instanceId(tab) } : {})} />
                </Suspense>
              </ErrorBoundary>
            </div>
          )
        })}
      </OverlayStackContext.Provider>
    </div>
  )
}

export default function StackWindowPage(): React.ReactElement {
  const [info, setInfo] = useState<StackInfo | null>(null)
  useEffect(() => {
    let cancelled = false
    window.electron?.overlay?.stackGet?.()
      .then((v) => { if (!cancelled && v) setInfo(v) })
      .catch(() => {})
    const off = window.electron?.overlay?.onStackChanged?.((v) => { if (!cancelled) setInfo(v) })
    return () => { cancelled = true; off?.() }
  }, [])
  if (!info) return <></>
  return <StackWindow key={info.key} info={info} />
}
