import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Sparkles, Coins } from 'lucide-react'
import { getProgressRecapAll, getProgressRecapFor } from '../services/api'
import type { CharacterRecap } from '../types/progress'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import BackfillLink from './BackfillLink'

const WINDOW_OPTIONS = [7, 30, 90] as const
type WindowDays = (typeof WINDOW_OPTIONS)[number]
type Scope = 'character' | 'all'

interface CharacterRecapPanelProps {
  characterName: string
}

function hasActivity(r: CharacterRecap): boolean {
  return (
    r.levels_gained > 0 ||
    r.aas_gained > 0 ||
    r.spells_scribed > 0 ||
    r.skill_ups > 0 ||
    r.tradeskill_ups > 0 ||
    (r.has_snapshot_data && r.coin_delta !== 0)
  )
}

function formatCoin(copper: number): string {
  const sign = copper < 0 ? '-' : '+'
  const abs = Math.abs(copper)
  const plat = Math.floor(abs / 1000)
  if (plat >= 1000) return `${sign}${(plat / 1000).toFixed(1)}k pp`
  if (plat > 0) return `${sign}${plat} pp`
  return `${sign}${abs} cp`
}

export default function CharacterRecapPanel({ characterName }: CharacterRecapPanelProps): React.ReactElement {
  const [windowDays, setWindowDays] = useState<WindowDays>(30)
  const [scope, setScope] = useState<Scope>('character')
  const [recap, setRecap] = useState<CharacterRecap | null>(null)
  const [allRecaps, setAllRecaps] = useState<CharacterRecap[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(() => {
    setError(null)
    const req =
      scope === 'character'
        ? getProgressRecapFor(windowDays, characterName).then((r) => { setRecap(r); setAllRecaps(null) })
        : getProgressRecapAll(windowDays).then((r) => { setAllRecaps(r); setRecap(null) })
    return req.catch((err: unknown) => setError(err instanceof Error ? err.message : 'Failed to load recap'))
  }, [scope, windowDays, characterName])

  useEffect(() => {
    setLoading(true)
    load().finally(() => setLoading(false))
  }, [load])

  // Refresh silently (no loading flash) when a new milestone or Quarmy export
  // lands — the same load() call re-fetches without resetting recap/allRecaps
  // to null first.
  const handleWs = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ProgressEvent || msg.type === WSEvent.ZealQuarmy) load()
  }, [load])
  useWebSocket(handleWs)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1 rounded-lg p-0.5" style={{ backgroundColor: 'var(--color-surface-2)' }}>
          {WINDOW_OPTIONS.map((d) => (
            <button
              key={d}
              onClick={() => setWindowDays(d)}
              className="rounded px-3 py-1 text-xs font-medium transition-colors"
              style={{
                backgroundColor: windowDays === d ? 'var(--color-surface)' : 'transparent',
                color: windowDays === d ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
                cursor: 'pointer',
              }}
            >
              {d}d
            </button>
          ))}
        </div>
        <div className="flex items-center gap-1 rounded-lg p-0.5" style={{ backgroundColor: 'var(--color-surface-2)' }}>
          {(['character', 'all'] as const).map((s) => (
            <button
              key={s}
              onClick={() => setScope(s)}
              className="rounded px-3 py-1 text-xs font-medium transition-colors"
              style={{
                backgroundColor: scope === s ? 'var(--color-surface)' : 'transparent',
                color: scope === s ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
                cursor: 'pointer',
              }}
            >
              {s === 'character' ? 'This Character' : 'All Characters'}
            </button>
          ))}
        </div>
      </div>

      {loading ? (
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
      ) : error ? (
        <p className="text-sm" style={{ color: '#f87171' }}>{error}</p>
      ) : scope === 'character' && recap ? (
        <SingleCharacterRecap recap={recap} windowDays={windowDays} />
      ) : scope === 'all' && allRecaps ? (
        <AllCharactersRecap recaps={allRecaps} windowDays={windowDays} />
      ) : null}
    </div>
  )
}

function SingleCharacterRecap({ recap, windowDays }: { recap: CharacterRecap; windowDays: number }): React.ReactElement {
  if (!hasActivity(recap)) {
    return (
      <div
        className="flex flex-col items-center gap-3 rounded-lg py-12 text-center"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <Sparkles size={28} style={{ color: 'var(--color-muted)' }} />
        <div>
          <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            No progression activity in the last {windowDays} days
          </p>
          <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Level, AA, spell, and skill milestones are parsed from this character's log as they happen.
          </p>
        </div>
        <div className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Have older activity in your log file?
          <BackfillLink />
        </div>
      </div>
    )
  }

  const levelLabel =
    recap.start_level && recap.end_level && recap.start_level !== recap.end_level
      ? `${recap.start_level} → ${recap.end_level}`
      : recap.levels_gained > 0
        ? `+${recap.levels_gained}`
        : '—'

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <StatTile label="Levels" value={levelLabel} />
        <StatTile label="AA Points" value={`+${recap.aas_gained}`} />
        <StatTile label="Spells Scribed" value={String(recap.spells_scribed)} />
        <StatTile label="Skill-Ups" value={String(recap.skill_ups)} />
        <StatTile label="Tradeskill-Ups" value={String(recap.tradeskill_ups)} />
        <StatTile label="Active Days" value={`${recap.active_days}/${windowDays}`} />
      </div>

      {recap.has_snapshot_data && (
        <div
          className="flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <Coins size={14} style={{ color: 'var(--color-primary)' }} />
          <span style={{ color: 'var(--color-muted-foreground)' }}>Coin change:</span>
          <span style={{ color: 'var(--color-foreground)', fontWeight: 500 }}>{formatCoin(recap.coin_delta)}</span>
        </div>
      )}
      {!recap.has_snapshot_data && (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          Coin and total tracking starts from now — it can't be recovered from past logs. Check back after this
          character has played a bit more.
        </p>
      )}

      <DailyActivityStrip recap={recap} windowDays={windowDays} />
    </div>
  )
}

function StatTile({ label, value }: { label: string; value: string }): React.ReactElement {
  return (
    <div
      className="rounded-lg px-3 py-2.5"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{label}</p>
      <p className="mt-0.5 text-lg font-semibold tabular-nums" style={{ color: 'var(--color-foreground)' }}>
        {value}
      </p>
    </div>
  )
}

function DailyActivityStrip({ recap, windowDays }: { recap: CharacterRecap; windowDays: number }): React.ReactElement {
  const counts = useMemo(() => {
    const byDate = new Map<string, number>()
    for (const b of recap.daily_activity ?? []) byDate.set(b.date, b.count)

    const days: { date: string; count: number }[] = []
    const end = new Date()
    end.setHours(0, 0, 0, 0)
    for (let i = windowDays - 1; i >= 0; i--) {
      const d = new Date(end)
      d.setDate(d.getDate() - i)
      const key = d.toISOString().slice(0, 10)
      days.push({ date: key, count: byDate.get(key) ?? 0 })
    }
    return days
  }, [recap.daily_activity, windowDays])

  const max = Math.max(1, ...counts.map((d) => d.count))

  return (
    <div
      className="rounded-lg p-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Daily activity</p>
      <div className="flex items-end gap-px" style={{ height: 40 }}>
        {counts.map((d) => (
          <div
            key={d.date}
            title={`${d.date}: ${d.count} milestone${d.count === 1 ? '' : 's'}`}
            className="flex-1 rounded-t"
            style={{
              height: `${Math.max(6, (d.count / max) * 100)}%`,
              backgroundColor: d.count > 0 ? 'var(--color-primary)' : 'var(--color-border)',
              minWidth: 2,
            }}
          />
        ))}
      </div>
    </div>
  )
}

function AllCharactersRecap({ recaps, windowDays }: { recaps: CharacterRecap[]; windowDays: number }): React.ReactElement {
  const active = recaps.filter(hasActivity)

  if (active.length === 0) {
    return (
      <div
        className="flex flex-col items-center gap-2 rounded-lg py-12 text-center"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <Sparkles size={28} style={{ color: 'var(--color-muted)' }} />
        <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          No progression activity across any character in the last {windowDays} days
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {active.map((r, i) => (
        <div
          key={r.character}
          className="flex items-center gap-3 rounded-lg px-4 py-3"
          style={{
            backgroundColor: i === 0
              ? 'color-mix(in srgb, var(--color-primary) 8%, var(--color-surface))'
              : 'var(--color-surface)',
            border: `1px solid ${i === 0 ? 'color-mix(in srgb, var(--color-primary) 40%, transparent)' : 'var(--color-border)'}`,
          }}
        >
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>{r.character}</p>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {r.active_days} active day{r.active_days === 1 ? '' : 's'}
              {r.start_level && r.end_level && r.start_level !== r.end_level ? ` · Lv ${r.start_level} → ${r.end_level}` : ''}
            </p>
          </div>
          <div className="flex items-center gap-4 text-xs tabular-nums" style={{ color: 'var(--color-muted-foreground)' }}>
            {r.levels_gained > 0 && <span>+{r.levels_gained} lvl</span>}
            {r.aas_gained > 0 && <span>+{r.aas_gained} AA</span>}
            {r.spells_scribed > 0 && <span>+{r.spells_scribed} spells</span>}
            {r.skill_ups + r.tradeskill_ups > 0 && <span>+{r.skill_ups + r.tradeskill_ups} skills</span>}
          </div>
        </div>
      ))}
    </div>
  )
}
