import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type {
  MapGeometry,
  MapPOI,
  MapPOICategory,
  MapRenderMode,
  MapZone,
} from '../../types/map'
import type { PlayerPosition } from '../../hooks/usePlayerPosition'

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
  // detail is the optional secondary boundary layer, drawn thinner and dimmer
  // beneath the primary one so it reads as texture rather than competing with
  // the structure that defines the zone.
  detail?: MapGeometry | null
  showDetail?: boolean
  // outline is the clean layer; drawn instead of geometry+detail in outline mode.
  outline?: MapGeometry | null
  mode?: MapRenderMode
  // colorByHeight tints geometry along an elevation ramp instead of drawing it
  // in one colour.
  colorByHeight?: boolean
  pois: MapPOI[]
  // visibleCategories gates POI layers; undefined shows all.
  visibleCategories?: Set<MapPOICategory>
  // highlights are drawn prominently on top of everything else. A list rather
  // than a single pin because an NPC routinely has several spawn points, and
  // showing only one would quietly imply it is the only place it appears.
  highlights?: MapHighlight[]
  // playerPos is the live position from ZealPipes, when connected. Carries its
  // own zone, because a position from a zone other than the one on screen must
  // not be drawn — it would put the arrow at plausible-looking coordinates on
  // the wrong map, which is worse than showing nothing.
  playerPos?: PlayerPosition | null
  // followDepth drives the depth window from the player's height (5b).
  followDepth?: boolean
  // followPlayer keeps the view centred on the player.
  followPlayer?: boolean
  onPOIClick?: (poi: MapPOI) => void
  // height is a pixel height, or 'fill' to take whatever the parent gives —
  // which needs the parent to have a definite height of its own.
  height?: number | 'fill'
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
  // Hues chosen against the height ramp (#4c6ef5 through #e2703a) as well as
  // the other categories, since geometry and pins share the canvas. Pins are
  // filled discs and geometry is hairlines, which buys some tolerance, but not
  // enough to reuse a hue outright.
  locked: { color: '#fb7185', short: 'L' },   // rose — blocked
  switch: { color: '#a3e635', short: 'S' },   // lime — operate this
  teleport: { color: '#22d3ee', short: 'T' }, // cyan — movement
}

// Geometry colour by technique, in detailed mode. Contours are elevation lines,
// not walls, so they get a distinct hue — reading them as structure would be
// misleading, and the colour is the only cue that distinguishes them.
const GEOMETRY_COLOR: Record<MapZone['technique'], string> = {
  boundary: '#7dd3fc',
  contours: '#5eead4',
  silhouette: '#a78bfa',
}

// Outline mode uses one neutral colour for every zone. Colour-coding by
// technique is meaningful in detailed mode, where the lines genuinely mean
// different things; in outline mode they all mean "edge you cannot walk
// through", so varying the hue per zone would signal a difference that is not
// there — and looking the same everywhere is the whole point of the mode.
const OUTLINE_COLOR = '#cbd5e1'

// HEIGHT_RAMP tints geometry by elevation — low to high.
//
// This is the one thing hand-drawn EQ maps do that a single-colour rendering
// cannot: Brewall draws the tunnels beneath Fungus Grove and the temple above
// them in different colours, so you can tell at a glance that two lines
// crossing on screen are nowhere near each other in the world. Since every
// segment already carries its height, we can derive that instead of hand-
// authoring it.
//
// Sequential (cool -> neutral -> warm), never a rainbow: elevation is ordered
// data, and a hue cycle destroys the ordering — nothing about magenta says
// "above green". Deliberately desaturated so the saturated POI pins stay the
// most prominent thing on the canvas; the geometry is context, the pins are the
// answer to a question.
const HEIGHT_RAMP = [
  '#4c6ef5', // deepest — under everything
  '#3aa8c1',
  '#5eead4',
  '#cbd5e1', // mid — the level most zones are mostly at
  '#e8c88a',
  '#e8a04c',
  '#e2703a', // highest — rooftops, upper platforms
]

// HIGHLIGHT_COLOR marks "the thing you asked about" — an NPC's spawn point, or
// the POI just clicked.
//
// Magenta on purpose: it is the one hue not used by the height ramp (blue
// through orange) or any POI category, so a highlight can never be mistaken for
// map data no matter what it lands on top of.
const HIGHLIGHT_COLOR = '#ff4fd8'

// HEIGHT_BANDS is how many steps the ramp is quantised into. Matched to the
// ramp length: more bands than colours would repeat hues at different heights,
// which is worse than no colour at all.
const HEIGHT_BANDS = HEIGHT_RAMP.length

// NEUTRAL_BAND is the index of the uncoloured stop in the ramp.
const NEUTRAL_BAND = 3

// bandAlpha dims geometry the further it sits from the zone's main level.
//
// Colour alone told you which level a line was on but gave every level the same
// visual weight, so a zone whose cavern shell sits well above its tunnels came
// out mostly hot orange with the tunnels lost inside it. Fading by distance from
// the main level makes prominence track usefulness: the floor you are most
// likely standing on is the brightest thing, and the rest reads as context
// without losing its colour.
const bandAlpha = (band: number): number => 1 - 0.13 * Math.abs(band - NEUTRAL_BAND)

// heightScale maps a segment height to a ramp index.
//
// Anchored on the zone's *modal* height rather than stretched between its
// extremes. Anchoring to min/max sounds more principled and looks far worse:
// Fungus Grove spans -495..66 but nearly all of it sits near the top, so a
// linear stretch painted most of the zone in one hot colour and reserved the
// cool half of the ramp for a handful of segments. The map came out looking
// like a thermal camera.
//
// Anchoring the neutral stop to where the zone actually is makes colour mean
// "unusually high or low for this zone" instead of "height", which is both the
// more useful statement and what hand-drawn maps effectively say — a neutral
// base with the tunnels beneath and structures above called out.
//
// Steps stay uniform in world units, so equal colour steps are equal height
// steps; only the ends saturate.
interface HeightScale {
  bandWidth: number
  offset: number
  zLo: number
  zHi: number
  indexOf: (z: number) => number
}

function heightScale(zLo: number, zHi: number, coords: Int16Array, count: number): HeightScale {
  const bandWidth = Math.max(1, (zHi - zLo) / HEIGHT_BANDS)
  const rawBand = (z: number): number => Math.floor((z - zLo) / bandWidth)

  // Histogram of raw bands, so the busiest one can take the neutral colour.
  const hist = new Array(HEIGHT_BANDS + 2).fill(0)
  for (let i = 0; i < count; i++) {
    const o = i * 6
    const b = rawBand((coords[o + 2] + coords[o + 5]) / 2)
    hist[Math.max(0, Math.min(HEIGHT_BANDS + 1, b))]++
  }
  let modal = 0
  for (let b = 1; b < hist.length; b++) if (hist[b] > hist[modal]) modal = b

  const offset = NEUTRAL_BAND - modal
  return {
    bandWidth,
    offset,
    zLo,
    zHi,
    indexOf: (z) => Math.max(0, Math.min(HEIGHT_BANDS - 1, rawBand(z) + offset)),
  }
}

export function ZoneMap({
  zone,
  geometry,
  detail,
  showDetail = true,
  outline,
  mode = 'outline',
  colorByHeight = true,
  pois,
  visibleCategories,
  highlights,
  playerPos,
  followDepth = true,
  followPlayer = false,
  onPOIClick,
  height = 520,
  showLabels = true,
}: ZoneMapProps): React.ReactElement {
  const canvasRef = useRef<HTMLCanvasElement | null>(null)
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const [view, setView] = useState({ zoom: 1, panX: 0, panY: 0 })
  const fill = height === 'fill'
  const [size, setSize] = useState({ w: 800, h: fill ? 520 : height })
  const drag = useRef<{ x: number; y: number; panX: number; panY: number } | null>(null)

  // Depth window, as an explicit floor and ceiling height. null means "show
  // everything".
  //
  // A floor/ceiling pair rather than the focus±thickness pair this started as:
  // two independent ends is how the in-game and PQDI sliders work, it reads
  // directly as "show me between these two heights", and it can express an
  // asymmetric window — the whole zone below a ledge but nothing above it —
  // which a centre-and-width control cannot.
  const [manualDepth, setManualDepth] = useState<{ lo: number; hi: number } | null>(null)

  // Auto-depth: when the player's live position is on this map and no window has
  // been set by hand, centre one on their own height.
  //
  // This is what the Z-window work was for. Standing in a Necropolis tunnel, the
  // map shows the level you are on without touching a slider — the single most
  // useful thing the depth data can do, and it needs no input at all.
  //
  // Manual always wins, and never silently: dragging a thumb sets manualDepth,
  // Reset clears it and hands control back to auto. A window that moved under
  // you while you were reading it would be worse than no automation.
  // autoOff lets the auto window be dismissed outright. Without it the only
  // escape from a followed window was to drag both thumbs to the extremes, which
  // works but is not something anyone would guess.
  const [autoOff, setAutoOff] = useState(false)
  const autoWindow = useMemo(() => {
    if (autoOff || !followDepth || !playerPos || playerPos.zone !== zone.zone) return null
    // Wide enough to hold the floor you are on plus its walls, and scaled a
    // little by how tall the zone is.
    const half = Math.max(50, Math.round(zone.z_span / 12))
    return { lo: Math.round(playerPos.z - half), hi: Math.round(playerPos.z + half) }
  }, [autoOff, followDepth, playerPos, zone.zone, zone.z_span])

  const depth = manualDepth ?? autoWindow
  const setDepth = setManualDepth

  const shown = useMemo(
    () => pois.filter((p) => !visibleCategories || visibleCategories.has(p.category)),
    [pois, visibleCategories],
  )

  // Height tinting is only meaningful where there is vertical range to describe.
  const tinted = colorByHeight && zone.z_span >= 40
  // Built from the layer actually being drawn, since the modal height depends on
  // which segments are on screen.
  const primaryLayer = mode === 'outline' ? outline : geometry
  const zScale = useMemo(() => {
    if (!tinted || !primaryLayer || primaryLayer.count === 0) return null
    return heightScale(
      zone.min_z,
      Math.max(zone.max_z, zone.min_z + 1),
      primaryLayer.coords,
      primaryLayer.count,
    )
  }, [tinted, primaryLayer, zone.min_z, zone.max_z])


  useEffect(() => {
    const el = wrapRef.current
    if (!el) return
    // In fill mode the wrapper's height comes from the parent's layout, not
    // from its contents, so measuring it here is not circular.
    const measure = (): void =>
      setSize({ w: el.clientWidth, h: fill ? el.clientHeight : (height as number) })
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    measure()
    return () => ro.disconnect()
  }, [height, fill])

  // Reset the view whenever the zone changes, so switching zones doesn't leave
  // you panned off into empty space.
  useEffect(() => {
    setView({ zoom: 1, panX: 0, panY: 0 })
    setManualDepth(null)
    setAutoOff(false)
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

  // Follow mode: keep the player centred.
  //
  // Written as a pan correction rather than by deriving the transform from the
  // player each frame, so zoom and the fit-to-zone maths stay untouched and
  // switching follow off leaves the view exactly where it was rather than
  // snapping.
  //
  // Deliberately not in the same effect as the canvas draw: this sets state, and
  // running it from the draw effect would make every frame schedule a re-render.
  useEffect(() => {
    if (!followPlayer || !playerPos || playerPos.zone !== zone.zone) return
    const cx = playerPos.x * base.scale + base.offX
    const cy = playerPos.y * base.scale + base.offY
    setView((v) => {
      const panX = size.w / 2 - cx * v.zoom
      const panY = size.h / 2 - cy * v.zoom
      // Skip sub-pixel corrections, or a stationary player re-renders forever on
      // every heartbeat.
      if (Math.abs(panX - v.panX) < 0.5 && Math.abs(panY - v.panY) < 0.5) return v
      return { ...v, panX, panY }
    })
  }, [followPlayer, playerPos, zone.zone, base, size.w, size.h])

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
    //
    // Line weight carries hierarchy, which is most of what separates a drawn
    // map from a wireframe dump. The primary layer defines the zone and gets
    // full weight; the detail layer is texture and gets a hairline, so it adds
    // information without competing.
    const drawLayer = (
      g: MapGeometry,
      color: string,
      width: number,
      alphaScale: number,
    ): void => {
      const c = g.coords
      ctx.lineWidth = width
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
      const ALPHAS = (depth ? [1, 0.5, 0.25, 0.1] : [1]).map((a) => a * alphaScale)
      // Fade steps are sized relative to the window, so a narrow window fades
      // off sharply and a wide one trails away gently.
      const step = Math.max(1, (depth ? depth.hi - depth.lo : 0) / 2)
      const bucketOf = (zMid: number): number => {
        if (!depth) return 0
        const d = zMid < depth.lo ? depth.lo - zMid : zMid > depth.hi ? zMid - depth.hi : 0
        if (d === 0) return 0
        return Math.min(ALPHAS.length - 1, 1 + Math.floor(d / step))
      }

      // One path per (opacity bucket, height band). Both are derived from the
      // same Z, so a single pass assigns each segment to its pair and the total
      // work stays one visit per segment per bucket — the same as before colour
      // was added, rather than one extra full scan per ramp step.
      const nBands = zScale ? HEIGHT_BANDS : 1
      const paths: number[][] = Array.from({ length: ALPHAS.length * nBands }, () => [])
      for (let i = 0; i < g.count; i++) {
        const o = i * 6
        const zMid = (c[o + 2] + c[o + 5]) / 2
        const band = zScale ? zScale.indexOf(zMid) : 0
        paths[bucketOf(zMid) * nBands + band].push(i)
      }

      // Draw faintest first so in-focus lines land on top, and within an
      // opacity bucket draw low bands first so upper structures read as being
      // on top of what they sit above.
      for (let b = ALPHAS.length - 1; b >= 0; b--) {
        for (let band = 0; band < nBands; band++) {
          const idx = paths[b * nBands + band]
          if (idx.length === 0) continue
          ctx.beginPath()
          ctx.strokeStyle = zScale ? HEIGHT_RAMP[band] : color
          ctx.globalAlpha = ALPHAS[b] * (zScale ? bandAlpha(band) : 1)
          for (const i of idx) {
            const o = i * 6
            const [x1, y1] = toScreen(c[o], c[o + 1])
            const [x2, y2] = toScreen(c[o + 3], c[o + 4])
            ctx.moveTo(x1, y1)
            ctx.lineTo(x2, y2)
          }
          ctx.stroke()
        }
      }
      ctx.globalAlpha = 1
    }

    if (mode === 'outline') {
      // A touch heavier than the detailed primary: it is the only layer, so it
      // carries the whole drawing.
      if (outline && outline.count > 0) drawLayer(outline, OUTLINE_COLOR, 1.3, 1)
    } else {
      const primaryColor = GEOMETRY_COLOR[zone.technique] ?? '#7dd3fc'
      // Detail underneath, so primary structure always reads on top.
      if (showDetail && detail && detail.count > 0) {
        drawLayer(detail, primaryColor, 0.55, 0.45)
      }
      if (geometry && geometry.count > 0) {
        drawLayer(geometry, primaryColor, 1.15, 1)
      }
    }

    // ── POIs ──
    //
    // Labels are decluttered: one is skipped if its box overlaps a label
    // already drawn. Without this, dense zones stack several labels on the same
    // few pixels and the glyphs superimpose into something that reads as
    // corrupted text rather than as overlapping words.
    //
    // Pins are always drawn — only the text is suppressed, so nothing is hidden,
    // and zooming in frees space for more labels.
    const placed: { x0: number; y0: number; x1: number; y1: number }[] = []
    const fitsLabel = (x: number, y: number, w: number): boolean => {
      const box = { x0: x, y0: y - 9, x1: x + w, y1: y + 3 }
      for (const b of placed) {
        if (box.x0 < b.x1 && box.x1 > b.x0 && box.y0 < b.y1 && box.y1 > b.y0) return false
      }
      placed.push(box)
      return true
    }

    ctx.font = '10px ui-monospace, monospace'
    for (const p of shown) {
      const style = CATEGORY_STYLE[p.category]
      if (!style) continue
      const [px, py] = toScreen(p.x, p.y)
      if (px < -20 || py < -20 || px > size.w + 20 || py > size.h + 20) continue

      const dimmed = depth ? p.z < depth.lo || p.z > depth.hi : false
      ctx.globalAlpha = dimmed ? 0.2 : 1
      ctx.fillStyle = style.color
      ctx.beginPath()
      ctx.arc(px, py, 3.5, 0, Math.PI * 2)
      ctx.fill()

      if (showLabels && view.zoom > 1.8 && !dimmed) {
        const w = ctx.measureText(p.label).width
        if (fitsLabel(px + 6, py + 3, w)) {
          ctx.fillStyle = 'rgba(226,232,240,0.85)'
          ctx.fillText(p.label, px + 6, py + 3)
        }
      }
      ctx.globalAlpha = 1
    }

    // ── highlights ──
    for (const h of highlights ?? []) {
      const [px, py] = toScreen(h.x, h.y)
      // Drawn twice: a dark casing first, then the marker on top.
      //
      // A single stroke cannot be relied on to contrast with anything, and the
      // white it used to be became invisible the moment outline mode started
      // drawing the geometry in near-white — the crosshairs vanished into the
      // walls in exactly the view where "where is this NPC" matters most.
      // Casing-then-stroke is how map symbols stay legible over arbitrary
      // background, and it costs one extra pass over a handful of markers.
      const ring = (): void => {
        ctx.beginPath()
        ctx.arc(px, py, 9, 0, Math.PI * 2)
        ctx.moveTo(px - 14, py); ctx.lineTo(px - 6, py)
        ctx.moveTo(px + 6, py); ctx.lineTo(px + 14, py)
        ctx.moveTo(px, py - 14); ctx.lineTo(px, py - 6)
        ctx.moveTo(px, py + 6); ctx.lineTo(px, py + 14)
        ctx.stroke()
      }
      ctx.lineWidth = 4.5
      ctx.strokeStyle = 'rgba(0,0,0,0.75)'
      ring()
      ctx.lineWidth = 2
      ctx.strokeStyle = HIGHLIGHT_COLOR
      ring()

      if (h.label) {
        ctx.font = '11px ui-monospace, monospace'
        ctx.lineWidth = 3
        ctx.strokeStyle = 'rgba(0,0,0,0.75)'
        ctx.strokeText(h.label, px + 13, py - 10)
        ctx.fillStyle = HIGHLIGHT_COLOR
        ctx.fillText(h.label, px + 13, py - 10)
      }
    }

    // ── player ──
    if (playerPos && playerPos.zone === zone.zone) {
      const [px, py] = toScreen(playerPos.x, playerPos.y)
      ctx.save()
      ctx.translate(px, py)
      // EQ heading is 0-512 counter-clockwise with 0 = north.
      ctx.rotate(-((playerPos.heading ?? 0) / 512) * Math.PI * 2)
      ctx.beginPath()
      ctx.moveTo(0, -7)
      ctx.lineTo(4.5, 6)
      ctx.lineTo(0, 3)
      ctx.lineTo(-4.5, 6)
      ctx.closePath()
      // Cased like the highlights, for the same reason: white-on-white in
      // outline mode is invisible.
      ctx.lineWidth = 3
      ctx.strokeStyle = 'rgba(0,0,0,0.75)'
      ctx.stroke()
      ctx.fillStyle = '#f8fafc'
      ctx.fill()
      ctx.restore()
    }
  }, [geometry, shown, zone, size, toScreen, view.zoom, depth, highlights, playerPos, showLabels, detail, showDetail, outline, mode, zScale])

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
    <div ref={wrapRef} className={`relative w-full select-none${fill ? ' h-full' : ''}`}>
      <canvas
        ref={canvasRef}
        style={{
          width: '100%',
          height: fill ? '100%' : height,
          // Canvas is inline by default, so without this the baseline gap
          // leaves a few stray pixels under it in fill mode.
          display: 'block',
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
          {mode === 'outline'
            ? 'outline'
            : zone.technique === 'contours'
              ? 'elevation contours'
              : zone.technique}{' '}
          · {view.zoom.toFixed(1)}×
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

      {zScale && <HeightLegend scale={zScale} />}
      <DepthControl
        zone={zone}
        depth={depth}
        onChange={setDepth}
        auto={manualDepth === null && autoWindow !== null}
        onDisableAuto={() => setAutoOff(true)}
        // Reset means "back to the default for this situation": the followed
        // window when a live position is available, all levels otherwise. It has
        // to be enabled after auto is dismissed too, or dismissing it would be a
        // one-way door.
        canReset={manualDepth !== null || autoOff}
        onReset={() => {
          setManualDepth(null)
          setAutoOff(false)
        }}
      />
    </div>
  )
}

// HeightLegend keys the elevation ramp to actual world heights.
//
// Not optional decoration: a colour ramp with no key is just decoration. The
// point of tinting by height is to know that an orange line is above a blue one
// and roughly by how much, and the numbers are what make that readable rather
// than merely pretty. Values are z as EQ reports it, so they can be compared
// against /loc directly.
function HeightLegend({ scale }: { scale: HeightScale }): React.ReactElement {
  // Each stop is shown at the width of the height range that actually maps to
  // it, so the legend describes the anchored scale rather than a plain min-max
  // gradient. The two end stops absorb everything past the ramp and so come out
  // wider; drawing them all equal would misstate the mapping.
  const { zLo, zHi, bandWidth, offset } = scale
  const cells = HEIGHT_RAMP.map((color, ci) => {
    // Raw band ci - offset maps here; the first and last stops also absorb
    // everything beyond the ramp in their direction.
    const from = ci === 0 ? zLo : zLo + (ci - offset) * bandWidth
    const to = ci === HEIGHT_RAMP.length - 1 ? zHi : zLo + (ci - offset + 1) * bandWidth
    const lo = Math.max(zLo, Math.min(zHi, from))
    const hi = Math.max(zLo, Math.min(zHi, to))
    return { color, span: Math.max(0, hi - lo) }
  }).filter((c) => c.span > 0)

  const total = cells.reduce((a, c) => a + c.span, 0) || 1

  return (
    <div
      className="absolute right-2 bottom-2 flex items-center gap-1.5 rounded px-2 py-1"
      style={{ backgroundColor: 'var(--color-surface-2)' }}
      title="Line colour is height: cool below the zone's main level, warm above it"
    >
      <span className="text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {Math.round(zLo)}
      </span>
      <div className="flex overflow-hidden rounded" style={{ height: 5, width: 90 }}>
        {cells.map((c) => (
          <div
            key={c.color}
            style={{ backgroundColor: c.color, width: `${(c.span / total) * 100}%` }}
          />
        ))}
      </div>
      <span className="text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {Math.round(zHi)}
      </span>
    </div>
  )
}

// DepthControl fades geometry outside a height window, set by dragging a floor
// and a ceiling along one track.
//
// Always visible rather than hidden behind a toggle: in a zone with stacked
// tunnels the flattened view is the one that needs explaining, so the control
// that fixes it should not need discovering first.
//
// A continuous window rather than a list of floors because only some zones have
// discrete storeys — Akheva's levels are continuous ramps, so any level list
// would be arbitrary cuts.
function DepthControl({
  zone,
  depth,
  onChange,
  auto = false,
  onDisableAuto,
  canReset = false,
  onReset,
}: {
  zone: MapZone
  depth: { lo: number; hi: number } | null
  onChange: (d: { lo: number; hi: number } | null) => void
  onDisableAuto?: () => void
  canReset?: boolean
  onReset?: () => void
  // auto reports that the window is tracking the player's height rather than
  // having been set by hand. Surfaced because a control that moves on its own
  // has to say so — otherwise it reads as a bug.
  auto?: boolean
}): React.ReactElement | null {
  // Flat zones have nothing to disambiguate.
  if (zone.z_span < 120) return null

  const floor = zone.min_z
  const ceil = Math.max(zone.max_z, zone.min_z + 1)
  const active = depth !== null
  const cur = depth ?? { lo: floor, hi: ceil }

  // Thumbs cannot cross: dragging one past the other pushes it instead, which is
  // what every dual-range control does and avoids an inverted, empty window.
  // Step proportional to the zone, not 1 unit: a 560-unit zone would otherwise
  // take 560 arrow presses to cross, and single-unit precision is meaningless
  // against geometry banded tens of units apart.
  const step = Math.max(1, Math.round((ceil - floor) / 120))
  const pct = (v: number): number => ((v - floor) / (ceil - floor)) * 100
  const setLo = (v: number): void => onChange({ lo: Math.min(v, cur.hi - 1), hi: cur.hi })
  const setHi = (v: number): void => onChange({ lo: cur.lo, hi: Math.max(v, cur.lo + 1) })

  return (
    <div
      className="absolute bottom-2 left-2 flex items-center gap-2.5 rounded px-2.5 py-1.5"
      style={{ backgroundColor: 'var(--color-surface-2)' }}
    >
      <span
        className="text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: active ? 'var(--color-primary)' : 'var(--color-muted)' }}
        title={
          auto
            ? 'Following your height in game — drag either end to take over'
            : 'Fade everything outside a height range, to read one level of a multi-level zone'
        }
      >
        Height
      </span>

      <div className="range-dual w-44">
        <div className="range-dual-track" />
        <div
          className="range-dual-fill"
          style={{ left: `${pct(cur.lo)}%`, width: `${pct(cur.hi) - pct(cur.lo)}%` }}
        />
        {/* Dragging either thumb engages the filter, so there is no separate
            "enable" step to find. */}
        <input
          type="range"
          min={floor}
          max={ceil}
          step={step}
          value={cur.lo}
          onChange={(e) => setLo(Number(e.target.value))}
          title="Floor — hide everything below this height"
        />
        <input
          type="range"
          min={floor}
          max={ceil}
          step={step}
          value={cur.hi}
          onChange={(e) => setHi(Number(e.target.value))}
          title="Ceiling — hide everything above this height"
        />
      </div>

      <span
        className="w-24 text-right text-[10px] tabular-nums"
        style={{ color: 'var(--color-muted)' }}
      >
        {active ? `${cur.lo} → ${cur.hi}` : 'all levels'}
      </span>
      {auto && (
        <button
          onClick={onDisableAuto}
          className="rounded px-1 text-[9px] font-semibold uppercase tracking-wider"
          style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
          title="Following your height in game — click to stop and show all levels"
        >
          Auto
        </button>
      )}
      <button
        onClick={() => (onReset ? onReset() : onChange(null))}
        disabled={!active && !canReset}
        className="text-[10px] font-medium disabled:opacity-30"
        style={{ color: 'var(--color-primary)' }}
        title="Back to the default — your own height when in this zone, all levels otherwise"
      >
        Reset
      </button>
    </div>
  )
}
