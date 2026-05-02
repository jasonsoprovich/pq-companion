import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Search, X, Clock } from 'lucide-react'
import { searchSpells, getSpell, getSpellCrossRefs } from '../services/api'
import type { Spell, SpellCrossRefs } from '../types/spell'
import {
  buildSpellTriggerPrefill,
  castableClasses,
  castableClassesShort,
  durationLabel,
  durationScales,
  effectDescription,
  msLabel,
  resistLabel,
  skillLabel,
  targetLabel,
  zoneTypeLabel,
} from '../lib/spellHelpers'
import CreateTriggerModal from '../components/CreateTriggerModal'
import { SpellIcon } from '../components/Icon'

const SPELL_CLASSES: { index: number; abbr: string; full: string }[] = [
  { index: 0,  abbr: 'WAR', full: 'Warrior' },
  { index: 1,  abbr: 'CLR', full: 'Cleric' },
  { index: 2,  abbr: 'PAL', full: 'Paladin' },
  { index: 3,  abbr: 'RNG', full: 'Ranger' },
  { index: 4,  abbr: 'SHD', full: 'Shadow Knight' },
  { index: 5,  abbr: 'DRU', full: 'Druid' },
  { index: 6,  abbr: 'MNK', full: 'Monk' },
  { index: 7,  abbr: 'BRD', full: 'Bard' },
  { index: 8,  abbr: 'ROG', full: 'Rogue' },
  { index: 9,  abbr: 'SHM', full: 'Shaman' },
  { index: 10, abbr: 'NEC', full: 'Necromancer' },
  { index: 11, abbr: 'WIZ', full: 'Wizard' },
  { index: 12, abbr: 'MAG', full: 'Magician' },
  { index: 13, abbr: 'ENC', full: 'Enchanter' },
  { index: 14, abbr: 'BST', full: 'Beastlord' },
]

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (spell: Spell) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [classIndex, setClassIndex] = useState(-1)
  const [minLevel, setMinLevel] = useState('')
  const [maxLevel, setMaxLevel] = useState('')
  const [spells, setSpells] = useState<Spell[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string, cls: number, min: string, max: string) => {
    setLoading(true)
    setError(null)
    const minLvl = parseInt(min) || 0
    const maxLvl = parseInt(max) || 0
    searchSpells(q, 50, 0, cls, minLvl, maxLvl)
      .then((res) => {
        setSpells(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, classIndex, minLevel, maxLevel), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, classIndex, minLevel, maxLevel, runSearch])

  useEffect(() => {
    runSearch('', -1, '', '')
  }, [runSearch])

  const selectStyle = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
  }

  const levelInputStyle = {
    backgroundColor: 'transparent',
    color: 'var(--color-foreground)',
    width: '3.5rem',
  }

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
          placeholder="Search spells…"
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

      {/* Filters */}
      <div
        className="flex flex-col gap-1.5 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {/* Class filter */}
        <select
          value={classIndex}
          onChange={(e) => setClassIndex(Number(e.target.value))}
          className="w-full rounded px-2 py-1 text-xs outline-none"
          style={selectStyle}
        >
          <option value={-1}>All Classes</option>
          {SPELL_CLASSES.map((c) => (
            <option key={c.index} value={c.index}>{c.abbr} — {c.full}</option>
          ))}
          <option value={15}>NPC Only</option>
        </select>

        {/* Level range — only useful with a class selected */}
        <div className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>
          <span>Lvl</span>
          <input
            type="number"
            min={1}
            max={255}
            placeholder="min"
            value={minLevel}
            onChange={(e) => setMinLevel(e.target.value)}
            disabled={classIndex < 0 || classIndex === 15}
            className="rounded border px-1.5 py-0.5 text-xs outline-none disabled:opacity-40"
            style={{ ...levelInputStyle, borderColor: 'var(--color-border)' }}
          />
          <span>–</span>
          <input
            type="number"
            min={1}
            max={255}
            placeholder="max"
            value={maxLevel}
            onChange={(e) => setMaxLevel(e.target.value)}
            disabled={classIndex < 0 || classIndex === 15}
            className="rounded border px-1.5 py-0.5 text-xs outline-none disabled:opacity-40"
            style={{ ...levelInputStyle, borderColor: 'var(--color-border)' }}
          />
          {(classIndex >= 0 || minLevel || maxLevel) && (
            <button
              onClick={() => { setClassIndex(-1); setMinLevel(''); setMaxLevel('') }}
              className="ml-auto shrink-0"
              title="Clear filters"
            >
              <X size={11} style={{ color: 'var(--color-muted)' }} />
            </button>
          )}
        </div>
      </div>

      {/* Result count */}
      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} spells`}
      </div>

      {/* Results list */}
      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>
            {error}
          </p>
        )}
        {!error &&
          spells.map((spell) => (
            <button
              key={spell.id}
              onClick={() => onSelect(spell)}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor:
                  selectedId === spell.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  selectedId === spell.id
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              <SpellIcon id={spell.new_icon} name={spell.name} size={28} />
              <div className="min-w-0 flex-1">
                <div
                  className="truncate text-sm font-medium"
                  style={{
                    color:
                      selectedId === spell.id
                        ? 'var(--color-primary)'
                        : 'var(--color-foreground)',
                  }}
                >
                  {spell.name}
                </div>
                <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {castableClassesShort(spell.class_levels)}
                  {spell.mana > 0 && ` · ${spell.mana} mana`}
                </div>
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
  spell: Spell | null
}

function DetailPanel({ spell }: DetailPanelProps): React.ReactElement {
  const navigate = useNavigate()
  const [crossRefs, setCrossRefs] = useState<SpellCrossRefs | null>(null)
  const [showTriggerModal, setShowTriggerModal] = useState(false)

  useEffect(() => {
    if (!spell) { setCrossRefs(null); return }
    getSpellCrossRefs(spell.id)
      .then(setCrossRefs)
      .catch(() => setCrossRefs(null))
  }, [spell?.id])

  if (!spell) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select a spell to view details
        </p>
      </div>
    )
  }

  const classes = castableClasses(spell.class_levels)
  const hasDuration = spell.buff_duration > 0
  const hasAoE = spell.aoe_range > 0
  const isScalingDuration = durationScales(spell.buff_duration_formula, spell.buff_duration)
  const zoneType = zoneTypeLabel(spell.zone_type)

  // Collect active effect slots with human-readable descriptions
  const activeEffects = spell.effect_ids
    .map((id, i) => ({
      id,
      base: spell.effect_base_values[i] ?? 0,
      description: effectDescription(id, spell.effect_base_values[i] ?? 0, spell.buff_duration),
    }))
    .filter((e) => e.description !== '')

  const flags: string[] = []
  if (spell.is_discipline) flags.push('DISCIPLINE')
  if (spell.no_dispell) flags.push('NO DISPELL')

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4">
      {/* Header */}
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <SpellIcon id={spell.new_icon} name={spell.name} size={40} />
          <div>
            <h2
              className="text-xl font-bold leading-tight"
              style={{ color: 'var(--color-primary)' }}
            >
              {spell.name}
            </h2>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              {skillLabel(spell.skill) && (
                <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                  {skillLabel(spell.skill)}
                </span>
              )}
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
          </div>
        </div>
        {hasDuration && (
          <button
            onClick={() => setShowTriggerModal(true)}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium shrink-0"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-background)',
              border: '1px solid transparent',
            }}
            title="Create a trigger and timer from this spell"
          >
            <Clock size={12} />
            Create Timer Trigger
          </button>
        )}
      </div>

      {showTriggerModal && (
        <CreateTriggerModal
          prefill={buildSpellTriggerPrefill(spell)}
          onClose={() => setShowTriggerModal(false)}
        />
      )}

      <div className="flex flex-col gap-3">
        {/* Casting */}
        <Section title="Casting">
          {skillLabel(spell.skill) && <StatRow label="Skill" value={skillLabel(spell.skill)} />}
          <StatRow label="Mana Cost" value={spell.mana > 0 ? spell.mana : 'None'} />
          <StatRow label="Cast Time" value={msLabel(spell.cast_time)} />
          {spell.recast_time > 0 && (
            <StatRow label="Recast Time" value={msLabel(spell.recast_time)} />
          )}
          {spell.recovery_time > 0 && (
            <StatRow label="Recovery" value={msLabel(spell.recovery_time)} />
          )}
          {hasDuration && (
            <StatRow
              label={isScalingDuration ? 'Max Duration' : 'Duration'}
              value={durationLabel(spell.buff_duration_formula, spell.buff_duration)}
            />
          )}
        </Section>

        {/* Targeting */}
        <Section title="Targeting">
          <StatRow label="Target" value={targetLabel(spell.target_type)} />
          <StatRow label="Resist" value={resistLabel(spell.resist_type)} />
          {spell.range > 0 && <StatRow label="Range" value={`${spell.range} units`} />}
          {hasAoE && <StatRow label="AoE Range" value={`${spell.aoe_range} units`} />}
          {zoneType && <StatRow label="Zone Type" value={zoneType} />}
        </Section>

        {/* Classes */}
        <Section title="Classes">
          {classes.length > 0 ? (
            <div className="flex flex-wrap gap-x-4 gap-y-1 py-0.5">
              {classes.map((c) => (
                <div key={c.abbr} className="flex items-baseline gap-1 text-sm">
                  <span style={{ color: 'var(--color-foreground)' }}>{c.full}</span>
                  <span
                    className="text-xs"
                    style={{ color: 'var(--color-muted-foreground)' }}
                  >
                    Lv {c.level}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              NPC Only
            </span>
          )}
        </Section>

        {/* Effects */}
        {activeEffects.length > 0 && (
          <Section title="Effects">
            {activeEffects.map((e, i) => (
              <div key={i} className="py-0.5 text-sm" style={{ color: 'var(--color-foreground)' }}>
                {e.description}
              </div>
            ))}
          </Section>
        )}

        {/* Flavor text */}
        {(spell.cast_on_you || spell.cast_on_other || spell.spell_fades) && (
          <Section title="Messages">
            {spell.cast_on_you && (
              <div className="py-0.5 text-sm">
                <span style={{ color: 'var(--color-muted-foreground)' }}>On you: </span>
                <span className="italic" style={{ color: 'var(--color-foreground)' }}>
                  {spell.cast_on_you}
                </span>
              </div>
            )}
            {spell.cast_on_other && (
              <div className="py-0.5 text-sm">
                <span style={{ color: 'var(--color-muted-foreground)' }}>On other: </span>
                <span className="italic" style={{ color: 'var(--color-foreground)' }}>
                  {spell.cast_on_other}
                </span>
              </div>
            )}
            {spell.spell_fades && (
              <div className="py-0.5 text-sm">
                <span style={{ color: 'var(--color-muted-foreground)' }}>Fades: </span>
                <span className="italic" style={{ color: 'var(--color-foreground)' }}>
                  {spell.spell_fades}
                </span>
              </div>
            )}
          </Section>
        )}

        {/* Taught by */}
        {crossRefs && crossRefs.scroll_items.length > 0 && (
          <Section title="Taught by">
            {crossRefs.scroll_items.map((item) => (
              <button
                key={item.id}
                onClick={() => navigate(`/items?select=${item.id}`)}
                className="flex w-full items-center border-t py-0.5 text-left text-sm first:border-t-0"
                style={{ borderColor: 'var(--color-border)' }}
              >
                <span
                  className="underline decoration-dotted"
                  style={{ color: 'var(--color-primary)' }}
                >
                  {item.name}
                </span>
              </button>
            ))}
          </Section>
        )}

        {/* Items with this effect */}
        {crossRefs && crossRefs.effect_items.length > 0 && (
          <Section title="Items with this effect">
            {(['click', 'worn', 'proc', 'focus'] as const).map((type) => {
              const items = crossRefs.effect_items.filter((i) => i.effect_type === type)
              if (items.length === 0) return null
              return (
                <div key={type} className="py-1 first:pt-0">
                  <div
                    className="mb-0.5 text-[10px] font-medium uppercase tracking-wide"
                    style={{ color: 'var(--color-muted)' }}
                  >
                    {type}
                  </div>
                  {items.map((item) => (
                    <button
                      key={item.id}
                      onClick={() => navigate(`/items?select=${item.id}`)}
                      className="flex w-full items-center border-t py-0.5 text-left text-sm"
                      style={{ borderColor: 'var(--color-border)' }}
                    >
                      <span
                        className="underline decoration-dotted"
                        style={{ color: 'var(--color-primary)' }}
                      >
                        {item.name}
                      </span>
                    </button>
                  ))}
                </div>
              )
            })}
          </Section>
        )}

        {/* Info */}
        <Section title="Info">
          <StatRow label="Spell ID" value={spell.id} />
        </Section>
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function SpellsPage(): React.ReactElement {
  const [selected, setSelected] = useState<Spell | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) return
    getSpell(id)
      .then(setSelected)
      .catch(() => {/* ignore */})
      .finally(() => setSearchParams({}, { replace: true }))
  }, [searchParams, setSearchParams])

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel spell={selected} />
    </div>
  )
}
