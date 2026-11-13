/**
 * ArrangeView — the Overlays hub's Arrange tab: a to-scale miniature of every
 * monitor with the active layout's windows drawn where they are. Open
 * windows are solid, hidden ones dashed, closed ones dimmed at the spot
 * they'll open. Dragging a box moves the real window (or sets where a closed
 * one opens); clicking one opens its settings.
 *
 * Positions are re-read every second, so moving a window in game shows here
 * too (paused while a box is being dragged). The box last grabbed draws on
 * top, so overlapping windows can each be reached.
 */
import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Layers } from 'lucide-react'
import { OVERLAY_CATALOG, overlayWindowLabel } from '../../lib/overlayCatalog'
import type { ArrangeRect, ArrangeState, ArrangeWindow } from '../../types/electron'
import type { HubSelection } from './selection'

const POLL_MS = 1000
const CLICK_SLOP_PX = 3
const PAD = 12

function windowLabel(w: ArrangeWindow): string {
  if (w.tabs) {
    return w.tabs
      .map((t) => (t.startsWith('customTimer:') ? w.groupNames?.[t] || 'Timer window' : overlayWindowLabel(t, [])))
      .join(' + ')
  }
  if (w.key.startsWith('customTimer:')) return w.groupName || 'Timer window'
  return overlayWindowLabel(w.key, [])
}

function selectionFor(key: string): HubSelection | null {
  if (key.startsWith('stack:')) return { kind: 'stack', key }
  if (key.startsWith('customTimer:')) return { kind: 'group', id: key.slice('customTimer:'.length) }
  const entry = OVERLAY_CATALOG.find((e) => e.name === key)
  return entry ? { kind: 'overlay', panel: entry.panel } : null
}

function colorFor(w: ArrangeWindow): string {
  if (w.tabs) return '#c9a84c'
  if (w.key.startsWith('customTimer:')) return '#38bdf8'
  return OVERLAY_CATALOG.find((e) => e.name === w.key)?.color ?? '#c9a84c'
}

// The rectangle around every monitor.
function desktopBounds(displays: ArrangeState['displays']): ArrangeRect {
  const minX = Math.min(...displays.map((d) => d.bounds.x))
  const minY = Math.min(...displays.map((d) => d.bounds.y))
  const maxX = Math.max(...displays.map((d) => d.bounds.x + d.bounds.width))
  const maxY = Math.max(...displays.map((d) => d.bounds.y + d.bounds.height))
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY }
}

type Drag = { key: string; startX: number; startY: number; origX: number; origY: number; moved: boolean }

export default function ArrangeView({
  onOpenSettings,
}: {
  onOpenSettings: (sel: HubSelection) => void
}): React.ReactElement {
  const [state, setState] = useState<ArrangeState | null>(null)
  const [width, setWidth] = useState(0)
  const wrapRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef<Drag | null>(null)
  // Where the dragged box is, in screen coordinates, until it's dropped.
  const [dragPos, setDragPos] = useState<{ key: string; x: number; y: number } | null>(null)
  const [front, setFront] = useState<string | null>(null)
  const supported = typeof window.electron?.overlay?.arrangeGet === 'function'

  const refresh = useCallback(() => {
    if (dragRef.current) return
    window.electron?.overlay?.arrangeGet?.().then(setState).catch(() => {})
  }, [])

  useEffect(() => {
    if (!supported) return
    refresh()
    const id = setInterval(refresh, POLL_MS)
    return () => clearInterval(id)
  }, [supported, refresh])

  useLayoutEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const ro = new ResizeObserver(() => setWidth(el.clientWidth))
    ro.observe(el)
    setWidth(el.clientWidth)
    return () => ro.disconnect()
  }, [])

  if (!supported) {
    return (
      <p className="p-4 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Arranging windows needs the desktop app.
      </p>
    )
  }

  const displays = state?.displays ?? []
  const desk = displays.length > 0 ? desktopBounds(displays) : null
  const maxHeight = Math.max(240, window.innerHeight * 0.62)
  const scale = desk && width > 0 ? Math.min((width - PAD * 2) / desk.width, (maxHeight - PAD * 2) / desk.height) : 0
  const toPx = (r: ArrangeRect): React.CSSProperties =>
    desk
      ? {
          left: PAD + (r.x - desk.x) * scale,
          top: PAD + (r.y - desk.y) * scale,
          width: Math.max(r.width * scale, 6),
          height: Math.max(r.height * scale, 6),
        }
      : {}

  const onPointerDown = (e: React.PointerEvent, w: ArrangeWindow): void => {
    if (e.button !== 0) return
    e.preventDefault()
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    setFront(w.key)
    dragRef.current = { key: w.key, startX: e.clientX, startY: e.clientY, origX: w.bounds.x, origY: w.bounds.y, moved: false }
  }

  const onPointerMove = (e: React.PointerEvent, w: ArrangeWindow): void => {
    const d = dragRef.current
    if (!d || d.key !== w.key || !desk || scale <= 0) return
    const dx = e.clientX - d.startX
    const dy = e.clientY - d.startY
    if (!d.moved && Math.hypot(dx, dy) < CLICK_SLOP_PX) return
    d.moved = true
    // Keep the window on the desktop.
    const x = Math.min(Math.max(d.origX + dx / scale, desk.x), desk.x + desk.width - w.bounds.width)
    const y = Math.min(Math.max(d.origY + dy / scale, desk.y), desk.y + desk.height - Math.min(w.bounds.height, 40))
    setDragPos({ key: w.key, x, y })
  }

  const onPointerUp = (w: ArrangeWindow): void => {
    const d = dragRef.current
    dragRef.current = null
    if (!d || d.key !== w.key) return
    if (!d.moved) {
      setDragPos(null)
      const sel = selectionFor(w.key)
      if (sel) onOpenSettings(sel)
      return
    }
    const pos = dragPos && dragPos.key === w.key ? dragPos : null
    if (!pos) return
    // Show it at the drop spot until the next read confirms it.
    setState((s) => s && {
      ...s,
      windows: s.windows.map((x) => (x.key === w.key ? { ...x, bounds: { ...x.bounds, x: pos.x, y: pos.y } } : x)),
    })
    setDragPos(null)
    window.electron?.overlay?.arrangeMove(w.key, pos.x, pos.y).then(() => setTimeout(refresh, 300)).catch(() => {})
  }

  const windows = state?.windows ?? []
  // Closed first, so open windows draw on top.
  const order = [...windows].sort((a, b) => Number(a.state !== 'closed') - Number(b.state !== 'closed'))

  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto p-4">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        <span>Drag a window to move it. Click one for its settings.</span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-2.5 w-4 rounded-sm" style={{ border: '1.5px solid var(--color-primary)' }} />
          Open
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-2.5 w-4 rounded-sm" style={{ border: '1.5px dashed var(--color-primary)' }} />
          Hidden
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block h-2.5 w-4 rounded-sm" style={{ border: '1.5px solid var(--color-border)', opacity: 0.6 }} />
          Closed (opens here)
        </span>
      </div>
      <div ref={wrapRef} className="w-full">
        {desk && scale > 0 && (
          <div
            className="relative rounded-lg"
            style={{
              width: desk.width * scale + PAD * 2,
              height: desk.height * scale + PAD * 2,
              backgroundColor: 'var(--color-surface)',
              border: '1px solid var(--color-border)',
            }}
          >
            {displays.map((d) => (
              <div
                key={d.id}
                className="absolute rounded"
                style={{
                  ...toPx(d.bounds),
                  backgroundColor: 'var(--color-background)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <span
                  className="absolute bottom-1 right-2 text-[10px]"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  {d.label}{d.isPrimary ? ' (main)' : ''} · {d.bounds.width}×{d.bounds.height}
                </span>
              </div>
            ))}
            {windows.length === 0 && (
              <p
                className="absolute inset-0 flex items-center justify-center text-xs"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                No windows in this layout yet. Pop out overlays from the Library.
              </p>
            )}
            {order.map((w) => {
              const pos = dragPos?.key === w.key ? { ...w.bounds, x: dragPos.x, y: dragPos.y } : w.bounds
              const color = colorFor(w)
              const closed = w.state === 'closed'
              const label = windowLabel(w)
              return (
                <div
                  key={w.key}
                  role="button"
                  tabIndex={0}
                  onPointerDown={(e) => onPointerDown(e, w)}
                  onPointerMove={(e) => onPointerMove(e, w)}
                  onPointerUp={() => onPointerUp(w)}
                  onKeyDown={(e) => {
                    if (e.key !== 'Enter') return
                    const sel = selectionFor(w.key)
                    if (sel) onOpenSettings(sel)
                  }}
                  title={`${label} · ${w.state}${w.collapsed ? ', collapsed' : ''} — drag to move, click for settings`}
                  className="absolute flex cursor-grab items-start gap-1 overflow-hidden rounded px-1 py-0.5 active:cursor-grabbing"
                  style={{
                    ...toPx(pos),
                    zIndex: dragPos?.key === w.key ? 10 : front === w.key ? 5 : closed ? 1 : 2,
                    backgroundColor: closed ? 'transparent' : `${color}22`,
                    borderWidth: 1.5,
                    borderStyle: w.state === 'hidden' ? 'dashed' : 'solid',
                    borderColor: closed ? 'var(--color-border)' : color,
                    opacity: closed ? 0.6 : 1,
                    touchAction: 'none',
                    userSelect: 'none',
                  }}
                >
                  {w.tabs && <Layers size={9} className="mt-px shrink-0" style={{ color }} />}
                  <span
                    className="truncate text-[10px] font-semibold leading-tight"
                    style={{ color: closed ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}
                  >
                    {label}
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
