import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Crosshair, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { useNPCOverlaySections } from '../hooks/useNPCOverlaySections'
import { useWishlistItemIds } from '../hooks/useWishlistItemIds'
import { useOverlayEntityLinks } from '../hooks/useOverlayEntityLinks'
import { openEntityInMain } from '../lib/overlayNav'
import { useTargetTimers } from '../hooks/useTargetTimers'
import { useTargetPlayer } from '../hooks/useTargetPlayer'
import OverlayLockButton from '../components/OverlayLockButton'
import TargetPinButton from '../components/TargetPinButton'
import CopyTargetStatsButton from '../components/CopyTargetStatsButton'
import { ItemIcon } from '../components/Icon'
import { ResistChip } from '../components/ResistChip'
import NPCCasterSummarySection from '../components/overlays/NPCCasterSummarySection'
import TargetTimerList from '../components/overlays/TargetTimerList'
import TargetPlayerCard from '../components/overlays/TargetPlayerCard'
import { getOverlayNPCTarget, getNPCLoot, getNPCFaction } from '../services/api'
import { className, bodyTypeName, npcRunSpeedPct, npcLevelLabel } from '../lib/npcHelpers'
import { effectiveDropPct, rarityColor } from '../lib/lootHelpers'
import type { TargetState, SpecialAbility, TargetVariant, NPCCasterSummary } from '../types/overlay'
import type { NPC, NPCLootTable, LootDrop, NPCFaction } from '../types/npc'
import type { NPCOverlaySections } from '../types/config'

// ── Ability badge colours ──────────────────────────────────────────────────────
// Yellow  = special attacks (direct combat threat to the player)
// Red     = damage/magic immunities (NPC can't be killed normally)
// Orange  = crowd-control immunities (NPC resists player CC tactics)
// Gray    = passive/informational

// Dangerous melee specials: Summon, Enrage, Rampage, Area Rampage, Flurry,
// Triple Attack, Dual Wield.
const ATTACK_ABILITIES = new Set([1, 2, 3, 4, 5, 6, 7])
// Damage-blocking immunities: Melee, Magic, Non-Bane Melee, Non-Magical
// Melee, Harm-from-Client, Ranged Spells.
const DAMAGE_IMMUNE_ABILITIES = new Set([19, 20, 22, 23, 26, 35])
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

// ── Resist chip (colour-coded by element) ─────────────────────────────────────
//
// Matches the in-game Quarm /con palette: red for fire, blue for cold,
// dark green for poison, light green for disease, and a neutral
// off-white for magic. Background uses the same hue at low opacity so
// the chip reads as a tinted bubble rather than a solid colour block.

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

type View = 'stats' | 'loot' | 'timers'

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
      <button style={btn(view === 'timers')} onClick={() => onChange('timers')}>Timers</button>
    </div>
  )
}

// ── No-target state ────────────────────────────────────────────────────────────

function NoTarget({ zone, chrome = true }: { zone?: string; chrome?: boolean }): React.ReactElement {
  return (
    <div style={{
      display: 'flex', flex: 1, flexDirection: 'column', alignItems: 'center',
      justifyContent: 'center', gap: 6, padding: 12,
      // Idle placeholder — fade with the chrome, it isn't live content.
      opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease',
    }}>
      <Crosshair size={24} style={{ color: 'rgba(255,255,255,0.2)' }} />
      <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', margin: 0 }}>No target</p>
      {zone && <p style={{ fontSize: 10, color: 'rgba(255,255,255,0.25)', margin: 0 }}>{zone}</p>}
    </div>
  )
}

// ── NPC content ────────────────────────────────────────────────────────────────

// ── Loot content ───────────────────────────────────────────────────────────────

function LootContent({
  npcId,
  wishlistItemIds,
  onItemClick,
}: {
  npcId: number
  wishlistItemIds: Set<number>
  // When set, item rows become clickable links that open the item in the main
  // database explorer. Undefined leaves them as plain text.
  onItemClick?: (itemId: number) => void
}): React.ReactElement {
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
  const ownDrops = loot?.drops ?? []
  const zoneDrops = loot?.zone_wide_drops ?? []
  if (ownDrops.length === 0 && zoneDrops.length === 0) {
    return <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 2px' }}>No loot table for this NPC.</p>
  }

  const renderDropList = (drops: LootDrop[]) => drops.map((drop) => (
    <div key={drop.id}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingBottom: 2 }}>
        <span style={{ fontSize: 9, fontWeight: 600, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
          {drop.name ? `${drop.name} · ` : ''}
          {drop.multiplier > 1 ? `×${drop.multiplier} · ` : ''}
          {drop.probability < 100 ? `${drop.probability}% chance` : 'Always drops'}
        </span>
      </div>
      {drop.items.map((item) => {
        const eff = effectiveDropPct(drop, item)
        const wished = wishlistItemIds.has(item.item_id)
        return (
          <div
            key={`${drop.id}-${item.item_id}`}
            title={
              onItemClick
                ? wished
                  ? 'On your wishlist · click to open in the item database'
                  : 'Open in the item database'
                : wished
                  ? 'On your wishlist'
                  : undefined
            }
            onClick={onItemClick ? () => onItemClick(item.item_id) : undefined}
            role={onItemClick ? 'button' : undefined}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: '1px 4px 1px 2px',
              borderTop: '1px solid rgba(255,255,255,0.05)',
              // Subtle green cue for wishlisted drops. A left accent + faint
              // tint, leaving the item-name text color free to keep encoding
              // drop rarity. transparent border when not wished avoids any
              // row-to-row layout shift.
              borderLeft: wished ? '2px solid #22c55e' : '2px solid transparent',
              backgroundColor: wished ? 'rgba(34,197,94,0.10)' : 'transparent',
              cursor: onItemClick ? 'pointer' : 'default',
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
                textDecoration: onItemClick ? 'underline dotted' : undefined,
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
  ))

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {renderDropList(ownDrops)}
      {zoneDrops.length > 0 && (
        <>
          <div style={{ paddingTop: 4 }}>
            <span style={{ fontSize: 9, fontWeight: 700, color: 'rgba(180, 200, 255, 0.85)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
              {loot?.zone_wide_label || 'Zone-wide loot'}
            </span>
          </div>
          {renderDropList(zoneDrops)}
        </>
      )}
    </div>
  )
}

// VariantRibbon surfaces same-name ambiguity (e.g. a bare "Venril Sathir"
// row and a "#Venril_Sathir" row with a different ability set) so the user
// understands the blocks below show multiple candidate NPCs, not one
// resolved target.
function VariantRibbon({ variants }: { variants: TargetVariant[] }): React.ReactElement {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 4 }}>
      <span style={{ fontSize: 9, fontWeight: 600, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
        Variants:
      </span>
      {variants.map((v) => (
        <span
          key={v.npc.id}
          style={{ backgroundColor: 'rgba(255,255,255,0.08)', color: 'rgba(255,255,255,0.85)', fontSize: 10, fontWeight: 600, borderRadius: 3, padding: '1px 6px' }}
        >
          {className(v.npc.class)} · L{npcLevelLabel(v.npc)}
        </span>
      ))}
    </div>
  )
}

// FactionSection fetches and renders the targeted NPC's faction — its primary
// (standing) faction plus the per-faction hits taken on a kill. Keyed by
// npc_types.id, which the target already carries, so it's a simple per-id
// fetch like LootContent. Stays silent while loading or when the NPC has no
// faction so the overlay stays compact. Mirrors the dashboard FactionSection
// in NPCPanel.tsx, restyled with inline styles to match this window.
function FactionSection({ npcId }: { npcId: number }): React.ReactElement | null {
  const [faction, setFaction] = useState<NPCFaction | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setFaction(null)
    getNPCFaction(npcId)
      .then((f) => { if (!cancelled) setFaction(f) })
      .catch(() => { if (!cancelled) setFaction(null) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [npcId])

  if (loading || !faction) return null
  const hasPrimary = !!faction.primary_faction_name
  if (!hasPrimary && faction.hits.length === 0) return null

  return (
    <div>
      <p style={{ fontSize: 9, fontWeight: 600, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: '0.08em', margin: '0 0 4px' }}>
        Faction
      </p>
      {hasPrimary && (
        <p style={{ fontSize: 12, color: 'rgba(255,255,255,0.9)', margin: '0 0 4px' }}>
          {faction.primary_faction_name}
        </p>
      )}
      {faction.hits.length > 0 && (
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
          {faction.hits.map((hit) => (
            <span
              key={hit.faction_id}
              style={{ backgroundColor: 'rgba(255,255,255,0.08)', color: 'rgba(255,255,255,0.7)', fontSize: 10, borderRadius: 3, padding: '2px 6px' }}
            >
              {hit.faction_name}
              <span
                style={{
                  marginLeft: 4,
                  color: hit.value > 0 ? '#22c55e' : hit.value < 0 ? '#f87171' : 'rgba(255,255,255,0.4)',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {hit.value > 0 ? `+${hit.value}` : hit.value}
              </span>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

// StatsBody renders the stats/loot view for a single NPC. Used directly for a
// single resolved target and looped per variant when the target name is
// ambiguous; variantLabel (when set) prefixes the block as a divider header.
function StatsBody({
  npc,
  abilities,
  casterSummary,
  sections,
  view,
  wishlistItemIds,
  variantLabel,
  onItemClick,
  onSpellClick,
}: {
  npc: NPC
  abilities: SpecialAbility[]
  casterSummary?: NPCCasterSummary
  sections: NPCOverlaySections
  view: View
  wishlistItemIds: Set<number>
  variantLabel?: string
  onItemClick?: (itemId: number) => void
  onSpellClick?: (spellId: number) => void
}): React.ReactElement {
  const shown = abilities.filter((a) => a.value !== 0)
  return (
    <>
      {variantLabel && (
        <div style={{ borderBottom: '1px solid rgba(255,255,255,0.08)', paddingBottom: 2 }}>
          <span style={{ fontSize: 10, fontWeight: 700, color: 'rgba(180, 200, 255, 0.85)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            {variantLabel}
          </span>
        </div>
      )}
      {view === 'loot' ? (
        <LootContent npcId={npc.id} wishlistItemIds={wishlistItemIds} onItemClick={onItemClick} />
      ) : (
        <>
          {sections.identity && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="Lv" value={npcLevelLabel(npc)} color="#c9a84c" />
              <Chip value={className(npc.class)} />
              <Chip value={npc.race_name} />
              <Chip value={bodyTypeName(npc.body_type)} />
            </div>
          )}

          {sections.combat && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="HP" value={npc.hp.toLocaleString()} color="#22c55e" />
              {npc.mana > 0 && (
                <Chip label="Mana" value={npc.mana.toLocaleString()} color="#3b82f6" />
              )}
              <Chip label="AC" value={npc.ac} />
              <Chip label="DMG" value={`${npc.min_dmg}-${npc.max_dmg}`} color="#ef4444" />
              <Chip label="Atk/Rd" value={npc.attack_count < 0 ? 'default' : npc.attack_count} />
              <Chip label="Speed" value={`${npcRunSpeedPct(npc.run_speed)}%`} />
            </div>
          )}

          {sections.resists && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <ResistChip type="magic"   value={npc.mr} />
              <ResistChip type="cold"    value={npc.cr} />
              <ResistChip type="fire"    value={npc.fr} />
              <ResistChip type="disease" value={npc.dr} />
              <ResistChip type="poison"  value={npc.pr} />
            </div>
          )}

          {sections.attributes && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              <Chip label="STR" value={npc.str} />
              <Chip label="STA" value={npc.sta} />
              <Chip label="DEX" value={npc.dex} />
              <Chip label="AGI" value={npc.agi} />
              <Chip label="INT" value={npc.int} />
              <Chip label="WIS" value={npc.wis} />
              <Chip label="CHA" value={npc.cha} />
            </div>
          )}

          {sections.special_abilities && shown.length > 0 && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
              {shown.map((a) => (
                <AbilityBadge key={a.code} ability={a} />
              ))}
            </div>
          )}

          {casterSummary && (
            <NPCCasterSummarySection
              summary={casterSummary}
              sections={sections}
              theme={{
                heading: 'rgba(255,255,255,0.4)',
                muted: 'rgba(255,255,255,0.45)',
                chipBg: 'rgba(255,255,255,0.08)',
                chipText: 'rgba(255,255,255,0.85)',
              }}
              onSpellClick={onSpellClick}
            />
          )}

          {sections.faction && <FactionSection npcId={npc.id} />}
        </>
      )}
    </>
  )
}

function NPCContent({
  state,
  view,
  sections,
  wishlistItemIds,
}: {
  state: TargetState
  view: View
  sections: NPCOverlaySections
  wishlistItemIds: Set<number>
}): React.ReactElement {
  const npc = state.npc_data
  const abilities = (state.special_abilities ?? []).filter((a) => a.value !== 0)
  const variants = state.variants ?? []
  const isAmbiguous = variants.length >= 2

  // When overlay entity links are enabled, loot items and castable spells
  // become clickable links that bring the main window forward and open the
  // entity in the database explorer. Disabled → undefined → plain text.
  const linksEnabled = useOverlayEntityLinks()
  const onItemClick = linksEnabled ? (id: number) => openEntityInMain('item', id) : undefined
  const onSpellClick = linksEnabled ? (id: number) => openEntityInMain('spell', id) : undefined

  // Player lookup only runs when there's no DB record — that's the case where
  // the target is a player. (Timer subscription lives in TargetTimersView,
  // mounted only for the Timers tab, so its 1Hz updates don't re-render the
  // whole stats/loot tree.)
  const { player, loading: playerLoading } = useTargetPlayer(state.target_name, !npc)

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
        {isAmbiguous && (
          <div style={{ marginTop: 4 }}>
            <VariantRibbon variants={variants} />
          </div>
        )}
      </div>

      {view === 'timers' ? (
        <TargetTimersView targetName={state.target_name} />
      ) : npc ? (
        isAmbiguous ? (
          variants.map((v) => (
            <StatsBody
              key={v.npc.id}
              npc={v.npc}
              abilities={v.special_abilities}
              casterSummary={v.caster_summary}
              sections={sections}
              view={view}
              wishlistItemIds={wishlistItemIds}
              variantLabel={`${className(v.npc.class)} · L${npcLevelLabel(v.npc)}`}
              onItemClick={onItemClick}
              onSpellClick={onSpellClick}
            />
          ))
        ) : (
          <StatsBody
            npc={npc}
            abilities={abilities}
            casterSummary={state.caster_summary}
            sections={sections}
            view={view}
            wishlistItemIds={wishlistItemIds}
            onItemClick={onItemClick}
            onSpellClick={onSpellClick}
          />
        )
      ) : view === 'loot' ? (
        <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', margin: 0, padding: '4px 0' }}>
          No loot — this target isn&rsquo;t an NPC in the database.
        </p>
      ) : (
        <TargetPlayerCard player={player} loading={playerLoading} />
      )}
    </div>
  )
}

// TargetTimersView isolates the ~1Hz spell-timer subscription (useTargetTimers)
// so it only mounts when the Timers tab is active. Previously the hook lived at
// the top of the body, so any active timer re-rendered the whole stats/loot
// tree every second even on the other tabs.
function TargetTimersView({ targetName }: { targetName: string | undefined }): React.ReactElement {
  const timers = useTargetTimers(targetName)
  return <TargetTimerList timers={timers} />
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function NPCOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('npc')
  const onDragMouseDown = useWindowDrag()
  const [target, setTarget] = useState<TargetState | null>(null)
  const [view, setView] = useState<View>('stats')
  // Pin holds the displayed target and ignores incoming swaps until unpinned.
  // Ref mirrors state so the WS handler stays stable (no re-subscribe).
  const [pinned, setPinned] = useState(false)
  const pinnedRef = useRef(false)
  const sections = useNPCOverlaySections('popout')
  const wishlistItemIds = useWishlistItemIds()

  useEffect(() => {
    getOverlayNPCTarget()
      .then(setTarget)
      .catch(() => setTarget(null))
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.OverlayNPCTarget) return
    if (pinnedRef.current) return
    setTarget(msg.data as TargetState)
  }, [])

  const togglePin = useCallback(() => {
    setPinned((p) => {
      const next = !p
      pinnedRef.current = next
      // On unpin, snap back to the live target.
      if (!next) getOverlayNPCTarget().then(setTarget).catch(() => {})
      return next
    })
  }, [])

  useWebSocket(handleMessage)

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        color: 'rgba(255,255,255,0.85)',
        fontFamily: 'system-ui, sans-serif',
        overflow: 'hidden',
        borderRadius: 8,
        border: `1px solid rgba(255,255,255,${chrome ? 0.1 : 0})`,
        // Pin indicator stays visible even when the chrome fades.
        outline: pinned ? '2px solid #c9a84c' : undefined,
        outlineOffset: pinned ? -2 : undefined,
        transition: 'background-color 0.4s ease, border-color 0.4s ease',
      }}
    >
      {/* Drag region header */}
      <div
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 10px',
          borderBottom: '1px solid rgba(255,255,255,0.08)',
          flexShrink: 0,
          // Fade-when-inactive: hide the title bar (tabs, lock, close) with
          // the rest of the chrome; pointerEvents off so invisible buttons
          // can't be clicked.
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
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
          {target?.npc_data && (
            <CopyTargetStatsButton state={target} idleColor="rgba(255,255,255,0.4)" size={13} />
          )}
          {(target?.has_target || pinned) && (
            <TargetPinButton pinned={pinned} onToggle={togglePin} />
          )}
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
        <NPCContent state={target} view={view} sections={sections} wishlistItemIds={wishlistItemIds} />
      ) : (
        <NoTarget zone={target.current_zone} chrome={chrome} />
      )}
    </div>
  )
}
