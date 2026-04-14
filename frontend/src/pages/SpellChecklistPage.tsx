import React, { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  BookOpen,
  CheckCircle2,
  Circle,
  ExternalLink,
  RefreshCw,
  AlertCircle,
} from 'lucide-react'
import { getSpellsByClass, getZealSpellbook } from '../services/api'
import type { Spell } from '../types/spell'
import type { Spellbook } from '../types/zeal'

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
  const [spells, setSpells] = useState<Spell[]>([])
  const [spellbook, setSpellbook] = useState<Spellbook | null>(null)
  const [loadingSpells, setLoadingSpells] = useState(true)
  const [loadingBook, setLoadingBook] = useState(true)
  const [spellError, setSpellError] = useState<string | null>(null)
  const navigate = useNavigate()

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
      .then((res) => setSpellbook(res.spellbook))
      .catch(() => setSpellbook(null))
      .finally(() => setLoadingBook(false))
  }, [])

  useEffect(() => { loadSpells(classIndex) }, [classIndex, loadSpells])
  useEffect(() => { loadSpellbook() }, [loadSpellbook])

  function handleClassChange(idx: number) {
    setClassIndex(idx)
    try { localStorage.setItem(LS_CLASS_KEY, String(idx)) } catch { /* ignore */ }
  }

  function handleSelectSpell(id: number) {
    navigate(`/spells?select=${id}`)
  }

  const knownIds = new Set(spellbook?.spell_ids ?? [])

  const filteredSpells = spells.filter((s) => {
    if (filter === 'known') return knownIds.has(s.id)
    if (filter === 'missing') return !knownIds.has(s.id)
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
        {/* Row 1: icon + title + class selector */}
        <div className="flex items-center gap-3">
          <BookOpen size={16} style={{ color: 'var(--color-primary)' }} />
          <span className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            Spell Checklist
          </span>

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

        {/* Row 2: filter tabs + stats */}
        <div className="flex items-center gap-3">
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
          <div className="flex h-32 items-center justify-center">
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
    </div>
  )
}
