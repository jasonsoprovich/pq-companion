/**
 * useFitHeight — shrinks a popout's window to its rows, never past the
 * height the user set (Settings → the overlay → Fit height to rows).
 *
 * The page hands over its title bar and a wrapper around its rows whose
 * height is the rows' own (not stretched to the window). Whenever either
 * changes size, the wanted height goes to the main process as a fraction of
 * the window, like collapse, so CSS zoom and display scaling cancel out.
 * The main process keeps the cap: the height you dragged the window to.
 *
 * Off while collapsed (collapse owns the height then) and inside a stack
 * (the stack window holds several pages).
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import type React from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { fitHeightOn } from '../lib/fitHeight'
import { useOverlayStack } from './overlayStackContext'

// Height kept for the "No active ..." placeholder while there are no rows.
const EMPTY_BODY_PX = 64

// The setting for one window, or null until the config has loaded, and a
// count of config loads: the overlay's zoom lives in config too, and a zoom
// change resizes everything without the rows' layout size changing.
function useFitSetting(key: string): [boolean | null, number] {
  const [on, setOn] = useState<boolean | null>(null)
  const [loads, setLoads] = useState(0)
  const load = useCallback(() => {
    getConfig()
      .then((c) => {
        setOn(fitHeightOn(c, key))
        setLoads((n) => n + 1)
      })
      .catch(() => {})
  }, [key])
  useEffect(() => { load() }, [load])
  useWebSocket(
    useCallback((msg: WsMessage) => {
      if (msg.type === WSEvent.ConfigUpdated) load()
    }, [load]),
  )
  return [on, loads]
}

export function useFitHeight({
  key,
  collapsed,
  collapseReady,
  rows,
  headerRef,
  contentRef,
}: {
  key: string
  collapsed: boolean
  // The stored collapse state has loaded: until then a window reopened
  // collapsed reads as expanded, and its title-bar height would become
  // the cap.
  collapseReady: boolean
  rows: number
  headerRef: React.RefObject<HTMLElement>
  contentRef: React.RefObject<HTMLElement>
}): void {
  const stack = useOverlayStack()
  const [setting, configLoads] = useFitSetting(key)
  const on = setting === true && !stack && collapseReady
  const rowsRef = useRef(rows)
  rowsRef.current = rows

  // Measured a frame later, so a zoom or row change has been laid out.
  // Header and rows both by bounding rect, the space collapse measures in.
  // One frame pending at most, and it rechecks: turned off, collapsed or
  // unmounted in the meantime, it sends nothing.
  const eligibleRef = useRef(false)
  eligibleRef.current = on && !collapsed
  const frameRef = useRef(0)
  useEffect(() => () => cancelAnimationFrame(frameRef.current), [])
  const send = useCallback(() => {
    if (!on || collapsed) return
    cancelAnimationFrame(frameRef.current)
    frameRef.current = requestAnimationFrame(() => {
      if (!eligibleRef.current || window.innerHeight <= 0) return
      const header = headerRef.current?.getBoundingClientRect().height ?? 0
      const body = rowsRef.current > 0
        ? contentRef.current?.getBoundingClientRect().height ?? 0
        : EMPTY_BODY_PX * (document.documentElement.currentCSSZoom || 1)
      // +2 for the root's 1px top and bottom border.
      const px = header + body + 2
      window.electron?.overlay?.fitHeightSet?.(true, px / window.innerHeight).catch(() => {})
    })
  }, [on, collapsed, headerRef, contentRef])

  // Turned off: give the window its full height back. Main ignores it when
  // the window isn't fitted.
  useEffect(() => {
    if (setting === false && !stack) cancelAnimationFrame(frameRef.current)
    if (setting === false && !stack) window.electron?.overlay?.fitHeightSet?.(false, 0).catch(() => {})
  }, [setting, stack])

  useEffect(() => { send() }, [send, rows, configLoads])

  // The rows' wrapper only exists while there are rows: observe it anew
  // when it appears.
  const hasRows = rows > 0
  useEffect(() => {
    if (!on) return
    const ro = new ResizeObserver(() => send())
    if (headerRef.current) ro.observe(headerRef.current)
    if (contentRef.current) ro.observe(contentRef.current)
    window.addEventListener('resize', send)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', send)
    }
  }, [on, send, headerRef, contentRef, hasRows])
}
