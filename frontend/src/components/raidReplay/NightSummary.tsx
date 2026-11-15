import React, { useMemo, useState } from 'react'
import { ArrowDown, ArrowUp } from 'lucide-react'
import { classColor } from '../../lib/raidColors'
import type { RaiderSummary } from '../../types/raidReplay'

// NightSummary is Raid Replay's per-raider table for a whole raid night:
// when each raider joined and left the raid, how long they spent in the
// recorder's zone, how often they left it, deaths, and time standing still.
// Everything comes from the recorder's own Zeal raid frames and log, so
// "still" is positions only: a healer holding a spot reads the same as
// someone AFK.

// LONG_STILL_MS is a single still stretch long enough to flag.
const LONG_STILL_MS = 10 * 60_000

type SortKey =
  | 'name' | 'level' | 'joined' | 'left' | 'in_zone' | 'away' | 'deaths' | 'still' | 'longest'

function fmtClock(ms: number): string {
  return new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function fmtSpan(ms: number): string {
  if (ms <= 0) return '—'
  const m = Math.round(ms / 60000)
  if (m < 1) return '<1m'
  return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${m % 60}m`
}

const COLUMNS: { key: SortKey; label: string; title: string; right?: boolean }[] = [
  { key: 'name', label: 'Raider', title: 'Name' },
  { key: 'level', label: 'Lvl', title: 'Highest level seen', right: true },
  { key: 'joined', label: 'Joined', title: 'When they joined the raid. "start": already in it when your roster began.', right: true },
  { key: 'left', label: 'Left', title: 'When they left the raid for good', right: true },
  { key: 'in_zone', label: 'In zone', title: 'Time spent in your zone, and the share of the recorded time', right: true },
  { key: 'away', label: 'Away', title: 'Times they left your zone mid-recording (ported, died and released, went to fetch someone)', right: true },
  { key: 'deaths', label: 'Deaths', title: 'Deaths seen in your log', right: true },
  { key: 'still', label: 'Still', title: 'Total time standing in one spot for 2+ minutes. Positions only, so busy and AFK look the same.', right: true },
  { key: 'longest', label: 'Longest', title: 'Longest single stretch standing still', right: true },
]

function sortValue(r: RaiderSummary, key: SortKey): number | string {
  switch (key) {
    case 'name': return r.name.toLowerCase()
    case 'level': return r.level
    case 'joined': return r.joined_at
    case 'left': return r.left_at
    case 'in_zone': return r.in_zone_ms
    case 'away': return r.away
    case 'deaths': return r.deaths
    case 'still': return r.still_ms
    case 'longest': return r.longest_still_ms
  }
}

export function NightSummary({ summary, recordedMs, rosterTracked, palette, onPick }: {
  summary: RaiderSummary[]
  /** Total recorded time across the night's segments, for the in-zone %. */
  recordedMs: number
  rosterTracked: boolean
  palette: Parameters<typeof classColor>[0]
  /** A raider's name was clicked: show them on the map. */
  onPick: (r: RaiderSummary) => void
}): React.ReactElement {
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: 'name', desc: false })

  const rows = useMemo(() => {
    const out = [...summary]
    out.sort((a, b) => {
      const va = sortValue(a, sort.key)
      const vb = sortValue(b, sort.key)
      const c = typeof va === 'string' ? va.localeCompare(vb as string) : (va as number) - (vb as number)
      return sort.desc ? -c : c
    })
    return out
  }, [summary, sort])

  const muted = { color: 'var(--color-muted)' }
  return (
    <div className="flex h-full min-h-0 flex-col gap-2">
      {!rosterTracked && (
        <p className="text-[11px]" style={muted}>
          This night was recorded before joins and leaves were logged, so
          Joined and Left are blank. Raiders only seen out of your zone
          aren&apos;t listed.
        </p>
      )}
      <div className="min-h-0 flex-1 overflow-auto rounded border" style={{ borderColor: 'var(--color-border)' }}>
        <table className="w-full text-xs">
          <thead className="sticky top-0" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <tr>
              {COLUMNS.map((c) => (
                <th
                  key={c.key}
                  title={c.title}
                  className={`cursor-pointer select-none px-2 py-1.5 font-semibold ${c.right ? 'text-right' : 'text-left'}`}
                  onClick={() =>
                    setSort((s) => ({ key: c.key, desc: s.key === c.key ? !s.desc : c.key !== 'name' }))
                  }
                >
                  <span className="inline-flex items-center gap-0.5">
                    {c.label}
                    {sort.key === c.key && (sort.desc ? <ArrowDown size={10} /> : <ArrowUp size={10} />)}
                  </span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => {
              const pct = recordedMs > 0 ? Math.round((r.in_zone_ms / recordedMs) * 100) : 0
              const longFlag = r.longest_still_ms >= LONG_STILL_MS
              return (
                <tr key={r.name} className="border-t" style={{ borderColor: 'var(--color-border)' }}>
                  <td className="px-2 py-1">
                    <button
                      className="flex items-center gap-1.5 text-left hover:underline"
                      title={r.in_zone_ms > 0 ? 'Show on the map' : 'Never in your zone'}
                      onClick={() => onPick(r)}
                      disabled={r.in_zone_ms === 0}
                    >
                      <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: classColor(palette, r.class) }} />
                      {r.name}
                    </button>
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums" style={muted}>{r.level || ''}</td>
                  <td className="px-2 py-1 text-right tabular-nums">
                    {r.joined_at ? fmtClock(r.joined_at) : <span style={muted}>{rosterTracked ? 'start' : '—'}</span>}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">
                    {r.left_at ? fmtClock(r.left_at) : <span style={muted}>—</span>}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">
                    {r.in_zone_ms > 0 ? (
                      <>
                        {fmtSpan(r.in_zone_ms)} <span style={muted}>{pct}%</span>
                      </>
                    ) : (
                      <span style={muted}>never</span>
                    )}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">{r.away || <span style={muted}>0</span>}</td>
                  <td className="px-2 py-1 text-right tabular-nums" style={{ color: r.deaths ? '#ef4444' : undefined }}>
                    {r.deaths || <span style={muted}>0</span>}
                  </td>
                  <td className="px-2 py-1 text-right tabular-nums">{fmtSpan(r.still_ms)}</td>
                  <td
                    className="px-2 py-1 text-right tabular-nums"
                    style={{ color: longFlag ? '#f59e0b' : undefined }}
                    title={longFlag ? 'Stood in one spot 10+ minutes' : undefined}
                  >
                    {fmtSpan(r.longest_still_ms)}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
