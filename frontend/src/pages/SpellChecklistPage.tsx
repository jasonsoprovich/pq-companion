import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  BookOpen,
  CheckCircle2,
  Circle,
  ExternalLink,
  RefreshCw,
  AlertCircle,
  X,
} from 'lucide-react'
import { getConfig, getSpell, getSpellsByClass, getZealSpellbook } from '../services/api'
import type { Spell } from '../types/spell'
import type { Spellbook } from '../types/zeal'
import {
  castableClasses,
  durationLabel,
  durationScales,
  effectDescription,
  msLabel,
  resistLabel,
  skillLabel,
  targetLabel,
  zoneTypeLabel,
} from '../lib/spellHelpers'

// ── Class definitions ──────────────────────────────────────────────────────────

const CLASSES: { index: number; abbr: string; full: string }[] = [
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

const LS_CLASS_KEY = 'pq-companion:spell-checklist-class'
const DEFAULT_CLASS = 13 // Enchanter

type Filter = 'all' | 'known' | 'missing'

interface LevelFilter {
  min: string
  max: string
}

// ── Helpers ────────────────────────────────────────────────────────────────────

function classLevel(spell: Spell, classIndex: number): number {
  return spell.class_levels[classIndex] ?? 255
}

function savedClass(): number {
  try {
    const v = localStorage.getItem(LS_CLASS_KEY)
    if (v !== null) {
      const n = parseInt(v, 10)
      if (n >= 0 && n <= 14) return n
    }
  } catch {
    // ignore
  }
  return DEFAULT_CLASS
}

// ── Spell detail modal ─────────────────────────────────────────────────────────

interface SpellDetailModalProps {
  spell: Spell
  onClose: () => void
  onOpenInExplorer: (id: number) => void
}

function SpellDetailModal({ spell, onClose, onOpenInExplorer }: SpellDetailModalProps): React.ReactElement {
  const classes = castableClasses(spell.class_levels)
  const hasDuration = spell.buff_duration > 0
  const hasAoE = spell.aoe_range > 0
  const isScalingDuration = durationScales(spell.buff_duration_formula, spell.buff_duration)
  const zoneType = zoneTypeLabel(spell.zone_type)

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

  function StatRow({ label, value }: { label: string; value: string | number }) {
    return (
      <div className="flex justify-between py-0.5 text-sm">
        <span style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
        <span style={{ color: 'var(--color-foreground)' }}>{value}</span>
      </div>
    )
  }

  function Section({ title, children }: { title: string; children: React.ReactNode }) {
    return (
      <div>
        <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
          {title}
        </div>
        <div className="rounded border px-3 py-1" style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}>
          {children}
        </div>
      </div>
    )
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="relative flex flex-col w-full max-w-lg max-h-[80vh] rounded-lg overflow-hidden"
        style={{ backgroundColor: 'var(--color-background)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Modal header */}
        <div
          className="shrink-0 flex items-start justify-between px-5 pt-4 pb-3"
          style={{ borderBottom: '1px solid var(--color-border)' }}
        >
          <div>
            <h2 className="text-lg font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
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
                  style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-primary)', border: '1px solid var(--color-border)' }}
                >
                  {f}
                </span>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0 ml-3">
            <button
              onClick={() => onOpenInExplorer(spell.id)}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
              title="Open in Spell Explorer"
            >
              <ExternalLink size={11} />
              Explorer
            </button>
            <button onClick={onClose} title="Close">
              <X size={16} style={{ color: 'var(--color-muted)' }} />
            </button>
          </div>
        </div>

        {/* Modal body */}
        <div className="flex-1 overflow-y-auto px-5 py-4 flex flex-col gap-3">
          <Section title="Casting">
            {skillLabel(spell.skill) && <StatRow label="Skill" value={skillLabel(spell.skill)} />}
            <StatRow label="Mana Cost" value={spell.mana > 0 ? spell.mana : 'None'} />
            <StatRow label="Cast Time" value={msLabel(spell.cast_time)} />
            {spell.recast_time > 0 && <StatRow label="Recast Time" value={msLabel(spell.recast_time)} />}
            {spell.recovery_time > 0 && <StatRow label="Recovery" value={msLabel(spell.recovery_time)} />}
            {hasDuration && (
              <StatRow
                label={isScalingDuration ? 'Max Duration' : 'Duration'}
                value={durationLabel(spell.buff_duration_formula, spell.buff_duration)}
              />
            )}
          </Section>

          <Section title="Targeting">
            <StatRow label="Target" value={targetLabel(spell.target_type)} />
            <StatRow label="Resist" value={resistLabel(spell.resist_type)} />
            {spell.range > 0 && <StatRow label="Range" value={`${spell.range} units`} />}
            {hasAoE && <StatRow label="AoE Range" value={`${spell.aoe_range} units`} />}
            {zoneType && <StatRow label="Zone Type" value={zoneType} />}
          </Section>

          <Section title="Classes">
            {classes.length > 0 ? (
              <div className="flex flex-wrap gap-x-4 gap-y-1 py-0.5">
                {classes.map((c) => (
                  <div key={c.abbr} className="flex items-baseline gap-1 text-sm">
                    <span style={{ color: 'var(--color-foreground)' }}>{c.full}</span>
                    <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Lv {c.level}</span>
                  </div>
                ))}
              </div>
            ) : (
              <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>NPC Only</span>
            )}
          </Section>

          {activeEffects.length > 0 && (
            <Section title="Effects">
              {activeEffects.map((e, i) => (
                <div key={i} className="py-0.5 text-sm" style={{ color: 'var(--color-foreground)' }}>
                  {e.description}
                </div>
              ))}
            </Section>
          )}

          {(spell.cast_on_you || spell.cast_on_other || spell.spell_fades) && (
            <Section title="Messages">
              {spell.cast_on_you && (
                <div className="py-0.5 text-sm">
                  <span style={{ color: 'var(--color-muted-foreground)' }}>On you: </span>
                  <span className="italic" style={{ color: 'var(--color-foreground)' }}>{spell.cast_on_you}</span>
                </div>
              )}
              {spell.cast_on_other && (
                <div className="py-0.5 text-sm">
                  <span style={{ color: 'var(--color-muted-foreground)' }}>On other: </span>
                  <span className="italic" style={{ color: 'var(--color-foreground)' }}>{spell.cast_on_other}</span>
                </div>
              )}
              {spell.spell_fades && (
                <div className="py-0.5 text-sm">
                  <span style={{ color: 'var(--color-muted-foreground)' }}>Fades: </span>
                  <span className="italic" style={{ color: 'var(--color-foreground)' }}>{spell.spell_fades}</span>
                </div>
              )}
            </Section>
          )}

          <Section title="Info">
            <StatRow label="Spell ID" value={spell.id} />
          </Section>
        </div>
      </div>
    </div>
  )
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface SpellRowProps {
  spell: Spell
  classIndex: number
  known: boolean
  onSelect: (id: number) => void
}

function SpellRow({ spell, classIndex, known, onSelect }: SpellRowProps): React.ReactElement {
  const level = classLevel(spell, classIndex)
  return (
    <div
      className="group flex items-center gap-3 px-4 py-2 transition-colors cursor-pointer"
      style={{ borderBottom: '1px solid var(--color-border)' }}
      onClick={() => onSelect(spell.id)}
    >
      {/* Known indicator */}
      <div className="shrink-0 w-4">
        {known ? (
          <CheckCircle2
            size={15}
            style={{ color: 'var(--color-primary)' }}
          />
        ) : (
          <Circle
            size={15}
            style={{ color: 'var(--color-muted)' }}
          />
        )}
      </div>

      {/* Spell name */}
      <div className="flex-1 min-w-0">
        <span
          className="text-sm truncate"
          style={{
            color: known ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
          }}
        >
          {spell.name}
        </span>
      </div>

      {/* Level badge */}
      <span
        className="shrink-0 text-[11px] tabular-nums"
        style={{ color: 'var(--color-muted)' }}
      >
        Lv {level}
      </span>

      {/* Mana cost */}
      {spell.mana > 0 && (
        <span
          className="shrink-0 text-[11px] tabular-nums w-16 text-right"
          style={{ color: 'var(--color-muted)' }}
        >
          {spell.mana}m
        </span>
      )}

      {/* Open in explorer */}
      <button
        onClick={(e) => { e.stopPropagation(); onSelect(spell.id) }}
        className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
        title="View in Spell Explorer"
      >
        <ExternalLink size={12} style={{ color: 'var(--color-muted)' }} />
      </button>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function SpellChecklistPage(): React.ReactElement {
  const [classIndex, setClassIndex] = useState<number>(savedClass)
  const [filter, setFilter] = useState<Filter>('all')
  const [levelFilter, setLevelFilter] = useState<LevelFilter>({ min: '', max: '' })
  const [spells, setSpells] = useState<Spell[]>([])
  const [spellbook, setSpellbook] = useState<Spellbook | null>(null)
  const [characterName, setCharacterName] = useState<string>('')
  const [loadingSpells, setLoadingSpells] = useState(true)
  const [loadingBook, setLoadingBook] = useState(true)
  const [spellError, setSpellError] = useState<string | null>(null)
  const [modalSpell, setModalSpell] = useState<Spell | null>(null)
  const navigate = useNavigate()

  // On mount, fetch config to auto-detect character class and name.
  useEffect(() => {
    getConfig()
      .then((cfg) => {
        if (cfg.character) setCharacterName(cfg.character)
        if (cfg.character_class >= 0 && cfg.character_class <= 14) {
          setClassIndex(cfg.character_class)
        }
      })
      .catch(() => { /* non-fatal: fall back to localStorage */ })
  }, [])

  const loadSpells = useCallback((idx: number) => {
    setLoadingSpells(true)
    setSpellError(null)
    getSpellsByClass(idx, 1000, 0)
      .then((res) => setSpells(res.items ?? []))
      .catch((err: Error) => setSpellError(err.message))
      .finally(() => setLoadingSpells(false))
  }, [])

  const loadSpellbook = useCallback(() => {
    setLoadingBook(true)
    getZealSpellbook()
      .then((res) => {
        setSpellbook(res.spellbook)
        if (res.spellbook?.character) setCharacterName(res.spellbook.character)
      })
      .catch(() => setSpellbook(null))
      .finally(() => setLoadingBook(false))
  }, [])

  useEffect(() => { loadSpells(classIndex) }, [classIndex, loadSpells])
  useEffect(() => { loadSpellbook() }, [loadSpellbook])

  function handleClassChange(idx: number) {
    setClassIndex(idx)
    setLevelFilter({ min: '', max: '' })
    try { localStorage.setItem(LS_CLASS_KEY, String(idx)) } catch { /* ignore */ }
  }

  function handleSelectSpell(id: number) {
    getSpell(id)
      .then(setModalSpell)
      .catch(() => { /* non-fatal */ })
  }

  function handleOpenInExplorer(id: number) {
    setModalSpell(null)
    navigate(`/spells?select=${id}`)
  }

  const knownIds = new Set(spellbook?.spell_ids ?? [])

  const minLvl = parseInt(levelFilter.min) || 0
  const maxLvl = parseInt(levelFilter.max) || 0

  const filteredSpells = spells.filter((s) => {
    if (filter === 'known' && !knownIds.has(s.id)) return false
    if (filter === 'missing' && knownIds.has(s.id)) return false
    const lvl = classLevel(s, classIndex)
    if (minLvl > 0 && lvl < minLvl) return false
    if (maxLvl > 0 && lvl > maxLvl) return false
    return true
  })

  const knownCount = spells.filter((s) => knownIds.has(s.id)).length
  const loading = loadingSpells || loadingBook

  return (
    <div className="flex h-full flex-col" style={{ backgroundColor: 'var(--color-background)' }}>
      {/* ── Header ── */}
      <div
        className="shrink-0 flex flex-col gap-2 border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {/* Row 1: icon + title + character name + class selector */}
        <div className="flex items-center gap-3">
          <BookOpen size={18} style={{ color: 'var(--color-primary)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Spell Checklist
          </span>
          {characterName && (
            <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              — {characterName}
            </span>
          )}

          <div className="flex-1" />

          {/* Class selector */}
          <select
            value={classIndex}
            onChange={(e) => handleClassChange(Number(e.target.value))}
            className="text-xs rounded px-2 py-1 outline-none"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            {CLASSES.map((c) => (
              <option key={c.index} value={c.index}>
                {c.abbr} — {c.full}
              </option>
            ))}
          </select>
        </div>

        {/* Row 2: filter tabs + level range + stats */}
        <div className="flex items-center gap-3 flex-wrap">
          {/* Filter tabs */}
          <div
            className="flex rounded overflow-hidden text-xs"
            style={{ border: '1px solid var(--color-border)' }}
          >
            {(['all', 'known', 'missing'] as Filter[]).map((f) => (
              <button
                key={f}
                onClick={() => setFilter(f)}
                className="px-3 py-1 capitalize transition-colors"
                style={{
                  backgroundColor:
                    filter === f ? 'var(--color-surface-2)' : 'transparent',
                  color:
                    filter === f
                      ? 'var(--color-primary)'
                      : 'var(--color-muted-foreground)',
                  borderRight: f !== 'missing' ? '1px solid var(--color-border)' : 'none',
                }}
              >
                {f}
              </button>
            ))}
          </div>

          {/* Level range filter */}
          <div className="flex items-center gap-1 text-xs" style={{ color: 'var(--color-muted)' }}>
            <span>Lvl</span>
            <input
              type="number"
              min={1}
              max={255}
              placeholder="min"
              value={levelFilter.min}
              onChange={(e) => setLevelFilter((f) => ({ ...f, min: e.target.value }))}
              className="rounded border px-1.5 py-0.5 outline-none bg-transparent"
              style={{ borderColor: 'var(--color-border)', color: 'var(--color-foreground)', width: '3.5rem' }}
            />
            <span>–</span>
            <input
              type="number"
              min={1}
              max={255}
              placeholder="max"
              value={levelFilter.max}
              onChange={(e) => setLevelFilter((f) => ({ ...f, max: e.target.value }))}
              className="rounded border px-1.5 py-0.5 outline-none bg-transparent"
              style={{ borderColor: 'var(--color-border)', color: 'var(--color-foreground)', width: '3.5rem' }}
            />
            {(levelFilter.min || levelFilter.max) && (
              <button onClick={() => setLevelFilter({ min: '', max: '' })} title="Clear level filter">
                <X size={11} style={{ color: 'var(--color-muted)' }} />
              </button>
            )}
          </div>

          <div className="flex-1" />

          {/* Stats */}
          {!loadingSpells && !spellError && (
            <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
              {spellbook
                ? `${knownCount} / ${spells.length} known`
                : `${spells.length} spells`}
            </span>
          )}

          {/* Refresh */}
          <button
            onClick={() => { loadSpells(classIndex); loadSpellbook() }}
            className="flex items-center gap-1 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
        </div>

        {/* Spellbook status */}
        {!loadingBook && (
          <div
            className="flex items-center gap-2 rounded px-3 py-1.5 text-xs"
            style={{
              backgroundColor: spellbook
                ? 'var(--color-surface)'
                : 'rgba(255,200,50,0.06)',
              border: `1px solid ${spellbook ? 'var(--color-border)' : 'rgba(255,200,50,0.3)'}`,
            }}
          >
            {spellbook ? (
              <>
                <CheckCircle2 size={12} style={{ color: 'var(--color-primary)' }} />
                <span style={{ color: 'var(--color-muted-foreground)' }}>
                  {spellbook.character}
                </span>
                <span style={{ color: 'var(--color-muted)' }}>
                  · exported {new Date(spellbook.exported_at).toLocaleString()}
                </span>
              </>
            ) : (
              <>
                <AlertCircle size={12} style={{ color: '#f59e0b' }} />
                <span style={{ color: '#f59e0b' }}>
                  No Zeal spellbook export found — known spells cannot be determined.
                </span>
                <button
                  onClick={() => navigate('/settings')}
                  className="ml-1 underline"
                  style={{ color: '#f59e0b' }}
                >
                  Check Settings
                </button>
              </>
            )}
          </div>
        )}
      </div>

      {/* ── Column headers ── */}
      <div
        className="shrink-0 flex items-center gap-3 px-4 py-1.5 text-[10px] font-semibold uppercase tracking-widest"
        style={{
          color: 'var(--color-muted)',
          borderBottom: '1px solid var(--color-border)',
          backgroundColor: 'var(--color-surface)',
        }}
      >
        <div className="w-4 shrink-0" />
        <div className="flex-1">Spell</div>
        <div className="shrink-0 w-12">Level</div>
        <div className="shrink-0 w-16 text-right">Mana</div>
        <div className="shrink-0 w-5" />
      </div>

      {/* ── Body ── */}
      <div className="flex-1 overflow-y-auto">
        {loading && (
          <div className="flex h-full items-center justify-center">
            <RefreshCw
              size={20}
              className="animate-spin"
              style={{ color: 'var(--color-muted)' }}
            />
          </div>
        )}

        {!loading && spellError && (
          <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
            <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
            <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>
              {spellError}
            </p>
            <button
              onClick={() => loadSpells(classIndex)}
              className="text-xs px-3 py-1.5 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              Retry
            </button>
          </div>
        )}

        {!loading && !spellError && filteredSpells.length === 0 && (
          <div className="flex h-full items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              {filter === 'known'
                ? 'No known spells for this class.'
                : filter === 'missing'
                  ? 'All spells known!'
                  : 'No spells found for this class.'}
            </p>
          </div>
        )}

        {!loading && !spellError && filteredSpells.map((spell) => (
          <SpellRow
            key={spell.id}
            spell={spell}
            classIndex={classIndex}
            known={knownIds.has(spell.id)}
            onSelect={handleSelectSpell}
          />
        ))}
      </div>

      {modalSpell && (
        <SpellDetailModal
          spell={modalSpell}
          onClose={() => setModalSpell(null)}
          onOpenInExplorer={handleOpenInExplorer}
        />
      )}
    </div>
  )
}
