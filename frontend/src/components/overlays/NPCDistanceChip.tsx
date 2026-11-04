import React from 'react'
import type { NPCTargetDistance } from '../../hooks/useNPCTargetDistance'
import { useLiveValue } from '../../hooks/useLiveValue'
import { LIVE_TTL, STALE_OPACITY, fmtLiveAge } from '../../lib/live'
import { useInZealStaleFrame } from './ZealStaleFrame'

// Range colours, fixed rather than theme tokens: the floating overlay window
// can't rely on the app's theme stylesheet (see NPCCasterSummarySection).
const IN_RANGE = '#4ade80'
const EDGE = '#facc15'
const OUT_OF_RANGE = '#f87171'
const MUTED = 'rgba(255,255,255,0.45)'

// The distance can move a few units between the server's check and ours, and
// model size also counts server-side, so the last 10% of a range is "edge".
const EDGE_FRACTION = 0.9

export function rangeColor(distance: number, range: number): string {
  if (distance > range) return OUT_OF_RANGE
  if (distance > range * EDGE_FRACTION) return EDGE
  return IN_RANGE
}

// inReach reports whether a spell with the given reach could land on a player
// standing `distance` away. False when either is unknown.
export function inReach(
  reach: number | undefined,
  distance: number | null | undefined,
): boolean {
  return reach != null && reach > 0 && distance != null && distance <= reach
}

function rangeLine(label: string, distance: number, range: number): string {
  const verdict =
    distance > range
      ? 'out of range'
      : distance > range * EDGE_FRACTION
        ? 'at the edge'
        : 'in range'
  return `${label} ${range}: ${verdict}`
}

// NPCDistanceChip shows the live player→target distance, coloured against the
// user's configured cast range. Renders nothing when this Zeal build doesn't
// report target info, and "n/a" when Zeal withheld the position (250+ away).
// Once Zeal stops updating it the chip dims with its age, then hides
// (LIVE_TTL.targetDistance).
//
// size 'md' matches the popout overlay's stat chips (LV, class, resists: 12px
// value, 10px uppercase label); 'sm' matches the dashboard panel's 10px badges.
export default function NPCDistanceChip({
  dist,
  size = 'sm',
}: {
  dist: NPCTargetDistance
  size?: 'sm' | 'md'
}): React.ReactElement | null {
  const live = useLiveValue(dist.at, LIVE_TTL.targetDistance)
  const inStaleFrame = useInZealStaleFrame()
  const aged = dist.at ? live.freshness : 'fresh'
  if (!dist.has_descriptors || aged === 'gone') return null
  const stale = aged === 'stale'
  const staleNote = stale ? `\nZeal hasn't updated this since then (${fmtLiveAge(live.ageMs)}).` : ''

  const md = size === 'md'
  const base: React.CSSProperties = {
    display: 'inline-flex',
    alignItems: 'baseline',
    gap: 4,
    borderRadius: 3,
    padding: md ? '3px 7px' : '1px 6px',
    fontSize: md ? 12 : 10,
    lineHeight: md ? 1.4 : undefined,
    fontWeight: 600,
    backgroundColor: md ? 'rgba(255,255,255,0.06)' : 'rgba(255,255,255,0.08)',
    fontVariantNumeric: 'tabular-nums',
    opacity: stale && !inStaleFrame ? STALE_OPACITY : undefined,
  }
  const label = (text: string): React.ReactNode =>
    md ? (
      <span
        style={{
          fontSize: 10,
          fontWeight: 500,
          textTransform: 'uppercase',
          color: 'rgba(255,255,255,0.4)',
        }}
      >
        {text}
      </span>
    ) : (
      `${text} `
    )

  if (dist.distance == null) {
    return (
      <span
        style={{ ...base, color: MUTED }}
        title={"Target is 250+ units away. Zeal only reports a target's position within 250 units." + staleNote}
      >
        {label('Dist')}
        n/a
      </span>
    )
  }

  const d = dist.distance
  const lines = [`Distance to target: ${d}`, rangeLine('Cast range', d, dist.castRange)]
  if (dist.rangedRange > 0) lines.push(rangeLine('Ranged', d, dist.rangedRange))
  lines.push(
    'Approximate: the server also counts model size, and range-extending ' +
      'focus effects and AAs are only included if you entered them in ' +
      'Settings.',
  )

  return (
    <span style={{ ...base, color: rangeColor(d, dist.castRange) }} title={lines.join('\n') + staleNote}>
      {label('Dist')}
      {d}
      {dist.rangedRange > 0 && (
        <span
          style={{
            color: rangeColor(d, dist.rangedRange),
            fontWeight: 500,
            fontSize: md ? 10 : undefined,
            textTransform: md ? 'uppercase' : undefined,
          }}
        >
          · rng
        </span>
      )}
    </span>
  )
}
