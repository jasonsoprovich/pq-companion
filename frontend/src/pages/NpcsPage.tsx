import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Check, Copy, Search, X, Bell } from 'lucide-react'
import { searchNPCs, getNPC, getNPCSpawns, getNPCLoot, getNPCFaction } from '../services/api'
import type { NPC, NPCSpawns, NPCLootTable, NPCFaction } from '../types/npc'
import {
  bodyTypeName,
  className,
  npcDisplayName,
  npcSpecialAbilities,
  specialAbilityAlertPattern,
  type SpecialAbility,
} from '../lib/npcHelpers'
import CreateTriggerModal, { type TriggerPrefill } from '../components/CreateTriggerModal'
import { ItemIcon } from '../components/Icon'

function formatRespawnTime(seconds: number): string {
  if (seconds <= 0) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  const parts: string[] = []
  if (h > 0) parts.push(`${h}h`)
  if (m > 0) parts.push(`${m}m`)
  if (s > 0 || parts.length === 0) parts.push(`${s}s`)
  return parts.join(' ')
}

function formatNPCName(name: string): string {
  return name.replace(/_/g, ' ')
}

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (npc: NPC) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [npcs, setNpcs] = useState<NPC[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showPlaceholders, setShowPlaceholders] = useState(true)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string, placeholders: boolean) => {
    setLoading(true)
    setError(null)
    searchNPCs(q, 50, 0, placeholders)
      .then((res) => {
        setNpcs(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, showPlaceholders), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, showPlaceholders, runSearch])

  useEffect(() => {
    runSearch('', showPlaceholders)
  }, [runSearch])

  return (
    <div
      className="flex w-72 shrink-0 flex-col border-r"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {/* Search input */}
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
        <input
          type="text"
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
          style={{ color: 'var(--color-foreground)' }}
          placeholder="Search NPCs…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          spellCheck={false}
        />
        {query && (
          <button onClick={() => setQuery('')} className="shrink-0">
            <X size={12} style={{ color: 'var(--color-muted)' }} />
          </button>
        )}
      </div>

      {/* Result count + placeholder toggle */}
      <div
        className="flex items-center justify-between border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        <span>{loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} NPCs`}</span>
        <button
          onClick={() => setShowPlaceholders((v) => !v)}
          title={showPlaceholders ? 'Hide placeholder NPCs' : 'Show placeholder NPCs'}
          className="rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors"
          style={{
            backgroundColor: showPlaceholders ? 'var(--color-surface-2)' : 'transparent',
            color: showPlaceholders ? 'var(--color-primary)' : 'var(--color-muted)',
            border: `1px solid ${showPlaceholders ? 'var(--color-primary)' : 'var(--color-border)'}`,
          }}
        >
          +placeholders
        </button>
      </div>

      {/* Results list */}
      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>
            {error}
          </p>
        )}
        {!error &&
          npcs.map((npc) => (
            <button
              key={npc.id}
              onClick={() => onSelect(npc)}
              className="w-full px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor:
                  selectedId === npc.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  selectedId === npc.id
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              <div
                className="truncate text-sm font-medium"
                style={{
                  color:
                    selectedId === npc.id
                      ? 'var(--color-primary)'
                      : 'var(--color-foreground)',
                }}
              >
                {npcDisplayName(npc)}
              </div>
              <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {`Lv ${npc.level} ${className(npc.class)}`}
              </div>
            </button>
          ))}
      </div>
    </div>
  )
}

// ── Detail panel helpers ───────────────────────────────────────────────────────

interface StatRowProps {
  label: string
  value: string | number
}

function StatRow({ label, value }: StatRowProps): React.ReactElement {
  return (
    <div className="flex justify-between py-0.5 text-sm">
      <span style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

interface SectionProps {
  title: string
  children: React.ReactNode
}

function Section({ title, children }: SectionProps): React.ReactElement {
  return (
    <div>
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      <div
        className="rounded border px-3 py-1"
        style={{
          backgroundColor: 'var(--color-surface)',
          borderColor: 'var(--color-border)',
        }}
      >
        {children}
      </div>
    </div>
  )
}

// ── Special Abilities list with popover ────────────────────────────────────────

interface SpecialAbilitiesListProps {
  abilities: SpecialAbility[]
}

function SpecialAbilitiesList({ abilities }: SpecialAbilitiesListProps): React.ReactElement {
  const [activeCode, setActiveCode] = useState<number | null>(null)
  const [alertPrefill, setAlertPrefill] = useState<TriggerPrefill | null>(null)

  function toggle(code: number) {
    setActiveCode((prev) => (prev === code ? null : code))
  }

  function openCreateAlert(sa: SpecialAbility) {
    const known = specialAbilityAlertPattern(sa.code)
    setAlertPrefill({
      name: `${sa.name} Alert`,
      pattern: known?.pattern ?? '',
      displayText: known?.text ?? sa.name.toUpperCase(),
      displayColor: '#ff4444',
      timerType: 'none',
    })
    setActiveCode(null)
  }

  return (
    <div className="flex flex-wrap gap-1.5 py-1">
      {abilities.map((sa) => (
        <div key={sa.code} className="relative">
          <button
            onClick={() => toggle(sa.code)}
            className="rounded px-2 py-0.5 text-[11px] font-medium transition-colors"
            style={{
              backgroundColor: activeCode === sa.code ? 'var(--color-surface-3, var(--color-surface-2))' : 'var(--color-surface-2)',
              color: activeCode === sa.code ? 'var(--color-primary)' : 'var(--color-foreground)',
              border: `1px solid ${activeCode === sa.code ? 'var(--color-primary)' : 'var(--color-border)'}`,
              cursor: 'pointer',
            }}
          >
            {sa.name}
          </button>
          {activeCode === sa.code && (
            <div
              className="absolute left-0 top-full z-10 mt-1 w-64 rounded border p-2 text-xs shadow-lg"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor: 'var(--color-border)',
                color: 'var(--color-muted-foreground)',
              }}
            >
              {sa.description && <p className="mb-1.5">{sa.description}</p>}
              <button
                onClick={() => openCreateAlert(sa)}
                className="flex items-center gap-1.5 text-[11px] px-2 py-0.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-background)',
                  border: '1px solid transparent',
                }}
              >
                <Bell size={10} /> Create Alert
              </button>
            </div>
          )}
        </div>
      ))}
      {alertPrefill && (
        <CreateTriggerModal
          prefill={alertPrefill}
          onClose={() => setAlertPrefill(null)}
        />
      )}
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

interface DetailPanelProps {
  npc: NPC | null
}

function DetailPanel({ npc }: DetailPanelProps): React.ReactElement {
  const navigate = useNavigate()
  const [spawns, setSpawns] = useState<NPCSpawns | null>(null)
  const [loot, setLoot] = useState<NPCLootTable | null>(null)
  const [faction, setFaction] = useState<NPCFaction | null>(null)
  const [bulkCopied, setBulkCopied] = useState<number | null>(null)

  useEffect(() => {
    if (!npc) { setSpawns(null); setLoot(null); setFaction(null); return }
    getNPCSpawns(npc.id)
      .then(setSpawns)
      .catch(() => setSpawns({ spawn_points: [], spawn_groups: [] }))
    getNPCLoot(npc.id)
      .then(setLoot)
      .catch(() => setLoot(null))
    if (npc.npc_faction_id > 0) {
      getNPCFaction(npc.id)
        .then(setFaction)
        .catch(() => setFaction(null))
    } else {
      setFaction(null)
    }
  }, [npc?.id])

  function copyBulkLinks(dropId: number, items: { item_id: number; item_name: string }[]) {
    const links = items.map((it) => `\\aITEM ${it.item_id} 0 0 0 0 0:${it.item_name}\\a/`).join('\n')
    navigator.clipboard.writeText(links).then(() => {
      setBulkCopied(dropId)
      setTimeout(() => setBulkCopied(null), 2000)
    })
  }

  if (!npc) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select an NPC to view details
        </p>
      </div>
    )
  }

  const specialAbilities = npcSpecialAbilities(npc)

  const flags: string[] = []
  if (npc.raid_target) flags.push('RAID TARGET')
  if (npc.rare_spawn) flags.push('RARE SPAWN')

  const hasStats = npc.str || npc.sta || npc.dex || npc.agi || npc.int || npc.wis || npc.cha
  const hasResists = npc.mr || npc.cr || npc.dr || npc.fr || npc.pr

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4">
      {/* Header */}
      <div className="mb-4">
        <h2
          className="text-xl font-bold leading-tight"
          style={{ color: 'var(--color-primary)' }}
        >
          {npcDisplayName(npc)}
        </h2>
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            {`Level ${npc.level} ${className(npc.class)} · ${npc.race_name}`}
          </span>
          {flags.map((f) => (
            <span
              key={f}
              className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-primary)',
                border: '1px solid var(--color-border)',
              }}
            >
              {f}
            </span>
          ))}
        </div>
        <div className="mt-0.5 text-xs" style={{ color: 'var(--color-muted)' }}>
          {bodyTypeName(npc.body_type)}
        </div>
      </div>

      <div className="flex flex-col gap-3">
        {/* Combat */}
        <Section title="Combat">
          <StatRow label="HP" value={npc.hp.toLocaleString()} />
          {npc.mana > 0 && <StatRow label="Mana" value={npc.mana.toLocaleString()} />}
          <StatRow label="Damage" value={`${npc.min_dmg}–${npc.max_dmg}`} />
          {npc.attack_count > 0 && (
            <StatRow label="Attacks" value={npc.attack_count} />
          )}
          <StatRow label="AC" value={npc.ac} />
        </Section>

        {/* Attributes */}
        {hasStats ? (
          <Section title="Attributes">
            {npc.str > 0 && <StatRow label="STR" value={npc.str} />}
            {npc.sta > 0 && <StatRow label="STA" value={npc.sta} />}
            {npc.dex > 0 && <StatRow label="DEX" value={npc.dex} />}
            {npc.agi > 0 && <StatRow label="AGI" value={npc.agi} />}
            {npc.int > 0 && <StatRow label="INT" value={npc.int} />}
            {npc.wis > 0 && <StatRow label="WIS" value={npc.wis} />}
            {npc.cha > 0 && <StatRow label="CHA" value={npc.cha} />}
          </Section>
        ) : null}

        {/* Resists */}
        {hasResists ? (
          <Section title="Resists">
            {npc.mr > 0 && <StatRow label="Magic" value={npc.mr} />}
            {npc.cr > 0 && <StatRow label="Cold" value={npc.cr} />}
            {npc.dr > 0 && <StatRow label="Disease" value={npc.dr} />}
            {npc.fr > 0 && <StatRow label="Fire" value={npc.fr} />}
            {npc.pr > 0 && <StatRow label="Poison" value={npc.pr} />}
          </Section>
        ) : null}

        {/* Special Abilities */}
        {specialAbilities.length > 0 && (
          <Section title="Special Abilities">
            <SpecialAbilitiesList abilities={specialAbilities} />
          </Section>
        )}

        {/* Behavior */}
        <Section title="Behavior">
          {npc.aggro_radius > 0 && (
            <StatRow label="Aggro Radius" value={npc.aggro_radius} />
          )}
          <StatRow label="Run Speed" value={npc.run_speed.toFixed(2)} />
          <StatRow label="Size" value={npc.size.toFixed(1)} />
        </Section>

        {/* Info */}
        <Section title="Info">
          <StatRow label="NPC ID" value={npc.id} />
          {npc.exp_pct !== 100 && <StatRow label="Exp %" value={`${npc.exp_pct}%`} />}
          {npc.merchant_id > 0 && (
            <StatRow label="Merchant ID" value={npc.merchant_id} />
          )}
          {npc.npc_spells_id > 0 && (
            <StatRow label="Spells ID" value={npc.npc_spells_id} />
          )}
          {npc.npc_faction_id > 0 && !faction && (
            <StatRow label="Faction ID" value={npc.npc_faction_id} />
          )}
          {npc.spell_scale !== 100 && (
            <StatRow label="Spell Scale" value={`${npc.spell_scale.toFixed(0)}%`} />
          )}
          {npc.heal_scale !== 100 && (
            <StatRow label="Heal Scale" value={`${npc.heal_scale.toFixed(0)}%`} />
          )}
        </Section>

        {/* Faction */}
        {faction && (
          <Section title="Faction">
            {faction.primary_faction_name && (
              <StatRow label="Primary" value={faction.primary_faction_name} />
            )}
            {faction.hits.length > 0 && (
              <div className="py-1">
                <div
                  className="mb-1 text-[10px] font-medium uppercase tracking-wide"
                  style={{ color: 'var(--color-muted)' }}
                >
                  On Kill
                </div>
                {faction.hits.map((hit) => (
                  <div key={hit.faction_id} className="flex justify-between border-t py-0.5 text-sm" style={{ borderColor: 'var(--color-border)' }}>
                    <span style={{ color: 'var(--color-muted-foreground)' }}>{hit.faction_name}</span>
                    <span
                      style={{
                        color: hit.value > 0 ? 'var(--color-primary)' : hit.value < 0 ? '#f87171' : 'var(--color-muted)',
                        fontVariantNumeric: 'tabular-nums',
                      }}
                    >
                      {hit.value > 0 ? `+${hit.value}` : hit.value}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </Section>
        )}

        {/* Loot Table */}
        {loot && loot.drops.length > 0 && (
          <Section title="Loot Table">
            {loot.drops.map((drop) => (
              <div key={drop.id} className="mb-2 last:mb-0">
                <div className="flex items-center justify-between pt-1 pb-0.5">
                  <span className="text-[11px] font-medium" style={{ color: 'var(--color-muted)' }}>
                    {drop.multiplier > 1 ? `×${drop.multiplier} · ` : ''}
                    {drop.probability < 100 ? `${drop.probability}% chance` : 'Always drops'}
                  </span>
                  <button
                    onClick={() => copyBulkLinks(drop.id, drop.items)}
                    title="Bulk copy in-game links"
                    className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-medium transition-colors"
                    style={{
                      backgroundColor: 'var(--color-surface)',
                      borderColor: 'var(--color-border)',
                      color: bulkCopied === drop.id ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                    }}
                  >
                    {bulkCopied === drop.id ? <Check size={10} /> : <Copy size={10} />}
                    {bulkCopied === drop.id ? 'Copied!' : 'Copy links'}
                  </button>
                </div>
                {drop.items.map((item) => (
                  <button
                    key={item.item_id}
                    onClick={() => navigate(`/items?select=${item.item_id}`)}
                    className="flex w-full items-center gap-2 border-t py-0.5 text-left text-sm"
                    style={{ borderColor: 'var(--color-border)' }}
                  >
                    <ItemIcon id={item.item_icon} name={item.item_name} size={20} />
                    <span
                      className="truncate underline decoration-dotted flex-1"
                      style={{ color: 'var(--color-primary)' }}
                    >
                      {item.item_name}
                    </span>
                    <span className="ml-3 shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
                      {item.chance.toFixed(1)}%
                      {item.multiplier > 1 && ` ×${item.multiplier}`}
                    </span>
                  </button>
                ))}
              </div>
            ))}
          </Section>
        )}

        {/* Spawns */}
        {spawns && (spawns.spawn_points?.length ?? 0) > 0 && (
          <Section title="Spawns">
            <div
              className="mb-1 grid gap-x-3 pt-1 text-[11px] font-medium"
              style={{
                gridTemplateColumns: '1fr auto auto auto auto',
                color: 'var(--color-muted)',
              }}
            >
              <span>Zone</span>
              <span className="text-right">Y</span>
              <span className="text-right">X</span>
              <span className="text-right">Z</span>
              <span className="text-right">Respawn</span>
            </div>
            {spawns.spawn_points.map((sp) => (
              <div
                key={sp.id}
                className="grid items-center gap-x-3 border-t py-0.5 text-sm"
                style={{
                  gridTemplateColumns: '1fr auto auto auto auto',
                  borderColor: 'var(--color-border)',
                }}
              >
                <span className="truncate" style={{ color: 'var(--color-foreground)' }}>
                  {sp.zone_name || sp.zone}
                </span>
                <span className="font-mono text-xs text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.y.toFixed(0)}
                </span>
                <span className="font-mono text-xs text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.x.toFixed(0)}
                </span>
                <span className="font-mono text-xs text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.z.toFixed(0)}
                </span>
                <span className="text-xs text-right whitespace-nowrap" style={{ color: 'var(--color-muted-foreground)' }}>
                  {sp.fast_respawn_time > 0
                    ? `${formatRespawnTime(sp.fast_respawn_time)} / ${formatRespawnTime(sp.respawn_time)}`
                    : formatRespawnTime(sp.respawn_time)}
                </span>
              </div>
            ))}
          </Section>
        )}

        {/* Spawn Groups */}
        {spawns && (spawns.spawn_groups?.length ?? 0) > 0 && (
          <Section title="Spawn Groups">
            {spawns.spawn_groups.map((sg, i) => (
              <div
                key={sg.id}
                className={i > 0 ? 'mt-3' : ''}
              >
                <div className="flex items-center justify-between pt-1 pb-0.5">
                  <span className="text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
                    {sg.name}
                  </span>
                  <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    {sg.fast_respawn_time > 0
                      ? `${formatRespawnTime(sg.fast_respawn_time)} / ${formatRespawnTime(sg.respawn_time)}`
                      : formatRespawnTime(sg.respawn_time)}
                  </span>
                </div>
                {sg.members.map((m) => (
                  <div
                    key={m.npc_id}
                    className="flex items-center justify-between border-t py-0.5 text-sm"
                    style={{ borderColor: 'var(--color-border)' }}
                  >
                    <span
                      className="truncate"
                      style={{
                        color: m.npc_id === npc.id
                          ? 'var(--color-primary)'
                          : 'var(--color-foreground)',
                      }}
                    >
                      {formatNPCName(m.name)}
                    </span>
                    <span className="ml-3 shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
                      {m.chance}%
                    </span>
                  </div>
                ))}
              </div>
            ))}
          </Section>
        )}
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function NpcsPage(): React.ReactElement {
  const [selected, setSelected] = useState<NPC | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) return
    getNPC(id)
      .then(setSelected)
      .catch(() => {/* ignore */})
      .finally(() => setSearchParams({}, { replace: true }))
  }, [searchParams, setSearchParams])

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel npc={selected} />
    </div>
  )
}
