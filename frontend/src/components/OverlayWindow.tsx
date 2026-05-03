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

  const dragRef = useRef<DragState | null>(null)
  const resizeRef = useRef<ResizeState | null>(null)
  // Latest values for the mouseup handler — refs avoid recreating listeners.
  const layoutRef = useRef({ pos, size })
  layoutRef.current = { pos, size }
  const onLayoutChangeRef = useRef(onLayoutChange)
  onLayoutChangeRef.current = onLayoutChange

  // Attach/detach document-level mouse handlers once.
  useEffect(() => {
    const onMove = (e: MouseEvent): void => {
      if (dragRef.current) {
        const d = dragRef.current
        setPos({
          x: Math.max(0, d.posX + (e.clientX - d.startX)),
          y: Math.max(0, d.posY + (e.clientY - d.startY)),
        })
        return
      }

      if (resizeRef.current) {
        const r = resizeRef.current
        const dx = e.clientX - r.startX
        const dy = e.clientY - r.startY

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
      }
    }

    const onUp = (): void => {
      const wasInteracting = dragRef.current !== null || resizeRef.current !== null
      dragRef.current = null
      resizeRef.current = null
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
    }
  }, [minWidth, minHeight, snapGridSize])

  const startDrag = useCallback((e: React.MouseEvent): void => {
    e.preventDefault()
    dragRef.current = { startX: e.clientX, startY: e.clientY, posX: pos.x, posY: pos.y }
  }, [pos])

  const startResize = useCallback((dir: ResizeDir) => (e: React.MouseEvent): void => {
    e.preventDefault()
    e.stopPropagation()
    resizeRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      startW: size.w,
      startH: size.h,
      startPosX: pos.x,
      startPosY: pos.y,
      dir,
    }
  }, [pos, size])

  return (
    <div
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
