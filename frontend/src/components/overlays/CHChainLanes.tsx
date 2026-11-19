/**
 * CHChainLanes — the alternate "lanes" style for the CH Chain overlay, shared
 * by the dashboard card (CHChainPanel) and the popped-out window
 * (CHChainOverlayWindowPage). Requested by clerics coming from Quarm Tools.
 *
 * One horizontal lane per heal target. Each lane is a name badge followed by
 * a track of one column per second of the CH cast (1 … 10). Every callout is
 * a block labelled with its chain number that slides right → left as the
 * cast runs; the heal lands when the block's left edge reaches the name
 * badge. Purely a different rendering of the same ch_chain / ch_chain_2
 * timer feed the list view reads — no timing logic of its own beyond
 * interpolating position between WebSocket broadcasts.
 */
import React, { useEffect, useRef, useState } from 'react'
import { GanttChart, List } from 'lucide-react'
import type { ActiveTimer } from '../../types/timer'
import type { TimerAppearance } from '../../hooks/useTimerAppearance'
import { parseLabel, positionLetter } from '../../lib/chChainLabel'
import { chainPositionColor } from '../../lib/chChainPositionColor'
import type { ChainStyle } from '../../lib/chChainStyle'

// Seconds of cast the track spans when no timer says otherwise. Complete
// Heal (and Druid Renewal) are 10s casts.
const DEFAULT_SPAN = 10
// Repaint cadence for the interpolated block positions (~30fps is smooth
// enough for a block crossing a few hundred px in 10s).
const FRAME_MS = 33
// After a callout lands the backend drops its timer, but Quarm Tools keeps
// the block sliding on underneath the name badge until it's gone. Keep a
// landed block around this long past landing so it can do the same (the
// track clips it at the badge edge).
const LAND_OVERRUN_SECS = 1.2
// Only callouts this close to landing when they leave the feed count as
// "landed". Anything else (cleared chain, tank swap) just disappears.
const LANDED_EPSILON_SECS = 0.6
// Same "landing soon" cut-off as the list view's bar (last third of the cast).
const LANDING_FRACTION = 0.34
// Block colours match the list view's bar states: counting down, landing,
// possible miss.
const BLOCK_BLUE = '59,130,246'
const BLOCK_GREEN = '34,197,94'
const BLOCK_RED = '239,68,68'

const MISS_TITLE =
  "No confirming cast-begin line seen for this callout's caster in time — may have fizzled, been interrupted, or skipped"

type Variant = 'overlay' | 'panel'

interface Palette {
  track: string
  divider: string
  colLabel: string
  badgeBg: string
  badgeBorder: string
  badgeText: string
}

// Neutral surfaces and muted labels like the rest of the app's overlays; the
// only colour comes from the blocks themselves.
const PALETTES: Record<Variant, Palette> = {
  overlay: {
    track: 'rgba(0,0,0,0.25)',
    divider: 'rgba(255,255,255,0.08)',
    colLabel: 'rgba(255,255,255,0.22)',
    badgeBg: 'rgba(255,255,255,0.07)',
    badgeBorder: 'rgba(255,255,255,0.12)',
    badgeText: 'rgba(255,255,255,0.9)',
  },
  panel: {
    track: 'var(--color-surface)',
    divider: 'var(--color-border)',
    colLabel: 'var(--color-muted)',
    badgeBg: 'var(--color-surface-2)',
    badgeBorder: 'var(--color-border)',
    badgeText: 'var(--color-foreground)',
  },
}

// useNowMs ticks a wall clock while `active`, so blocks glide between the
// backend's once-a-second timer broadcasts instead of stepping.
function useNowMs(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    let raf = 0
    let last = 0
    const loop = (t: number): void => {
      if (t - last >= FRAME_MS) {
        last = t
        setNow(Date.now())
      }
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    return () => cancelAnimationFrame(raf)
  }, [active])
  return now
}

// rawRemaining interpolates a callout's time-to-land from its fixed start
// time (negative once it has landed). Falls back to the broadcast value if
// starts_at can't be parsed.
function rawRemaining(t: ActiveTimer, now: number): number {
  const start = Date.parse(t.starts_at)
  if (Number.isNaN(start)) return t.remaining_seconds
  return t.duration_seconds - (now - start) / 1000
}

// blockLabel is the block's text: the chain number on the main chain, the
// letter on the secondary (ramp) chain, as in the list view's badge.
function blockLabel(position: number, letters: boolean): string {
  if (position === Number.MAX_SAFE_INTEGER) return '?'
  if (letters) return positionLetter(position)
  return String(position)
}

function blockTitle(t: ActiveTimer, missed: boolean): string {
  const parts = [t.spell_name.replace(/\s{2,}/g, ' ')]
  if (t.caster_mana_pct !== undefined) parts.push(`${t.caster_mana_pct}% mana`)
  if (missed) parts.push(MISS_TITLE)
  return parts.join(' · ')
}

export default function CHChainLanes({
  timers,
  letters,
  appearance,
  variant,
  placeholder,
}: {
  /** The active chain's timers, already filtered and sorted by start. */
  timers: ActiveTimer[]
  letters: boolean
  appearance: TimerAppearance
  variant: Variant
  /** Shown when there's nothing to draw. Rendered here rather than by the
   *  caller so this component stays mounted while landed blocks finish
   *  sliding under the badge after the feed empties. */
  placeholder?: React.ReactNode
}): React.ReactElement {
  const palette = PALETTES[variant]

  // Landed blocks ("ghosts") that keep sliding under the name badge after
  // their timer leaves the feed. Bookkept against the wall clock here, in
  // render, so the animation clock below knows whether it still has work.
  const prevRef = useRef<Map<string, ActiveTimer>>(new Map())
  const ghostsRef = useRef<Map<string, ActiveTimer>>(new Map())
  const wall = Date.now()
  const liveIds = new Set(timers.map((t) => t.id))
  for (const [id, t] of prevRef.current) {
    if (!liveIds.has(id) && rawRemaining(t, wall) <= LANDED_EPSILON_SECS) {
      ghostsRef.current.set(id, t)
    }
  }
  for (const [id, t] of ghostsRef.current) {
    if (liveIds.has(id) || rawRemaining(t, wall) < -LAND_OVERRUN_SECS) {
      ghostsRef.current.delete(id)
    }
  }
  prevRef.current = new Map(timers.map((t) => [t.id, t]))
  const ghosts = [...ghostsRef.current.values()]
  const now = useNowMs(timers.length > 0 || ghosts.length > 0)
  const shown = [...timers, ...ghosts]

  // Lane order = order each target first appeared, kept stable while that
  // target has a live callout so lanes never reshuffle mid-fight. A target
  // whose last callout lands drops out; if it returns it goes to the end.
  const laneOrderRef = useRef<string[]>([])
  const present = new Set(shown.map((t) => t.target_name))
  const order = laneOrderRef.current.filter((n) => present.has(n))
  for (const t of shown) {
    if (!order.includes(t.target_name)) order.push(t.target_name)
  }
  laneOrderRef.current = order

  const span = Math.max(
    DEFAULT_SPAN,
    ...shown.map((t) => (t.duration_seconds > 0 ? t.duration_seconds : 0)),
  )
  const cols = Math.round(span)
  const laneHeight = Math.max(22, appearance.nameFontSize + 12)
  // Near-solid blocks (the column numbers shouldn't show through), still
  // nudged by the user's bar-fill setting.
  const blockAlpha = Math.min(1, 0.8 + appearance.fillOpacity * 0.3)

  if (shown.length === 0) return <>{placeholder}</>

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4, padding: 6 }}>
      {order.map((target) => {
        const laneTimers = shown.filter((t) => t.target_name === target)
        return (
          <div key={target} style={{ display: 'flex', alignItems: 'stretch', height: laneHeight, flexShrink: 0 }}>
            <div
              title={target}
              style={{
                width: 84,
                flexShrink: 0,
                display: 'flex',
                alignItems: 'center',
                padding: '0 8px',
                backgroundColor: palette.badgeBg,
                border: `1px solid ${palette.badgeBorder}`,
                borderRadius: '4px 0 0 4px',
                color: palette.badgeText,
                fontSize: appearance.nameFontSize,
                fontWeight: 500,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{target}</span>
            </div>
            <div
              style={{
                position: 'relative',
                flex: 1,
                minWidth: 0,
                overflow: 'hidden',
                backgroundColor: palette.track,
                border: `1px solid ${palette.badgeBorder}`,
                borderLeft: 'none',
                borderRadius: '0 4px 4px 0',
              }}
            >
              {/* Column grid: one cell per second, labelled with the seconds
                  left until a block sitting there lands. */}
              {Array.from({ length: cols }, (_, i) => (
                <div
                  key={i}
                  style={{
                    position: 'absolute',
                    top: 0,
                    bottom: 0,
                    left: `${(i / span) * 100}%`,
                    width: `${(1 / span) * 100}%`,
                    borderRight: i < cols - 1 ? `1px solid ${palette.divider}` : 'none',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 9,
                    color: palette.colLabel,
                    fontVariantNumeric: 'tabular-nums',
                    pointerEvents: 'none',
                  }}
                >
                  {i + 1}
                </div>
              ))}
              {laneTimers.map((t) => {
                const missed = t.possible_miss ?? false
                const { position } = parseLabel(t.spell_name)
                // Live blocks stop at the badge (a flagged miss lingers there
                // red); landed ghosts carry on underneath it.
                const raw = rawRemaining(t, now)
                const rem = Math.min(
                  span,
                  liveIds.has(t.id) ? Math.max(0, raw) : Math.max(-LAND_OVERRUN_SECS, raw),
                )
                const landing = !missed && rem / span < LANDING_FRACTION
                const rgb = missed ? BLOCK_RED : landing ? BLOCK_GREEN : BLOCK_BLUE
                return (
                  <div
                    key={t.id}
                    title={blockTitle(t, missed)}
                    style={{
                      position: 'absolute',
                      top: 2,
                      bottom: 2,
                      left: `${(rem / span) * 100}%`,
                      width: `${(1 / span) * 100}%`,
                      minWidth: 24,
                      boxSizing: 'border-box',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      padding: '0 3px',
                      backgroundColor: `rgba(${rgb},${blockAlpha})`,
                      // The chain position's badge colour, so "the 3 spot"
                      // reads the same as in the list view.
                      borderLeft: `3px solid ${chainPositionColor(position)}`,
                      borderRadius: 3,
                      color: '#fff',
                      fontSize: appearance.nameFontSize,
                      fontWeight: 700,
                      lineHeight: 1,
                      fontVariantNumeric: 'tabular-nums',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      transition: 'background-color 0.2s linear',
                    }}
                  >
                    {blockLabel(position, letters)}
                  </div>
                )
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ChainStyleToggle is the header's List / Lanes icon switch, shared by both
// renderers so the dashboard card and the popped-out window match.
export function ChainStyleToggle({
  style,
  onChange,
  variant,
}: {
  style: ChainStyle
  onChange: (s: ChainStyle) => void
  variant: Variant
}): React.ReactElement {
  const btn = (active: boolean): React.CSSProperties => ({
    display: 'flex',
    alignItems: 'center',
    background: active
      ? variant === 'overlay' ? 'rgba(255,255,255,0.12)' : 'var(--color-surface-2)'
      : 'transparent',
    color: active
      ? variant === 'overlay' ? 'rgba(255,255,255,0.9)' : 'var(--color-foreground)'
      : variant === 'overlay' ? 'rgba(255,255,255,0.4)' : 'var(--color-muted)',
    border: 'none',
    cursor: 'pointer',
    padding: '2px 4px',
    borderRadius: 3,
    lineHeight: 1,
  })
  return (
    <div
      className="no-drag"
      style={{ display: 'inline-flex', gap: 2, backgroundColor: 'rgba(0,0,0,0.25)', borderRadius: 4, padding: 1 }}
    >
      <button style={btn(style === 'list')} onClick={() => onChange('list')} title="List view">
        <List size={11} />
      </button>
      <button style={btn(style === 'lanes')} onClick={() => onChange('lanes')} title="Lanes view (one lane per target)">
        <GanttChart size={11} />
      </button>
    </div>
  )
}
