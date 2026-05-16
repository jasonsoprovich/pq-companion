import React, { useCallback, useEffect, useState } from 'react'
import { Crosshair, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import OverlayLockButton from '../components/OverlayLockButton'
import { ItemIcon } from '../components/Icon'
import { getOverlayNPCTarget, getNPCLoot } from '../services/api'
import { className, bodyTypeName } from '../lib/npcHelpers'
import { effectiveDropPct, rarityColor } from '../lib/lootHelpers'
import type { TargetState, SpecialAbility } from '../types/overlay'
import type { NPCLootTable } from '../types/npc'

// ── Ability badge colours ──────────────────────────────────────────────────────
// Yellow  = special attacks (direct combat threat to the player)
// Red     = damage/magic immunities (NPC can't be killed normally)
// Orange  = crowd-control immunities (NPC resists player CC tactics)
// Gray    = passive/informational

// Dangerous melee specials: Summon, Enrage, Rampage, Area Rampage, Flurry,
// Triple Attack, Dual Wield.
const ATTACK_ABILITIES = new Set([1, 2, 3, 4, 5, 6, 7])
// Damage-blocking immunities: Melee, Magic, Non-Bane Melee, Non-Magical
// Melee, Harm-from-Client.
const DAMAGE_IMMUNE_ABILITIES = new Set([19, 20, 22, 23, 35])
// Crowd-control immunities: Slow, Mez, Charm, Stun, Snare, Fear, Dispel,
// Fleeing, Aggro, Taunt, Pacify, Haste, Disarm, Riposte.
const CC_IMMUNE_ABILITIES = new Set([12, 13, 14, 15, 16, 17, 18, 21, 24, 28, 31, 51, 52, 53])

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
        fontSize: 11,
        fontWeight: 600,
        borderRadius: 3,
        padding: '2px 6px',
        lineHeight: 1.4,
      }}
    >
      {ability.name || `Ability ${ability.code}`}
    </span>
  )
}

// ── Inline chip (label + value) ────────────────────────────────────────────────

function Chip({ label, value, color }: { label?: string; value: string | number; color?: string }): React.ReactElement {
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'baseline',
        gap: 4,
        backgroundColor: 'rgba(255,255,255,0.06)',
        borderRadius: 3,
        padding: '3px 7px',
        fontSize: 12,
        lineHeight: 1.4,
      }}
    >
      {label && (
        <span style={{ color: 'rgba(255,255,255,0.4)', fontWeight: 600, fontSize: 10, textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          {label}
        </span>
      )}
      <span style={{ color: color ?? 'rgba(255,255,255,0.9)', fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
        {value}
      </span>
    </span>
  )
}

// ── View toggle (Stats ↔ Loot) ────────────────────────────────────────────────

type View = 'stats' | 'loot'

function ViewToggle({ view, onChange }: { view: View; onChange: (v: View) => void }): React.ReactElement {
  const btn = (active: boolean): React.CSSProperties => ({
    background: active ? 'rgba(255,255,255,0.12)' : 'transparent',
    color: active ? 'rgba(255,255,255,0.9)' : 'rgba(255,255,255,0.4)',
    border: 'none',
    cursor: 'pointer',
    fontSize: 10,
    fontWeight: 600,
    padding: '2px 8px',
    borderRadius: 3,
    lineHeight: 1.4,
  })
  return (
    <div className="no-drag" style={{ display: 'inline-flex', gap: 2, backgroundColor: 'rgba(0,0,0,0.25)', borderRadius: 4, padding: 1 }}>
      <button style={btn(view === 'stats')} onClick={() => onChange('stats')}>Stats</button>
      <button style={btn(view === 'loot')} onClick={() => onChange('loot')}>Loot</button>
    </div>
  )
}

// ── No-target state ────────────────────────────────────────────────────────────

function NoTarget({ zone }: { zone?: string }): React.ReactElement {
  return (
    <div style={{ display: 'flex', flex: 1, flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6, padding: 12 }}>
      <Crosshair size={24} style={{ color: 'rgba(255,255,255,0.2)' }} />
      <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', margin: 0 }}>No target</p>
      {zone && <p style={{ fontSize: 10, color: 'rgba(255,255,255,0.25)', margin: 0 }}>{zone}</p>}
    </div>
  )
}

// ── NPC content ────────────────────────────────────────────────────────────────

// ── Loot content ───────────────────────────────────────────────────────────────

function LootContent({ npcId }: { npcId: number }): React.ReactElement {
  const [loot, setLoot] = useState<NPCLootTable | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    setLoading(true)
    setError(false)
    setLoot(null)
    getNPCLoot(npcId)
      .then((data) => setLoot(data))
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }, [npcId])

  if (loading) {
    return <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 2px' }}>Loading loot…</p>
  }
  if (error) {
    return <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 2px' }}>Failed to load loot.</p>
  }
  if (!loot || loot.drops.length === 0) {
    return <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 2px' }}>No loot table for this NPC.</p>
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {loot.drops.map((drop) => (
        <div key={drop.id}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingBottom: 2 }}>
            <span style={{ fontSize: 9, fontWeight: 600, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
              {drop.multiplier > 1 ? `×${drop.multiplier} · ` : ''}
              {drop.probability < 100 ? `${drop.probability}% chance` : 'Always drops'}
            </span>
          </div>
          {drop.items.map((item) => {
            const eff = effectiveDropPct(drop, item)
            return (
              <div
                key={`${drop.id}-${item.item_id}`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  padding: '1px 0',
                  borderTop: '1px solid rgba(255,255,255,0.05)',
                }}
              >
                <ItemIcon id={item.item_icon} name={item.item_name} size={18} />
                <span
                  style={{
                    flex: 1,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    fontSize: 11,
                    color: rarityColor(eff),
                    fontWeight: 500,
                  }}
                >
                  {item.item_name}
                </span>
                <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.6)', fontVariantNumeric: 'tabular-nums', flexShrink: 0 }}>
                  {item.chance.toFixed(1)}%
                  {item.multiplier > 1 && ` ×${item.multiplier}`}
                </span>
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}

function NPCContent({ state, view }: { state: TargetState; view: View }): React.ReactElement {
  const npc = state.npc_data
  const abilities = (state.special_abilities ?? []).filter((a) => a.value !== 0)

  return (
    <div style={{ flex: 1, overflowY: 'auto', padding: '8px 12px', display: 'flex', flexDirection: 'column', gap: 7 }}>
      {/* Target name + zone + timestamp */}
      <div>
        <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
          <p style={{ fontSize: 15, fontWeight: 700, color: 'rgba(255,255,255,0.92)', margin: 0, lineHeight: 1.2 }}>
            {state.target_name ?? 'Unknown'}
          </p>
          <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.25)', flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}>
            {new Date(state.last_updated).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
          </span>
        </div>
        {state.pet_owner && (
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.55)', fontStyle: 'italic', margin: '2px 0 0' }}>
            Pet of {state.pet_owner}
          </p>
        )}
        {state.current_zone && (
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: '2px 0 0' }}>{state.current_zone}</p>
        )}
        {state.hp_percent >= 0 && (
          <div style={{ marginTop: 5 }}>
            <div
              style={{
                position: 'relative',
                height: 7,
                width: '100%',
                backgroundColor: 'rgba(255,255,255,0.1)',
                borderRadius: 3,
                overflow: 'hidden',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  right: 'auto',
                  width: `${state.hp_percent}%`,
                  backgroundColor:
                    state.hp_percent > 50 ? '#22c55e' : state.hp_percent >= 20 ? '#eab308' : '#ef4444',
                  transition: 'width 150ms, background-color 150ms',
                }}
              />
            </div>
            <div
              style={{
                marginTop: 2,
                fontSize: 10,
                color: 'rgba(255,255,255,0.45)',
                textAlign: 'right',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {state.hp_percent}% HP
            </div>
          </div>
        )}
        {npc && (npc.raid_target === 1 || npc.rare_spawn === 1) && (
          <div style={{ marginTop: 4, display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {npc.raid_target === 1 && (
              <span style={{ backgroundColor: '#7c3aed', color: '#fff', fontSize: 11, fontWeight: 700, borderRadius: 3, padding: '2px 6px' }}>RAID</span>
            )}
            {npc.rare_spawn === 1 && (
              <span style={{ backgroundColor: '#b45309', color: '#fff', fontSize: 11, fontWeight: 700, borderRadius: 3, padding: '2px 6px' }}>RARE</span>
            )}
          </div>
        )}
      </div>

      {npc ? (
        view === 'loot' ? (
          <LootContent npcId={npc.id} />
        ) : (
          <>
            {/* Identity */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="Lv" value={npc.level} color="#c9a84c" />
              <Chip value={className(npc.class)} />
              <Chip value={npc.race_name} />
              <Chip value={bodyTypeName(npc.body_type)} />
            </div>

            {/* Combat */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="HP" value={npc.hp.toLocaleString()} color="#22c55e" />
              <Chip label="AC" value={npc.ac} />
              <Chip label="DMG" value={`${npc.min_dmg}-${npc.max_dmg}`} color="#ef4444" />
              <Chip label="Atk" value={npc.attack_count < 0 ? '—' : npc.attack_count} />
            </div>

            {/* Resists */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="MR" value={npc.mr} />
              <Chip label="CR" value={npc.cr} />
              <Chip label="DR" value={npc.dr} />
              <Chip label="FR" value={npc.fr} />
              <Chip label="PR" value={npc.pr} />
            </div>

            {/* Attributes */}
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="STR" value={npc.str} />
              <Chip label="STA" value={npc.sta} />
              <Chip label="DEX" value={npc.dex} />
              <Chip label="AGI" value={npc.agi} />
              <Chip label="INT" value={npc.int} />
              <Chip label="WIS" value={npc.wis} />
              <Chip label="CHA" value={npc.cha} />
            </div>

            {/* Special Abilities */}
            {abilities.length > 0 && (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                {abilities.map((a) => (
                  <AbilityBadge key={a.code} ability={a} />
                ))}
              </div>
            )}
          </>
        )
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
  const { locked, toggleLocked, enableInteraction, enableClickThrough } = useOverlayLock()
  const [target, setTarget] = useState<TargetState | null>(null)
  const [view, setView] = useState<View>('stats')

  useEffect(() => {
    getOverlayNPCTarget()
      .then(setTarget)
      .catch(() => setTarget(null))
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.OverlayNPCTarget) return
    setTarget(msg.data as TargetState)
  }, [])

  useWebSocket(handleMessage)

  return (
    <div
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
        className={locked ? 'no-drag' : 'drag-region'}
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
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Crosshair size={13} style={{ color: '#c9a84c' }} />
          <ViewToggle view={view} onChange={setView} />
        </div>
        <div
          className="no-drag"
          style={{ display: 'flex', alignItems: 'center', gap: 6 }}
        >
          <OverlayLockButton locked={locked} onToggle={toggleLocked} size={12} />
          <button
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
      </div>

      {/* Content */}
      {target === null ? (
        <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <p style={{ fontSize: 12, color: 'rgba(255,255,255,0.3)', margin: 0 }}>Loading…</p>
        </div>
      ) : target.has_target ? (
        <NPCContent state={target} view={view} />
      ) : (
        <NoTarget zone={target.current_zone} />
      )}
    </div>
  )
}
