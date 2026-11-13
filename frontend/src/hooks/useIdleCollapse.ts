/**
 * useIdleCollapse — folds a popout to its title bar once it has had nothing
 * to show for its configured delay, and opens it again when it has.
 *
 * The page says whether it's `active` (live timers, a target, a fight...).
 * Your own chevron wins until the next activity:
 * - a window you collapsed yourself stays collapsed when activity starts;
 * - a window you expanded while idle stays open until it has been active
 *   and gone idle again.
 * Whether the current collapse was ours is kept in localStorage, so it
 * survives the window being reopened or reloaded.
 *
 * `allowed` is false where collapsing is off (display-only lock mode).
 * `delaySecs` overrides the configured delay (a stack works out its own).
 *
 * Inside a stack the page only reports `active` to the stack, which
 * collapses as a whole when every tab is idle.
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { idleCollapseSecs } from '../lib/idleCollapse'
import { useOverlayStack } from './overlayStackContext'

const AUTO_PREFIX = 'pq-idle-collapsed:'

function readAuto(key: string): boolean {
  try { return localStorage.getItem(AUTO_PREFIX + key) === '1' } catch { return false }
}

function writeAuto(key: string, on: boolean): void {
  try {
    if (on) localStorage.setItem(AUTO_PREFIX + key, '1')
    else localStorage.removeItem(AUTO_PREFIX + key)
  } catch {
    // storage unavailable: the flag just doesn't survive a reload
  }
}

// The configured delay for one window, in seconds (0 = off), kept current.
function useIdleDelay(key: string): number {
  const [secs, setSecs] = useState(0)
  const load = useCallback(() => {
    getConfig().then((c) => setSecs(idleCollapseSecs(c, key))).catch(() => {})
  }, [key])
  useEffect(() => { load() }, [load])
  useWebSocket(
    useCallback((msg: WsMessage) => {
      if (msg.type === WSEvent.ConfigUpdated) load()
    }, [load]),
  )
  return secs
}

export function useIdleCollapse({
  key,
  active,
  collapsed,
  ready,
  setCollapsed,
  allowed = true,
  delaySecs: delayOverride,
}: {
  key: string
  active: boolean
  collapsed: boolean
  // The stored collapse state has loaded (see useOverlayCollapse).
  ready: boolean
  setCollapsed: (collapse: boolean) => void
  allowed?: boolean
  delaySecs?: number
}): void {
  const configured = useIdleDelay(key)
  const delaySecs = delayOverride ?? configured
  const stack = useOverlayStack()
  const reportActive = stack?.reportActive
  useEffect(() => {
    reportActive?.(key, active)
  }, [reportActive, key, active])
  const on = allowed && delaySecs > 0 && !!key
  // The current collapse was ours.
  const autoRef = useRef(readAuto(key))
  // Allowed to collapse when idle. Off after you expand an idle window,
  // back on with the next activity.
  const armedRef = useRef(true)
  // A change we asked for, so the change effect doesn't take it for yours.
  const expectRef = useRef<boolean | null>(null)
  const prevRef = useRef<boolean | null>(null)
  const activeRef = useRef(active)
  activeRef.current = active

  const setAuto = useCallback((v: boolean) => {
    autoRef.current = v
    writeAuto(key, v)
  }, [key])

  const request = useCallback((v: boolean) => {
    expectRef.current = v
    setCollapsed(v)
  }, [setCollapsed])

  // Tell our own collapses from yours (or a layout switch).
  useEffect(() => {
    if (!ready) return
    if (prevRef.current === null) {
      prevRef.current = collapsed
      // Expanded by other means while this window was closed.
      if (!collapsed) setAuto(false)
      return
    }
    if (prevRef.current === collapsed) return
    prevRef.current = collapsed
    if (expectRef.current === collapsed) {
      expectRef.current = null
      return
    }
    expectRef.current = null
    setAuto(false)
    if (!collapsed && !activeRef.current) armedRef.current = false
  }, [ready, collapsed, setAuto])

  useEffect(() => {
    if (!ready) return
    if (active || !on) {
      if (active) armedRef.current = true
      // Activity, or the setting was turned off: undo our own collapse.
      if (collapsed && autoRef.current) {
        setAuto(false)
        request(false)
      }
      return
    }
    if (collapsed || !armedRef.current) return
    const t = setTimeout(() => {
      if (activeRef.current) return
      setAuto(true)
      request(true)
    }, delaySecs * 1000)
    return () => clearTimeout(t)
  }, [ready, active, on, collapsed, delaySecs, request, setAuto])
}
