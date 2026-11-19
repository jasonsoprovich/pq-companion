import React from 'react'
import { Eye } from 'lucide-react'
import type { FactionTally } from '../types/faction'
import {
  BUCKET_ORDER,
  BUCKET_LABEL,
  BUCKET_COLOR,
  BUCKET_MIN,
  bucketForTotal,
  bucketIndex,
  type FactionBucket,
} from '../lib/factionBuckets'

// FactionStandingBar draws the faction's /con scale to value, from the
// lowest to the highest total this character can reach (caps + modifier),
// with the estimated range as a band on top. With no estimate yet it falls
// back to the bucket-only bar of the last /con reading.

function fmt(v: number): string {
  return v > 0 ? `+${v}` : `${v}`
}

function BucketOnlyBar({
  bucket,
  suspect,
}: {
  bucket?: string
  suspect?: boolean
}): React.ReactElement {
  const idx = bucket ? bucketIndex(bucket) : -1
  return (
    <div className="flex items-center gap-1.5">
      <div className="flex flex-1 gap-0.5">
        {BUCKET_ORDER.map((b, i) => {
          const active = i === idx
          return (
            <div
              key={b}
              title={BUCKET_LABEL[b]}
              className="h-2 flex-1 rounded-sm transition-opacity"
              style={{
                backgroundColor: BUCKET_COLOR[b],
                opacity: idx === -1 ? 0.25 : active ? 1 : 0.2,
                outline: active ? '1px solid var(--color-foreground)' : 'none',
              }}
            />
          )
        })}
      </div>
      <span
        className="w-28 shrink-0 text-right text-[11px]"
        style={{
          color: idx === -1
            ? 'var(--color-muted-foreground)'
            : 'var(--color-foreground)',
        }}
        title={idx === -1
          ? 'Consider an NPC of this faction in-game to set a baseline'
          : undefined}
      >
        {idx === -1 ? 'Needs /con' : BUCKET_LABEL[bucket as FactionBucket]}
      </span>
      {suspect && <SuspectIcon />}
    </div>
  )
}

function SuspectIcon(): React.ReactElement {
  return (
    <span title="Reading taken while illusioned, invisible, feigned, engaged or under a faction buff — may not reflect your true faction">
      <Eye size={12} style={{ color: 'var(--color-warning, #f59e0b)' }} />
    </span>
  )
}

// nextGoal describes how far the estimate is from the next bucket up.
function nextGoal(
  lo: number,
  hi: number,
  maxTotal: number,
): string | null {
  const top = bucketForTotal(hi)
  const idx = bucketIndex(top)
  if (idx >= BUCKET_ORDER.length - 1) return null
  const next = BUCKET_ORDER[idx + 1]
  const need = BUCKET_MIN[next]
  if (need > maxTotal) {
    return `${BUCKET_LABEL[next]} is out of reach: this faction caps at ` +
      `${BUCKET_LABEL[bucketForTotal(maxTotal)]} for you`
  }
  const least = need - hi
  const most = need - lo
  const pts = least === most ? `${least}` : `${least}–${most}`
  return `~${pts} pts to ${BUCKET_LABEL[next]}`
}

export default function FactionStandingBar({
  tally,
}: {
  tally?: FactionTally
}): React.ReactElement {
  const info = tally?.info
  if (!tally || !tally.range_known || !info) {
    return (
      <div className="flex flex-col gap-1">
        <BucketOnlyBar
          bucket={tally?.last_bucket}
          suspect={tally?.last_consider_suspect}
        />
        {tally && info && (
          <div
            className="text-[11px]"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            No standing estimate yet — a clean <code>/con</code> of this
            faction, or a &ldquo;could not possibly get any worse/better&rdquo;
            line, starts one.
          </div>
        )}
      </div>
    )
  }

  const m = info.modifier
  const minT = info.min_cap + m
  const maxT = info.max_cap + m
  const span = Math.max(1, maxT - minT)
  const pct = (v: number): number =>
    Math.min(100, Math.max(0, ((v - minT) / span) * 100))
  const lo = tally.range_lo + m
  const hi = tally.range_hi + m
  const loB = bucketForTotal(lo)
  const hiB = bucketForTotal(hi)
  const label = loB === hiB
    ? BUCKET_LABEL[loB]
    : `${BUCKET_LABEL[loB]} – ${BUCKET_LABEL[hiB]}`
  const goal = nextGoal(lo, hi, maxT)
  // Outline the last /con only when it's no older than the anchor — an
  // older reading says nothing about where the estimate is now.
  const conFresh = !!tally.last_bucket && !!tally.last_considered_at &&
    (!tally.range_at ||
      Date.parse(tally.last_considered_at) >= Date.parse(tally.range_at))
  const conIdx = conFresh ? bucketIndex(tally.last_bucket as string) : -1
  const exact = tally.range_lo === tally.range_hi
  const personal = exact
    ? fmt(tally.range_lo)
    : `${fmt(tally.range_lo)} to ${fmt(tally.range_hi)}`
  const anchor = tally.range_source === 'cap'
    ? 'anchored at the faction cap'
    : 'anchored by /con'

  // Segments: each bucket's slice of [minT, maxT], to value.
  const segments = BUCKET_ORDER.map((b, i) => {
    const start = Math.max(minT, BUCKET_MIN[b])
    const end = i === BUCKET_ORDER.length - 1
      ? maxT
      : Math.min(maxT, BUCKET_MIN[BUCKET_ORDER[i + 1]])
    return { b, i, start, end }
  }).filter((s) => s.end > s.start)

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-1.5">
        <div className="relative h-3 flex-1">
          <div className="absolute inset-x-0 top-0.5 flex h-2">
            {segments.map((s) => (
              <div
                key={s.b}
                title={`${BUCKET_LABEL[s.b]} (${fmt(Math.ceil(s.start))} to ` +
                  `${fmt(Math.floor(s.end))} on the /con scale)`}
                className="h-2 rounded-sm"
                style={{
                  width: `${((s.end - s.start) / span) * 100}%`,
                  backgroundColor: BUCKET_COLOR[s.b],
                  opacity: 0.35,
                  outline: s.i === conIdx
                    ? '1px solid var(--color-foreground)'
                    : 'none',
                }}
              />
            ))}
          </div>
          <div
            className="absolute top-0 h-3 rounded-sm"
            title={`Estimated: ${personal} personal`}
            style={{
              left: `${pct(lo)}%`,
              width: `max(3px, ${pct(hi) - pct(lo)}%)`,
              backgroundColor: 'var(--color-foreground)',
              opacity: 0.85,
            }}
          />
        </div>
        <span
          className="w-28 shrink-0 text-right text-[11px]"
          style={{ color: 'var(--color-foreground)' }}
        >
          {label}
        </span>
        {tally.last_consider_suspect && <SuspectIcon />}
      </div>
      <div
        className="flex flex-wrap gap-x-3 text-[11px]"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        <span title={`Personal faction ${personal}, ${anchor}. /con adds ` +
          `${fmt(m)} for your race/class/deity.`}
        >
          Est. {exact ? fmt(lo) : `${fmt(lo)} to ${fmt(hi)}`} on the /con
          scale
        </span>
        {goal && <span>{goal}</span>}
        {!info.modifier_exact && (
          <span title="Your character's race, class or deity isn't known yet (a Zeal export fills them in), and this faction has modifiers for it">
            approx. modifier
          </span>
        )}
      </div>
    </div>
  )
}
