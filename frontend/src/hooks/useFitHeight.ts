/**
 * useFitHeight — shrinks a popout's window to its rows, never past the
 * height the user set (Settings → the overlay → Fit height to rows).
 *
 * The page hands over its root, its scrolling list and a wrapper around
 * its rows whose height is the rows' own (not stretched to the window).
 * The window wants to be its root minus the list plus the rows, which
 * counts every bar around the list (title bar, filters, a quick-add form)
 * without naming them. It goes to the main process as a fraction of the
 * root, all measured by bounding rect, so CSS zoom and display scaling
 * cancel out. The main process keeps the cap: the height you dragged the
 * window to.
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
  fromBottom = false,
  rootRef,
  listRef,
  contentRef,
}: {
  key: string
  collapsed: boolean
  // The stored collapse state has loaded: until then a window reopened
  // collapsed reads as expanded, and its title-bar height would become
  // the cap.
  collapseReady: boolean
  rows: number
  // Rows stack from the bottom: keep the window's bottom edge still, so
  // the newest row stays put, and grow upward.
  fromBottom?: boolean
  rootRef: React.RefObject<HTMLElement>
  listRef: React.RefObject<HTMLElement>
  contentRef: React.RefObject<HTMLElement>
}): void {
  const stack = useOverlayStack()
  const [setting, configLoads] = useFitSetting(key)
  const on = setting === true && !stack && collapseReady
  const rowsRef = useRef(rows)
  rowsRef.current = rows

  // Measured a frame later, so a zoom or row change has been laid out.
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
      if (!eligibleRef.current) return
      const root = rootRef.current?.getBoundingClientRect().height ?? 0
      const list = listRef.current?.getBoundingClientRect().height ?? 0
      if (root <= 0) return
      const body = rowsRef.current > 0
        ? contentRef.current?.getBoundingClientRect().height ?? 0
        : EMPTY_BODY_PX * (document.documentElement.currentCSSZoom || 1)
      const fraction = (root - list + body) / root
      window.electron?.overlay?.fitHeightSet?.(true, fraction, fromBottom).catch(() => {})
    })
  }, [on, collapsed, fromBottom, rootRef, listRef, contentRef])

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
    // The list shrinks when a bar around it grows; the rows when they change.
    if (listRef.current) ro.observe(listRef.current)
    if (contentRef.current) ro.observe(contentRef.current)
    window.addEventListener('resize', send)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', send)
    }
  }, [on, send, listRef, contentRef, hasRows])
}
