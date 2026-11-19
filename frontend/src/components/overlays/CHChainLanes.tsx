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
import type { ChainStyle } from '../../lib/chChainStyle'

// Seconds of cast the track spans when no timer says otherwise. Complete
// Heal (and Druid Renewal) are 10s casts.
const DEFAULT_SPAN = 10
// Repaint cadence for the interpolated block positions (~30fps is smooth
// enough for a block crossing a few hundred px in 10s).
const FRAME_MS = 33

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

const PALETTES: Record<Variant, Palette> = {
  overlay: {
    track: 'rgba(0,0,0,0.35)',
    divider: 'rgba(255,255,255,0.22)',
    colLabel: 'rgba(248,113,113,0.65)',
    badgeBg: 'rgba(234,88,12,0.85)',
    badgeBorder: 'rgba(0,0,0,0.6)',
    badgeText: '#fff',
  },
  panel: {
    track: 'var(--color-surface-2)',
    divider: 'var(--color-border)',
    colLabel: 'rgba(248,113,113,0.65)',
    badgeBg: 'rgba(234,88,12,0.85)',
    badgeBorder: 'var(--color-border)',
    badgeText: '#fff',
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

// remainingSecs interpolates a callout's time-to-land from its fixed start
// time. Falls back to the broadcast value if starts_at can't be parsed.
function remainingSecs(t: ActiveTimer, now: number): number {
  const start = Date.parse(t.starts_at)
  if (Number.isNaN(start)) return Math.max(0, t.remaining_seconds)
  return Math.max(0, t.duration_seconds - (now - start) / 1000)
}

// blockLabel is the block's text: a zero-padded number on the main chain
// ("001", matching how chains are usually called), the letter on the
// secondary (ramp) chain.
function blockLabel(position: number, letters: boolean): string {
  if (position === Number.MAX_SAFE_INTEGER) return '?'
  if (letters) return positionLetter(position)
  return String(position).padStart(3, '0')
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
}: {
  /** The active chain's timers, already filtered and sorted by start. */
  timers: ActiveTimer[]
  letters: boolean
  appearance: TimerAppearance
  variant: Variant
}): React.ReactElement {
  const palette = PALETTES[variant]
  const now = useNowMs(timers.length > 0)

  // Lane order = order each target first appeared, kept stable while that
  // target has a live callout so lanes never reshuffle mid-fight. A target
  // whose last callout lands drops out; if it returns it goes to the end.
  const laneOrderRef = useRef<string[]>([])
  const present = new Set(timers.map((t) => t.target_name))
  const order = laneOrderRef.current.filter((n) => present.has(n))
  for (const t of timers) {
    if (!order.includes(t.target_name)) order.push(t.target_name)
  }
  laneOrderRef.current = order

  const span = Math.max(
    DEFAULT_SPAN,
    ...timers.map((t) => (t.duration_seconds > 0 ? t.duration_seconds : 0)),
  )
  const cols = Math.round(span)
  const laneHeight = Math.max(22, appearance.nameFontSize + 12)
  // Solid-looking blocks by default (they're the whole point of the view),
  // still nudged by the user's bar-fill setting.
  const blockAlpha = Math.min(1, 0.6 + appearance.fillOpacity * 0.7)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 3, padding: 6 }}>
      {order.map((target) => {
        const laneTimers = timers.filter((t) => t.target_name === target)
        return (
          <div key={target} style={{ display: 'flex', alignItems: 'stretch', height: laneHeight, flexShrink: 0 }}>
            <div
              title={target}
              style={{
                width: 84,
                flexShrink: 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                padding: '0 4px',
                backgroundColor: palette.badgeBg,
                border: `1px solid ${palette.badgeBorder}`,
                borderRadius: 4,
                color: palette.badgeText,
                fontSize: appearance.nameFontSize,
                fontWeight: 600,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                textShadow: '0 1px 2px rgba(0,0,0,0.8)',
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
                border: `1px solid ${palette.divider}`,
                borderLeft: 'none',
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
                const rem = Math.min(span, remainingSecs(t, now))
                return (
                  <div
                    key={t.id}
                    title={blockTitle(t, missed)}
                    style={{
                      position: 'absolute',
                      top: 1,
                      bottom: 1,
                      left: `${(rem / span) * 100}%`,
                      width: `${(1 / span) * 100}%`,
                      minWidth: 26,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      backgroundColor: missed
                        ? `rgba(220,38,38,${blockAlpha})`
                        : `rgba(22,163,74,${blockAlpha})`,
                      borderRadius: 2,
                      color: '#fff',
                      fontSize: appearance.nameFontSize + 1,
                      fontWeight: 700,
                      fontVariantNumeric: 'tabular-nums',
                      textShadow: '0 1px 2px rgba(0,0,0,0.7)',
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
