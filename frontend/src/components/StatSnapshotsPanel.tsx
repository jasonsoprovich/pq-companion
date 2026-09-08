import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Trash2 } from 'lucide-react'
import {
  getCharacterStatSnapshots,
  deleteCharacterStatSnapshot,
  type StoredStatSnapshot,
  type MyStatsSnapshot,
  type MyStatsMeleeBlock,
} from '../services/api'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

function EmptyState({ message, hint }: { message: string; hint: string }): React.ReactElement {
  return (
    <div className="py-8 text-center">
      <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
        {message}
      </p>
      <p className="mx-auto mt-1 max-w-md text-xs" style={{ color: 'var(--color-muted)' }}>
        {hint}
      </p>
    </div>
  )
}

interface Props {
  characterID: number | null
}

// One row in a comparison table: how to read the value from a snapshot, a
// plain-English "what is what" hint, and which direction is an improvement
// (for the delta colour; null = don't colour).
interface Metric {
  label: string
  hint: string
  get: (s: MyStatsSnapshot) => number | null
  better: 'up' | 'down' | null
  fmt?: (n: number) => string
}

const one = (n: number) => n.toFixed(1)

const DEFENSIVE: Metric[] = [
  {
    label: 'AC (display)',
    hint: 'The AC number on your inventory screen — (raw mitigation + raw avoidance) × 1000/847.',
    get: (s) => s.ac_display,
    better: 'up',
  },
  {
    label: 'Mitigation (post-cap)',
    hint: 'Armor-class mitigation after the era softcap — how much incoming melee damage you shrug off.',
    get: (s) => s.mitigation,
    better: 'up',
  },
  {
    label: 'Mitigation cap',
    hint: 'The softcap (Luclin) or hardcap (pre-Luclin) your mitigation is measured against.',
    get: (s) => s.mitigation_cap,
    better: null,
  },
  {
    label: 'Avoidance (with AAs)',
    hint: 'Chance to take zero damage from a hit, including Combat Agility and friends.',
    get: (s) => s.avoidance,
    better: 'up',
  },
  {
    label: 'Raw mitigation',
    hint: 'Mitigation AC before the softcap (the "Mit:" term in the AC line).',
    get: (s) => s.ac_raw_mit,
    better: 'up',
  },
  {
    label: 'Raw avoidance',
    hint: 'Avoidance before the Combat Agility AA (the "Avoidance:" term in the AC line).',
    get: (s) => s.ac_raw_avoid,
    better: 'up',
  },
  {
    label: 'Movement modifier',
    hint: 'Net run-speed modifier — SoW etc. after any snare/enrage.',
    get: (s) => s.movement_modifier_pct,
    better: null,
    fmt: (n) => `${n > 0 ? '+' : ''}${n}%`,
  },
]

function meleeMetrics(slot: 'Primary' | 'Secondary'): Metric[] {
  const pick = (s: MyStatsSnapshot): MyStatsMeleeBlock | undefined =>
    s.melee.find((bl) => bl.slot === slot)
  return [
    {
      label: 'Offense',
      hint: 'Melee offense — weapon skill + STR bonus + spell ATK + item ATK. Drives the damage multiplier and the mitigation matchup.',
      get: (s) => pick(s)?.offense ?? null,
      better: 'up',
    },
    {
      label: '  ↳ from weapon skill',
      hint: 'The part of Offense from your trained weapon-skill value.',
      get: (s) => pick(s)?.offense_skill ?? null,
      better: 'up',
    },
    {
      label: '  ↳ from STR',
      hint: 'The part of Offense from Strength above 75.',
      get: (s) => pick(s)?.offense_stat ?? null,
      better: 'up',
    },
    {
      label: '  ↳ from item ATK',
      hint: 'The part of Offense from worn ATK on your gear.',
      get: (s) => pick(s)?.offense_item_atk ?? null,
      better: 'up',
    },
    {
      label: 'To Hit',
      hint: 'Your to-hit rating vs the target’s avoidance — sets how often you connect.',
      get: (s) => pick(s)?.to_hit ?? null,
      better: 'up',
    },
    {
      label: 'Display ATK',
      hint: 'The ATK number on your inventory screen — (offense + to hit) × 1000/744.',
      get: (s) => pick(s)?.display_atk ?? null,
      better: 'up',
    },
    {
      label: 'Damage / swing (avg)',
      hint: 'Average damage of one swing after both the mitigation and damage-multiplier rolls.',
      get: (s) => pick(s)?.dmg_avg ?? null,
      better: 'up',
      fmt: one,
    },
    {
      label: 'DPS (avg, no haste)',
      hint: 'Sustained single-weapon DPS with no haste — a stable baseline for comparing setups.',
      get: (s) => pick(s)?.dps_avg ?? null,
      better: 'up',
      fmt: one,
    },
    {
      label: 'DPS (avg, with haste)',
      hint: 'Same, with your current haste applied.',
      get: (s) => pick(s)?.dps_haste_avg ?? pick(s)?.dps_avg ?? null,
      better: 'up',
      fmt: one,
    },
  ]
}

function fmtWhen(unix: number): string {
  return new Date(unix * 1000).toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

export function StatSnapshotsPanel({ characterID }: Props): React.ReactElement {
  const [snaps, setSnaps] = useState<StoredStatSnapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // Ids of the (up to two) snapshots being compared, in pick order.
  const [selected, setSelected] = useState<number[]>([])

  const load = useCallback(() => {
    if (characterID == null) {
      setSnaps([])
      setLoading(false)
      return Promise.resolve()
    }
    return getCharacterStatSnapshots(characterID)
      .then((r) => {
        setSnaps(r.snapshots ?? [])
        setError(null)
      })
      .catch((e: unknown) =>
        setError(e instanceof Error ? e.message : 'Failed to load stat snapshots'),
      )
  }, [characterID])

  useEffect(() => {
    setLoading(true)
    load().finally(() => setLoading(false))
  }, [load])

  const handleWs = useCallback(
    (msg: WsMessage) => {
      if (msg.type === WSEvent.CharacterStatSnapshot) load()
    },
    [load],
  )
  useWebSocket(handleWs)

  // Default the comparison to the two most recent captures; keep any live
  // selection the user made.
  useEffect(() => {
    setSelected((prev) => {
      const live = prev.filter((id) => snaps.some((s) => s.id === id))
      if (live.length > 0) return live
      return snaps.slice(0, 2).map((s) => s.id)
    })
  }, [snaps])

  const toggle = (id: number) => {
    setSelected((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length < 2) return [...prev, id]
      return [prev[1], id]
    })
  }

  const onDelete = async (id: number) => {
    if (characterID == null) return
    await deleteCharacterStatSnapshot(characterID, id)
    load()
  }

  const [a, b] = useMemo(() => {
    const find = (id: number | undefined) =>
      id == null ? undefined : snaps.find((s) => s.id === id)
    return [find(selected[0]), find(selected[1])]
  }, [selected, snaps])

  const hasSecondary =
    (a?.snapshot.melee.some((m) => m.slot === 'Secondary') ?? false) ||
    (b?.snapshot.melee.some((m) => m.slot === 'Secondary') ?? false)

  if (loading) {
    return (
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Loading…
      </p>
    )
  }
  if (error) {
    return (
      <p className="text-sm" style={{ color: 'var(--color-danger, #f87171)' }}>
        {error}
      </p>
    )
  }
  if (snaps.length === 0) {
    return (
      <EmptyState
        message="No stat snapshots yet"
        hint="In-game, turn logging on (/log on) and run /mystats. PQ Companion captures the output automatically. Run it again after a gear or buff change to compare the two side by side."
      />
    )
  }

  const primaryWeapon = (s: StoredStatSnapshot | undefined, slot: 'Primary' | 'Secondary') =>
    s?.snapshot.melee.find((m) => m.slot === slot)?.weapon

  return (
    <div className="flex flex-col gap-4" style={{ minHeight: 0 }}>
      <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
        Captured automatically from <code>/mystats</code>. Pick up to two to compare.
        Numbers are Zeal’s own client-side estimate (disciplines and
        double-attack aren’t modelled yet).
      </p>

      <div className="flex flex-wrap gap-2">
        {snaps.map((s) => {
          const idx = selected.indexOf(s.id)
          const on = idx >= 0
          return (
            <div
              key={s.id}
              className="flex items-center gap-2 rounded px-2 py-1 text-sm"
              style={{
                border: `1px solid ${on ? 'var(--color-primary, #6366f1)' : 'var(--color-border)'}`,
                backgroundColor: on ? 'var(--color-surface-2)' : 'var(--color-surface)',
              }}
            >
              <button
                onClick={() => toggle(s.id)}
                className="flex items-center gap-2"
                style={{ color: 'var(--color-foreground)' }}
              >
                {on && (
                  <span
                    className="rounded px-1 text-xs font-semibold"
                    style={{ backgroundColor: 'var(--color-primary, #6366f1)', color: '#fff' }}
                  >
                    {idx === 0 ? 'A' : 'B'}
                  </span>
                )}
                {fmtWhen(s.captured_at)}
              </button>
              <button
                onClick={() => onDelete(s.id)}
                title="Delete this snapshot"
                style={{ color: 'var(--color-muted)' }}
              >
                <Trash2 size={13} />
              </button>
            </div>
          )
        })}
      </div>

      {a && (
        <>
          <ComparisonTable title="Defensive & misc" metrics={DEFENSIVE} a={a} b={b} />
          <ComparisonTable
            title={`Melee — Primary hand (${primaryWeapon(a, 'Primary') ?? '—'}${
              b && primaryWeapon(b, 'Primary') !== primaryWeapon(a, 'Primary')
                ? ` → ${primaryWeapon(b, 'Primary') ?? '—'}`
                : ''
            })`}
            metrics={meleeMetrics('Primary')}
            a={a}
            b={b}
          />
          {hasSecondary && (
            <ComparisonTable
              title={`Melee — Secondary hand (${primaryWeapon(a, 'Secondary') ?? '—'}${
                b && primaryWeapon(b, 'Secondary') !== primaryWeapon(a, 'Secondary')
                  ? ` → ${primaryWeapon(b, 'Secondary') ?? '—'}`
                  : ''
              })`}
              metrics={meleeMetrics('Secondary')}
              a={a}
              b={b}
            />
          )}
        </>
      )}
    </div>
  )
}

function ComparisonTable({
  title,
  metrics,
  a,
  b,
}: {
  title: string
  metrics: Metric[]
  a: StoredStatSnapshot
  b: StoredStatSnapshot | undefined
}): React.ReactElement {
  return (
    <div>
      <h3
        className="mb-1 text-xs font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </h3>
      <div className="overflow-x-auto">
        <table className="w-full text-sm" style={{ borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ color: 'var(--color-muted)' }}>
              <th className="py-1 pr-3 text-left font-medium">Metric</th>
              <th className="py-1 px-3 text-right font-medium">
                A · {fmtWhen(a.captured_at)}
              </th>
              {b && (
                <>
                  <th className="py-1 px-3 text-right font-medium">
                    B · {fmtWhen(b.captured_at)}
                  </th>
                  <th className="py-1 pl-3 text-right font-medium">Δ</th>
                </>
              )}
            </tr>
          </thead>
          <tbody>
            {metrics.map((m) => (
              <Row key={m.label} m={m} a={a} b={b} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function Row({
  m,
  a,
  b,
}: {
  m: Metric
  a: StoredStatSnapshot
  b: StoredStatSnapshot | undefined
}): React.ReactElement | null {
  const va = m.get(a.snapshot)
  const vb = b ? m.get(b.snapshot) : null
  if (va == null && vb == null) return null

  const fmt = m.fmt ?? ((n: number) => String(n))
  const delta = va != null && vb != null ? vb - va : null
  let deltaColor = 'var(--color-muted)'
  if (delta != null && delta !== 0 && m.better) {
    const good = m.better === 'up' ? delta > 0 : delta < 0
    deltaColor = good ? 'var(--color-success, #4ade80)' : 'var(--color-danger, #f87171)'
  }

  return (
    <tr style={{ borderTop: '1px solid var(--color-border)' }}>
      <td className="py-1 pr-3" title={m.hint} style={{ color: 'var(--color-foreground)' }}>
        {m.label}
      </td>
      <td className="py-1 px-3 text-right" style={{ color: 'var(--color-foreground)' }}>
        {va != null ? fmt(va) : '—'}
      </td>
      {b && (
        <td className="py-1 px-3 text-right" style={{ color: 'var(--color-foreground)' }}>
          {vb != null ? fmt(vb) : '—'}
        </td>
      )}
      {b && (
        <td className="py-1 pl-3 text-right" style={{ color: deltaColor }}>
          {delta == null ? '' : delta === 0 ? '·' : `${delta > 0 ? '+' : ''}${fmt(delta)}`}
        </td>
      )}
    </tr>
  )
}
