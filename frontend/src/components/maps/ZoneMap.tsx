import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { MapGeometry, MapPOI, MapPOICategory, MapZone } from '../../types/map'

// MapHighlight is a prominent marker drawn over the map, independent of the
// generated POI set — an NPC's spawn point, or the POI the user just clicked.
export interface MapHighlight {
  x: number
  y: number
  z?: number
  label?: string
}

// ZoneMap draws a zone's geometry and POIs on a canvas with pan/zoom and a
// depth control.
//
// Canvas rather than SVG: a large zone is tens of thousands of segments, and as
// DOM nodes that stalls badly while panning. Everything here redraws from flat
// typed arrays each frame.

export interface ZoneMapProps {
  zone: MapZone
  geometry: MapGeometry | null
  pois: MapPOI[]
  // visibleCategories gates POI layers; undefined shows all.
  visibleCategories?: Set<MapPOICategory>
  // highlights are drawn prominently on top of everything else. A list rather
  // than a single pin because an NPC routinely has several spawn points, and
  // showing only one would quietly imply it is the only place it appears.
  highlights?: MapHighlight[]
  // playerPos is the live position from ZealPipes, when connected.
  playerPos?: { x: number; y: number; z: number; heading?: number } | null
  onPOIClick?: (poi: MapPOI) => void
  height?: number
  showLabels?: boolean
}

const CATEGORY_STYLE: Record<MapPOICategory, { color: string; short: string }> = {
  vendor: { color: '#c9a84c', short: 'V' },
  raid_target: { color: '#ef4444', short: 'R' },
  trap: { color: '#f59e0b', short: '!' },
  zone_line: { color: '#22c55e', short: 'Z' },
  ground_spawn: { color: '#3b82f6', short: 'G' },
  tradeskill: { color: '#a78bfa', short: 'T' },
  succor: { color: '#5eead4', short: 'S' },
  door: { color: '#94a3b8', short: 'D' },
}

// Geometry colour by technique. Contours are elevation lines, not walls, so
// they get a distinct hue — reading them as structure would be misleading.
const GEOMETRY_COLOR: Record<MapZone['technique'], string> = {
  boundary: '#7dd3fc',
  contours: '#5eead4',
  silhouette: '#a78bfa',
}

export function ZoneMap({
  zone,
  geometry,
  pois,
  visibleCategories,
  highlights,
  playerPos,
  onPOIClick,
  height = 520,
  showLabels = true,
}: ZoneMapProps): React.ReactElement {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const [view, setView] = useState({ zoom: 1, panX: 0, panY: 0 })
  const [size, setSize] = useState({ w: 800, h: height })
  const drag = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null)

  // Depth window. null means "show everything"; otherwise geometry outside
  // ±range of the focus height fades, which is the only workable way to read
  // zones whose floors are continuous ramps rather than discrete storeys.
  const [depth, setDepth] = useState<{ focus: number; range: number } | null>(null)

  const shown = useMemo(
    () => pois.filter((p) => !visibleCategories || visibleCategories.has(p.category)),
    [pois, visibleCategories],
  )

  useEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const ro = new ResizeObserver(() => {
      setSize({ w: el.clientWidth, h: height })
    })
    ro.observe(el)
    setSize({ w: el.clientWidth, h: height })
    return () => ro.disconnect()
  }, [height])

  // Reset the view whenever the zone changes, so switching zones doesn't leave
  // you panned off into empty space.
  useEffect(() => {
    setView({ zoom: 1, panX: 0, panY: 0 })
    setDepth(null)
  }, [zone.zone])

  // Fit-to-zone transform: map units -> screen pixels.
  //
  // Orientation, established from game geography rather than from comparing
  // renderings (which is how it was got wrong before — both sides of the
  // comparison used this same code, so a global flip was invisible):
  //
  //   East Commonlands borders West Commonlands at game_x = -1621 in a zone
  //   spanning -1666..3746, so east is NEGATIVE game_x, i.e. +game_x = west.
  //   South Qeynos exits sit at game_y = -151/-26, so +game_y = north.
  //
  // Map space stores f1 = -game_x (east-positive) and f2 = -game_y
  // (south-positive). A north-up, east-right map therefore needs screen X to
  // grow with f1 AND screen Y to grow with f2 — no inversion on either axis.
  // Inverting Y mirrored every map vertically.
  const base = useMemo(() => {
    const w = Math.max(1, zone.max_x - zone.min_x)
    const h = Math.max(1, zone.max_y - zone.min_y)
    const pad = 16
    const scale = Math.min((size.w - pad * 2) / w, (size.h - pad * 2) / h)
    return {
      scale,
      offX: (size.w - w * scale) / 2 - zone.min_x * scale,
      offY: (size.h - h * scale) / 2 - zone.min_y * scale,
    }
  }, [zone, size])

  const toScreen = useCallback(
    (x: number, y: number): [number, number] => [
      (x * base.scale + base.offX) * view.zoom + view.panX,
      (y * base.scale + base.offY) * view.zoom + view.panY,
    ],
    [base, view],
  )

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const dpr = window.devicePixelRatio || 1
    canvas.width = size.w * dpr
    canvas.height = size.h * dpr
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, size.w, size.h)

    // ── geometry ──
    if (geometry && geometry.count > 0) {
      const c = geometry.coords
      const color = GEOMETRY_COLOR[zone.technique] ?? '#7dd3fc'
      ctx.lineWidth = 1
      ctx.lineCap = 'round'

      // Graduated fade by height, in a few opacity buckets.
      //
      // Silhouette geometry is sliced into height bands, so a zone with stacked
      // tunnels arrives as several overlapping levels. A hard in/out cut would
      // make everything off-level vanish; fading instead keeps the surrounding
      // structure legible while the focused level stands out — which is what
      // makes a multi-level map readable rather than a tangle.
      //
      // Bucketed rather than per-segment alpha because globalAlpha cannot vary
      // within a path: a continuous ramp would force one stroke() per segment,
      // and these zones run to tens of thousands.
      const ALPHAS = depth ? [1, 0.5, 0.25, 0.1] : [1]
      const bucketOf = (zMid: number): number => {
        if (!depth) return 0
        const d = Math.abs(zMid - depth.focus)
        if (d <= depth.range) return 0
        // Each further band out steps down one bucket.
        return Math.min(ALPHAS.length - 1, 1 + Math.floor((d - depth.range) / depth.range))
      }

      // Draw faintest first so in-focus lines land on top.
      for (let b = ALPHAS.length - 1; b >= 0; b--) {
        ctx.beginPath()
        ctx.strokeStyle = color
        ctx.globalAlpha = ALPHAS[b]
        let any = false
        for (let i = 0; i < geometry.count; i++) {
          const o = i * 6
          if (bucketOf((c[o + 2] + c[o + 5]) / 2) !== b) continue
          any = true
          const [x1, y1] = toScreen(c[o], c[o + 1])
          const [x2, y2] = toScreen(c[o + 3], c[o + 4])
          ctx.moveTo(x1, y1)
          ctx.lineTo(x2, y2)
        }
        if (any) ctx.stroke()
      }
      ctx.globalAlpha = 1
    }

    // ── POIs ──
    for (const p of shown) {
      const style = CATEGORY_STYLE[p.category]
      if (!style) continue
      const [px, py] = toScreen(p.x, p.y)
      if (px < -20 || py < -20 || px > size.w + 20 || py > size.h + 20) continue

      const dimmed = depth ? Math.abs(p.z - depth.focus) > depth.range : false
      ctx.globalAlpha = dimmed ? 0.2 : 1
      ctx.fillStyle = style.color
      ctx.beginPath()
      ctx.arc(px, py, 3.5, 0, Math.PI * 2)
      ctx.fill()

      if (showLabels && view.zoom > 1.8 && !dimmed) {
        ctx.font = '10px ui-monospace, monospace'
        ctx.fillStyle = 'rgba(226,232,240,0.85)'
        ctx.fillText(p.label, px + 6, py + 3)
      }
      ctx.globalAlpha = 1
    }

    // ── highlights ──
    for (const h of highlights ?? []) {
      const [px, py] = toScreen(h.x, h.y)
      ctx.strokeStyle = '#f8fafc'
      ctx.lineWidth = 2
      ctx.beginPath()
      ctx.arc(px, py, 9, 0, Math.PI * 2)
      ctx.stroke()
      ctx.beginPath()
      ctx.moveTo(px - 14, py); ctx.lineTo(px - 6, py)
      ctx.moveTo(px + 6, py); ctx.lineTo(px + 14, py)
      ctx.moveTo(px, py - 14); ctx.lineTo(px, py - 6)
      ctx.moveTo(px, py + 6); ctx.lineTo(px, py + 14)
      ctx.stroke()
      if (h.label) {
        ctx.font = '11px ui-monospace, monospace'
        ctx.fillStyle = '#f8fafc'
        ctx.fillText(h.label, px + 13, py - 10)
      }
    }

    // ── player ──
    if (playerPos) {
      const [px, py] = toScreen(playerPos.x, playerPos.y)
      ctx.save()
      ctx.translate(px, py)
      // EQ heading is 0-512 counter-clockwise with 0 = north.
      ctx.rotate(-((playerPos.heading ?? 0) / 512) * Math.PI * 2)
      ctx.fillStyle = '#f8fafc'
      ctx.beginPath()
      ctx.moveTo(0, -7)
      ctx.lineTo(4.5, 6)
      ctx.lineTo(0, 3)
      ctx.lineTo(-4.5, 6)
      ctx.closePath()
      ctx.fill()
      ctx.restore()
    }
  }, [geometry, shown, zone, size, toScreen, view.zoom, depth, highlights, playerPos, showLabels])

  // Wheel zoom is attached natively with passive:false. React registers wheel
  // as a passive listener, so preventDefault() there is ignored — it never
  // stopped the page scrolling and logged a console error on every tick (401
  // of them in one session).
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const handler = (e: WheelEvent): void => {
      e.preventDefault()
      const rect = canvas.getBoundingClientRect()
      const mx = e.clientX - rect.left
      const my = e.clientY - rect.top
      setView((v) => {
        // Zoom toward the cursor rather than the centre, so you can dive into
        // a corner without chasing it with the pan.
        const next = Math.min(40, Math.max(0.5, v.zoom * (e.deltaY < 0 ? 1.15 : 1 / 1.15)))
        const k = next / v.zoom
        return { zoom: next, panX: mx - (mx - v.panX) * k, panY: my - (my - v.panY) * k }
      })
    }
    canvas.addEventListener('wheel', handler, { passive: false })
    return () => canvas.removeEventListener('wheel', handler)
  }, [])

  const hitTest = (mx: number, my: number): MapPOI | null => {
    let best: MapPOI | null = null
    let bestD = 10
    for (const p of shown) {
      const [px, py] = toScreen(p.x, p.y)
      const d = Math.hypot(px - mx, py - my)
      if (d < bestD) {
        bestD = d
        best = p
      }
    }
    return best
  }

  return (
    <div ref={wrapRef} className="relative w-full select-none">
      <canvas
        ref={canvasRef}
        style={{
          width: '100%',
          height,
          background: 'var(--color-background)',
          cursor: drag.current ? 'grabbing' : 'grab',
          borderRadius: 6,
        }}
        onPointerDown={(e) => {
          ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
          drag.current = { x: e.clientX, y: e.clientY, panX: view.panX, panY: view.panY }
        }}
        onPointerMove={(e) => {
          // Snapshot the drag origin before calling setView. The updater runs
          // during a later render pass, by which time onPointerUp may already
          // have cleared drag.current — dereferencing it in there threw and,
          // with no boundary above this component, took the whole app down to
          // a black screen.
          const d = drag.current
          if (!d) return
          const dx = e.clientX - d.x
          const dy = e.clientY - d.y
          setView((v) => ({ ...v, panX: d.panX + dx, panY: d.panY + dy }))
        }}
        onPointerUp={(e) => {
          const moved =
            drag.current && Math.hypot(e.clientX - drag.current.x, e.clientY - drag.current.y) > 3
          drag.current = null
          if (moved || !onPOIClick) return
          const rect = canvasRef.current?.getBoundingClientRect()
          if (!rect) return
          const hit = hitTest(e.clientX - rect.left, e.clientY - rect.top)
          if (hit) onPOIClick(hit)
        }}
      />

      <div className="absolute right-2 top-2 flex flex-col items-end gap-1">
        <span
          className="rounded px-1.5 py-0.5 text-[10px]"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
        >
          {zone.technique === 'contours' ? 'elevation contours' : zone.technique} · {view.zoom.toFixed(1)}×
        </span>
        <button
          onClick={() => setView({ zoom: 1, panX: 0, panY: 0 })}
          className="rounded border px-1.5 py-0.5 text-[10px]"
          style={{
            backgroundColor: 'var(--color-surface)',
            borderColor: 'var(--color-border)',
            color: 'var(--color-muted-foreground)',
          }}
        >
          Reset view
        </button>
      </div>

      <DepthControl zone={zone} depth={depth} onChange={setDepth} />
    </div>
  )
}

// DepthControl fades geometry outside a height window.
//
// The slider is always visible rather than hidden behind a toggle: in a zone
// with stacked tunnels the flattened view is the one that needs explaining, so
// the control that fixes it should not need discovering first.
//
// A sliding window rather than a floor picker because only some zones have
// discrete storeys — akheva's floors are continuous ramps, so any level list
// would be arbitrary cuts.
function DepthControl({
  zone,
  depth,
  onChange,
}: {
  zone: MapZone
  depth: { focus: number; range: number } | null
  onChange: (d: { focus: number; range: number } | null) => void
}): React.ReactElement | null {
  // Flat zones have nothing to disambiguate.
  if (zone.z_span < 120) return null
  const lo = Math.round(-zone.z_span / 2)
  const hi = Math.round(zone.z_span / 2)
  const active = depth !== null
  const cur = depth ?? { focus: 0, range: Math.round(zone.z_span / 8) }

  return (
    <div
      className="absolute bottom-2 left-2 flex items-center gap-2 rounded px-2 py-1"
      style={{ backgroundColor: 'var(--color-surface-2)' }}
    >
      <span
        className="text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: active ? 'var(--color-primary)' : 'var(--color-muted)' }}
      >
        Depth
      </span>
      <input
        type="range"
        min={lo}
        max={hi}
        value={cur.focus}
        // Dragging the slider engages the filter, so there is no separate
        // "enable" step to find.
        onChange={(e) => onChange({ ...cur, focus: Number(e.target.value) })}
        className="w-36"
        title="Height to focus on"
      />
      <input
        type="range"
        min={Math.max(10, Math.round(zone.z_span / 40))}
        max={Math.round(zone.z_span / 2)}
        value={cur.range}
        onChange={(e) => onChange({ ...cur, range: Number(e.target.value) })}
        className="w-20"
        title="How much depth to keep in focus"
      />
      <span
        className="w-24 text-right text-[10px] tabular-nums"
        style={{ color: 'var(--color-muted)' }}
      >
        {active ? `${cur.focus} ±${cur.range}` : 'all levels'}
      </span>
      <button
        onClick={() => onChange(null)}
        disabled={!active}
        className="text-[10px] font-medium disabled:opacity-30"
        style={{ color: 'var(--color-primary)' }}
        title="Show all levels again"
      >
        Reset
      </button>
    </div>
  )
}
