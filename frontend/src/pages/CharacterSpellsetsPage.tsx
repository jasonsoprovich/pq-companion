import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Library,
  RefreshCw,
  Search,
  X,
  AlertTriangle,
} from 'lucide-react'
import {
  getAllSpellsets,
  getSpellsByClass,
  getZealSpellbook,
  listCharacters,
  type Character,
} from '../services/api'
import type { Spell } from '../types/spell'
import type { Spellset, SpellsetFile } from '../types/zeal'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { SpellIcon } from '../components/Icon'

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

const EMPTY_SLOT = -1

// ── Picker modal ───────────────────────────────────────────────────────────────

interface SlotPickerProps {
  classIndex: number
  characterLevel: number
  knownIds: Set<number>
  allSpells: Spell[]
  currentSpellId: number
  spellsetName: string
  slotIndex: number
  onPick: (spell: Spell) => void
  onClose: () => void
}

function SlotPicker({
  classIndex,
  characterLevel,
  knownIds,
  allSpells,
  currentSpellId,
  spellsetName,
  slotIndex,
  onPick,
  onClose,
}: SlotPickerProps): React.ReactElement {
  const [query, setQuery] = useState('')

  const eligible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return allSpells
      .filter((s) => {
        const reqLevel = s.class_levels?.[classIndex] ?? 255
        if (reqLevel >= 254) return false
        if (reqLevel > characterLevel) return false
        if (!knownIds.has(s.id)) return false
        if (q && !s.name.toLowerCase().includes(q)) return false
        return true
      })
      .sort((a, b) => {
        const al = a.class_levels[classIndex] ?? 255
        const bl = b.class_levels[classIndex] ?? 255
        if (al !== bl) return al - bl
        return a.name.localeCompare(b.name)
      })
  }, [allSpells, classIndex, characterLevel, knownIds, query])

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 520,
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        }}
      >
        <div
          className="flex items-center gap-2 px-3 py-2 border-b"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <Search size={14} style={{ color: 'var(--color-muted)' }} />
          <div className="flex-1 min-w-0">
            <div className="text-xs" style={{ color: 'var(--color-muted)' }}>
              {spellsetName} · Gem {slotIndex + 1}
            </div>
            <input
              type="text"
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Filter known spells…"
              className="w-full bg-transparent text-sm outline-none"
              style={{ color: 'var(--color-foreground)' }}
            />
          </div>
          <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }}>
            <X size={14} />
          </button>
        </div>

        <div style={{ flex: 1, overflowY: 'auto' }}>
          {eligible.length === 0 && (
            <p className="px-4 py-6 text-sm text-center" style={{ color: 'var(--color-muted)' }}>
              No eligible spells. Spells must be in the character's spellbook export
              and within their level cap for this class.
            </p>
          )}
          {eligible.map((s) => {
            const lvl = s.class_levels[classIndex] ?? 0
            const isCurrent = s.id === currentSpellId
            return (
              <button
                key={s.id}
                onClick={() => onPick(s)}
                disabled={isCurrent}
                className="flex w-full items-center gap-2.5 px-3 py-2 text-left transition-colors border-t hover:bg-(--color-surface-2) disabled:opacity-40"
                style={{ borderColor: 'var(--color-border)' }}
              >
                <SpellIcon id={s.new_icon} name={s.name} size={24} />
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium truncate" style={{ color: 'var(--color-foreground)' }}>
                    {s.name}
                  </div>
                  <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    Level {lvl}
                    {isCurrent && ' · current gem'}
                  </div>
                </div>
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}

// ── Confirm modal ──────────────────────────────────────────────────────────────

interface ConfirmModalProps {
  title: string
  message: React.ReactNode
  confirmLabel?: string
  onConfirm: () => void
  onCancel: () => void
}

function ConfirmModal({
  title,
  message,
  confirmLabel = 'Confirm',
  onConfirm,
  onCancel,
}: ConfirmModalProps): React.ReactElement {
  return (
    <div
      onClick={onCancel}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 440,
        }}
      >
        <div
          className="flex items-center gap-2 px-4 py-3 border-b"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <AlertTriangle size={16} style={{ color: 'var(--color-warning, #f59e0b)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {title}
          </span>
        </div>
        <div className="px-4 py-3 text-sm" style={{ color: 'var(--color-foreground)' }}>
          {message}
        </div>
        <div className="flex justify-end gap-2 px-4 py-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
          <button
            onClick={onCancel}
            className="px-3 py-1.5 text-sm rounded"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="px-3 py-1.5 text-sm font-medium rounded"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-primary-foreground, #fff)',
            }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Spellset card ──────────────────────────────────────────────────────────────

interface SpellsetCardProps {
  spellset: Spellset
  spellsById: Map<number, Spell>
  onSlotClick: (slotIndex: number) => void
}

function SpellsetCard({ spellset, spellsById, onSlotClick }: SpellsetCardProps): React.ReactElement {
  return (
    <div
      className="rounded-lg border"
      style={{
        backgroundColor: 'var(--color-surface)',
        borderColor: 'var(--color-border)',
      }}
    >
      <div
        className="px-3 py-2 border-b text-sm font-medium"
        style={{
          color: 'var(--color-foreground)',
          borderColor: 'var(--color-border)',
        }}
      >
        {spellset.name}
      </div>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-1 p-2">
        {spellset.spell_ids.map((id, idx) => {
          const spell = id > 0 ? spellsById.get(id) : null
          const isEmpty = id === EMPTY_SLOT || id <= 0
          return (
            <button
              key={idx}
              onClick={() => onSlotClick(idx)}
              className="flex items-center gap-2 px-2 py-1.5 rounded text-left transition-colors hover:bg-(--color-surface-2)"
              style={{
                border: '1px solid var(--color-border)',
              }}
            >
              <span
                className="text-[10px] font-mono shrink-0"
                style={{ color: 'var(--color-muted)', width: 14 }}
              >
                {idx + 1}
              </span>
              {!isEmpty && spell ? (
                <>
                  <SpellIcon id={spell.new_icon} name={spell.name} size={20} />
                  <span
                    className="text-xs truncate"
                    style={{ color: 'var(--color-foreground)' }}
                  >
                    {spell.name}
                  </span>
                </>
              ) : !isEmpty ? (
                <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                  Unknown (#{id})
                </span>
              ) : (
                <span className="text-xs italic" style={{ color: 'var(--color-muted)' }}>
                  Empty
                </span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

// ── Sub-tabs (filtered to chars with a spellset file) ──────────────────────────

interface FilteredSubTabsProps {
  value: string
  onChange: (next: string) => void
  characters: string[]
  active: string
}

function FilteredSubTabs({
  value,
  onChange,
  characters,
  active,
}: FilteredSubTabsProps): React.ReactElement {
  return (
    <div
      className="flex items-center gap-1 border-b px-4 shrink-0 overflow-x-auto"
      style={{
        borderColor: 'var(--color-border)',
        backgroundColor: 'var(--color-surface)',
      }}
    >
      {characters.map((name) => {
        const isActive = name === value
        const isLogged = name === active
        return (
          <button
            key={name}
            onClick={() => onChange(name)}
            className="px-3 py-2 text-xs font-medium transition-colors whitespace-nowrap"
            style={{
              color: isActive ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              borderBottom: isActive
                ? '2px solid var(--color-primary)'
                : '2px solid transparent',
            }}
            title={isLogged ? `${name} (active character)` : name}
          >
            {name}
            {isLogged && (
              <span
                className="ml-1 text-[9px] uppercase tracking-wider"
                style={{ color: isActive ? 'var(--color-primary)' : 'var(--color-muted)' }}
              >
                ●
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function CharacterSpellsetsPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [files, setFiles] = useState<SpellsetFile[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [viewed, setViewed] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [classSpells, setClassSpells] = useState<Spell[]>([])
  const [knownIds, setKnownIds] = useState<Set<number>>(new Set())
  const [referenced, setReferenced] = useState<Map<number, Spell>>(new Map())

  const [picker, setPicker] = useState<{ slot: number; setIndex: number } | null>(null)
  const [pendingPick, setPendingPick] = useState<{ slot: number; setIndex: number; spell: Spell } | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    Promise.all([getAllSpellsets(), listCharacters()])
      .then(([sets, chars]) => {
        setFiles(sets.characters ?? [])
        setCharacters(chars.characters ?? [])
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  // Pick initial viewed character: prefer active, else first with a file.
  useEffect(() => {
    if (files.length === 0) {
      if (viewed !== '') setViewed('')
      return
    }
    const names = files.map((f) => f.character)
    if (viewed && names.includes(viewed)) return
    const next = names.find((n) => n === active) ?? names[0]
    setViewed(next)
  }, [files, active, viewed])

  const viewedFile = files.find((f) => f.character === viewed) ?? null
  const viewedChar = characters.find((c) => c.name === viewed) ?? null
  const classIndex = viewedChar?.class ?? -1
  const characterLevel = viewedChar?.level ?? 0

  // Load the class spell catalog + the character's spellbook for filtering.
  useEffect(() => {
    if (!viewed || classIndex < 0) {
      setClassSpells([])
      setKnownIds(new Set())
      return
    }
    let cancelled = false
    getSpellsByClass(classIndex, 1000, 0)
      .then((res) => {
        if (!cancelled) setClassSpells(res.items ?? [])
      })
      .catch(() => { if (!cancelled) setClassSpells([]) })

    const isActive = active && viewed.toLowerCase() === active.toLowerCase()
    getZealSpellbook(isActive ? undefined : viewed)
      .then((res) => {
        if (cancelled) return
        setKnownIds(new Set(res.spellbook?.spell_ids ?? []))
      })
      .catch(() => { if (!cancelled) setKnownIds(new Set()) })

    return () => { cancelled = true }
  }, [viewed, classIndex, active])

  // Build name lookup from the class spell catalog. For any referenced spell
  // not in the catalog (e.g. an off-class scroll loaded into a gem — unusual
  // but possible), we don't fall back to a per-ID fetch here; the slot just
  // renders as "Unknown (#id)". Pull-by-id can be added if it becomes a problem.
  const spellsById = useMemo(() => {
    const m = new Map<number, Spell>()
    for (const s of classSpells) m.set(s.id, s)
    for (const [id, s] of referenced) if (!m.has(id)) m.set(id, s)
    return m
  }, [classSpells, referenced])

  // Backfill any referenced spell IDs that aren't in the class catalog by
  // fetching them once. Keeps the off-class display clean.
  useEffect(() => {
    if (!viewedFile) return
    const missing: number[] = []
    const seen = new Set<number>()
    for (const set of viewedFile.spellsets) {
      for (const id of set.spell_ids) {
        if (id <= 0) continue
        if (seen.has(id)) continue
        seen.add(id)
        if (!spellsById.has(id)) missing.push(id)
      }
    }
    if (missing.length === 0) return
    let cancelled = false
    // Lightweight per-ID fetch via the existing /api/spells/{id} endpoint.
    Promise.all(
      missing.map((id) =>
        fetch(`/api/spells/${id}`).then((r) => (r.ok ? r.json() : null)).catch(() => null),
      ),
    ).then((arr) => {
      if (cancelled) return
      setReferenced((prev) => {
        const next = new Map(prev)
        for (const s of arr) {
          if (s && typeof s.id === 'number') next.set(s.id, s as Spell)
        }
        return next
      })
    })
    return () => { cancelled = true }
  }, [viewedFile, spellsById])

  const filenames = files.map((f) => f.character)

  function handleSlotClick(setIndex: number, slot: number) {
    if (!viewedFile || classIndex < 0) return
    setPicker({ setIndex, slot })
  }

  function handlePick(spell: Spell) {
    if (!picker) return
    setPendingPick({ ...picker, spell })
    setPicker(null)
  }

  function handleConfirmReplace() {
    if (!pendingPick || !viewedFile) return
    // TODO(spellset-write): persist this change back to the .ini file once the
    // write path lands. For now this is a no-op so the UI flow is testable.
    // Optimistically update local view so the user sees the new gem.
    const { setIndex, slot, spell } = pendingPick
    setFiles((prev) =>
      prev.map((f) => {
        if (f.character !== viewedFile.character) return f
        const sets = f.spellsets.map((s, i) => {
          if (i !== setIndex) return s
          const ids = s.spell_ids.slice()
          ids[slot] = spell.id
          return { ...s, spell_ids: ids }
        })
        return { ...f, spellsets: sets }
      }),
    )
    setPendingPick(null)
  }

  const currentSpellId = picker && viewedFile
    ? viewedFile.spellsets[picker.setIndex]?.spell_ids[picker.slot] ?? -1
    : -1
  const pickerSetName = picker && viewedFile
    ? viewedFile.spellsets[picker.setIndex]?.name ?? ''
    : ''

  return (
    <div className="flex h-full flex-col" style={{ backgroundColor: 'var(--color-background)' }}>
      {filenames.length > 0 && (
        <FilteredSubTabs
          value={viewed}
          onChange={setViewed}
          characters={filenames}
          active={active}
        />
      )}

      <div
        className="shrink-0 flex items-center gap-3 border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Library size={18} style={{ color: 'var(--color-primary)' }} />
        <div className="flex-1 min-w-0">
          <div className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Spellsets
          </div>
          {viewedChar && (
            <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {viewedChar.name} · Level {viewedChar.level} {classIndex >= 0 ? CLASS_NAMES[classIndex] : ''}
            </div>
          )}
        </div>
        <button
          onClick={load}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Reload spellset files"
        >
          <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Refresh
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-4 py-4">
        {loading && (
          <div className="flex items-center justify-center py-12">
            <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {!loading && error && (
          <p className="text-sm text-center py-12" style={{ color: 'var(--color-danger, #ef4444)' }}>
            {error}
          </p>
        )}
        {!loading && !error && filenames.length === 0 && (
          <div className="text-sm text-center py-12" style={{ color: 'var(--color-muted)' }}>
            No spellset exports found. Camp out of EverQuest with Zeal enabled to
            generate <code>&lt;CharName&gt;_spellsets.ini</code> in your EQ folder.
          </div>
        )}
        {!loading && !error && viewedFile && (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 max-w-6xl">
            {viewedFile.spellsets.map((s, i) => (
              <SpellsetCard
                key={`${s.name}-${i}`}
                spellset={s}
                spellsById={spellsById}
                onSlotClick={(slot) => handleSlotClick(i, slot)}
              />
            ))}
          </div>
        )}
      </div>

      {picker && viewedFile && (
        <SlotPicker
          classIndex={classIndex}
          characterLevel={characterLevel}
          knownIds={knownIds}
          allSpells={classSpells}
          currentSpellId={currentSpellId}
          spellsetName={pickerSetName}
          slotIndex={picker.slot}
          onPick={handlePick}
          onClose={() => setPicker(null)}
        />
      )}

      {pendingPick && viewedFile && (
        <ConfirmModal
          title="Replace gem?"
          confirmLabel="Replace"
          onConfirm={handleConfirmReplace}
          onCancel={() => setPendingPick(null)}
          message={
            <div className="space-y-2">
              <p>
                Replace gem {pendingPick.slot + 1} of{' '}
                <span style={{ color: 'var(--color-foreground)' }}>
                  <strong>{viewedFile.spellsets[pendingPick.setIndex]?.name}</strong>
                </span>{' '}
                with <strong>{pendingPick.spell.name}</strong>?
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
                Writing back to <code>{viewedFile.character}_spellsets.ini</code> is not yet
                implemented — this preview updates the on-screen view only.
              </p>
            </div>
          }
        />
      )}
    </div>
  )
}
