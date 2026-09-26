import React, { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Flag, RefreshCw, AlertCircle,
  ChevronDown, ChevronRight, ScrollText, ListChecks, Waypoints,
} from 'lucide-react'
import { getPopFlagDataset, getPopFlags, setPopFlag } from '../services/api'
import type { PoPFlagStatus, PoPResolved } from '../types/popflag'
import { STEP_KIND_META, STEP_KIND_ORDER, ROLE_META } from '../lib/popFlagKind'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket } from '../hooks/useWebSocket'
import CharacterSubTabs from '../components/CharacterSubTabs'
import ImportSeerModal from '../components/ImportSeerModal'
import { PopFlagRow } from '../components/PopFlagRow'

// Flow view is lazy — it's only needed once a user switches off the checklist.
const PoPFlagFlowPanel = lazy(() => import('./PoPFlagFlowPanel'))

// ── Helpers ──────────────────────────────────────────────────────────────────

// buildEmptyResolved synthesizes a resolver result from the dataset alone, for
// when no character is selected (or the store is unavailable): every flag shows
// as not-done, and lock state is computed from prereqs (all unmet → locked).
function buildEmptyResolved(flags: PoPFlagStatus[]): PoPResolved {
  const tiers = new Map<number, { done: number; total: number }>()
  const zones = new Map<string, { done: number; total: number }>()
  let total = 0
  for (const f of flags) {
    // Mirror the backend tally: optional rows (keys/keyrings/bonus) and any-of
    // members don't count toward completion.
    if (f.optional || f.group) continue
    total++
    const t = tiers.get(f.tier) ?? { done: 0, total: 0 }
    t.total++
    tiers.set(f.tier, t)
    const z = zones.get(f.zone) ?? { done: 0, total: 0 }
    z.total++
    zones.set(f.zone, z)
  }
  return {
    flags,
    tiers: [...tiers.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([tier, c]) => ({ tier, key: tierLabel(tier), label: tierLabel(tier), ...c })),
    zones: [...zones.entries()].map(([zone, c]) => ({ key: zone, label: zone, ...c })),
    done: 0,
    total,
  }
}

function tierLabel(t: number): string {
  return t === 5 ? 'Plane of Time' : `Tier ${t}`
}

// pendingLabel maps a 'cl_*' checklist flag name to the display text the
// server itself prints for it (popflags.cpp's PopFlagsPrintPending calls) —
// falls back to a stripped/title-cased name for a checklist flag the parser
// doesn't recognize (forward-compat with a future upstream addition).
const PENDING_LABELS: Record<string, string> = {
  cl_grummus: 'Grummus',
  cl_maze: "Thelin's hedge maze",
  cl_behemoth: 'Manaetic Behemoth',
  cl_aerindar: 'Aerin`Dar',
  cl_terris: 'Terris Thule',
  cl_bertox: 'Bertoxxulous',
  cl_keeper: 'Keeper of Sorrows',
  cl_saryrn: 'Saryrn',
  cl_vallon: 'Vallon Zek',
  cl_tallon: 'Tallon Zek',
  cl_rallos: 'Rallos Zek',
  cl_karana: 'Karana',
  cl_solusek: 'Solusek Ro',
}

function pendingLabel(name: string): string {
  return PENDING_LABELS[name] ?? name.replace(/^cl_/, '').replace(/^./, (c) => c.toUpperCase())
}

// ── Sub-components ────────────────────────────────────────────────────────────

function ProgressBar({ done, total }: { done: number; total: number }): React.ReactElement {
  const pct = total === 0 ? 0 : Math.round((done / total) * 100)
  const complete = done === total && total > 0
  return (
    <div className="flex items-center gap-2">
      <div
        className="h-1.5 flex-1 rounded-full overflow-hidden"
        style={{ backgroundColor: 'var(--color-surface-2)' }}
      >
        <div
          className="h-full rounded-full transition-all"
          style={{
            width: `${pct}%`,
            backgroundColor: complete ? 'var(--color-success)' : 'var(--color-primary)',
          }}
        />
      </div>
      <span
        className="text-[10px] tabular-nums shrink-0"
        style={{ color: complete ? 'var(--color-success)' : 'var(--color-muted-foreground)' }}
      >
        {done} / {total}
      </span>
    </div>
  )
}

interface TierCardProps {
  tier: number
  flags: PoPFlagStatus[]
  done: number
  total: number
  canToggle: boolean
  busyId: string | null
  onToggle: (flag: PoPFlagStatus) => void
  onConfirm: (flag: PoPFlagStatus) => void
  allFlags: PoPFlagStatus[]
  requiredByDone: Set<string>
  defaultOpen: boolean
}

function TierCard({
  tier, flags, done, total, canToggle, busyId, onToggle, onConfirm, allFlags, requiredByDone, defaultOpen,
}: TierCardProps): React.ReactElement {
  const [open, setOpen] = useState(defaultOpen)
  const complete = done === total && total > 0

  // Group this tier's flags by zone, preserving dataset order.
  const zones = useMemo(() => {
    const order: string[] = []
    const byZone = new Map<string, PoPFlagStatus[]>()
    for (const f of flags) {
      if (!byZone.has(f.zone)) {
        byZone.set(f.zone, [])
        order.push(f.zone)
      }
      byZone.get(f.zone)!.push(f)
    }
    return order.map((z) => ({ zone: z, flags: byZone.get(z)! }))
  }, [flags])

  return (
    <div
      className="rounded-lg overflow-hidden"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${complete ? 'var(--color-success)' : 'var(--color-border)'}`,
      }}
    >
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left"
        onClick={() => setOpen((v) => !v)}
      >
        <span style={{ color: complete ? 'var(--color-success)' : 'var(--color-muted)' }}>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
        <span className="flex-1 text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {tierLabel(tier)}
        </span>
        <div className="w-32 shrink-0">
          <ProgressBar done={done} total={total} />
        </div>
      </button>
      {open && (
        <div className="border-t" style={{ borderColor: 'var(--color-border)' }}>
          {zones.map(({ zone, flags: zoneFlags }) => (
            <div key={zone}>
              <div
                className="px-4 py-1.5 text-[10px] font-semibold uppercase tracking-wider"
                style={{ color: 'var(--color-muted)', backgroundColor: 'var(--color-surface-2)' }}
              >
                {zone}
              </div>
              {zoneFlags.map((f) => (
                <PopFlagRow
                  key={f.id}
                  flag={f}
                  allFlags={allFlags}
                  requiredByDone={requiredByDone}
                  canToggle={canToggle}
                  busy={busyId === f.id}
                  onToggle={onToggle}
                  onConfirm={onConfirm}
                />
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function PoPFlaggingPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewedCharacter, setViewedCharacter] = useState('')
  const [resolved, setResolved] = useState<PoPResolved | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [showImport, setShowImport] = useState(false)
  const [view, setView] = useState<'checklist' | 'flow'>('checklist')

  // Default the viewed character to the active character once known.
  useEffect(() => {
    if (!viewedCharacter && active) setViewedCharacter(active)
  }, [active, viewedCharacter])

  // Monotonic token so only the most recent fetch wins. Without it, the
  // mount-time empty-preview load (viewedCharacter '') can resolve AFTER the
  // real per-character load and clobber it with a blank state. POST handlers
  // bump it too, so a fresh toggle result can't be overwritten by a slow GET.
  const loadSeq = useRef(0)

  const load = useCallback(() => {
    const seq = ++loadSeq.current
    setLoading(true)
    setError(null)
    const p = viewedCharacter
      ? getPopFlags(viewedCharacter)
      : getPopFlagDataset().then((d) => buildEmptyResolved(d.flags as PoPFlagStatus[]))
    p
      .then((r) => { if (seq === loadSeq.current) setResolved(r) })
      .catch((err: Error) => { if (seq === loadSeq.current) setError(err.message) })
      .finally(() => { if (seq === loadSeq.current) setLoading(false) })
  }, [viewedCharacter])

  useEffect(() => { load() }, [load])

  // Live refresh when a Seer reading (paste-in or live-log) commits for the
  // viewed character.
  useWebSocket((msg) => {
    if (msg.type !== 'popflag.snapshot') return
    const snapChar = (msg.data as { character?: string } | null)?.character ?? ''
    if (snapChar && viewedCharacter && snapChar.toLowerCase() === viewedCharacter.toLowerCase()) {
      load()
    }
  })

  // Locked flags can't be checked (the UI disables them), so a toggle failure
  // here is rare/transient — leave the page as-is rather than surfacing an
  // error. The backend stays authoritative either way.
  // applyAuthoritative records a server response from a user action and
  // invalidates any in-flight load so a slow GET can't revert it.
  const applyAuthoritative = useCallback((r: PoPResolved) => {
    loadSeq.current++
    setResolved(r)
  }, [])

  const onToggle = useCallback((flag: PoPFlagStatus) => {
    if (!viewedCharacter) return
    setBusyId(flag.id)
    setPopFlag(viewedCharacter, flag.id, !flag.done)
      .then(applyAuthoritative)
      .catch(() => {})
      .finally(() => setBusyId(null))
  }, [viewedCharacter, applyAuthoritative])

  // Promote an auto-detected flag to a confirmed manual row.
  const onConfirm = useCallback((flag: PoPFlagStatus) => {
    if (!viewedCharacter) return
    setBusyId(flag.id)
    setPopFlag(viewedCharacter, flag.id, true)
      .then(applyAuthoritative)
      .catch(() => {})
      .finally(() => setBusyId(null))
  }, [viewedCharacter, applyAuthoritative])

  // Flags that a currently-done flag depends on — these can't be un-checked
  // (retraction must go top-down).
  const requiredByDone = useMemo(() => {
    const s = new Set<string>()
    for (const f of resolved?.flags ?? []) {
      if (f.done) for (const p of f.prereqs) s.add(p)
    }
    return s
  }, [resolved])

  // Group flags by tier, preserving the (already tier-ordered) tier tallies.
  const tiers = useMemo(() => {
    if (!resolved) return []
    const byTier = new Map<number, PoPFlagStatus[]>()
    for (const f of resolved.flags) {
      if (!byTier.has(f.tier)) byTier.set(f.tier, [])
      byTier.get(f.tier)!.push(f)
    }
    return resolved.tiers.map((t) => ({
      progress: t,
      flags: byTier.get(t.tier ?? 0) ?? [],
    }))
  }, [resolved])

  if (loading && !resolved) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  // Full-page error only on an initial load failure (nothing to show yet).
  // Action failures keep the page and surface as a dismissible banner.
  if (error && !resolved) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
        <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
        <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>{error}</p>
        <button
          onClick={load}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Retry
        </button>
      </div>
    )
  }

  const canToggle = viewedCharacter !== ''

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-2 border-b px-4 py-2.5 shrink-0"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        <Flag size={16} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          PoP Flags
        </span>
        {resolved && (
          <div className="ml-4 w-48">
            <ProgressBar done={resolved.done} total={resolved.total} />
          </div>
        )}
        <div className="ml-4 flex items-center gap-1">
          {([['checklist', 'Checklist', ListChecks], ['flow', 'Flow', Waypoints]] as const).map(
            ([v, label, Icon]) => {
              const isActive = view === v
              return (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
                  style={{
                    backgroundColor: isActive ? 'var(--color-surface-2)' : 'transparent',
                    color: isActive ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                    border: `1px solid ${isActive ? 'var(--color-border)' : 'transparent'}`,
                  }}
                >
                  <Icon size={12} />
                  {label}
                </button>
              )
            },
          )}
        </div>
        <div className="ml-auto flex items-center gap-2">
          {canToggle && (
            <button
              onClick={() => setShowImport(true)}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <ScrollText size={11} />
              Sync from game
            </button>
          )}
          <button
            onClick={load}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
        </div>
      </div>

      {/* Per-character switcher */}
      <CharacterSubTabs value={viewedCharacter} onChange={setViewedCharacter} />

      {!canToggle && (
        <p className="px-4 py-2 text-[11px] shrink-0" style={{ color: 'var(--color-muted)' }}>
          Select a character above to track flags. Showing the full flag list as a preview.
        </p>
      )}

      {/* Pending checklist memories — named by the last Seer/#popflags reading
          but not yet turned into a real character flag. Hidden once the
          character has the Plane of Time flag (the server stops tracking
          these the moment 'time' is granted). */}
      {(() => {
        const pending = resolved?.pending ?? []
        const timeDone = resolved?.flags.find((f) => f.id === 'potime')?.done ?? false
        if (pending.length === 0 || timeDone) return null
        return (
          <div
            className="flex items-start gap-2 px-4 py-2 text-[11px] shrink-0"
            style={{ backgroundColor: 'rgba(245,158,11,0.10)', color: '#f59e0b' }}
          >
            <AlertCircle size={13} className="mt-0.5 shrink-0" />
            <span>
              Pending checklist {pending.length === 1 ? 'memory' : 'memories'} named:{' '}
              {pending.map(pendingLabel).join(', ')}. Sit near Seer Mal Nae`Shi and say{' '}
              <code>unlock memories</code>, then re-sync.
            </span>
          </div>
        )
      })()}

      {/* Step-type legend — explains the per-row icon/colour coding. */}
      <div
        className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-4 py-1.5 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <span className="text-[10px] uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
          Step type
        </span>
        {STEP_KIND_ORDER.map((k) => {
          const m = STEP_KIND_META[k]
          const Icon = m.icon
          return (
            <span
              key={k}
              className="inline-flex items-center gap-1 text-[11px]"
              title={m.tip}
              style={{ color: m.color }}
            >
              <Icon size={12} />
              {m.label}
            </span>
          )
        })}
        <span
          className="mx-1 h-3 w-px shrink-0"
          style={{ backgroundColor: 'var(--color-border)' }}
          aria-hidden
        />
        <span className="text-[10px] uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
          Not counted
        </span>
        {(['key', 'keyring', 'optional'] as const).map((r) => {
          const m = ROLE_META[r]
          const Icon = m.icon
          return (
            <span
              key={r}
              className="inline-flex items-center gap-1 text-[11px]"
              title={m.tip}
              style={{ color: m.color }}
            >
              <Icon size={12} />
              {m.label}
            </span>
          )
        })}
      </div>

      {view === 'flow' ? (
        <Suspense
          fallback={
            <div className="flex flex-1 items-center justify-center">
              <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          }
        >
          <PoPFlagFlowPanel
            flags={resolved?.flags ?? []}
            canToggle={canToggle}
            busyId={busyId}
            onToggle={onToggle}
            onConfirm={onConfirm}
            requiredByDone={requiredByDone}
          />
        </Suspense>
      ) : (
        /* Tier cards */
        <div className="flex-1 overflow-y-auto p-4 space-y-3">
          {tiers.map(({ progress, flags }) => (
            <TierCard
              key={progress.tier}
              tier={progress.tier ?? 0}
              flags={flags}
              done={progress.done}
              total={progress.total}
              canToggle={canToggle}
              busyId={busyId}
              onToggle={onToggle}
              onConfirm={onConfirm}
              allFlags={resolved?.flags ?? []}
              requiredByDone={requiredByDone}
              defaultOpen={progress.done < progress.total}
            />
          ))}
        </div>
      )}

      {showImport && canToggle && (
        <ImportSeerModal
          character={viewedCharacter}
          onClose={() => setShowImport(false)}
          onCommitted={applyAuthoritative}
        />
      )}
    </div>
  )
}
