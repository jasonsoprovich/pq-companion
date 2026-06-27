import React from 'react'
import { Crosshair, ShieldAlert, Users } from 'lucide-react'
import type {
  MobThreat,
  ThreatState,
  RaidThreatState,
  RaidMob,
  RaidEntry,
} from '../../types/overlay'
import { useDPSClassColors } from '../../hooks/useDPSClassColors'
import { combatantBarColor, combatantClassHex } from '../../lib/combatantColor'

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
          <span style={{ opacity: 0.6 }}> · {mob.tps_live.toFixed(0)}/s</span>
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
            <span style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', lineHeight: 1.2 }}>
              <span style={{ fontSize: 14, fontWeight: 700, color: '#e8d59a', fontVariantNumeric: 'tabular-nums' }}>
                {state.target.tps_live.toFixed(0)}
                <span style={{ fontSize: 10, fontWeight: 400, opacity: 0.7 }}> hate/sec</span>
              </span>
              <span style={{ fontSize: 9.5, color: 'var(--color-muted, rgba(255,255,255,0.45))' }}>
                avg {state.target.tps.toFixed(0)}/s
              </span>
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

// ── Raid-estimate mode ──────────────────────────────────────────────────────
// A per-mob, per-player ESTIMATED hate view (overlay:raidthreat). Each player
// row is sized to its share of the mob's top hate and coloured by class. See
// internal/raidthreat for the model and its (substantial) limitations.

export type ThreatMode = 'personal' | 'raid'

// ThreatModeToggle is the compact Solo/Raid segmented control shown in the
// overlay header when raid mode is enabled.
export function ThreatModeToggle({
  mode,
  onChange,
}: {
  mode: ThreatMode
  onChange: (m: ThreatMode) => void
}): React.ReactElement {
  const opts: { key: ThreatMode; label: string }[] = [
    { key: 'personal', label: 'Solo' },
    { key: 'raid', label: 'Raid' },
  ]
  return (
    <div style={{ display: 'flex', borderRadius: 4, overflow: 'hidden', border: '1px solid rgba(255,255,255,0.12)' }}>
      {opts.map((o) => (
        <button
          key={o.key}
          onClick={() => onChange(o.key)}
          title={o.key === 'raid' ? 'Estimated raid-wide threat' : 'Your personal threat'}
          style={{
            fontSize: 9.5,
            lineHeight: 1,
            padding: '2px 6px',
            border: 'none',
            cursor: 'pointer',
            background: mode === o.key ? 'rgba(201,168,76,0.25)' : 'transparent',
            color: mode === o.key ? '#e8d59a' : 'var(--color-muted, rgba(255,255,255,0.5))',
            fontWeight: mode === o.key ? 700 : 400,
          }}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

const CONF_LABEL: Record<string, string> = {
  dot_undercount: 'DoT?',
  heal_undercount: 'heal?',
  class_unknown: 'class?',
}
const CONF_TITLE: Record<string, string> = {
  dot_undercount: "DoT ticks aren't in your log — this hate is understated",
  heal_undercount: "Heals on others aren't in your log — this hate is understated",
  class_unknown: 'Class unknown — no hate adjustment applied',
}

function ConfBadges({ flags }: { flags?: string[] }): React.ReactElement | null {
  if (!flags || flags.length === 0) return null
  return (
    <>
      {flags.map((f) => (
        <span
          key={f}
          title={CONF_TITLE[f] ?? f}
          style={{
            fontSize: 8,
            padding: '0 3px',
            marginLeft: 3,
            borderRadius: 2,
            background: 'rgba(255,180,90,0.16)',
            color: 'rgba(255,200,130,0.9)',
            whiteSpace: 'nowrap',
          }}
        >
          {CONF_LABEL[f] ?? f}
        </span>
      ))}
    </>
  )
}

function RaidPlayerRow({
  entry,
  palette,
}: {
  entry: RaidEntry
  palette: ReturnType<typeof useDPSClassColors>
}): React.ReactElement {
  const pct = Math.max(2, entry.hate_pct * 100)
  const accent = entry.is_you ? '#c9a84c' : combatantClassHex(entry.class, palette)
  const bar = entry.is_you ? 'rgba(201,168,76,0.30)' : combatantBarColor(entry.class, palette)
  return (
    <div style={{ padding: '2px 0' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', gap: 6, fontSize: 11, marginBottom: 1 }}>
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontWeight: entry.is_you ? 700 : 400,
            color: entry.is_you ? '#e8d59a' : 'var(--color-text, rgba(255,255,255,0.82))',
          }}
        >
          {entry.name}
          {entry.is_pet && entry.owner_name && (
            <span style={{ opacity: 0.55, fontStyle: 'italic' }}> ({entry.owner_name}'s pet)</span>
          )}
          <ConfBadges flags={entry.confidence} />
        </span>
        <span style={{ flexShrink: 0, color: 'var(--color-muted, rgba(255,255,255,0.55))', fontVariantNumeric: 'tabular-nums' }}>
          {fmt(entry.hate)}
          <span style={{ opacity: 0.6 }}> · {Math.round(entry.hate_pct * 100)}%</span>
        </span>
      </div>
      <div style={{ height: 4, borderRadius: 2, background: 'rgba(255,255,255,0.07)', overflow: 'hidden' }}>
        <div style={{ width: `${pct}%`, height: '100%', background: bar, borderLeft: `2px solid ${accent}`, borderRadius: 2, transition: 'width 0.25s ease' }} />
      </div>
    </div>
  )
}

function RaidMobCard({
  mob,
  palette,
}: {
  mob: RaidMob
  palette: ReturnType<typeof useDPSClassColors>
}): React.ReactElement {
  return (
    <div
      style={{
        padding: '6px 9px',
        marginBottom: 8,
        borderRadius: 5,
        background: mob.is_target ? 'rgba(201,168,76,0.10)' : 'rgba(255,255,255,0.03)',
        border: `1px solid ${mob.is_target ? 'rgba(201,168,76,0.25)' : 'rgba(255,255,255,0.06)'}`,
      }}
    >
      <div
        style={{
          fontSize: 12,
          fontWeight: 700,
          marginBottom: 3,
          color: mob.is_target ? '#e8d59a' : 'var(--color-text, rgba(255,255,255,0.82))',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {mob.is_target && <Crosshair size={10} style={{ marginRight: 3, verticalAlign: '-1px', color: '#c9a84c' }} />}
        {mob.name}
      </div>
      {mob.players.map((p) => (
        <RaidPlayerRow key={p.name} entry={p} palette={palette} />
      ))}
    </div>
  )
}

export function RaidThreatContent({ state }: { state: RaidThreatState | null }): React.ReactElement {
  const palette = useDPSClassColors()

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
        <Users size={26} style={{ opacity: 0.2 }} />
        <p style={{ fontSize: 12, margin: 0 }}>No raid threat tracked</p>
        <p style={{ fontSize: 11, margin: 0, opacity: 0.7, textAlign: 'center' }}>
          Estimated from damage the raid does near you.
        </p>
      </div>
    )
  }

  return (
    <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column', padding: '8px 10px' }}>
      {state.mobs.map((mob) => (
        <RaidMobCard key={mob.name} mob={mob} palette={palette} />
      ))}
      <div style={{ marginTop: 'auto', paddingTop: 8, fontSize: 9.5, lineHeight: 1.4, color: 'var(--color-muted, rgba(255,255,255,0.4))' }}>
        Estimated — proximity &amp; class limited. DoTs, heals, taunts, and
        out-of-range players aren&apos;t counted.
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
