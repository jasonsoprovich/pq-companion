/**
 * ArrangeView — the Overlays hub's Arrange tab: a to-scale miniature of every
 * monitor, filling the tab, with the active layout's windows drawn where they
 * are. Open windows are solid, hidden ones dashed, closed ones dimmed at the
 * spot they'll open.
 *
 * - Clicking a box opens a closed window at its spot, shows a hidden one, or
 *   brings an open one to the front. Settings stay on the Library cards.
 * - Dragging a box moves the real window (or sets where a closed one opens).
 * - Dropping a box onto another box's title strip stacks them, as dropping
 *   one window on another's title bar does in game.
 * - A stack box lists its tabs as chips: drag one out to unstack it there,
 *   or onto another box's title strip to move it into that window.
 *
 * Positions are re-read every second, so moving a window in game shows here
 * too (paused during a drag). The box last picked draws on top, so
 * overlapping windows can each be reached.
 */
import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Layers } from 'lucide-react'
import { OVERLAY_CATALOG, overlayWindowLabel } from '../../lib/overlayCatalog'
import type { ArrangeRect, ArrangeState, ArrangeWindow } from '../../types/electron'
import { INSTANCE_COLOR, INSTANCE_FALLBACK_LABEL, instanceKind } from '../../lib/instanceKeys'

const POLL_MS = 1000
const CLICK_SLOP_PX = 3
const PAD = 12
// Discord Voice embeds an external page and can't share a window.
const UNSTACKABLE = new Set(['discordVoice'])

function tabLabel(w: ArrangeWindow, tab: string): string {
  const kind = instanceKind(tab)
  return kind ? w.groupNames?.[tab] || INSTANCE_FALLBACK_LABEL[kind] : overlayWindowLabel(tab, [])
}

function windowLabel(w: ArrangeWindow): string {
  if (w.tabs) return w.tabs.map((t) => tabLabel(w, t)).join(' + ')
  const kind = instanceKind(w.key)
  if (kind) return w.groupName || INSTANCE_FALLBACK_LABEL[kind]
  return overlayWindowLabel(w.key, [])
}

function colorFor(key: string, isStack: boolean): string {
  if (isStack) return '#c9a84c'
  const kind = instanceKind(key)
  if (kind) return INSTANCE_COLOR[kind]
  return OVERLAY_CATALOG.find((e) => e.name === key)?.color ?? '#c9a84c'
}

// The rectangle around every monitor.
function desktopBounds(displays: ArrangeState['displays']): ArrangeRect {
  const minX = Math.min(...displays.map((d) => d.bounds.x))
  const minY = Math.min(...displays.map((d) => d.bounds.y))
  const maxX = Math.max(...displays.map((d) => d.bounds.x + d.bounds.width))
  const maxY = Math.max(...displays.map((d) => d.bounds.y + d.bounds.height))
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY }
}

type BoxDrag = { key: string; startX: number; startY: number; origX: number; origY: number; moved: boolean }
type ChipDrag = { stack: string; tab: string; label: string; x: number; y: number; moved: boolean }

// `compact` drops the how-to line, for hosts that explain it themselves (the
// overlay setup wizard).
export default function ArrangeView({ compact = false }: { compact?: boolean }): React.ReactElement {
  const [state, setState] = useState<ArrangeState | null>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })
  const wrapRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef<BoxDrag | null>(null)
  // Where the dragged box is, in screen coordinates, until it's dropped.
  const [dragPos, setDragPos] = useState<{ key: string; x: number; y: number } | null>(null)
  const [chip, setChip] = useState<ChipDrag | null>(null)
  const chipRef = useRef<ChipDrag | null>(null)
  // The box whose title strip a drop would stack onto.
  const [dropTarget, setDropTarget] = useState<string | null>(null)
  const [front, setFront] = useState<string | null>(null)
  const supported = typeof window.electron?.overlay?.arrangeGet === 'function'

  const refresh = useCallback(() => {
    if (dragRef.current || chipRef.current) return
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
    const measure = (): void => setSize({ w: el.clientWidth, h: el.clientHeight })
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    measure()
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
  const windows = state?.windows ?? []
  const desk = displays.length > 0 ? desktopBounds(displays) : null
  const scale =
    desk && size.w > 0 && size.h > 0
      ? Math.min((size.w - PAD * 2) / desk.width, (size.h - PAD * 2) / desk.height)
      : 0
  const toPx = (r: ArrangeRect): { left: number; top: number; width: number; height: number } =>
    desk
      ? {
          left: PAD + (r.x - desk.x) * scale,
          top: PAD + (r.y - desk.y) * scale,
          width: Math.max(r.width * scale, 6),
          height: Math.max(r.height * scale, 6),
        }
      : { left: 0, top: 0, width: 0, height: 0 }
  // A box's title strip: its window's title bar, at least a label's height.
  const stripPx = Math.max(16, 36 * scale)

  // Pointer position inside the canvas.
  const canvasPoint = (clientX: number, clientY: number): { x: number; y: number } => {
    const r = canvasRef.current?.getBoundingClientRect()
    return r ? { x: clientX - r.left, y: clientY - r.top } : { x: 0, y: 0 }
  }
  const toScreen = (p: { x: number; y: number }): { x: number; y: number } =>
    desk ? { x: desk.x + (p.x - PAD) / scale, y: desk.y + (p.y - PAD) / scale } : p
  const boxAt = (p: { x: number; y: number }, w: ArrangeWindow): boolean => {
    const r = toPx(w.bounds)
    return p.x >= r.left && p.x < r.left + r.width && p.y >= r.top && p.y < r.top + r.height
  }
  // The window whose title strip is under the pointer, other than `skip`.
  const stripTargetAt = (p: { x: number; y: number }, skip: string[]): string | null => {
    const hits = windows.filter((w) => {
      if (skip.includes(w.key) || UNSTACKABLE.has(w.key)) return false
      const r = toPx(w.bounds)
      return p.x >= r.left && p.x < r.left + r.width && p.y >= r.top && p.y < r.top + Math.min(stripPx, r.height)
    })
    if (hits.length === 0) return null
    return (hits.find((w) => w.key === front) ?? hits[hits.length - 1]).key
  }

  const settle = (): void => {
    setTimeout(refresh, 300)
    setTimeout(refresh, 1200)
  }

  // ── Box drags ──
  const onBoxDown = (e: React.PointerEvent, w: ArrangeWindow): void => {
    if (e.button !== 0) return
    e.preventDefault()
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    setFront(w.key)
    dragRef.current = { key: w.key, startX: e.clientX, startY: e.clientY, origX: w.bounds.x, origY: w.bounds.y, moved: false }
  }

  const onBoxMove = (e: React.PointerEvent, w: ArrangeWindow): void => {
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
    setDropTarget(UNSTACKABLE.has(w.key) ? null : stripTargetAt(canvasPoint(e.clientX, e.clientY), [w.key]))
  }

  const onBoxUp = (w: ArrangeWindow): void => {
    const d = dragRef.current
    dragRef.current = null
    const target = dropTarget
    setDropTarget(null)
    if (!d || d.key !== w.key) return
    if (!d.moved) {
      setDragPos(null)
      window.electron?.overlay?.arrangeOpen(w.key).then(settle).catch(() => {})
      return
    }
    const pos = dragPos && dragPos.key === w.key ? dragPos : null
    setDragPos(null)
    if (target) {
      window.electron?.overlay?.stackMerge(target, w.key).then(settle).catch(() => {})
      setFront(target)
      return
    }
    if (!pos) return
    // Show it at the drop spot until the next read confirms it.
    setState((s) => s && {
      ...s,
      windows: s.windows.map((x) => (x.key === w.key ? { ...x, bounds: { ...x.bounds, x: pos.x, y: pos.y } } : x)),
    })
    window.electron?.overlay?.arrangeMove(w.key, pos.x, pos.y).then(settle).catch(() => {})
  }

  // ── Tab chip drags (unstack) ──
  const onChipDown = (e: React.PointerEvent, w: ArrangeWindow, tab: string): void => {
    if (e.button !== 0) return
    e.preventDefault()
    e.stopPropagation()
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    setFront(w.key)
    const next = { stack: w.key, tab, label: tabLabel(w, tab), x: e.clientX, y: e.clientY, moved: false }
    chipRef.current = next
    setChip(next)
  }

  const onChipMove = (e: React.PointerEvent): void => {
    const c = chipRef.current
    if (!c) return
    e.stopPropagation()
    const moved = c.moved || Math.hypot(e.clientX - c.x, e.clientY - c.y) >= CLICK_SLOP_PX
    const next = { ...c, x: e.clientX, y: e.clientY, moved }
    chipRef.current = next
    setChip(next)
    if (moved) setDropTarget(stripTargetAt(canvasPoint(e.clientX, e.clientY), [c.stack]))
  }

  const onChipUp = (e: React.PointerEvent): void => {
    const c = chipRef.current
    chipRef.current = null
    setChip(null)
    const target = dropTarget
    setDropTarget(null)
    if (!c || !c.moved) return
    e.stopPropagation()
    if (target) {
      window.electron?.overlay?.stackMerge(target, c.tab).then(settle).catch(() => {})
      return
    }
    const p = canvasPoint(e.clientX, e.clientY)
    const stack = windows.find((w) => w.key === c.stack)
    if (stack && boxAt(p, stack)) return
    // Its window opens with its title bar under the pointer.
    const s = toScreen({ x: p.x - 8, y: p.y - 6 })
    window.electron?.overlay?.stackRemove(c.tab, true, s.x, s.y).then(settle).catch(() => {})
  }

  // Closed first, so open windows draw on top; the last picked on top of all.
  const order = [...windows].sort((a, b) => Number(a.state !== 'closed') - Number(b.state !== 'closed'))

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <div className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        {!compact && (
          <span>
            Click a window to open it or bring it forward, and drag it to move it. Drop one on another&rsquo;s
            title strip to stack them; drag a tab out of a stack to unstack it.
          </span>
        )}
        <span className="flex items-center gap-3">
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
        </span>
      </div>
      <div ref={wrapRef} className="relative min-h-0 flex-1">
        {desk && scale > 0 && (
          <div
            ref={canvasRef}
            className="relative mx-auto overflow-hidden rounded-lg"
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
                <span className="absolute bottom-1 right-2 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
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
              const isStack = !!w.tabs
              const color = colorFor(w.key, isStack)
              const closed = w.state === 'closed'
              const label = windowLabel(w)
              const isTarget = dropTarget === w.key
              return (
                <div
                  key={w.key}
                  role="button"
                  tabIndex={0}
                  onPointerDown={(e) => onBoxDown(e, w)}
                  onPointerMove={(e) => onBoxMove(e, w)}
                  onPointerUp={() => onBoxUp(w)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') window.electron?.overlay?.arrangeOpen(w.key).then(settle).catch(() => {})
                  }}
                  title={`${label} · ${w.state}${w.collapsed ? ', collapsed' : ''} — click to ${
                    closed ? 'open it' : w.state === 'hidden' ? 'show it' : 'bring it forward'
                  }, drag to move`}
                  className="absolute flex cursor-grab flex-col items-stretch gap-0.5 overflow-hidden rounded px-1 py-0.5 active:cursor-grabbing"
                  style={{
                    ...toPx(pos),
                    zIndex: dragPos?.key === w.key ? 10 : front === w.key ? 5 : closed ? 1 : 2,
                    backgroundColor: isTarget ? 'rgba(201,168,76,0.25)' : closed ? 'transparent' : `${color}22`,
                    borderWidth: isTarget ? 2 : 1.5,
                    borderStyle: w.state === 'hidden' && !isTarget ? 'dashed' : 'solid',
                    borderColor: isTarget ? '#c9a84c' : closed ? 'var(--color-border)' : color,
                    opacity: closed && !isTarget ? 0.6 : 1,
                    boxShadow: front === w.key ? '0 0 0 1px rgba(255,255,255,0.35)' : undefined,
                    touchAction: 'none',
                    userSelect: 'none',
                  }}
                >
                  <span className="flex min-w-0 items-center gap-1">
                    {isStack && <Layers size={9} className="shrink-0" style={{ color }} />}
                    <span
                      className="truncate text-[10px] font-semibold leading-tight"
                      style={{ color: closed ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}
                    >
                      {isTarget ? 'Drop to stack' : label}
                    </span>
                  </span>
                  {isStack && !isTarget && (
                    <span className="flex min-w-0 flex-wrap gap-0.5">
                      {w.tabs!.map((tab) => (
                        <span
                          key={tab}
                          onPointerDown={(e) => onChipDown(e, w, tab)}
                          onPointerMove={onChipMove}
                          onPointerUp={onChipUp}
                          title={`Drag ${tabLabel(w, tab)} out to unstack it`}
                          className="max-w-full cursor-grab truncate rounded px-1 text-[9px] leading-[14px]"
                          style={{
                            backgroundColor: 'rgba(0,0,0,0.35)',
                            border: '1px solid rgba(201,168,76,0.5)',
                            color: 'var(--color-foreground)',
                            opacity: chip?.tab === tab && chip.moved ? 0.35 : 1,
                            touchAction: 'none',
                          }}
                        >
                          {tabLabel(w, tab)}
                        </span>
                      ))}
                    </span>
                  )}
                </div>
              )
            })}
          </div>
        )}
        {chip?.moved && (
          <div
            className="pointer-events-none fixed z-50 rounded px-1.5 text-[10px] leading-[16px]"
            style={{
              left: chip.x + 6,
              top: chip.y + 4,
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid #c9a84c',
              color: 'var(--color-foreground)',
              boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
            }}
          >
            {dropTarget ? `Stack ${chip.label} here` : `Unstack ${chip.label}`}
          </div>
        )}
      </div>
    </div>
  )
}
