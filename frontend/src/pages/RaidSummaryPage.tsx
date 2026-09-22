import React, { useEffect, useMemo, useState } from 'react'
import { RefreshCw, Users, AlertTriangle } from 'lucide-react'
import { useLiveRaidRoster } from '../hooks/useLiveRaidRoster'
import { getRaidTaxonomy } from '../services/api'
import RosterStatusBanner from '../components/raids/RosterStatusBanner'
import type { RaidTaxonomy, RaidRosterMember, ClassCode } from '../types/raid'

// Canonical EQ class order (matches Zeal's 1-indexed class ids / the class
// select on every character page) — used to order the class-count grid and
// the missing-class warning consistently, instead of whatever order the
// taxonomy's class_names object happens to iterate in.
const CLASS_ORDER: ClassCode[] = [
  'war', 'clr', 'pal', 'rng', 'sk', 'dru', 'mnk', 'brd', 'rog', 'shm', 'nec', 'wiz', 'mag', 'enc', 'bst',
]

// Roles a raid can't function without at all — flagged more prominently than
// a merely-absent utility class. Kept small and deliberately conservative:
// this tab is a general "who's here" dashboard, not an encounter-specific
// MIN/REC check (that's what Raid Composition Check is for).
const CRITICAL_CLASSES = new Set<ClassCode>(['war', 'clr', 'shm', 'enc'])

function classLabel(taxonomy: RaidTaxonomy | null, code: ClassCode): string {
  return taxonomy?.class_names?.[code] ?? code
}

function Stat({ label, value, color }: { label: string; value: React.ReactNode; color?: string }): React.ReactElement {
  return (
    <div
      className="flex flex-col items-center justify-center rounded-lg px-4 py-3 gap-0.5"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', minWidth: '6rem' }}
    >
      <span className="text-xl font-semibold tabular-nums" style={{ color: color ?? 'var(--color-foreground)' }}>{value}</span>
      <span className="text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
    </div>
  )
}

function ClassCountCard({ label, count, critical }: { label: string; count: number; critical: boolean }): React.ReactElement {
  const missing = count === 0
  return (
    <div
      className="flex items-center justify-between gap-2 rounded-lg px-3 py-2"
      style={{
        backgroundColor: missing ? (critical ? 'rgba(220,38,38,0.12)' : 'var(--color-surface-2)') : 'var(--color-surface)',
        border: `1px solid ${missing && critical ? 'var(--color-danger)' : 'var(--color-border)'}`,
      }}
    >
      <span className="text-sm" style={{ color: missing ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}>{label}</span>
      <span
        className="text-sm font-semibold tabular-nums"
        style={{ color: missing ? (critical ? 'var(--color-danger)' : 'var(--color-muted-foreground)') : 'var(--color-primary)' }}
      >
        {count}
      </span>
    </div>
  )
}

export default function RaidSummaryPage(): React.ReactElement {
  const { roster, error, refresh } = useLiveRaidRoster()
  const [taxonomy, setTaxonomy] = useState<RaidTaxonomy | null>(null)

  useEffect(() => {
    getRaidTaxonomy().then(setTaxonomy).catch(() => setTaxonomy(null))
  }, [])

  const members = roster?.members ?? []
  const inRaid = !!roster?.in_raid && members.length > 0

  const { counts, unclassedCount, missing } = useMemo(() => {
    const counts = new Map<ClassCode, number>()
    let unclassedCount = 0
    for (const m of members) {
      const code = m.code as ClassCode | undefined
      if (!code) {
        unclassedCount++
        continue
      }
      counts.set(code, (counts.get(code) ?? 0) + 1)
    }
    // Computed unconditionally (an empty raid is trivially "missing" every
    // class) — the dashboard below always renders, so the grid's per-card
    // dimming and the summary stat stay consistent whether or not there's a
    // live raid right now. Only the alert banner's tone depends on inRaid.
    const missing = CLASS_ORDER.filter((c) => (counts.get(c) ?? 0) === 0)
    return { counts, unclassedCount, missing }
  }, [members])

  const sortedMembers = useMemo(() => {
    const rank = (m: RaidRosterMember): number => {
      const i = m.code ? CLASS_ORDER.indexOf(m.code as ClassCode) : -1
      return i === -1 ? CLASS_ORDER.length : i
    }
    return [...members].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
  }, [members])

  const missingCritical = missing.filter((c) => CRITICAL_CLASSES.has(c))
  const missingOther = missing.filter((c) => !CRITICAL_CLASSES.has(c))

  return (
    <div className="flex flex-col gap-4 px-6 py-4 overflow-auto" style={{ height: '100%' }}>
      <div className="flex items-center gap-3 flex-wrap">
        <h1 className="text-lg font-semibold flex items-center gap-2" style={{ color: 'var(--color-foreground)' }}>
          <Users size={18} /> Raid Summary
        </h1>
        <button
          onClick={() => void refresh()}
          className="flex items-center gap-1.5 px-2 py-1.5 text-sm rounded"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
        >
          <RefreshCw size={14} /> Refresh
        </button>
      </div>
      <p className="text-xs -mt-2" style={{ color: 'var(--color-muted-foreground)' }}>
        Live snapshot of the raid from the Zeal pipe — who&apos;s in, class counts, and any classes
        with nobody covering them. For a specific encounter&apos;s MIN/REC staffing, use the
        Composition Check tab.
      </p>

      {error ? (
        <div className="px-3 py-2 text-sm rounded" style={{ backgroundColor: 'var(--color-danger)', color: '#fff' }}>
          {error}
        </div>
      ) : null}

      <RosterStatusBanner roster={roster} />

      <div className="flex flex-wrap gap-3">
        <Stat label="Members" value={members.length} />
        <Stat label="Classed" value={members.length - unclassedCount} color={inRaid ? 'var(--color-success)' : undefined} />
        {unclassedCount > 0 && <Stat label="Unclassed" value={unclassedCount} color="var(--color-muted-foreground)" />}
        <Stat label="Classes missing" value={missing.length} color={inRaid && missingCritical.length > 0 ? 'var(--color-danger)' : undefined} />
      </div>

      {inRaid ? (
        missing.length > 0 && (
          <div
            className="rounded-lg px-4 py-3 flex items-start gap-2"
            style={{
              border: `1px solid ${missingCritical.length > 0 ? 'var(--color-danger)' : 'var(--color-border)'}`,
              backgroundColor: missingCritical.length > 0 ? 'rgba(220,38,38,0.10)' : 'var(--color-surface)',
            }}
          >
            <AlertTriangle size={15} style={{ color: missingCritical.length > 0 ? 'var(--color-danger)' : 'var(--color-muted-foreground)', marginTop: 1 }} />
            <div className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              {missingCritical.length > 0 && (
                <div>
                  <span className="font-semibold" style={{ color: 'var(--color-danger)' }}>No {missingCritical.map((c) => classLabel(taxonomy, c)).join(', ')}</span> in the raid.
                </div>
              )}
              {missingOther.length > 0 && (
                <div style={{ color: 'var(--color-muted-foreground)' }}>
                  Also missing: {missingOther.map((c) => classLabel(taxonomy, c)).join(', ')}
                </div>
              )}
            </div>
          </div>
        )
      ) : (
        // No live raid yet — the counts above are just all-zero placeholders,
        // not a real "everyone's missing" warning, so this stays neutral
        // instead of reusing the red alert styling.
        <div
          className="rounded-lg px-4 py-3 flex items-start gap-2"
          style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <RefreshCw size={15} style={{ color: 'var(--color-muted-foreground)', marginTop: 1 }} />
          <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Waiting for a live raid — member counts, class coverage, and the roster below will fill
            in automatically once Zeal reports a raid roster. No need to refresh by hand.
          </span>
        </div>
      )}

      <div>
        <p className="mb-1.5 text-[11px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted-foreground)' }}>
          Class counts
        </p>
        <div className="grid gap-1.5" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(150px, 1fr))' }}>
          {CLASS_ORDER.map((code) => (
            <ClassCountCard
              key={code}
              label={classLabel(taxonomy, code)}
              count={counts.get(code) ?? 0}
              critical={inRaid && CRITICAL_CLASSES.has(code)}
            />
          ))}
        </div>
      </div>

      <div
        className="rounded-lg flex flex-col min-h-0"
        style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)', maxHeight: '28rem' }}
      >
        <div
          className="px-3 py-2 text-sm font-semibold shrink-0"
          style={{ borderBottom: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
        >
          Members
        </div>
        {/* A fixed max-height + its own overflow-y-auto keeps a full 72-member
            raid scrollable within this box on its own, independent of how
            much room the stats/banner/class-count sections above take up. */}
        <div className="overflow-y-auto min-h-0">
          <div
            className="grid grid-cols-[1.4fr_60px_1fr_1fr_1fr] gap-3 px-3 py-1 text-[11px] uppercase tracking-wide sticky top-0"
            style={{ color: 'var(--color-muted-foreground)', backgroundColor: 'var(--color-surface)' }}
          >
            <span>Name</span>
            <span className="justify-self-end">Level</span>
            <span>Class</span>
            <span>Group</span>
            <span>Rank</span>
          </div>
          {sortedMembers.length === 0 ? (
            <div className="px-3 py-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              No members yet.
            </div>
          ) : (
            sortedMembers.map((m, i) => (
              <div
                key={`${m.name}-${i}`}
                className={`grid grid-cols-[1.4fr_60px_1fr_1fr_1fr] gap-3 items-center px-3 py-1.5 ${i % 2 === 1 ? 'bg-(--color-surface-2)/50' : ''}`}
              >
                <span className="text-sm truncate" style={{ color: 'var(--color-foreground)' }}>{m.name}</span>
                <span className="text-sm tabular-nums justify-self-end" style={{ color: 'var(--color-muted-foreground)' }}>{m.level ?? '—'}</span>
                <span className="text-sm" style={{ color: m.code ? 'var(--color-foreground)' : 'var(--color-danger)' }}>
                  {m.code ? classLabel(taxonomy, m.code as ClassCode) : 'Unknown'}
                </span>
                <span className="text-sm truncate" style={{ color: 'var(--color-muted-foreground)' }}>{m.group ?? '—'}</span>
                <span className="text-sm truncate" style={{ color: 'var(--color-muted-foreground)' }}>{m.rank ?? '—'}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
