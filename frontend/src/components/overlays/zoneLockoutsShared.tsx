import React from 'react'
import type { ZoneLockoutBoss } from '../../types/lockouts'

export type ZoneLockoutStatus = 'locked' | 'available' | 'unknown'

export interface ZoneLockoutRowData {
  targetName: string
  npcId?: number
  status: ZoneLockoutStatus
  remainingMs: number
}

// deriveZoneLockoutRow reduces a zone's raid-target boss (which carries every
// character's status, same shape as the Zones tab's Lockouts sub-tab) down to
// just the active character's status — this overlay is a single-character
// glance, not the all-characters view. A character with no captured entry for
// a boss is "unknown" (never `/sll`'d or kill-noticed it), distinct from an
// entry with expires_at = 0 which is an explicitly observed "available".
export function deriveZoneLockoutRow(
  boss: ZoneLockoutBoss,
  character: string,
  nowMs: number,
): ZoneLockoutRowData {
  const entry = boss.characters?.find(
    (c) => c.character.toLowerCase() === character.toLowerCase(),
  )
  if (!entry) {
    return { targetName: boss.target_name, npcId: boss.npc_id, status: 'unknown', remainingMs: 0 }
  }
  const remainingMs = entry.expires_at > 0 ? entry.expires_at * 1000 - nowMs : 0
  const available = entry.expires_at === 0 || remainingMs <= 0
  return {
    targetName: boss.target_name,
    npcId: boss.npc_id,
    status: available ? 'available' : 'locked',
    remainingMs: available ? 0 : remainingMs,
  }
}

// Locked-soonest-available first, then available, then unknown — the point of
// this overlay is "what am I still locked out of," so that's what leads.
export function sortZoneLockoutRows(rows: ZoneLockoutRowData[]): ZoneLockoutRowData[] {
  const rank = (s: ZoneLockoutStatus): number => (s === 'locked' ? 0 : s === 'available' ? 1 : 2)
  return [...rows].sort((a, b) => {
    const r = rank(a.status) - rank(b.status)
    if (r !== 0) return r
    if (a.status === 'locked') return a.remainingMs - b.remainingMs
    return a.targetName.localeCompare(b.targetName)
  })
}

// fmtLockoutRemaining renders a remaining-ms value coarsely (days/hours/
// minutes) — loot lockouts run hours to days, so second-level precision would
// just be visual noise here.
export function fmtLockoutRemaining(ms: number): string {
  let s = Math.max(0, Math.floor(ms / 1000))
  const d = Math.floor(s / 86400)
  s -= d * 86400
  const h = Math.floor(s / 3600)
  s -= h * 3600
  const m = Math.floor(s / 60)
  const parts: string[] = []
  if (d > 0) parts.push(`${d}d`)
  if (d > 0 || h > 0) parts.push(`${h}h`)
  parts.push(`${m}m`)
  return parts.join(' ')
}

function statusColor(status: ZoneLockoutStatus): string {
  if (status === 'available') return '#22c55e'
  if (status === 'locked') return '#ef4444'
  return '#6b7280'
}

function statusLabel(row: ZoneLockoutRowData): string {
  if (row.status === 'available') return 'Available'
  if (row.status === 'locked') return fmtLockoutRemaining(row.remainingMs)
  return 'Unknown'
}

interface ZoneLockoutRowProps {
  row: ZoneLockoutRowData
  variant: 'panel' | 'window'
  onNavigate?: (npcId: number) => void
}

// ZoneLockoutRow renders one boss's lockout status for the active character.
// Shared between the dashboard panel and the popout window, mirroring
// RespawnRow's panel/window split (respawnShared.tsx).
export function ZoneLockoutRow({ row, variant, onNavigate }: ZoneLockoutRowProps): React.ReactElement {
  const win = variant === 'window'
  const color = statusColor(row.status)
  const border = win ? '1px solid rgba(255,255,255,0.1)' : '1px solid var(--color-border)'
  const textShadow = win ? '0 1px 2px rgba(0,0,0,0.9)' : undefined
  const nameColor = win ? 'rgba(255,255,255,1)' : 'var(--color-foreground)'
  const clickable = !!row.npcId && !!onNavigate

  return (
    <div
      style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        gap: 8,
        padding: '4px 10px',
        borderBottom: border,
        flexShrink: 0,
      }}
    >
      <span
        onClick={clickable ? () => onNavigate!(row.npcId!) : undefined}
        title={clickable ? 'Open in the NPC database' : undefined}
        style={{
          fontSize: 12,
          color: nameColor,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          fontWeight: win ? 500 : 400,
          textShadow,
          cursor: clickable ? 'pointer' : undefined,
          textDecoration: clickable ? 'underline' : undefined,
          textDecorationStyle: clickable ? 'dotted' : undefined,
        }}
      >
        {row.targetName}
      </span>
      <span
        style={{
          fontSize: 11,
          color,
          fontVariantNumeric: 'tabular-nums',
          flexShrink: 0,
          fontWeight: win ? 600 : 400,
          textShadow,
        }}
      >
        {statusLabel(row)}
      </span>
    </div>
  )
}
