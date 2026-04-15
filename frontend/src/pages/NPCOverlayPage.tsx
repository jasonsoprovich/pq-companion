import React, { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Crosshair, AlertTriangle, CheckCircle2, Circle } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getOverlayNPCTarget, getLogStatus } from '../services/api'
import { className, raceName, bodyTypeName } from '../lib/npcHelpers'
import type { TargetState, SpecialAbility } from '../types/overlay'
import type { LogTailerStatus } from '../types/logEvent'

// ── Helpers ────────────────────────────────────────────────────────────────────

function ConnPill({
  state,
  status,
}: {
  state: string
  status: LogTailerStatus | null
}): React.ReactElement {
  let color: string
  let label: string
  if (state !== 'open') {
    color = state === 'connecting' ? '#f97316' : '#6b7280'
    label = state === 'connecting' ? 'Connecting…' : 'Disconnected'
  } else if (!status || !status.enabled || !status.file_exists) {
    color = '#f97316'
    label = 'No Log'
  } else {
    color = '#22c55e'
    label = 'Live'
  }
  return (
    <span className="flex items-center gap-1.5 text-xs" style={{ color }}>
      <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: color }} />
      {label}
    </span>
  )
}

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  if (!status) {
    return (
      <div
        className="flex items-center gap-2 rounded px-3 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
      >
        <Circle size={10} />
        Loading status…
      </div>
    )
  }
  if (!status.enabled) {
    return (
      <div
        className="flex items-center gap-2 rounded px-3 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316' }}
      >
        <AlertTriangle size={12} />
        Log parsing is disabled. Enable it in{' '}
        <Link to="/settings" className="underline" style={{ color: 'var(--color-primary)' }}>
          Settings
        </Link>
        .
      </div>
    )
  }
  if (!status.file_exists) {
    return (
      <div
        className="flex items-center gap-2 rounded px-3 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316' }}
      >
        <AlertTriangle size={12} />
        Log file not found:{' '}
        <span className="font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
          {status.file_path || '(not configured)'}
        </span>
      </div>
    )
  }
  return (
    <div
      className="flex items-center gap-2 rounded px-3 py-2 text-xs"
      style={{ backgroundColor: 'var(--color-surface-2)', color: '#22c55e' }}
    >
      <CheckCircle2 size={12} />
      <span>Tailing</span>
      <span className="font-mono" style={{ color: 'var(--color-muted)' }}>
        {status.file_path}
      </span>
    </div>
  )
}

// ── Ability badge colours ──────────────────────────────────────────────────────

const DANGER_ABILITIES = new Set([1, 2, 3, 4, 5, 12, 13])
const IMMUNE_ABILITIES = new Set([17, 18, 19, 20])

function abilityBadgeColor(code: number): string {
  if (DANGER_ABILITIES.has(code)) return '#dc2626'  // red
  if (IMMUNE_ABILITIES.has(code)) return '#f97316'  // orange
  return '#6b7280'                                   // gray
}

function AbilityBadge({ ability }: { ability: SpecialAbility }): React.ReactElement {
  return (
    <span
      className="rounded px-2 py-0.5 text-[11px] font-semibold text-white"
      style={{ backgroundColor: abilityBadgeColor(ability.code) }}
    >
      {ability.name || `Ability ${ability.code}`}
    </span>
  )
}

// ── Stat cell ──────────────────────────────────────────────────────────────────

function Stat({
  label,
  value,
  color,
}: {
  label: string
  value: string | number
  color?: string
}): React.ReactElement {
  return (
    <div
      className="flex flex-col items-center rounded px-2 py-1.5"
      style={{ backgroundColor: 'var(--color-surface-2)', minWidth: '4rem' }}
    >
      <span className="text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
        {label}
      </span>
      <span
        className="mt-0.5 text-sm font-semibold tabular-nums"
        style={{ color: color ?? 'var(--color-foreground)' }}
      >
        {value}
      </span>
    </div>
  )
}

// ── No-target state ────────────────────────────────────────────────────────────

function NoTarget({ zone }: { zone?: string }): React.ReactElement {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3">
      <Crosshair size={40} style={{ color: 'var(--color-muted)' }} />
      <p className="text-sm font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
        No target
      </p>
      {zone && (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          Zone: {zone}
        </p>
      )}
      <p className="max-w-xs text-center text-xs" style={{ color: 'var(--color-muted)' }}>
        Attack or engage an NPC and its info will appear here automatically.
      </p>
    </div>
  )
}

// ── NPC Card ───────────────────────────────────────────────────────────────────

function NPCCard({ state }: { state: TargetState }): React.ReactElement {
  const npc = state.npc_data
  const abilities = state.special_abilities ?? []

  const lastUpdated = new Date(state.last_updated).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })

  return (
    <div className="flex flex-1 flex-col gap-4 overflow-y-auto p-4">
      {/* Target name + meta */}
      <div
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
      >
        <div className="flex items-start justify-between gap-2">
          <div>
            <h2
              className="text-lg font-bold leading-tight"
              style={{ color: 'var(--color-foreground)' }}
            >
              {state.target_name ?? 'Unknown'}
            </h2>
            {state.current_zone && (
              <p className="mt-0.5 text-xs" style={{ color: 'var(--color-muted)' }}>
                {state.current_zone}
              </p>
            )}
          </div>
          <span className="shrink-0 text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
            {lastUpdated}
          </span>
        </div>

        {/* Flags */}
        {npc && (npc.raid_target === 1 || npc.rare_spawn === 1) && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {npc.raid_target === 1 && (
              <span className="rounded px-2 py-0.5 text-[11px] font-semibold text-white" style={{ backgroundColor: '#7c3aed' }}>
                RAID TARGET
              </span>
            )}
            {npc.rare_spawn === 1 && (
              <span className="rounded px-2 py-0.5 text-[11px] font-semibold text-white" style={{ backgroundColor: '#b45309' }}>
                RARE SPAWN
              </span>
            )}
          </div>
        )}
      </div>

      {npc ? (
        <>
          {/* Class / Race / Body */}
          <div>
            <p
              className="mb-2 text-[10px] font-semibold uppercase tracking-widest"
              style={{ color: 'var(--color-muted)' }}
            >
              Identity
            </p>
            <div className="flex flex-wrap gap-2">
              <Stat label="Level" value={npc.level} color="var(--color-primary)" />
              <Stat label="Class" value={className(npc.class)} />
              <Stat label="Race" value={raceName(npc.race)} />
              <Stat label="Body" value={bodyTypeName(npc.body_type)} />
            </div>
          </div>

          {/* Combat stats */}
          <div>
            <p
              className="mb-2 text-[10px] font-semibold uppercase tracking-widest"
              style={{ color: 'var(--color-muted)' }}
            >
              Combat
            </p>
            <div className="flex flex-wrap gap-2">
              <Stat label="HP" value={npc.hp.toLocaleString()} color="#22c55e" />
              <Stat label="AC" value={npc.ac} />
              <Stat label="Min DMG" value={npc.min_dmg} color="#ef4444" />
              <Stat label="Max DMG" value={npc.max_dmg} color="#ef4444" />
              <Stat label="Attacks" value={npc.attack_count} />
            </div>
          </div>

          {/* Resists */}
          <div>
            <p
              className="mb-2 text-[10px] font-semibold uppercase tracking-widest"
              style={{ color: 'var(--color-muted)' }}
            >
              Resists
            </p>
            <div className="flex flex-wrap gap-2">
              <Stat label="Magic" value={npc.mr} />
              <Stat label="Cold" value={npc.cr} />
              <Stat label="Disease" value={npc.dr} />
              <Stat label="Fire" value={npc.fr} />
              <Stat label="Poison" value={npc.pr} />
            </div>
          </div>

          {/* Attributes */}
          <div>
            <p
              className="mb-2 text-[10px] font-semibold uppercase tracking-widest"
              style={{ color: 'var(--color-muted)' }}
            >
              Attributes
            </p>
            <div className="flex flex-wrap gap-2">
              <Stat label="STR" value={npc.str} />
              <Stat label="STA" value={npc.sta} />
              <Stat label="DEX" value={npc.dex} />
              <Stat label="AGI" value={npc.agi} />
              <Stat label="INT" value={npc.int} />
              <Stat label="WIS" value={npc.wis} />
              <Stat label="CHA" value={npc.cha} />
            </div>
          </div>

          {/* Special Abilities */}
          {abilities.length > 0 && (
            <div>
              <p
                className="mb-2 text-[10px] font-semibold uppercase tracking-widest"
                style={{ color: 'var(--color-muted)' }}
              >
                Special Abilities
              </p>
              <div className="flex flex-wrap gap-1.5">
                {abilities
                  .filter((a) => a.value !== 0)
                  .map((a) => (
                    <AbilityBadge key={a.code} ability={a} />
                  ))}
              </div>
            </div>
          )}
        </>
      ) : (
        // Target name inferred from log but no DB match
        <div
          className="rounded px-3 py-2 text-xs"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
        >
          No database record found for this NPC. It may be a pet, player, or unknown entity.
        </div>
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function NPCOverlayPage(): React.ReactElement {
  const [target, setTarget] = useState<TargetState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)

  useEffect(() => {
    getOverlayNPCTarget()
      .then(setTarget)
      .catch(() => setTarget(null))

    getLogStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== 'overlay:npc_target') return
    setTarget(msg.data as TargetState)
  }, [])

  const wsState = useWebSocket(handleMessage)

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header */}
      <div
        className="flex shrink-0 items-center justify-between border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-2">
          <Crosshair size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            NPC Overlay
          </h1>
        </div>
        <ConnPill state={wsState} status={status} />
      </div>

      {/* Tailer status */}
      <div className="shrink-0 border-b px-4 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <StatusBar status={status} />
      </div>

      {/* Content */}
      {target === null ? (
        <div className="flex flex-1 items-center justify-center">
          <p className="text-sm" style={{ color: 'var(--color-muted)' }}>Loading…</p>
        </div>
      ) : target.has_target ? (
        <NPCCard state={target} />
      ) : (
        <NoTarget zone={target.current_zone} />
      )}
    </div>
  )
}
