import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { PoPFlagStatus } from '../types/popflag'
import { PopFlagRow } from '../components/PopFlagRow'
import { ZONE_ORDER, zoneColor } from '../lib/popFlagKind'

// PoPFlagFlowPanel renders the PoP flag progression as a tiered, zone-grouped
// flowchart shaped like Grimrose's (SoS) hand-verified bare-minimum flagging
// chart: one horizontal band per tier, one card per zone within a band (in
// Grimrose's reading order — see ZONE_ORDER), each card listing its own steps
// top-to-bottom exactly like the Checklist. Arrows connect a whole ZONE to the
// zone(s) it unlocks — not individual steps — matching how Grimrose's chart
// itself is drawn; a step-by-step dependency graph is what the old Graph view
// (removed) tried to be and was hard to read at ~60 nodes.
//
// Layout is plain CSS flex (bands wrap naturally at narrow widths); arrows are
// a single SVG overlay whose paths are computed from the zone cards' actual
// measured DOM rects, recomputed on resize. No graph library — the structure
// here (a handful of zones per tier, mostly tier-to-tier edges) doesn't need
// one, and a hand-rolled layout can match Grimrose's own box-and-arrow shape
// directly instead of an auto-layout algorithm's guess at it.

interface PoPFlagFlowPanelProps {
  flags: PoPFlagStatus[]
  canToggle: boolean
  busyId: string | null
  onToggle: (flag: PoPFlagStatus) => void
  onConfirm: (flag: PoPFlagStatus) => void
  requiredByDone: Set<string>
}

interface ZoneGroup {
  zone: string
  zoneShort: string
  tier: number
  flags: PoPFlagStatus[]
}

type ZoneStatus = 'done' | 'available' | 'locked'

function tierLabel(t: number): string {
  return t === 5 ? 'Plane of Time' : `Tier ${t}`
}

// zoneStatus summarises a zone card's border colour: done when every required
// (non-optional, non-group-member) step is done, locked when none of them are
// even reachable yet, available otherwise (in progress or ready to start).
function zoneStatus(flags: PoPFlagStatus[]): ZoneStatus {
  const relevant = flags.filter((f) => !f.optional && !f.group)
  if (relevant.length === 0) return 'available'
  if (relevant.every((f) => f.done)) return 'done'
  if (relevant.every((f) => f.locked && !f.done)) return 'locked'
  return 'available'
}

const STATUS_BORDER: Record<ZoneStatus, string> = {
  done: 'var(--color-success)',
  available: 'var(--color-border)',
  locked: 'rgba(248,113,113,0.45)',
}

interface Point { x: number; y: number }

// anchorPoints picks which pair of edges (top/bottom or left/right) to
// connect two rects with, based on which axis dominates their relative
// position — so a tier-to-tier edge (mostly vertical) reads top-to-bottom and
// a same-tier edge (mostly horizontal) reads left-to-right, matching how
// Grimrose draws each case.
function anchorPoints(from: DOMRect, to: DOMRect): { from: Point; to: Point; vertical: boolean } {
  const fc = { x: from.left + from.width / 2, y: from.top + from.height / 2 }
  const tc = { x: to.left + to.width / 2, y: to.top + to.height / 2 }
  const dy = tc.y - fc.y
  const dx = tc.x - fc.x
  if (Math.abs(dy) >= Math.abs(dx)) {
    return dy >= 0
      ? { from: { x: fc.x, y: from.bottom }, to: { x: tc.x, y: to.top }, vertical: true }
      : { from: { x: fc.x, y: from.top }, to: { x: tc.x, y: to.bottom }, vertical: true }
  }
  return dx >= 0
    ? { from: { x: from.right, y: fc.y }, to: { x: to.left, y: tc.y }, vertical: false }
    : { from: { x: from.left, y: fc.y }, to: { x: to.right, y: tc.y }, vertical: false }
}

function curvePath(from: Point, to: Point, vertical: boolean): string {
  if (vertical) {
    const midY = (from.y + to.y) / 2
    return `M ${from.x} ${from.y} C ${from.x} ${midY}, ${to.x} ${midY}, ${to.x} ${to.y}`
  }
  const midX = (from.x + to.x) / 2
  return `M ${from.x} ${from.y} C ${midX} ${from.y}, ${midX} ${to.y}, ${to.x} ${to.y}`
}

interface EdgePath {
  key: string
  from: string
  to: string
  d: string
  mid: Point
}

interface JoinBadge {
  zone: string
  pos: Point
}

export default function PoPFlagFlowPanel({
  flags, canToggle, busyId, onToggle, onConfirm, requiredByDone,
}: PoPFlagFlowPanelProps): React.ReactElement {
  // Group flags into zones (dataset order preserved within a zone), ordered
  // per ZONE_ORDER within each tier; an unlisted zone falls back to
  // first-seen order, appended after the known ones.
  const zones = useMemo<ZoneGroup[]>(() => {
    const byZone = new Map<string, ZoneGroup>()
    const firstSeen: string[] = []
    for (const f of flags) {
      let g = byZone.get(f.zone)
      if (!g) {
        g = { zone: f.zone, zoneShort: f.zone_short, tier: f.tier, flags: [] }
        byZone.set(f.zone, g)
        firstSeen.push(f.zone)
      }
      g.flags.push(f)
    }
    const order = [...ZONE_ORDER, ...firstSeen.filter((z) => !ZONE_ORDER.includes(z))]
    return order.filter((z) => byZone.has(z)).map((z) => byZone.get(z)!)
  }, [flags])

  const tiers = useMemo(() => {
    const byTier = new Map<number, ZoneGroup[]>()
    for (const z of zones) {
      if (!byTier.has(z.tier)) byTier.set(z.tier, [])
      byTier.get(z.tier)!.push(z)
    }
    return [...byTier.entries()].sort((a, b) => a[0] - b[0])
  }, [zones])

  // Cross-zone prereq edges, deduped to (sourceZone -> targetZone) pairs. A
  // target fed by 2+ distinct source zones gets an "ALL" badge (Grimrose
  // marks these joins explicitly, e.g. Plane of Torment needing both Crypt of
  // Decay and Plane of Nightmares).
  const { edgeList, joinZones } = useMemo(() => {
    const zoneOf = new Map(flags.map((f) => [f.id, f.zone]))
    const incoming = new Map<string, Set<string>>() // targetZone -> sourceZones
    for (const f of flags) {
      for (const p of f.prereqs) {
        const src = zoneOf.get(p)
        if (!src || src === f.zone) continue
        if (!incoming.has(f.zone)) incoming.set(f.zone, new Set())
        incoming.get(f.zone)!.add(src)
      }
    }
    const list: { from: string; to: string }[] = []
    const joins = new Set<string>()
    for (const [to, froms] of incoming) {
      if (froms.size > 1) joins.add(to)
      for (const from of froms) list.push({ from, to })
    }
    return { edgeList: list, joinZones: joins }
  }, [flags])

  const [focusedZone, setFocusedZone] = useState<string | null>(null)
  const inFocus = useMemo(() => {
    if (!focusedZone) return null
    const s = new Set<string>([focusedZone])
    for (const e of edgeList) {
      if (e.from === focusedZone) s.add(e.to)
      if (e.to === focusedZone) s.add(e.from)
    }
    return s
  }, [focusedZone, edgeList])

  const toggleFocus = useCallback((zone: string) => {
    setFocusedZone((cur) => (cur === zone ? null : zone))
  }, [])

  // ── Arrow layout — measured from the actual rendered zone cards ───────────
  const contentRef = useRef<HTMLDivElement | null>(null)
  const cardRefs = useRef(new Map<string, HTMLDivElement>())
  const setCardRef = useCallback((zone: string) => (el: HTMLDivElement | null) => {
    if (el) cardRefs.current.set(zone, el)
    else cardRefs.current.delete(zone)
  }, [])

  const [paths, setPaths] = useState<EdgePath[]>([])
  const [badges, setBadges] = useState<JoinBadge[]>([])
  const [svgSize, setSvgSize] = useState({ w: 0, h: 0 })

  const recompute = useCallback(() => {
    const content = contentRef.current
    if (!content) return
    const cRect = content.getBoundingClientRect()
    setSvgSize({ w: content.scrollWidth, h: content.scrollHeight })

    const nextPaths: EdgePath[] = []
    for (const e of edgeList) {
      const fromEl = cardRefs.current.get(e.from)
      const toEl = cardRefs.current.get(e.to)
      if (!fromEl || !toEl) continue
      const fr = fromEl.getBoundingClientRect()
      const tr = toEl.getBoundingClientRect()
      const { from, to, vertical } = anchorPoints(fr, tr)
      const rel = (p: Point): Point => ({ x: p.x - cRect.left, y: p.y - cRect.top })
      const f = rel(from)
      const t = rel(to)
      nextPaths.push({
        key: `${e.from}->${e.to}`,
        from: e.from,
        to: e.to,
        d: curvePath(f, t, vertical),
        mid: { x: (f.x + t.x) / 2, y: (f.y + t.y) / 2 },
      })
    }
    setPaths(nextPaths)

    const nextBadges: JoinBadge[] = []
    for (const zone of joinZones) {
      const el = cardRefs.current.get(zone)
      if (!el) continue
      const r = el.getBoundingClientRect()
      nextBadges.push({ zone, pos: { x: r.left - cRect.left + r.width / 2, y: r.top - cRect.top - 9 } })
    }
    setBadges(nextBadges)
  }, [edgeList, joinZones])

  useLayoutEffect(() => {
    recompute()
  }, [recompute, zones])

  useEffect(() => {
    const content = contentRef.current
    if (!content) return
    const ro = new ResizeObserver(() => recompute())
    ro.observe(content)
    window.addEventListener('resize', recompute)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', recompute)
    }
  }, [recompute])

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 px-4 py-2 shrink-0">
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Arrows connect a zone to what it unlocks. Click a zone's header to highlight what feeds it and what it
          feeds; click again to reset.
        </p>
        {joinZones.size > 0 && (
          <span
            className="ml-auto shrink-0 rounded px-1.5 py-0.5 text-[10px] uppercase tracking-wider"
            style={{ color: '#fb923c', backgroundColor: 'rgba(251,146,60,0.12)', border: '1px solid rgba(251,146,60,0.4)' }}
          >
            ALL = every source zone required
          </span>
        )}
      </div>
      <div className="flex-1 overflow-auto p-4" style={{ backgroundColor: 'var(--color-surface)' }}>
        <div ref={contentRef} className="relative flex flex-col gap-3">
          <svg
            width={svgSize.w}
            height={svgSize.h}
            style={{ position: 'absolute', inset: 0, pointerEvents: 'none', overflow: 'visible' }}
          >
            <defs>
              <marker id="popflow-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M 0 0 L 10 5 L 0 10 z" fill="#4b5563" />
              </marker>
            </defs>
            {paths.map((p) => {
              const dimmed = inFocus ? !(inFocus.has(p.from) && inFocus.has(p.to)) : false
              return (
                <path
                  key={p.key}
                  d={p.d}
                  fill="none"
                  stroke="#4b5563"
                  strokeWidth={1.5}
                  opacity={dimmed ? 0.08 : 0.75}
                  markerEnd="url(#popflow-arrow)"
                />
              )
            })}
          </svg>
          {badges.map((b) => {
            const dimmed = inFocus ? !inFocus.has(b.zone) : false
            return (
              <div
                key={b.zone}
                style={{
                  position: 'absolute',
                  left: b.pos.x,
                  top: b.pos.y,
                  transform: 'translate(-50%, -100%)',
                  opacity: dimmed ? 0.15 : 1,
                  pointerEvents: 'none',
                }}
                className="rounded px-1 py-0.5 text-[9px] font-bold uppercase tracking-wider"
              >
                <span style={{ color: '#fb923c', backgroundColor: '#0b0f17', padding: '0 3px', borderRadius: 3 }}>
                  ALL
                </span>
              </div>
            )
          })}

          {tiers.map(([tier, tierZones]) => (
            <div key={tier} className="flex items-stretch gap-3">
              <div
                className="flex shrink-0 items-center justify-center text-[10px] font-bold uppercase tracking-widest"
                style={{
                  writingMode: 'vertical-rl',
                  transform: 'rotate(180deg)',
                  color: 'var(--color-muted)',
                  width: 20,
                }}
              >
                {tierLabel(tier)}
              </div>
              <div className="flex flex-1 flex-wrap items-start gap-3">
                {tierZones.map((z) => {
                  const status = zoneStatus(z.flags)
                  const dimmed = inFocus ? !inFocus.has(z.zone) : false
                  const accent = zoneColor(z.zone)
                  return (
                    <div
                      key={z.zone}
                      ref={setCardRef(z.zone)}
                      className="flex flex-col rounded-lg overflow-hidden shrink-0"
                      style={{
                        width: 260,
                        backgroundColor: 'var(--color-surface-2)',
                        border: `1px solid ${STATUS_BORDER[status]}`,
                        opacity: dimmed ? 0.25 : 1,
                        transition: 'opacity 0.15s ease',
                      }}
                    >
                      <button
                        type="button"
                        onClick={() => toggleFocus(z.zone)}
                        className="flex items-center justify-between px-2.5 py-1.5 text-left"
                        style={{ backgroundColor: `${accent}26`, borderBottom: `2px solid ${accent}` }}
                        title="Click to highlight what feeds this zone and what it unlocks"
                      >
                        <span className="text-[11px] font-semibold" style={{ color: accent }}>
                          {z.zone}
                        </span>
                        <span className="text-[9px] uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
                          {z.zoneShort}
                        </span>
                      </button>
                      <div className="flex-1">
                        {z.flags.map((f) => (
                          <PopFlagRow
                            key={f.id}
                            flag={f}
                            allFlags={flags}
                            requiredByDone={requiredByDone}
                            canToggle={canToggle}
                            busy={busyId === f.id}
                            onToggle={onToggle}
                            onConfirm={onConfirm}
                            compact
                          />
                        ))}
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
