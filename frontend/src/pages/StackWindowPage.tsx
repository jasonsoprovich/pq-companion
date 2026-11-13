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
 */
import React, { Suspense, createRef, lazy, useCallback, useEffect, useMemo, useState } from 'react'
import { Layers } from 'lucide-react'
import { ErrorBoundary } from '../components/ErrorBoundary'
import OverlayLockButton from '../components/OverlayLockButton'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { OverlayStackContext, type OverlayStackValue } from '../hooks/overlayStackContext'
import { setStackPrefFallback, type OverlayName } from '../lib/overlays'
import { WSEvent } from '../lib/wsEvents'
import { getConfig } from '../services/api'
import type { StackInfo } from '../types/electron'

const PAGES: Record<string, React.LazyExoticComponent<React.ComponentType<{ groupId?: string }>>> = {
  dps: lazy(() => import('./DPSOverlayWindowPage')),
  hps: lazy(() => import('./HPSOverlayWindowPage')),
  buffTimer: lazy(() => import('./BuffTimerWindowPage')),
  filteredTimer: lazy(() => import('./FilteredTimerWindowPage')),
  killCounter: lazy(() => import('./KillCounterWindowPage')),
  pet: lazy(() => import('./PetWindowPage')),
  deathRecap: lazy(() => import('./DeathRecapWindowPage')),
  itemInfo: lazy(() => import('./ItemInfoWindowPage')),
  detrimTimer: lazy(() => import('./DetrimTimerWindowPage')),
  customTimer: lazy(() => import('./CustomTimerWindowPage')),
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
  hps: 'HPS',
  buffTimer: 'Buffs',
  filteredTimer: 'Filtered',
  killCounter: 'Kills',
  pet: 'Pet',
  deathRecap: 'Death',
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

const GROUP_PREFIX = 'customTimer:'

function baseKey(tab: string): string {
  return tab.startsWith(GROUP_PREFIX) ? 'customTimer' : tab
}

function tabLabel(tab: string, info: StackInfo): string {
  if (tab.startsWith(GROUP_PREFIX)) return info.groupNames[tab] || 'Timers'
  return TAB_LABELS[tab] ?? tab
}

const noop = (): void => {}

// Each tab's zoom from preferences.overlay_zoom_factors, kept current.
function useZoomFactors(): Record<string, number> {
  const [zooms, setZooms] = useState<Record<string, number>>({})
  const load = useCallback(() => {
    getConfig().then((c) => setZooms(c.preferences.overlay_zoom_factors ?? {})).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  useWebSocket(useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ConfigUpdated) load()
  }, [load]))
  return zooms
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
  const { collapsed, toggleCollapsed, headerRef } = useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const zooms = useZoomFactors()

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
      collapse: inertCollapse,
    }),
    [info.key, locked, mode, toggleLocked, rowsInteractive, opacity, chrome, inertCollapse],
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
                title={tabLabel(tab, info)}
                style={{
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
                {tabLabel(tab, info)}
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
                  <Page {...(tab.startsWith(GROUP_PREFIX) ? { groupId: tab.slice(GROUP_PREFIX.length) } : {})} />
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
