import React, { useCallback, useEffect, useRef, useState } from 'react'
import { GripVertical } from 'lucide-react'

interface OverlayWindowProps {
  title: React.ReactNode
  headerRight?: React.ReactNode
  defaultWidth?: number
  defaultHeight?: number
  defaultX?: number
  defaultY?: number
  minWidth?: number
  minHeight?: number
  /** When set, drag/resize end positions are rounded to a multiple of this value. */
  snapGridSize?: number
  /** Fires once at the end of a drag or resize (mouseup) with the final bounds. */
  onLayoutChange?: (bounds: { x: number; y: number; width: number; height: number }) => void
  children: React.ReactNode
}

function snap(value: number, grid?: number): number {
  if (!grid || grid <= 1) return value
  return Math.round(value / grid) * grid
}

function findScrollableAncestor(el: HTMLElement | null): HTMLElement | null {
  let node: HTMLElement | null = el?.parentElement ?? null
  while (node && node !== document.body) {
    const s = getComputedStyle(node)
    if (/(auto|scroll)/.test(s.overflowY) || /(auto|scroll)/.test(s.overflowX)) {
      return node
    }
    node = node.parentElement
  }
  return null
}

type ResizeDir = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

interface DragState {
  startX: number
  startY: number
  posX: number
  posY: number
}

interface ResizeState {
  startX: number
  startY: number
  startW: number
  startH: number
  startPosX: number
  startPosY: number
  dir: ResizeDir
}

const CURSOR_MAP: Record<ResizeDir, string> = {
  n: 'ns-resize',
  s: 'ns-resize',
  e: 'ew-resize',
  w: 'ew-resize',
  ne: 'nesw-resize',
  sw: 'nesw-resize',
  nw: 'nwse-resize',
  se: 'nwse-resize',
}

const HANDLE_SIZE = 6 // px
// Distance (in px) from the scroll container's edge at which a drag/resize
// begins to autoscroll the container in that direction.
const AUTOSCROLL_EDGE = 40
const AUTOSCROLL_MAX_SPEED = 18

/**
 * OverlayWindow — a draggable, resizable floating panel.
 *
 * Position the containing element as `position: relative` and sized to fill
 * the available space; this component positions itself absolutely within it.
 *
 * All future overlay panels should use this component so they share
 * consistent move/resize behaviour.
 */
export default function OverlayWindow({
  title,
  headerRight,
  defaultWidth = 380,
  defaultHeight = 420,
  defaultX = 24,
  defaultY = 24,
  minWidth = 260,
  minHeight = 180,
  snapGridSize,
  onLayoutChange,
  children,
}: OverlayWindowProps): React.ReactElement {
  const [pos, setPos] = useState({ x: defaultX, y: defaultY })
  const [size, setSize] = useState({ w: defaultWidth, h: defaultHeight })

  const rootRef = useRef<HTMLDivElement | null>(null)
  const dragRef = useRef<DragState | null>(null)
  const resizeRef = useRef<ResizeState | null>(null)
  // Scroll container captured at drag/resize start so we can (a) compensate
  // the live position for any scroll that happens during the gesture and
  // (b) autoscroll when the mouse approaches its edges.
  const scrollContainerRef = useRef<HTMLElement | null>(null)
  const initialScrollRef = useRef<{ top: number; left: number }>({ top: 0, left: 0 })
  const lastMouseRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 })
  const autoscrollRafRef = useRef<number | null>(null)
  // Latest values for the mouseup handler — refs avoid recreating listeners.
  const layoutRef = useRef({ pos, size })
  layoutRef.current = { pos, size }
  const onLayoutChangeRef = useRef(onLayoutChange)
  onLayoutChangeRef.current = onLayoutChange

  const applyDragPosition = useCallback((clientX: number, clientY: number): void => {
    const d = dragRef.current
    if (!d) return
    const sc = scrollContainerRef.current
    const dsx = sc ? sc.scrollLeft - initialScrollRef.current.left : 0
    const dsy = sc ? sc.scrollTop - initialScrollRef.current.top : 0
    setPos({
      x: Math.max(0, d.posX + (clientX - d.startX) + dsx),
      y: Math.max(0, d.posY + (clientY - d.startY) + dsy),
    })
  }, [])

  const applyResize = useCallback((clientX: number, clientY: number): void => {
    const r = resizeRef.current
    if (!r) return
    const sc = scrollContainerRef.current
    const dsx = sc ? sc.scrollLeft - initialScrollRef.current.left : 0
    const dsy = sc ? sc.scrollTop - initialScrollRef.current.top : 0
    const dx = (clientX - r.startX) + dsx
    const dy = (clientY - r.startY) + dsy

    let newW = r.startW
    let newH = r.startH
    let newX = r.startPosX
    let newY = r.startPosY

    if (r.dir.includes('e')) newW = Math.max(minWidth, r.startW + dx)
    if (r.dir.includes('s')) newH = Math.max(minHeight, r.startH + dy)
    if (r.dir.includes('w')) {
      newW = Math.max(minWidth, r.startW - dx)
      newX = r.startPosX + (r.startW - newW)
    }
    if (r.dir.includes('n')) {
      newH = Math.max(minHeight, r.startH - dy)
      newY = r.startPosY + (r.startH - newH)
    }

    setSize({ w: newW, h: newH })
    setPos({ x: Math.max(0, newX), y: Math.max(0, newY) })
  }, [minWidth, minHeight])

  const stopAutoscroll = useCallback((): void => {
    if (autoscrollRafRef.current !== null) {
      cancelAnimationFrame(autoscrollRafRef.current)
      autoscrollRafRef.current = null
    }
  }, [])

  const tickAutoscroll = useCallback((): void => {
    autoscrollRafRef.current = null
    const sc = scrollContainerRef.current
    const interacting = dragRef.current !== null || resizeRef.current !== null
    if (!sc || !interacting) return

    const r = sc.getBoundingClientRect()
    const { x: mx, y: my } = lastMouseRef.current
    let dx = 0
    let dy = 0
    if (my > r.bottom - AUTOSCROLL_EDGE) {
      dy = Math.min(AUTOSCROLL_MAX_SPEED, my - (r.bottom - AUTOSCROLL_EDGE))
    } else if (my < r.top + AUTOSCROLL_EDGE) {
      dy = -Math.min(AUTOSCROLL_MAX_SPEED, (r.top + AUTOSCROLL_EDGE) - my)
    }
    if (mx > r.right - AUTOSCROLL_EDGE) {
      dx = Math.min(AUTOSCROLL_MAX_SPEED, mx - (r.right - AUTOSCROLL_EDGE))
    } else if (mx < r.left + AUTOSCROLL_EDGE) {
      dx = -Math.min(AUTOSCROLL_MAX_SPEED, (r.left + AUTOSCROLL_EDGE) - mx)
    }
    if (dx !== 0 || dy !== 0) {
      sc.scrollBy(dx, dy)
      if (dragRef.current) applyDragPosition(mx, my)
      else if (resizeRef.current) applyResize(mx, my)
    }
    autoscrollRafRef.current = requestAnimationFrame(tickAutoscroll)
  }, [applyDragPosition, applyResize])

  const ensureAutoscrollLoop = useCallback((): void => {
    if (autoscrollRafRef.current !== null) return
    autoscrollRafRef.current = requestAnimationFrame(tickAutoscroll)
  }, [tickAutoscroll])

  // Attach/detach document-level mouse handlers once.
  useEffect(() => {
    const onMove = (e: MouseEvent): void => {
      lastMouseRef.current = { x: e.clientX, y: e.clientY }
      if (dragRef.current) {
        applyDragPosition(e.clientX, e.clientY)
        ensureAutoscrollLoop()
        return
      }
      if (resizeRef.current) {
        applyResize(e.clientX, e.clientY)
        ensureAutoscrollLoop()
      }
    }

    const onUp = (): void => {
      const wasInteracting = dragRef.current !== null || resizeRef.current !== null
      dragRef.current = null
      resizeRef.current = null
      scrollContainerRef.current = null
      stopAutoscroll()
      if (!wasInteracting) return
      // Snap final bounds to grid if requested, then notify the parent.
      const { pos: p, size: s } = layoutRef.current
      const x = Math.max(0, snap(p.x, snapGridSize))
      const y = Math.max(0, snap(p.y, snapGridSize))
      const width = Math.max(minWidth, snap(s.w, snapGridSize))
      const height = Math.max(minHeight, snap(s.h, snapGridSize))
      if (x !== p.x || y !== p.y) setPos({ x, y })
      if (width !== s.w || height !== s.h) setSize({ w: width, h: height })
      onLayoutChangeRef.current?.({ x, y, width, height })
    }

    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      stopAutoscroll()
    }
  }, [minWidth, minHeight, snapGridSize, applyDragPosition, applyResize, ensureAutoscrollLoop, stopAutoscroll])

  const captureScrollContainer = useCallback((): void => {
    const sc = findScrollableAncestor(rootRef.current)
    scrollContainerRef.current = sc
    initialScrollRef.current = {
      top: sc?.scrollTop ?? 0,
      left: sc?.scrollLeft ?? 0,
    }
  }, [])

  const startDrag = useCallback((e: React.MouseEvent): void => {
    e.preventDefault()
    captureScrollContainer()
    dragRef.current = { startX: e.clientX, startY: e.clientY, posX: pos.x, posY: pos.y }
  }, [pos, captureScrollContainer])

  const startResize = useCallback((dir: ResizeDir) => (e: React.MouseEvent): void => {
    e.preventDefault()
    e.stopPropagation()
    captureScrollContainer()
    resizeRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      startW: size.w,
      startH: size.h,
      startPosX: pos.x,
      startPosY: pos.y,
      dir,
    }
  }, [pos, size, captureScrollContainer])

  return (
    <div
      ref={rootRef}
      style={{
        position: 'absolute',
        left: pos.x,
        top: pos.y,
        width: size.w,
        height: size.h,
        minWidth,
        minHeight,
        zIndex: 10,
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        borderRadius: 8,
        overflow: 'hidden',
        boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
      }}
    >
      {/* ── Title bar / drag handle ─────────────────────────────────────── */}
      <div
        onMouseDown={startDrag}
        style={{
          cursor: 'grab',
          userSelect: 'none',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 10px',
          backgroundColor: 'var(--color-surface-2)',
          borderBottom: '1px solid var(--color-border)',
          flexShrink: 0,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <GripVertical size={12} style={{ color: 'var(--color-muted)', pointerEvents: 'none' }} />
          <span style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-foreground)' }}>
            {title}
          </span>
        </div>
        {headerRight && (
          <div
            style={{ display: 'flex', alignItems: 'center', gap: 6 }}
            onMouseDown={(e) => e.stopPropagation()} // don't start drag on controls
          >
            {headerRight}
          </div>
        )}
      </div>

      {/* ── Content ─────────────────────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
        {children}
      </div>

      {/* ── Edge resize handles ──────────────────────────────────────────── */}
      {/* Top */}
      <div onMouseDown={startResize('n')} style={{ position: 'absolute', top: 0, left: HANDLE_SIZE, right: HANDLE_SIZE, height: HANDLE_SIZE, cursor: CURSOR_MAP.n, zIndex: 20 }} />
      {/* Bottom */}
      <div onMouseDown={startResize('s')} style={{ position: 'absolute', bottom: 0, left: HANDLE_SIZE, right: HANDLE_SIZE, height: HANDLE_SIZE, cursor: CURSOR_MAP.s, zIndex: 20 }} />
      {/* Left */}
      <div onMouseDown={startResize('w')} style={{ position: 'absolute', left: 0, top: HANDLE_SIZE, bottom: HANDLE_SIZE, width: HANDLE_SIZE, cursor: CURSOR_MAP.w, zIndex: 20 }} />
      {/* Right */}
      <div onMouseDown={startResize('e')} style={{ position: 'absolute', right: 0, top: HANDLE_SIZE, bottom: HANDLE_SIZE, width: HANDLE_SIZE, cursor: CURSOR_MAP.e, zIndex: 20 }} />

      {/* ── Corner resize handles ─────────────────────────────────────────── */}
      <div onMouseDown={startResize('nw')} style={{ position: 'absolute', top: 0, left: 0, width: HANDLE_SIZE, height: HANDLE_SIZE, cursor: CURSOR_MAP.nw, zIndex: 21 }} />
      <div onMouseDown={startResize('ne')} style={{ position: 'absolute', top: 0, right: 0, width: HANDLE_SIZE, height: HANDLE_SIZE, cursor: CURSOR_MAP.ne, zIndex: 21 }} />
      <div onMouseDown={startResize('sw')} style={{ position: 'absolute', bottom: 0, left: 0, width: HANDLE_SIZE, height: HANDLE_SIZE, cursor: CURSOR_MAP.sw, zIndex: 21 }} />
      <div onMouseDown={startResize('se')} style={{ position: 'absolute', bottom: 0, right: 0, width: HANDLE_SIZE, height: HANDLE_SIZE, cursor: CURSOR_MAP.se, zIndex: 21 }} />
    </div>
  )
}
