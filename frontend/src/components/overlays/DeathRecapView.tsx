/**
 * DeathRecapView — the last 20 s before one death: who killed you, damage
 * and healing taken, CHs landed, time since the last CH, and the incoming
 * events newest first. Shared by the Combat Log page, the Death Recap
 * dashboard panel and its popout window.
 *
 * The `theme` prop picks the colour set: 'panel' uses the app's CSS tokens,
 * 'overlay' uses the translucent-white palette every popout overlay uses.
 */
import React from 'react'
import type { DeathRecap, DeathRecord, RecapEvent } from '../../types/combat'

export type DeathRecapTheme = 'panel' | 'overlay'

interface Palette {
  fg: string
  muted: string
  border: string
  chip: string
}

const PALETTES: Record<DeathRecapTheme, Palette> = {
  panel: {
    fg: 'var(--color-foreground)',
    muted: 'var(--color-muted)',
    border: 'var(--color-border)',
    chip: 'var(--color-surface-2)',
  },
  overlay: {
    fg: 'rgba(255,255,255,0.9)',
    muted: 'rgba(255,255,255,0.55)',
    border: 'rgba(255,255,255,0.08)',
    chip: 'rgba(255,255,255,0.06)',
  },
}

const DAMAGE = '#ef4444'
const HEAL = '#22c55e'
const CH = '#38bdf8'

function fmtSecs(s: number): string {
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${Math.round(s - m * 60)}s`
}

function Stat({ label, value, color, pal }: {
  label: string
  value: string
  color?: string
  pal: Palette
}): React.ReactElement {
  return (
    <div style={{ padding: '3px 7px', borderRadius: 4, backgroundColor: pal.chip, lineHeight: 1.25 }}>
      <div style={{ fontSize: 9, color: pal.muted, textTransform: 'uppercase', letterSpacing: 0.4 }}>{label}</div>
      <div style={{ fontSize: 12, fontWeight: 600, color: color ?? pal.fg, fontVariantNumeric: 'tabular-nums' }}>
        {value}
      </div>
    </div>
  )
}

function EventRow({ ev, pal }: { ev: RecapEvent; pal: Palette }): React.ReactElement {
  let color = pal.muted
  let amount = ''
  let what = ev.ability ?? ''
  switch (ev.kind) {
    case 'hit':
      color = DAMAGE
      amount = `-${(ev.amount ?? 0).toLocaleString()}`
      break
    case 'heal':
      color = HEAL
      amount = `+${(ev.amount ?? 0).toLocaleString()}`
      what = 'heal'
      break
    case 'ch':
      color = CH
      amount = 'CH'
      what = 'Complete Healing landed'
      break
    case 'miss':
      amount = what || 'miss'
      what = ''
      break
  }
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: '44px minmax(0,1fr) minmax(0,0.8fr) 64px',
        gap: '0 8px',
        padding: '2px 0',
        fontSize: 11,
        borderBottom: `1px solid ${pal.border}`,
      }}
    >
      <span style={{ color: pal.muted, fontVariantNumeric: 'tabular-nums' }}>{ev.offset.toFixed(1)}s</span>
      <span style={{ color: pal.fg, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {ev.source || '—'}
      </span>
      <span style={{ color: pal.muted, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {what}
      </span>
      <span style={{ color, textAlign: 'right', fontVariantNumeric: 'tabular-nums', fontWeight: 600 }}>
        {amount}
      </span>
    </div>
  )
}

export function RecapSummary({ recap, theme }: {
  recap: DeathRecap
  theme: DeathRecapTheme
}): React.ReactElement {
  const pal = PALETTES[theme]
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
      <Stat pal={pal} label="Killer" value={recap.killer || 'unknown'} color={DAMAGE} />
      <Stat
        pal={pal}
        label={`Taken (${recap.hits} hits)`}
        value={recap.damage_taken.toLocaleString()}
        color={DAMAGE}
      />
      <Stat pal={pal} label="Biggest hit" value={recap.biggest_hit.toLocaleString()} />
      <Stat
        pal={pal}
        label={`Healed (${recap.heals})`}
        value={recap.healing_taken.toLocaleString()}
        color={HEAL}
      />
      <Stat pal={pal} label="CHs" value={String(recap.chs)} color={CH} />
      <Stat
        pal={pal}
        label="Since last CH"
        value={recap.since_last_ch !== undefined ? fmtSecs(recap.since_last_ch) : 'none'}
      />
    </div>
  )
}

export default function DeathRecapView({ death, theme, maxEvents }: {
  death: DeathRecord
  theme: DeathRecapTheme
  maxEvents?: number
}): React.ReactElement {
  const pal = PALETTES[theme]
  const recap = death.recap
  if (!recap) {
    return (
      <div style={{ fontSize: 11, color: pal.muted, padding: '4px 0' }}>
        No recap was recorded for this death.
      </div>
    )
  }
  const events = [...recap.events].reverse()
  const shown = maxEvents ? events.slice(0, maxEvents) : events
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6, minHeight: 0 }}>
      <RecapSummary recap={recap} theme={theme} />
      {events.length === 0 ? (
        <div style={{ fontSize: 11, color: pal.muted }}>
          Nothing hit or healed you in the last {recap.window_seconds}s.
        </div>
      ) : (
        <div>
          {shown.map((ev, i) => <EventRow key={i} ev={ev} pal={pal} />)}
          {shown.length < events.length && (
            <div style={{ fontSize: 10, color: pal.muted, paddingTop: 2 }}>
              +{events.length - shown.length} earlier
            </div>
          )}
        </div>
      )}
    </div>
  )
}
