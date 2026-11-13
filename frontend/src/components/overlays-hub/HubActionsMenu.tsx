/**
 * HubActionsMenu — the ⋯ menu in the Overlays hub header, for actions on
 * every overlay at once: rerun overlay setup, close all popouts, recenter every window, clear all
 * timers (those two ask first), and turn collapse-when-idle on or off for
 * every overlay that offers it.
 */
import React, { useEffect, useRef, useState } from 'react'
import { ChevronsDownUp, ChevronsUpDown, Crosshair, MoreHorizontal, Trash2, Wand2, X } from 'lucide-react'
import { ConfirmModal } from '../ConfirmModal'
import { clearTimers, getConfig, listTimerGroups, saveConfigChanges } from '../../services/api'
import { useOverlayPopouts } from '../../hooks/useOverlayPopouts'
import { DEFAULT_IDLE_SECS, IDLE_COLLAPSE_OVERLAYS } from '../../lib/idleCollapse'

// On: every supported overlay and timer window collapses when idle, keeping
// a delay already picked (new ones get the default). Off: none do.
export async function setIdleCollapseForAll(on: boolean): Promise<void> {
  const base = await getConfig()
  let next: Record<string, number> = {}
  if (on) {
    const groups = await listTimerGroups().catch(() => [])
    const keys = [...IDLE_COLLAPSE_OVERLAYS, ...groups.map((g) => `customTimer:${g.id}`)]
    const cur = base.preferences.overlay_idle_collapse ?? {}
    next = Object.fromEntries(keys.map((k) => [k, cur[k] > 0 ? cur[k] : DEFAULT_IDLE_SECS]))
  }
  await saveConfigChanges(
    { ...base, preferences: { ...base.preferences, overlay_idle_collapse: next } },
    base,
  )
}

type Confirm = 'positions' | 'timers' | null

function MenuItem({
  icon: Icon,
  label,
  hint,
  disabled,
  onClick,
}: {
  icon: React.ElementType
  label: string
  hint: string
  disabled?: boolean
  onClick: () => void
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className="flex w-full items-start gap-2 rounded px-2 py-1.5 text-left hover:bg-[var(--color-surface-2)]"
      style={{ opacity: disabled ? 0.45 : 1, cursor: disabled ? 'not-allowed' : 'pointer' }}
    >
      <Icon size={12} className="mt-0.5 shrink-0" style={{ color: 'var(--color-muted-foreground)' }} />
      <span className="min-w-0">
        <span className="block text-xs" style={{ color: 'var(--color-foreground)' }}>{label}</span>
        <span className="block text-[11px] leading-snug" style={{ color: 'var(--color-muted-foreground)' }}>
          {hint}
        </span>
      </span>
    </button>
  )
}

export default function HubActionsMenu(): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [confirm, setConfirm] = useState<Confirm>(null)
  const wrapRef = useRef<HTMLDivElement>(null)
  const { visibility, closeAll, refresh } = useOverlayPopouts()
  const canReset = typeof window.electron?.overlay?.resetAllPositions === 'function'
  const anyOpen = visibility.open + visibility.hidden > 0

  useEffect(() => {
    if (!open) return
    refresh()
    const onDown = (e: MouseEvent): void => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, refresh])

  const pick = (fn: () => void): void => {
    setOpen(false)
    fn()
  }

  return (
    <div ref={wrapRef} className="relative shrink-0">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex items-center rounded px-1.5 py-1"
        style={{
          backgroundColor: open ? 'var(--color-surface-2)' : 'transparent',
          color: 'var(--color-foreground)',
          border: '1px solid var(--color-border)',
        }}
        title="More actions for every overlay"
        aria-label="More overlay actions"
      >
        <MoreHorizontal size={14} />
      </button>
      {open && (
        <div
          className="absolute right-0 z-50 flex w-72 flex-col gap-0.5 rounded-lg p-1.5"
          style={{
            top: 'calc(100% + 6px)',
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
          }}
        >
          {typeof window.electron?.overlay?.setupApply === 'function' && (
            <MenuItem
              icon={Wand2}
              label="Set up overlays…"
              hint="Pick overlays for your class and place them around your screen, as a new layout or in place of this one."
              onClick={() => pick(() => window.dispatchEvent(new Event('pq:open-overlay-setup')))}
            />
          )}
          <MenuItem
            icon={X}
            label="Close all popouts"
            hint="Closes every overlay window. The layout keeps them, so they come back next time."
            disabled={!anyOpen}
            onClick={() => pick(closeAll)}
          />
          {canReset && (
            <MenuItem
              icon={Crosshair}
              label="Reset all window positions"
              hint="Moves every overlay window to the middle of your main screen and unlocks it."
              onClick={() => pick(() => setConfirm('positions'))}
            />
          )}
          <MenuItem
            icon={Trash2}
            label="Clear all timers"
            hint="Clears every buff, detrimental and custom timer, e.g. after switching characters."
            onClick={() => pick(() => setConfirm('timers'))}
          />
          <MenuItem
            icon={ChevronsDownUp}
            label="Collapse idle overlays: on for all"
            hint={`Every overlay that can folds to its title bar after ${DEFAULT_IDLE_SECS}s with nothing to show. Change the delay on each card.`}
            onClick={() => pick(() => { setIdleCollapseForAll(true).catch(() => {}) })}
          />
          <MenuItem
            icon={ChevronsUpDown}
            label="Collapse idle overlays: off for all"
            hint="No overlay folds itself when idle. The chevron still collapses one by hand."
            onClick={() => pick(() => { setIdleCollapseForAll(false).catch(() => {}) })}
          />
        </div>
      )}
      {confirm === 'positions' && (
        <ConfirmModal
          title="Reset Window Positions"
          message="Recenter all pop-out overlay windows on your primary monitor and unlock them? This is the recovery path for an overlay that has drifted off-screen."
          confirmLabel="Reset Positions"
          onConfirm={() => {
            setConfirm(null)
            window.electron?.overlay?.resetAllPositions?.().catch(() => {})
          }}
          onCancel={() => setConfirm(null)}
        />
      )}
      {confirm === 'timers' && (
        <ConfirmModal
          title="Clear All Timers"
          message="Clear every active buff, detrimental, and custom timer? Handy after switching characters so the old character's buffs stop showing. This can't be undone."
          confirmLabel="Clear All Timers"
          onConfirm={() => {
            setConfirm(null)
            clearTimers('all').catch(() => {})
          }}
          onCancel={() => setConfirm(null)}
        />
      )}
    </div>
  )
}
