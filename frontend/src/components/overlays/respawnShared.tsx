import React from 'react'
import { X } from 'lucide-react'
import { removeRespawn } from '../../services/api'
import type { RespawnTimer } from '../../types/respawn'

// fmtClock renders a remaining-seconds value as M:SS, or "Hh Mm" for the long
// respawns (some named/raid mobs are hours to days). 0:00 reads as "popped".
export function fmtClock(secs: number): string {
  const s = Math.max(0, Math.ceil(secs))
  if (s >= 86400) {
    const d = Math.floor(s / 86400)
    const h = Math.floor((s % 86400) / 3600)
    return `${d}d ${h}h`
  }
  if (s >= 3600) {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return `${h}h ${m}m`
  }
  const m = Math.floor(s / 60)
  const ss = s % 60
  return `${m}:${ss.toString().padStart(2, '0')}`
}

function barColor(remaining: number, total: number): string {
  if (total <= 0) return '#a855f7'
  const pct = remaining / total
  if (pct > 0.5) return '#a855f7' // purple while plenty of time remains
  if (pct > 0.2) return '#f97316'
  return '#ef4444'
}

interface RespawnRowProps {
  timer: RespawnTimer
  /** Current zone short_name, used to tag/dim rows from other zones. */
  currentZone: string
  variant: 'panel' | 'window'
}

// RespawnRow renders one death/respawn countdown. Shared between the dashboard
// panel and the popout window; the `variant` switches the colour palette so the
// window stays legible at low opacity (mirrors BuffTimer's two renderers).
export function RespawnRow({ timer, currentZone, variant }: RespawnRowProps): React.ReactElement {
  const win = variant === 'window'
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColor(timer.remaining_seconds, timer.duration_seconds)
  const popped = timer.remaining_seconds <= 0
  const otherZone = !!currentZone && timer.zone !== currentZone
  const label = String(timer.label_index).padStart(2, '0')

  const nameColor = win
    ? otherZone ? 'rgba(255,255,255,0.55)' : 'rgba(255,255,255,1)'
    : otherZone ? 'var(--color-muted)' : 'var(--color-foreground)'
  const mutedColor = win ? 'rgba(255,255,255,0.6)' : 'var(--color-muted)'
  const border = win ? '1px solid rgba(255,255,255,0.1)' : '1px solid var(--color-border)'
  const textShadow = win ? '0 1px 2px rgba(0,0,0,0.9)' : undefined

  return (
    <div style={{ position: 'relative', padding: '3px 10px', borderBottom: border, overflow: 'hidden', flexShrink: 0 }}>
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${pct * 100}%`, backgroundColor: color,
          opacity: win ? 0.5 : 0.15,
          pointerEvents: 'none', transition: 'width 1s linear',
        }}
      />
      <div style={{ position: 'relative', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <div style={{ display: 'flex', alignItems: 'baseline', gap: 6, minWidth: 0, flex: 1 }}>
          <span style={{ fontSize: 12, color: nameColor, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: win ? 500 : 400, textShadow }}>
            {timer.npc_name}{' '}
            <span style={{ color: mutedColor, fontVariantNumeric: 'tabular-nums' }}>{label}</span>
          </span>
          {otherZone && (
            <span style={{ fontSize: 10, color: mutedColor, flexShrink: 0, textShadow }}>
              [{timer.zone}]
            </span>
          )}
          {timer.ambiguous && timer.min_seconds && timer.max_seconds ? (
            <span
              title="Multiple spawns with this name have different respawn times — this is the most likely estimate"
              style={{ fontSize: 10, color: mutedColor, flexShrink: 0, textShadow }}
            >
              ? {fmtClock(timer.min_seconds)}–{fmtClock(timer.max_seconds)}
            </span>
          ) : null}
        </div>
        <span
          style={{
            fontSize: 11,
            color: popped ? '#22c55e' : color,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: win ? 600 : 400,
            textShadow,
          }}
        >
          {popped ? 'POP' : fmtClock(timer.remaining_seconds)}
        </span>
        <button
          onClick={() => removeRespawn(timer.id).catch(() => {})}
          title="Remove this timer"
          style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 0, color: mutedColor, display: 'flex', alignItems: 'center', flexShrink: 0, lineHeight: 0 }}
        >
          <X size={11} />
        </button>
      </div>
    </div>
  )
}
