import React from 'react'
import { CheckCircle2, Circle, Lock } from 'lucide-react'
import type { PoPFlagStatus } from '../types/popflag'
import { stepKindMeta, roleMeta } from '../lib/popFlagKind'

// PopFlagRow renders one PoP flag as a checkable row: a coloured left stripe +
// icon for the step kind, a role badge for non-required rows, a lock icon
// with a "Needs: …" tooltip, and the provenance chip. Shared by the Checklist
// (PoPFlaggingPage's TierCard) and the Flow view (PoPFlagFlowPanel) so both
// views behave identically — same toggle rules, same badges, same tooltips.

// labelFor maps a prereq flag ID to its short label for the locked tooltip.
export function labelFor(flags: PoPFlagStatus[], id: string): string {
  return flags.find((f) => f.id === id)?.label ?? id
}

export function ProvenanceChip({
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
      {source === 'seer' ? 'Seer' : source === 'popflags' ? '#popflags' : 'manual'}
    </span>
  )
}

export interface PopFlagRowProps {
  flag: PoPFlagStatus
  allFlags: PoPFlagStatus[]
  requiredByDone: Set<string>
  canToggle: boolean
  busy: boolean
  onToggle: (flag: PoPFlagStatus) => void
  onConfirm: (flag: PoPFlagStatus) => void
  // compact drops the detail paragraph and tightens padding — used by the
  // Flow view's zone cards, where space is much tighter than a checklist tier.
  compact?: boolean
}

export function PopFlagRow({
  flag, allFlags, requiredByDone, canToggle, busy, onToggle, onConfirm, compact,
}: PopFlagRowProps): React.ReactElement {
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
      className={compact ? 'flex items-start gap-1.5 px-2 py-1.5' : 'flex items-start gap-2 px-4 py-2'}
      style={{
        borderTop: '1px solid var(--color-border)',
        borderLeft: `3px solid ${km ? km.color : 'transparent'}`,
        backgroundColor: km?.kind === 'timed_hail' && !flag.superseded ? km.bg : undefined,
        opacity,
        paddingLeft: isMember ? (compact ? '1.25rem' : '2.25rem') : undefined,
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
          <CheckCircle2 size={compact ? 13 : 16} style={{ color: 'var(--color-success)' }} />
        ) : (
          <Circle size={compact ? 13 : 16} style={{ color: 'var(--color-muted)' }} />
        )}
      </button>
      <div className="min-w-0 flex-1">
        <div className="flex items-center flex-wrap" title={compact ? flag.detail : undefined}>
          {KindIcon && (
            <span className="mr-1.5 shrink-0" title={km?.tip}>
              <KindIcon size={compact ? 11 : 13} style={{ color: km!.color }} />
            </span>
          )}
          <span
            className={compact ? 'text-[11px]' : 'text-sm'}
            style={{
              color: dimmed ? 'var(--color-muted)' : 'var(--color-foreground)',
              textDecoration: dimmed ? 'line-through' : 'none',
            }}
          >
            {flag.label}
          </span>
          {!compact && km && (
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
              {compact ? null : rm.label}
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
              <Lock size={compact ? 10 : 11} style={{ color: '#f87171' }} />
            </span>
          )}
          {!compact && flag.level ? (
            <span
              className="ml-2 shrink-0 text-[10px]"
              style={{ color: 'var(--color-muted)' }}
              title={`Min level to enter: ${flag.level}`}
            >
              L{flag.level}
            </span>
          ) : null}
          {!compact && flag.done && (
            <ProvenanceChip
              source={flag.source}
              onConfirm={canToggle && !busy ? () => onConfirm(flag) : undefined}
            />
          )}
        </div>
        {!compact && flag.detail && (
          <p className="mt-0.5 text-[11px] leading-snug" style={{ color: 'var(--color-muted)' }}>
            {flag.detail}
          </p>
        )}
      </div>
    </div>
  )
}
