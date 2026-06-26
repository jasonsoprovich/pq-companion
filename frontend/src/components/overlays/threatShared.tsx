import React from 'react'
import { Crosshair, ShieldAlert } from 'lucide-react'
import type { MobThreat, ThreatState } from '../../types/overlay'

// Shared render body for the Threat Meter, used by both the dashboard card
// (ThreatPanel) and the popped-out window (ThreatOverlayWindowPage) so the two
// surfaces stay identical. The number shown is the active character's ESTIMATED
// personal hate — see internal/threat for the model and its limits.

function fmt(n: number): string {
  return Math.round(n).toLocaleString()
}

// ThreatRow renders one mob as a labelled bar sized relative to the busiest
// mob on the list. The highlighted (current-target) mob is tinted amber.
function ThreatRow({ mob, max }: { mob: MobThreat; max: number }): React.ReactElement {
  const pct = max > 0 ? Math.max(2, (mob.hate / max) * 100) : 0
  const accent = mob.is_target ? '#c9a84c' : '#7c8aa5'
  return (
    <div style={{ padding: '3px 0' }}>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'baseline',
          gap: 6,
          fontSize: 11,
          marginBottom: 2,
        }}
      >
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontWeight: mob.is_target ? 700 : 400,
            color: mob.is_target ? '#e8d59a' : 'var(--color-text, rgba(255,255,255,0.82))',
          }}
        >
          {mob.is_target && (
            <Crosshair size={10} style={{ marginRight: 3, verticalAlign: '-1px', color: accent }} />
          )}
          {mob.name}
        </span>
        <span style={{ flexShrink: 0, color: 'var(--color-muted, rgba(255,255,255,0.5))', fontVariantNumeric: 'tabular-nums' }}>
          {fmt(mob.hate)}
          <span style={{ opacity: 0.6 }}> · {mob.tps.toFixed(0)}/s</span>
        </span>
      </div>
      <div style={{ height: 4, borderRadius: 2, background: 'rgba(255,255,255,0.07)', overflow: 'hidden' }}>
        <div style={{ width: `${pct}%`, height: '100%', background: accent, borderRadius: 2, transition: 'width 0.25s ease' }} />
      </div>
    </div>
  )
}

export function ThreatContent({ state }: { state: ThreatState | null }): React.ReactElement {
  if (state === null) {
    return (
      <div style={emptyWrap}>
        <p style={{ fontSize: 12, margin: 0 }}>Loading…</p>
      </div>
    )
  }

  if (!state.in_combat || state.mobs.length === 0) {
    return (
      <div style={emptyWrap}>
        <ShieldAlert size={26} style={{ opacity: 0.2 }} />
        <p style={{ fontSize: 12, margin: 0 }}>No threat tracked</p>
        <p style={{ fontSize: 11, margin: 0, opacity: 0.7, textAlign: 'center' }}>
          Engage a mob to estimate your hate.
        </p>
      </div>
    )
  }

  const max = state.mobs.reduce((m, x) => (x.hate > m ? x.hate : m), 0)

  return (
    <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column', padding: '8px 10px' }}>
      {state.target && (
        <div
          style={{
            padding: '8px 10px',
            marginBottom: 8,
            borderRadius: 5,
            background: 'rgba(201,168,76,0.10)',
            border: '1px solid rgba(201,168,76,0.25)',
          }}
        >
          <div style={{ fontSize: 13, fontWeight: 700, color: '#e8d59a', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {state.target.name}
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', marginTop: 3 }}>
            <span style={{ fontSize: 20, fontWeight: 700, color: '#f4e8c1', fontVariantNumeric: 'tabular-nums', lineHeight: 1 }}>
              {fmt(state.target.hate)}
            </span>
            <span style={{ fontSize: 11, color: 'var(--color-muted, rgba(255,255,255,0.55))' }}>
              {state.target.tps.toFixed(0)} hate/sec
            </span>
          </div>
        </div>
      )}

      {state.mobs.length > 1 && (
        <div style={{ display: 'flex', flexDirection: 'column' }}>
          {state.mobs.map((mob) => (
            <ThreatRow key={mob.name} mob={mob} max={max} />
          ))}
        </div>
      )}

      <div
        style={{
          marginTop: 'auto',
          paddingTop: 8,
          fontSize: 9.5,
          lineHeight: 1.4,
          color: 'var(--color-muted, rgba(255,255,255,0.4))',
        }}
      >
        Personal estimate — share for raid callouts.
        {state.hatemod_pct !== 0 && <> Hatemod {state.hatemod_pct > 0 ? '+' : ''}{state.hatemod_pct}%.</>}
      </div>
    </div>
  )
}

const emptyWrap: React.CSSProperties = {
  flex: 1,
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  color: 'var(--color-muted, rgba(255,255,255,0.5))',
  padding: 16,
}
