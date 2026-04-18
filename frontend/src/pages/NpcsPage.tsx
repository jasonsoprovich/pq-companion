import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search, X } from 'lucide-react'
import { searchNPCs, getNPC } from '../services/api'
import type { NPC } from '../types/npc'
import {
  bodyTypeName,
  className,
  npcDisplayName,
  parseSpecialAbilities,
} from '../lib/npcHelpers'

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
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string) => {
    setLoading(true)
    setError(null)
    searchNPCs(q, 50, 0)
      .then((res) => {
        setNpcs(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, runSearch])

  useEffect(() => {
    runSearch('')
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

      {/* Result count */}
      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} NPCs`}
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

// ── Detail panel ───────────────────────────────────────────────────────────────

interface DetailPanelProps {
  npc: NPC | null
}

function DetailPanel({ npc }: DetailPanelProps): React.ReactElement {
  if (!npc) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select an NPC to view details
        </p>
      </div>
    )
  }

  const specialAbilities = parseSpecialAbilities(npc.special_abilities)

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
            <div className="flex flex-wrap gap-1.5 py-1">
              {specialAbilities.map((sa) => (
                <span
                  key={sa.code}
                  className="rounded px-2 py-0.5 text-[11px] font-medium"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    color: 'var(--color-foreground)',
                    border: '1px solid var(--color-border)',
                  }}
                >
                  {sa.name}
                </span>
              ))}
            </div>
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
          {npc.loottable_id > 0 && (
            <StatRow label="Loot Table" value={npc.loottable_id} />
          )}
          {npc.merchant_id > 0 && (
            <StatRow label="Merchant ID" value={npc.merchant_id} />
          )}
          {npc.npc_spells_id > 0 && (
            <StatRow label="Spells ID" value={npc.npc_spells_id} />
          )}
          {npc.npc_faction_id > 0 && (
            <StatRow label="Faction ID" value={npc.npc_faction_id} />
          )}
          {npc.spell_scale !== 100 && (
            <StatRow label="Spell Scale" value={`${npc.spell_scale.toFixed(0)}%`} />
          )}
          {npc.heal_scale !== 100 && (
            <StatRow label="Heal Scale" value={`${npc.heal_scale.toFixed(0)}%`} />
          )}
        </Section>
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
