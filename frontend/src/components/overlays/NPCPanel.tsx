import React, { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Crosshair, AlertTriangle, CheckCircle2, Circle, ExternalLink } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { getOverlayNPCTarget, getLogStatus, getNPCLoot, getItem } from '../../services/api'
import { className, bodyTypeName } from '../../lib/npcHelpers'
import { effectiveDropPct, rarityColor } from '../../lib/lootHelpers'
import OverlayWindow from '../OverlayWindow'
import ItemDetailModal from '../ItemDetailModal'
import { ItemIcon } from '../Icon'
import type { TargetState, SpecialAbility } from '../../types/overlay'
import type { LogTailerStatus } from '../../types/logEvent'
import type { NPCLootTable } from '../../types/npc'
import type { Item } from '../../types/item'

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
  if (!loot || loot.drops.length === 0) {
    return <p className="px-1 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>No loot table for this NPC.</p>
  }

  return (
    <div className="flex flex-col gap-2">
      {loot.drops.map((drop) => (
        <div key={drop.id}>
          <p className="pb-0.5 text-[10px] font-semibold uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
            {drop.multiplier > 1 ? `×${drop.multiplier} · ` : ''}
            {drop.probability < 100 ? `${drop.probability}% chance` : 'Always drops'}
          </p>
          {drop.items.map((item) => {
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
          })}
        </div>
      ))}
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

function NPCCard({
  state,
  view,
  onItemClick,
}: {
  state: TargetState
  view: View
  onItemClick: (id: number) => void
}): React.ReactElement {
  const npc = state.npc_data
  const abilities = state.special_abilities ?? []

  const lastUpdated = new Date(state.last_updated).toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  })

  return (
    <div className="flex flex-1 flex-col gap-2 overflow-y-auto p-3">
      <div className="rounded-lg p-3" style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}>
        <div className="flex items-start justify-between gap-2">
          <div>
            <h2 className="text-base font-bold leading-tight" style={{ color: 'var(--color-foreground)' }}>
              {state.target_name ?? 'Unknown'}
            </h2>
            {state.current_zone && (
              <p className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>{state.current_zone}</p>
            )}
          </div>
          <span className="shrink-0 text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>{lastUpdated}</span>
        </div>

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
      </div>

      {npc ? (
        view === 'loot' ? (
          <LootSection npcId={npc.id} onItemClick={onItemClick} />
        ) : (
        <>
          <div>
            <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Identity</p>
            <div className="flex flex-wrap gap-1.5">
              <Stat label="Level" value={npc.level} color="var(--color-primary)" />
              <Stat label="Class" value={className(npc.class)} />
              <Stat label="Race" value={npc.race_name} />
              <Stat label="Body" value={bodyTypeName(npc.body_type)} />
            </div>
          </div>

          <div>
            <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Combat</p>
            <div className="flex flex-wrap gap-1.5">
              <Stat label="HP" value={npc.hp.toLocaleString()} color="#22c55e" />
              <Stat label="AC" value={npc.ac} />
              <Stat label="Min DMG" value={npc.min_dmg} color="#ef4444" />
              <Stat label="Max DMG" value={npc.max_dmg} color="#ef4444" />
              <Stat label="Attacks" value={npc.attack_count} />
            </div>
          </div>

          <div>
            <p className="mb-1 text-[9px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>Resists</p>
            <div className="flex flex-wrap gap-1.5">
              <Stat label="Magic" value={npc.mr} />
              <Stat label="Cold" value={npc.cr} />
              <Stat label="Disease" value={npc.dr} />
              <Stat label="Fire" value={npc.fr} />
              <Stat label="Poison" value={npc.pr} />
            </div>
          </div>

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

          {abilities.length > 0 && (
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
          <NPCCard state={target} view={view} onItemClick={handleItemClick} />
        ) : (
          <NoTarget zone={target.current_zone} />
        )}
      </OverlayWindow>
      <ItemDetailModal item={modalItem} open={modalOpen} onClose={() => setModalOpen(false)} />
    </>
  )
}
