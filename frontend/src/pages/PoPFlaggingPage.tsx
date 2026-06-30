import React, { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Flag, RefreshCw, AlertCircle, CheckCircle2, Circle, Lock,
  ChevronDown, ChevronRight, ScrollText, ListChecks, Share2,
} from 'lucide-react'
import { getPopFlagDataset, getPopFlags, setPopFlag } from '../services/api'
import type { PoPFlagStatus, PoPResolved } from '../types/popflag'
import { STEP_KIND_META, STEP_KIND_ORDER, stepKindMeta, roleMeta, ROLE_META } from '../lib/popFlagKind'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket } from '../hooks/useWebSocket'
import CharacterSubTabs from '../components/CharacterSubTabs'
import ImportSeerModal from '../components/ImportSeerModal'

// Graph view is lazy — @xyflow/react is ~4MB and most sessions stay on the
// checklist.
const PoPFlagGraphPanel = lazy(() => import('./PoPFlagGraphPanel'))

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

// labelFor maps a prereq flag ID to its short label for the locked tooltip.
function labelFor(flags: PoPFlagStatus[], id: string): string {
  return flags.find((f) => f.id === id)?.label ?? id
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

function ProvenanceChip({
  source, onConfirm,
}: { source?: string; onConfirm?: () => void }): React.ReactElement | null {
  if (!source) return null
  // Auto-detected flags are optimistic — render an amber, clickable chip the
  // user can click to confirm (promote to a manual row).
  if (source === 'auto') {
    return (
      <button
        type="button"
        onClick={onConfirm}
        disabled={!onConfirm}
        className="ml-2 shrink-0 rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
        title="Auto-detected from a kill — click to confirm"
        style={{
          backgroundColor: 'rgba(245,158,11,0.15)',
          color: '#f59e0b',
          border: '1px solid rgba(245,158,11,0.4)',
          cursor: onConfirm ? 'pointer' : 'default',
        }}
      >
        auto — confirm?
      </button>
    )
  }
  return (
    <span
      className="ml-2 shrink-0 rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        color: 'var(--color-muted)',
        border: '1px solid var(--color-border)',
      }}
    >
      {source === 'seer' ? 'Seer' : 'manual'}
    </span>
  )
}

interface FlagRowProps {
  flag: PoPFlagStatus
  allFlags: PoPFlagStatus[]
  requiredByDone: Set<string>
  canToggle: boolean
  busy: boolean
  onToggle: (flag: PoPFlagStatus) => void
  onConfirm: (flag: PoPFlagStatus) => void
}

function FlagRow({ flag, allFlags, requiredByDone, canToggle, busy, onToggle, onConfirm }: FlagRowProps): React.ReactElement {
  const missingLabels = (flag.missing ?? []).map((id) => labelFor(allFlags, id))
  const lockTitle = flag.locked ? `Needs: ${missingLabels.join(', ')}` : ''
  // Checking is blocked while prerequisites are unmet (must be done in order);
  // un-checking is blocked while a completed later step depends on this one
  // (must be retracted top-down). Confirming an already-done auto/seer
  // detection via the chip stays allowed.
  const lockedForCheck = flag.locked && !flag.done
  const lockedForUncheck = flag.done && requiredByDone.has(flag.id)
  // An any-of anchor satisfied via a checked member: toggling the anchor itself
  // would be a no-op (the member keeps it done), so steer the user to the
  // member instead.
  const anchorViaMember =
    flag.done && allFlags.some((o) => o.group === flag.id && o.done)
  const checkDisabled =
    !canToggle || busy || lockedForCheck || lockedForUncheck || anchorViaMember
  const checkTitle = !canToggle
    ? 'Select a character to track'
    : lockedForCheck
      ? `Complete prerequisites first — Needs: ${missingLabels.join(', ')}`
      : lockedForUncheck
        ? 'Required by a completed later step'
        : anchorViaMember
          ? 'Completed via an option below — uncheck that instead'
          : flag.done
            ? 'Mark not done'
            : 'Mark done'
  // Step-kind accent: a coloured left stripe + icon + chip so a player can tell
  // a raid kill from a must-act-now post-kill hail from solo homework. The
  // timed-hail kind also gets a faint row tint to make the easy-to-miss steps
  // stand out (left as transparent stripe when a kind is missing/unknown).
  const km = stepKindMeta(flag.step_kind)
  const KindIcon = km?.icon
  // Role badge (key / keyring / optional) for the non-required rows.
  const rm = roleMeta(flag.role)
  const RoleIcon = rm?.icon
  const isMember = !!flag.group
  // Superseded: an unchosen alternative in a satisfied any-of group — render it
  // faded + struck as "not needed". Dim optional rows that aren't done so they
  // read as "nice to have, not required".
  const dimmed = flag.done || flag.superseded
  const opacity = flag.superseded
    ? 0.45
    : flag.locked && !flag.done
      ? 0.6
      : rm && !flag.done
        ? 0.85
        : 1
  return (
    <div
      className="flex items-start gap-2 px-4 py-2"
      style={{
        borderTop: '1px solid var(--color-border)',
        borderLeft: `3px solid ${km ? km.color : 'transparent'}`,
        backgroundColor: km?.kind === 'timed_hail' && !flag.superseded ? km.bg : undefined,
        opacity,
        paddingLeft: isMember ? '2.25rem' : undefined,
      }}
    >
      <button
        type="button"
        onClick={() => onToggle(flag)}
        disabled={checkDisabled}
        className="mt-0.5 shrink-0"
        title={checkTitle}
        style={{ cursor: checkDisabled ? 'not-allowed' : 'pointer' }}
      >
        {flag.done ? (
          <CheckCircle2 size={16} style={{ color: 'var(--color-success)' }} />
        ) : (
          <Circle size={16} style={{ color: 'var(--color-muted)' }} />
        )}
      </button>
      <div className="min-w-0 flex-1">
        <div className="flex items-center">
          {KindIcon && (
            <span className="mr-1.5 shrink-0" title={km?.tip}>
              <KindIcon size={13} style={{ color: km!.color }} />
            </span>
          )}
          <span
            className="text-sm"
            style={{
              color: dimmed ? 'var(--color-muted)' : 'var(--color-foreground)',
              textDecoration: dimmed ? 'line-through' : 'none',
            }}
          >
            {flag.label}
          </span>
          {km && (
            <span
              className="ml-2 shrink-0 rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
              title={km.tip}
              style={{ color: km.color, backgroundColor: km.bg, border: `1px solid ${km.border}` }}
            >
              {km.label}
            </span>
          )}
          {rm && (
            <span
              className="ml-1.5 inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
              title={rm.tip}
              style={{ color: rm.color, backgroundColor: rm.bg, border: `1px solid ${rm.border}` }}
            >
              {RoleIcon && <RoleIcon size={9} />}
              {rm.label}
            </span>
          )}
          {flag.superseded && (
            <span
              className="ml-1.5 shrink-0 rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
              title="Another option in this group is done — this one is no longer needed."
              style={{ color: 'var(--color-muted)', backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
            >
              not needed
            </span>
          )}
          {flag.locked && !flag.done && (
            <span className="ml-1.5 shrink-0" title={lockTitle}>
              <Lock size={11} style={{ color: '#f87171' }} />
            </span>
          )}
          {flag.level ? (
            <span className="ml-2 shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>
              L{flag.level}
            </span>
          ) : null}
          {flag.done && (
            <ProvenanceChip
              source={flag.source}
              onConfirm={canToggle && !busy ? () => onConfirm(flag) : undefined}
            />
          )}
        </div>
        {flag.detail && (
          <p className="mt-0.5 text-[11px] leading-snug" style={{ color: 'var(--color-muted)' }}>
            {flag.detail}
          </p>
        )}
      </div>
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
                <FlagRow
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
  const [view, setView] = useState<'checklist' | 'graph'>('checklist')

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
          {([['checklist', 'Checklist', ListChecks], ['graph', 'Graph', Share2]] as const).map(
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
              Sync from Seer log
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

      {view === 'graph' ? (
        <Suspense
          fallback={
            <div className="flex flex-1 items-center justify-center">
              <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          }
        >
          <PoPFlagGraphPanel flags={resolved?.flags ?? []} />
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
