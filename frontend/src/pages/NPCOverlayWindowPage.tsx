import React, { useCallback, useEffect, useState } from 'react'
import { Crosshair, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayClickThrough } from '../hooks/useOverlayClickThrough'
import { getOverlayNPCTarget } from '../services/api'
import { className, bodyTypeName } from '../lib/npcHelpers'
import type { TargetState, SpecialAbility } from '../types/overlay'

// ── Ability badge colours ──────────────────────────────────────────────────────
// Yellow  = special attacks (direct combat threat to the player)
// Red     = damage/magic immunities (NPC can't be killed normally)
// Orange  = crowd-control immunities (NPC resists player CC tactics)
// Gray    = passive/informational

const ATTACK_ABILITIES = new Set([1, 2, 3, 4, 5, 6])
const DAMAGE_IMMUNE_ABILITIES = new Set([12, 13, 14, 15, 16, 20, 21, 31, 43])
const CC_IMMUNE_ABILITIES = new Set([17, 18, 19, 23])

function abilityBadgeColor(code: number): string {
  if (ATTACK_ABILITIES.has(code)) return '#ca8a04'
  if (DAMAGE_IMMUNE_ABILITIES.has(code)) return '#dc2626'
  if (CC_IMMUNE_ABILITIES.has(code)) return '#f97316'
  return '#6b7280'
}

function AbilityBadge({ ability }: { ability: SpecialAbility }): React.ReactElement {
  return (
    <span
      style={{
        backgroundColor: abilityBadgeColor(ability.code),
        color: '#fff',
        fontSize: 10,
        fontWeight: 600,
        borderRadius: 3,
        padding: '1px 6px',
      }}
    >
      {ability.name || `Ability ${ability.code}`}
    </span>
  )
}

// ── Stat cell ──────────────────────────────────────────────────────────────────

function Stat({ label, value, color }: { label: string; value: string | number; color?: string }): React.ReactElement {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        backgroundColor: 'rgba(255,255,255,0.06)',
        borderRadius: 4,
        padding: '4px 8px',
        minWidth: '3.5rem',
      }}
    >
      <span style={{ fontSize: 9, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'rgba(255,255,255,0.35)' }}>
        {label}
      </span>
      <span style={{ fontSize: 12, fontWeight: 600, color: color ?? 'rgba(255,255,255,0.85)', fontVariantNumeric: 'tabular-nums', marginTop: 1 }}>
        {value}
      </span>
    </div>
  )
}

// ── Section label ──────────────────────────────────────────────────────────────

function SectionLabel({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <p style={{ fontSize: 9, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.08em', color: 'rgba(255,255,255,0.3)', marginBottom: 4 }}>
      {children}
    </p>
  )
}

// ── No-target state ────────────────────────────────────────────────────────────

function NoTarget({ zone }: { zone?: string }): React.ReactElement {
  return (
    <div style={{ display: 'flex', flex: 1, flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, padding: 16 }}>
      <Crosshair size={32} style={{ color: 'rgba(255,255,255,0.2)' }} />
      <p style={{ fontSize: 12, color: 'rgba(255,255,255,0.4)', margin: 0 }}>No target</p>
      {zone && <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>{zone}</p>}
    </div>
  )
}

// ── NPC content ────────────────────────────────────────────────────────────────

function NPCContent({ state }: { state: TargetState }): React.ReactElement {
  const npc = state.npc_data
  const abilities = (state.special_abilities ?? []).filter((a) => a.value !== 0)

  return (
    <div style={{ flex: 1, overflowY: 'auto', padding: '8px 12px', display: 'flex', flexDirection: 'column', gap: 10 }}>
      {/* Target name */}
      <div
        style={{
          backgroundColor: 'rgba(255,255,255,0.06)',
          border: '1px solid rgba(255,255,255,0.1)',
          borderRadius: 6,
          padding: '8px 12px',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
          <div>
            <p style={{ fontSize: 14, fontWeight: 700, color: 'rgba(255,255,255,0.9)', margin: 0, lineHeight: 1.2 }}>
              {state.target_name ?? 'Unknown'}
            </p>
            {state.current_zone && (
              <p style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', margin: '2px 0 0' }}>{state.current_zone}</p>
            )}
          </div>
          <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.25)', flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}>
            {new Date(state.last_updated).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
          </span>
        </div>
        {npc && (npc.raid_target === 1 || npc.rare_spawn === 1) && (
          <div style={{ marginTop: 6, display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {npc.raid_target === 1 && (
              <span style={{ backgroundColor: '#7c3aed', color: '#fff', fontSize: 10, fontWeight: 600, borderRadius: 3, padding: '1px 6px' }}>
                RAID TARGET
              </span>
            )}
            {npc.rare_spawn === 1 && (
              <span style={{ backgroundColor: '#b45309', color: '#fff', fontSize: 10, fontWeight: 600, borderRadius: 3, padding: '1px 6px' }}>
                RARE SPAWN
              </span>
            )}
          </div>
        )}
      </div>

      {npc ? (
        <>
          {/* Identity */}
          <div>
            <SectionLabel>Identity</SectionLabel>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Stat label="Level" value={npc.level} color="#c9a84c" />
              <Stat label="Class" value={className(npc.class)} />
              <Stat label="Race" value={npc.race_name} />
              <Stat label="Body" value={bodyTypeName(npc.body_type)} />
            </div>
          </div>

          {/* Combat */}
          <div>
            <SectionLabel>Combat</SectionLabel>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Stat label="HP" value={npc.hp.toLocaleString()} color="#22c55e" />
              <Stat label="AC" value={npc.ac} />
              <Stat label="Min DMG" value={npc.min_dmg} color="#ef4444" />
              <Stat label="Max DMG" value={npc.max_dmg} color="#ef4444" />
              <Stat label="Attacks" value={npc.attack_count < 0 ? '—' : npc.attack_count} />
            </div>
          </div>

          {/* Resists */}
          <div>
            <SectionLabel>Resists</SectionLabel>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Stat label="Magic" value={npc.mr} />
              <Stat label="Cold" value={npc.cr} />
              <Stat label="Disease" value={npc.dr} />
              <Stat label="Fire" value={npc.fr} />
              <Stat label="Poison" value={npc.pr} />
            </div>
          </div>

          {/* Attributes */}
          <div>
            <SectionLabel>Attributes</SectionLabel>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
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
              <SectionLabel>Special Abilities</SectionLabel>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {abilities.map((a) => (
                  <AbilityBadge key={a.code} ability={a} />
                ))}
              </div>
            </div>
          )}
        </>
      ) : (
        <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 0' }}>
          No database record found for this NPC.
        </p>
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function NPCOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { enableInteraction, enableClickThrough } = useOverlayClickThrough()
  const [target, setTarget] = useState<TargetState | null>(null)

  useEffect(() => {
    getOverlayNPCTarget()
      .then(setTarget)
      .catch(() => setTarget(null))
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== 'overlay:npc_target') return
    setTarget(msg.data as TargetState)
  }, [])

  useWebSocket(handleMessage)

  return (
    <div
      onMouseLeave={enableClickThrough}
      style={{
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: `rgba(10,10,12,${opacity})`,
        color: 'rgba(255,255,255,0.85)',
        fontFamily: 'system-ui, sans-serif',
        overflow: 'hidden',
        borderRadius: 8,
        border: '1px solid rgba(255,255,255,0.1)',
      }}
    >
      {/* Drag region header */}
      <div
        className="drag-region"
        onMouseEnter={enableInteraction}
        onMouseLeave={enableClickThrough}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 10px',
          borderBottom: '1px solid rgba(255,255,255,0.08)',
          flexShrink: 0,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Crosshair size={13} style={{ color: '#c9a84c' }} />
          <span style={{ fontSize: 12, fontWeight: 600, color: 'rgba(255,255,255,0.7)' }}>NPC Target</span>
        </div>
        <button
          className="no-drag"
          onClick={() => window.electron?.overlay?.closeNPC()}
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'rgba(255,255,255,0.4)',
            padding: '2px 4px',
            lineHeight: 1,
            borderRadius: 3,
            display: 'flex',
            alignItems: 'center',
          }}
          title="Close"
        >
          <X size={13} />
        </button>
      </div>

      {/* Content */}
      {target === null ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <p style={{ fontSize: 12, color: 'rgba(255,255,255,0.3)', margin: 0 }}>Loading…</p>
        </div>
      ) : target.has_target ? (
        <NPCContent state={target} />
      ) : (
        <NoTarget zone={target.current_zone} />
      )}
    </div>
  )
}
