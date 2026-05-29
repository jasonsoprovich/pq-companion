import React, { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Crosshair, AlertTriangle, CheckCircle2, Circle, ExternalLink } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useNPCOverlaySections } from '../../hooks/useNPCOverlaySections'
import { WSEvent } from '../../lib/wsEvents'
import { getOverlayNPCTarget, getLogStatus, getNPCLoot, getItem } from '../../services/api'
import { className, bodyTypeName } from '../../lib/npcHelpers'
import { cleanLootDropLabel, effectiveDropPct, rarityColor } from '../../lib/lootHelpers'
import OverlayWindow from '../OverlayWindow'
import ItemDetailModal from '../ItemDetailModal'
import { ItemIcon } from '../Icon'
import { ResistChip } from '../ResistChip'
import type { TargetState, SpecialAbility, TargetVariant } from '../../types/overlay'
import type { LogTailerStatus } from '../../types/logEvent'
import type { NPC, NPCLootTable, LootDrop } from '../../types/npc'
import type { Item } from '../../types/item'
import type { NPCOverlaySections } from '../../types/config'

type View = 'stats' | 'loot'

interface NPCPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

function ConnPill({ state, status }: { state: string; status: LogTailerStatus | null }): React.ReactElement {
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
      <div className="flex items-center gap-2 px-3 py-2 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)', borderBottom: '1px solid var(--color-border)' }}>
        <Circle size={10} />
        Loading status…
      </div>
    )
  }
  if (!status.enabled) {
    return (
      <div className="flex items-center gap-2 px-3 py-2 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316', borderBottom: '1px solid var(--color-border)' }}>
        <AlertTriangle size={12} />
        Log parsing disabled. Enable in{' '}
        <Link to="/settings" className="underline" style={{ color: 'var(--color-primary)' }}>Settings</Link>.
      </div>
    )
  }
  if (!status.file_exists) {
    return (
      <div className="flex items-center gap-2 px-3 py-2 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316', borderBottom: '1px solid var(--color-border)' }}>
        <AlertTriangle size={12} />
        Log file not found
      </div>
    )
  }
  return (
    <div className="flex items-center gap-2 px-3 py-2 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', color: '#22c55e', borderBottom: '1px solid var(--color-border)' }}>
      <CheckCircle2 size={12} />
      <span>Tailing log</span>
    </div>
  )
}

// Dangerous melee specials: Summon, Enrage, Rampage, Area Rampage, Flurry,
// Triple Attack, Dual Wield.
const DANGER_ABILITIES = new Set([1, 2, 3, 4, 5, 6, 7])
// Hard immunities to highlight on the badge: Slow, Mez, Charm, Stun, Snare,
// Fear, Dispel, Melee, Magic, Aggro, Pacify.
const IMMUNE_ABILITIES = new Set([12, 13, 14, 15, 16, 17, 18, 19, 20, 24, 31])

function abilityBadgeColor(code: number): string {
  if (DANGER_ABILITIES.has(code)) return '#dc2626'
  if (IMMUNE_ABILITIES.has(code)) return '#f97316'
  return '#6b7280'
}

function AbilityBadge({ ability }: { ability: SpecialAbility }): React.ReactElement {
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-semibold text-white"
      style={{ backgroundColor: abilityBadgeColor(ability.code) }}
    >
      {ability.name || `Ability ${ability.code}`}
    </span>
  )
}

function hpBarColor(percent: number): string {
  if (percent > 50) return '#22c55e'
  if (percent >= 20) return '#eab308'
  return '#ef4444'
}

function TargetHPBar({ percent }: { percent: number }): React.ReactElement {
  const color = hpBarColor(percent)
  return (
    <div className="mt-2">
      <div
        className="relative h-2 w-full overflow-hidden rounded"
        style={{ backgroundColor: 'var(--color-surface)' }}
      >
        <div
          className="absolute inset-y-0 left-0 transition-all"
          style={{ width: `${percent}%`, backgroundColor: color, transitionDuration: '150ms' }}
        />
      </div>
      <div className="mt-0.5 text-right text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {percent}% HP
      </div>
    </div>
  )
}

function Stat({ label, value, color }: { label: string; value: string | number; color?: string }): React.ReactElement {
  return (
    <div className="flex flex-col items-center rounded px-2 py-1" style={{ backgroundColor: 'var(--color-surface-2)', minWidth: '3.25rem' }}>
      <span className="text-[9px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>{label}</span>
      <span className="text-xs font-semibold tabular-nums" style={{ color: color ?? 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

function ViewToggle({ view, onChange }: { view: View; onChange: (v: View) => void }): React.ReactElement {
  const cls = (active: boolean) =>
    `cursor-pointer rounded px-2 py-0.5 text-[11px] font-semibold transition-colors ${
      active ? 'text-white' : 'text-[color:var(--color-muted-foreground)] hover:text-[color:var(--color-foreground)]'
    }`
  return (
    <div className="inline-flex gap-0.5 rounded p-0.5" style={{ backgroundColor: 'var(--color-surface-2)' }}>
      <button
        className={cls(view === 'stats')}
        style={view === 'stats' ? { backgroundColor: 'var(--color-surface)' } : undefined}
        onClick={() => onChange('stats')}
      >
        Stats
      </button>
      <button
        className={cls(view === 'loot')}
        style={view === 'loot' ? { backgroundColor: 'var(--color-surface)' } : undefined}
        onClick={() => onChange('loot')}
      >
        Loot
      </button>
    </div>
  )
}

function LootSection({
  npcId,
  onItemClick,
}: {
  npcId: number
  onItemClick: (id: number) => void
}): React.ReactElement {
  const [loot, setLoot] = useState<NPCLootTable | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  useEffect(() => {
    setLoading(true)
    setError(false)
    setLoot(null)
    getNPCLoot(npcId)
      .then(setLoot)
      .catch(() => setError(true))
      .finally(() => setLoading(false))
  }, [npcId])

  if (loading) {
    return <p className="px-1 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>Loading loot…</p>
  }
  if (error) {
    return <p className="px-1 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>Failed to load loot.</p>
  }
  const ownDrops = loot?.drops ?? []
  const zoneDrops = loot?.zone_wide_drops ?? []
  if (ownDrops.length === 0 && zoneDrops.length === 0) {
    return <p className="px-1 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>No loot table for this NPC.</p>
  }

  type DropSection = { key: string; label: string | null; drops: LootDrop[] }

  // Group drops by cleaned label so MAGELO-GEN/main tables merge into one
  // unlabeled section and themed tables stay separate. If only a single
  // section results, the heading is suppressed entirely — the items just
  // read as the NPC's loot.
  const groupDrops = (drops: LootDrop[]): DropSection[] => {
    const sections: DropSection[] = []
    const byLabel = new Map<string, DropSection>()
    for (const drop of drops) {
      const label = cleanLootDropLabel(drop.name)
      const key = label ?? '__main__'
      const existing = byLabel.get(key)
      if (existing) {
        existing.drops.push(drop)
      } else {
        const section: DropSection = { key, label, drops: [drop] }
        byLabel.set(key, section)
        sections.push(section)
      }
    }
    return sections
  }

  const renderDropItems = (drops: LootDrop[]) =>
    drops.flatMap((drop) =>
      drop.items.map((item) => {
        const eff = effectiveDropPct(drop, item)
        return (
          <button
            key={`${drop.id}-${item.item_id}`}
            onClick={() => onItemClick(item.item_id)}
            className="flex w-full items-center gap-2 border-t py-0.5 text-left"
            style={{ borderColor: 'var(--color-border)' }}
          >
            <ItemIcon id={item.item_icon} name={item.item_name} size={20} />
            <span
              className="flex-1 truncate text-xs underline decoration-dotted"
              style={{ color: rarityColor(eff) }}
            >
              {item.item_name}
            </span>
            <span className="shrink-0 text-[11px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
              {item.chance.toFixed(1)}%
              {item.multiplier > 1 && ` ×${item.multiplier}`}
            </span>
          </button>
        )
      }),
    )

  const renderSectionHeading = (section: DropSection) => {
    if (!section.label) return null
    // Use the maximum table-level probability across merged drops; in practice
    // these are always either all 100% or a single non-100% value.
    const probability = section.drops.reduce((m, d) => Math.max(m, d.probability), 0)
    const multiplier = section.drops.reduce((m, d) => Math.max(m, d.multiplier), 1)
    return (
      <p className="pb-0.5 text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
        {section.label}
        {multiplier > 1 ? ` · ×${multiplier}` : ''}
        {probability < 100 ? ` · ${probability}% chance` : ''}
      </p>
    )
  }

  const renderDropList = (drops: LootDrop[]) => {
    const sections = groupDrops(drops)
    if (sections.length === 0) return null
    // Single-section case: drop the heading entirely. Multi-section case:
    // every section gets a heading, and the auto-merged "main" bucket gets a
    // generic "Main drops" label so the items don't read as if they belonged
    // to the themed section above them.
    const showHeadings = sections.length > 1
    return sections.map((section) => {
      const sectionWithLabel: DropSection = showHeadings && !section.label
        ? { ...section, label: 'Main drops' }
        : section
      return (
        <div key={section.key}>
          {showHeadings && renderSectionHeading(sectionWithLabel)}
          {renderDropItems(sectionWithLabel.drops)}
        </div>
      )
    })
  }

  return (
    <div className="flex flex-col gap-2">
      {renderDropList(ownDrops)}
      {zoneDrops.length > 0 && (
        <>
          <p className="pt-1 text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-primary)' }}>
            {loot?.zone_wide_label || 'Zone-wide loot'}
          </p>
          {renderDropList(zoneDrops)}
        </>
      )}
    </div>
  )
}

function NoTarget({ zone }: { zone?: string }): React.ReactElement {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-4">
      <Crosshair size={40} style={{ color: 'var(--color-muted)' }} />
      <p className="text-sm font-medium" style={{ color: 'var(--color-muted-foreground)' }}>No target</p>
      {zone && (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>Zone: {zone}</p>
      )}
      <p className="max-w-xs text-center text-xs" style={{ color: 'var(--color-muted)' }}>
        Attack or engage an NPC and its info will appear here automatically.
      </p>
    </div>
  )
}

// VariantRibbon surfaces same-name ambiguity (e.g. ssratemple's shissar
// revenant necro/SK split) so the user understands the panel below is
// showing multiple possible NPCs, not a single resolved one.
function VariantRibbon({ variants }: { variants: TargetVariant[] }): React.ReactElement {
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-1">
      <span className="text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
        Variants:
      </span>
      {variants.map((v) => (
        <span
          key={v.npc.id}
          className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
          style={{ backgroundColor: 'var(--color-surface)', color: 'var(--color-foreground)' }}
        >
          {className(v.npc.class)} · L{v.npc.level}
        </span>
      ))}
    </div>
  )
}

// NPCDetails renders the stats/loot view for one NPC. Used directly by
// NPCCard for single-variant targets, and looped per variant when the
// target name is ambiguous. variantLabel (when set) prefixes the section
// as a header divider so stacked variant blocks read clearly.
function NPCDetails({
  npc,
  abilities,
  sections,
  view,
  variantLabel,
  onItemClick,
}: {
  npc: NPC
  abilities: SpecialAbility[]
  sections: NPCOverlaySections
  view: View
  variantLabel?: string
  onItemClick: (id: number) => void
}): React.ReactElement {
  return (
    <div className="flex flex-col gap-2">
      {variantLabel && (
        <div className="flex items-center gap-2 border-b pb-1" style={{ borderColor: 'var(--color-border)' }}>
          <span className="text-[11px] font-bold uppercase tracking-wider" style={{ color: 'var(--color-primary)' }}>
            {variantLabel}
          </span>
        </div>
      )}
      {view === 'loot' ? (
        <LootSection npcId={npc.id} onItemClick={onItemClick} />
      ) : (
        <>
          {sections.identity && (
            <div>
              <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Identity</p>
              <div className="flex flex-wrap gap-1.5">
                <Stat label="Level" value={npc.level} color="var(--color-primary)" />
                <Stat label="Class" value={className(npc.class)} />
                <Stat label="Race" value={npc.race_name} />
                <Stat label="Body" value={bodyTypeName(npc.body_type)} />
              </div>
            </div>
          )}

          {sections.combat && (
            <div>
              <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Combat</p>
              <div className="flex flex-wrap gap-1.5">
                <Stat label="HP" value={npc.hp.toLocaleString()} color="#22c55e" />
                {npc.mana > 0 && (
                  <Stat label="Mana" value={npc.mana.toLocaleString()} color="#3b82f6" />
                )}
                <Stat label="AC" value={npc.ac} />
                <Stat label="Min DMG" value={npc.min_dmg} color="#ef4444" />
                <Stat label="Max DMG" value={npc.max_dmg} color="#ef4444" />
                <Stat label="Atk/Rd" value={npc.attack_count < 0 ? 'default' : npc.attack_count} />
                <Stat label="Speed" value={`${Math.round((npc.run_speed / 0.7) * 100)}%`} />
              </div>
            </div>
          )}

          {sections.resists && (
            <div>
              <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Resists</p>
              <div className="flex flex-wrap gap-1.5">
                <ResistChip type="magic"   value={npc.mr} />
                <ResistChip type="cold"    value={npc.cr} />
                <ResistChip type="fire"    value={npc.fr} />
                <ResistChip type="disease" value={npc.dr} />
                <ResistChip type="poison"  value={npc.pr} />
              </div>
            </div>
          )}

          {sections.attributes && (
            <div>
              <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Attributes</p>
              <div className="flex flex-wrap gap-1.5">
                <Stat label="STR" value={npc.str} />
                <Stat label="STA" value={npc.sta} />
                <Stat label="DEX" value={npc.dex} />
                <Stat label="AGI" value={npc.agi} />
                <Stat label="INT" value={npc.int} />
                <Stat label="WIS" value={npc.wis} />
                <Stat label="CHA" value={npc.cha} />
              </div>
            </div>
          )}

          {sections.special_abilities && abilities.length > 0 && (
            <div>
              <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Special Abilities</p>
              <div className="flex flex-wrap gap-1">
                {abilities.filter((a) => a.value !== 0).map((a) => (
                  <AbilityBadge key={a.code} ability={a} />
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function NPCCard({
  state,
  view,
  sections,
  onItemClick,
}: {
  state: TargetState
  view: View
  sections: NPCOverlaySections
  onItemClick: (id: number) => void
}): React.ReactElement {
  const npc = state.npc_data
  const abilities = state.special_abilities ?? []
  const variants = state.variants ?? []
  const isAmbiguous = variants.length >= 2

  const lastUpdated = new Date(state.last_updated).toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })

  return (
    <div className="flex flex-1 flex-col gap-2 overflow-y-auto p-3">
      <div className="rounded-lg p-3" style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h2 className="text-base font-bold leading-tight" style={{ color: 'var(--color-foreground)' }}>
              {state.target_name ?? 'Unknown'}
            </h2>
            {state.pet_owner && (
              <p className="mt-0.5 text-[11px] italic" style={{ color: 'var(--color-muted-foreground)' }}>
                Pet of {state.pet_owner}
              </p>
            )}
            {state.current_zone && (
              <p className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>{state.current_zone}</p>
            )}
          </div>
          <span className="shrink-0 text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>{lastUpdated}</span>
        </div>

        {state.is_corpse ? (
          <TargetHPBar percent={0} />
        ) : state.hp_percent >= 0 ? (
          <TargetHPBar percent={state.hp_percent} />
        ) : null}

        {npc && (npc.raid_target === 1 || npc.rare_spawn === 1) && (
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {npc.raid_target === 1 && (
              <span className="rounded px-1.5 py-0.5 text-[10px] font-semibold text-white" style={{ backgroundColor: '#7c3aed' }}>RAID TARGET</span>
            )}
            {npc.rare_spawn === 1 && (
              <span className="rounded px-1.5 py-0.5 text-[10px] font-semibold text-white" style={{ backgroundColor: '#b45309' }}>RARE SPAWN</span>
            )}
          </div>
        )}

        {isAmbiguous && <VariantRibbon variants={variants} />}
      </div>

      {npc ? (
        isAmbiguous ? (
          variants.map((v) => (
            <NPCDetails
              key={v.npc.id}
              npc={v.npc}
              abilities={v.special_abilities}
              sections={sections}
              view={view}
              variantLabel={`${className(v.npc.class)} · L${v.npc.level}`}
              onItemClick={onItemClick}
            />
          ))
        ) : (
          <NPCDetails
            npc={npc}
            abilities={abilities}
            sections={sections}
            view={view}
            onItemClick={onItemClick}
          />
        )
      ) : (
        <div className="rounded px-3 py-2 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
          No database record found for this NPC. It may be a pet, player, or unknown entity.
        </div>
      )}
    </div>
  )
}

export default function NPCPanel({
  defaultX = 660,
  defaultY = 24,
  defaultWidth = 380,
  defaultHeight = 600,
  snapGridSize,
  onLayoutChange,
}: NPCPanelProps): React.ReactElement {
  const [target, setTarget] = useState<TargetState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [view, setView] = useState<View>('stats')
  const [modalItem, setModalItem] = useState<Item | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const sections = useNPCOverlaySections('dashboard')

  useEffect(() => {
    getOverlayNPCTarget().then(setTarget).catch(() => setTarget(null))
    getLogStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.OverlayNPCTarget) return
    setTarget(msg.data as TargetState)
  }, [])

  const wsState = useWebSocket(handleMessage)

  const handleItemClick = useCallback((id: number) => {
    if (!id) return
    getItem(id)
      .then((item) => {
        setModalItem(item)
        setModalOpen(true)
      })
      .catch(() => {
        setModalItem(null)
        setModalOpen(false)
      })
  }, [])

  return (
    <>
      <OverlayWindow
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Crosshair size={13} style={{ color: 'var(--color-primary)' }} />
            <ViewToggle view={view} onChange={setView} />
          </span>
        }
        headerRight={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleNPC()}
                title="Pop out NPC overlay as floating window"
                style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
              >
                <ExternalLink size={12} />
              </button>
            )}
            <ConnPill state={wsState} status={status} />
          </div>
        }
        defaultWidth={defaultWidth}
        defaultHeight={defaultHeight}
        defaultX={defaultX}
        defaultY={defaultY}
        minWidth={260}
        minHeight={200}
        snapGridSize={snapGridSize}
        onLayoutChange={onLayoutChange}
      >
        <StatusBar status={status} />
        {target === null ? (
          <div className="flex flex-1 items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>Loading…</p>
          </div>
        ) : target.has_target ? (
          <NPCCard state={target} view={view} sections={sections} onItemClick={handleItemClick} />
        ) : (
          <NoTarget zone={target.current_zone} />
        )}
      </OverlayWindow>
      <ItemDetailModal item={modalItem} open={modalOpen} onClose={() => setModalOpen(false)} />
    </>
  )
}
