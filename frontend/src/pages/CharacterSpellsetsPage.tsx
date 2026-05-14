import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Check,
  Library,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Search,
  Trash2,
  Undo2,
  X,
  AlertTriangle,
} from 'lucide-react'
import {
  getAllSpellsets,
  getSpell,
  getSpellsByClass,
  getZealSpellbook,
  listCharacters,
  updateSpellsets,
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
const SPELLSET_SLOT_COUNT = 8  // mirrors backend zeal.SpellsetSlotCount

function deepCloneFiles(files: SpellsetFile[]): SpellsetFile[] {
  return files.map((f) => ({
    ...f,
    spellsets: f.spellsets.map((s) => ({ ...s, spell_ids: s.spell_ids.slice() })),
  }))
}

function fileIsDirty(current: SpellsetFile | null, original: SpellsetFile | null): boolean {
  if (!current || !original) return false
  if (current.spellsets.length !== original.spellsets.length) return true
  for (let i = 0; i < current.spellsets.length; i++) {
    const a = current.spellsets[i]
    const b = original.spellsets[i]
    if (a.name !== b.name) return true
    if (a.spell_ids.length !== b.spell_ids.length) return true
    for (let j = 0; j < a.spell_ids.length; j++) {
      if (a.spell_ids[j] !== b.spell_ids[j]) return true
    }
  }
  return false
}

// ── Picker modal ───────────────────────────────────────────────────────────────

interface SlotPickerProps {
  classIndex: number  // -1 when character's class is unknown
  characterLevel: number  // 0 when unknown
  knownIds: Set<number>
  allSpells: Spell[]  // class-castable spells; empty when classIndex < 0
  referencedSpells: Map<number, Spell>  // off-class spells we've already resolved
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
  referencedSpells,
  currentSpellId,
  spellsetName,
  slotIndex,
  onPick,
  onClose,
}: SlotPickerProps): React.ReactElement {
  const [query, setQuery] = useState('')

  const eligible = useMemo(() => {
    const q = query.trim().toLowerCase()
    // When the character's class is unknown, fall back to listing whatever
    // spells we've already resolved that are in the spellbook — without class
    // info we can't enforce the level cap, so we just trust the spellbook list.
    if (classIndex < 0) {
      const seen = new Set<number>()
      const pool: Spell[] = []
      const consider = (s: Spell) => {
        if (seen.has(s.id)) return
        if (!knownIds.has(s.id)) return
        seen.add(s.id)
        pool.push(s)
      }
      for (const s of allSpells) consider(s)
      for (const s of referencedSpells.values()) consider(s)
      return pool
        .filter((s) => !q || s.name.toLowerCase().includes(q))
        .sort((a, b) => a.name.localeCompare(b.name))
    }
    return allSpells
      .filter((s) => {
        const reqLevel = s.class_levels?.[classIndex] ?? 255
        if (reqLevel >= 254) return false
        if (characterLevel > 0 && reqLevel > characterLevel) return false
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
  }, [allSpells, referencedSpells, classIndex, characterLevel, knownIds, query])

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
              {classIndex < 0
                ? 'No eligible spells. The character\'s class is unknown — log in once with Zeal so a Quarmy export is created, then return here.'
                : "No eligible spells. Spells must be in the character's spellbook export and within their level cap for this class."}
            </p>
          )}
          {eligible.map((s) => {
            const lvl = classIndex >= 0 ? (s.class_levels[classIndex] ?? 0) : 0
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
                    {lvl > 0 ? `Level ${lvl}` : 'Known'}
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
  siblingNames: string[]  // other spellset names in this file, for dup detection
  defaultEditing?: boolean
  onSlotClick: (slotIndex: number) => void
  onRename: (next: string) => void
  onRemove: () => void
}

function SpellsetCard({
  spellset,
  spellsById,
  siblingNames,
  defaultEditing = false,
  onSlotClick,
  onRename,
  onRemove,
}: SpellsetCardProps): React.ReactElement {
  const [editing, setEditing] = useState(defaultEditing)
  const [draft, setDraft] = useState(spellset.name)
  const inputRef = React.useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) {
      setDraft(spellset.name)
      // Focus after the input renders.
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.select()
      })
    }
  }, [editing, spellset.name])

  const trimmed = draft.trim()
  const isDuplicate = trimmed !== spellset.name && siblingNames.includes(trimmed)
  const hasBracket = trimmed.includes(']') || trimmed.includes('[')
  const isInvalid = trimmed === '' || isDuplicate || hasBracket
  const validationMsg = trimmed === ''
    ? 'Name required'
    : hasBracket
      ? 'Brackets not allowed'
      : isDuplicate
        ? 'Duplicate name'
        : ''

  function commit() {
    if (isInvalid) return
    if (trimmed !== spellset.name) onRename(trimmed)
    setEditing(false)
  }

  function cancel() {
    setDraft(spellset.name)
    setEditing(false)
  }

  return (
    <div
      className="rounded-lg border"
      style={{
        backgroundColor: 'var(--color-surface)',
        borderColor: 'var(--color-border)',
      }}
    >
      <div
        className="flex items-center gap-2 px-3 py-2 border-b"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {editing ? (
          <>
            <input
              ref={inputRef}
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') commit()
                else if (e.key === 'Escape') cancel()
              }}
              className="flex-1 min-w-0 bg-transparent text-sm font-medium outline-none border-b"
              style={{
                color: 'var(--color-foreground)',
                borderColor: isInvalid ? 'var(--color-danger, #ef4444)' : 'var(--color-primary)',
              }}
              maxLength={64}
            />
            <button
              onClick={commit}
              disabled={isInvalid}
              className="shrink-0 disabled:opacity-30"
              style={{ color: 'var(--color-primary)' }}
              title={isInvalid ? validationMsg : 'Save name'}
            >
              <Check size={14} />
            </button>
            <button
              onClick={cancel}
              className="shrink-0"
              style={{ color: 'var(--color-muted-foreground)' }}
              title="Cancel rename"
            >
              <X size={14} />
            </button>
          </>
        ) : (
          <>
            <div
              className="flex-1 min-w-0 text-sm font-medium truncate"
              style={{ color: 'var(--color-foreground)' }}
              title={spellset.name}
            >
              {spellset.name}
            </div>
            <button
              onClick={() => setEditing(true)}
              className="shrink-0 opacity-60 hover:opacity-100"
              style={{ color: 'var(--color-muted-foreground)' }}
              title="Rename spellset"
            >
              <Pencil size={12} />
            </button>
            <button
              onClick={onRemove}
              className="shrink-0 opacity-60 hover:opacity-100"
              style={{ color: 'var(--color-danger, #ef4444)' }}
              title="Delete spellset"
            >
              <Trash2 size={12} />
            </button>
          </>
        )}
      </div>
      <div className="flex flex-col gap-1 p-2">
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
                    className="text-xs"
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

// ── Add-tile ───────────────────────────────────────────────────────────────────

function AddSpellsetTile({ onClick }: { onClick: () => void }): React.ReactElement {
  return (
    <button
      onClick={onClick}
      className="rounded-lg flex items-center justify-center gap-2 text-sm"
      style={{
        border: '2px dashed var(--color-border)',
        backgroundColor: 'transparent',
        color: 'var(--color-muted-foreground)',
        minHeight: 80,
      }}
      title="Add a new spellset"
    >
      <Plus size={16} /> Add Spellset
    </button>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function CharacterSpellsetsPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [files, setFiles] = useState<SpellsetFile[]>([])
  const [originalFiles, setOriginalFiles] = useState<SpellsetFile[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [viewed, setViewed] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [classSpells, setClassSpells] = useState<Spell[]>([])
  const [knownIds, setKnownIds] = useState<Set<number>>(new Set())
  const [referenced, setReferenced] = useState<Map<number, Spell>>(new Map())

  const [picker, setPicker] = useState<{ slot: number; setIndex: number } | null>(null)
  const [pendingPick, setPendingPick] = useState<{ slot: number; setIndex: number; spell: Spell } | null>(null)
  const [confirmAction, setConfirmAction] = useState<
    | { type: 'save' }
    | { type: 'cancel' }
    | { type: 'remove'; setIndex: number; setName: string }
    | null
  >(null)
  const [justAddedIdx, setJustAddedIdx] = useState<number | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    Promise.all([getAllSpellsets(), listCharacters()])
      .then(([sets, chars]) => {
        const fresh = sets.characters ?? []
        setFiles(fresh)
        setOriginalFiles(deepCloneFiles(fresh))
        setCharacters(chars.characters ?? [])
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  // Clear the "just-added" flag on the next render after the card mounts in
  // edit mode — otherwise switching characters or saving would leave the flag
  // pointing at an index that may have been claimed by a different spellset.
  useEffect(() => {
    if (justAddedIdx === null) return
    const t = setTimeout(() => setJustAddedIdx(null), 0)
    return () => clearTimeout(t)
  }, [justAddedIdx])

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

  // Load the class spell catalog (when class is known) + the character's
  // spellbook export. The spellbook load is independent of class so we still
  // have something useful when class is unset (e.g. a character imported only
  // from a spellbook export, before any Quarmy import).
  useEffect(() => {
    if (!viewed) {
      setClassSpells([])
      setKnownIds(new Set())
      return
    }
    let cancelled = false

    if (classIndex >= 0) {
      getSpellsByClass(classIndex, 1000, 0)
        .then((res) => { if (!cancelled) setClassSpells(res.items ?? []) })
        .catch(() => { if (!cancelled) setClassSpells([]) })
    } else {
      setClassSpells([])
    }

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
    Promise.all(
      missing.map((id) => getSpell(id).catch(() => null)),
    ).then((arr) => {
      if (cancelled) return
      setReferenced((prev) => {
        const next = new Map(prev)
        for (const s of arr) {
          if (s && typeof s.id === 'number') next.set(s.id, s)
        }
        return next
      })
    })
    return () => { cancelled = true }
  }, [viewedFile, spellsById])

  const filenames = files.map((f) => f.character)

  function handleSlotClick(setIndex: number, slot: number) {
    if (!viewedFile) return
    setPicker({ setIndex, slot })
  }

  function handleRename(setIndex: number, nextName: string) {
    if (!viewedFile) return
    setFiles((prev) =>
      prev.map((f) => {
        if (f.character !== viewedFile.character) return f
        const sets = f.spellsets.map((s, i) =>
          i === setIndex ? { ...s, name: nextName } : s,
        )
        return { ...f, spellsets: sets }
      }),
    )
  }

  function handleAddSpellset() {
    if (!viewedFile) return
    const used = new Set(viewedFile.spellsets.map((s) => s.name))
    let name = 'New Spellset'
    let n = 1
    while (used.has(name)) {
      n++
      name = `New Spellset ${n}`
    }
    const newIdx = viewedFile.spellsets.length
    setFiles((prev) =>
      prev.map((f) => {
        if (f.character !== viewedFile.character) return f
        return {
          ...f,
          spellsets: [
            ...f.spellsets,
            { name, spell_ids: new Array(SPELLSET_SLOT_COUNT).fill(EMPTY_SLOT) },
          ],
        }
      }),
    )
    setJustAddedIdx(newIdx)
  }

  function handleRequestRemove(setIndex: number) {
    if (!viewedFile) return
    const target = viewedFile.spellsets[setIndex]
    if (!target) return
    setConfirmAction({ type: 'remove', setIndex, setName: target.name })
  }

  function handleConfirmRemove() {
    if (!confirmAction || confirmAction.type !== 'remove' || !viewedFile) return
    const { setIndex } = confirmAction
    setFiles((prev) =>
      prev.map((f) => {
        if (f.character !== viewedFile.character) return f
        return { ...f, spellsets: f.spellsets.filter((_, i) => i !== setIndex) }
      }),
    )
    setConfirmAction(null)
  }

  function handlePick(spell: Spell) {
    if (!picker) return
    setPendingPick({ ...picker, spell })
    setPicker(null)
  }

  function handleConfirmReplace() {
    if (!pendingPick || !viewedFile) return
    // Local edit only — gets persisted when the user clicks Save.
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

  function handleSave() {
    if (!viewedFile) return
    setSaving(true)
    setError(null)
    updateSpellsets(viewedFile.character, viewedFile.spellsets)
      .then((res) => {
        if (!res.spellsets) return
        const saved = res.spellsets
        setFiles((prev) => prev.map((f) => (f.character === saved.character ? saved : f)))
        setOriginalFiles((prev) => {
          const next = deepCloneFiles(prev)
          const idx = next.findIndex((f) => f.character === saved.character)
          if (idx >= 0) next[idx] = deepCloneFiles([saved])[0]
          else next.push(deepCloneFiles([saved])[0])
          return next
        })
      })
      .catch((err: Error) => setError(`Save failed: ${err.message}`))
      .finally(() => {
        setSaving(false)
        setConfirmAction(null)
      })
  }

  function handleCancel() {
    if (!viewedFile) return
    const original = originalFiles.find((f) => f.character === viewedFile.character)
    if (!original) {
      setConfirmAction(null)
      return
    }
    setFiles((prev) =>
      prev.map((f) => (f.character === viewedFile.character ? deepCloneFiles([original])[0] : f)),
    )
    setConfirmAction(null)
  }

  const currentSpellId = picker && viewedFile
    ? viewedFile.spellsets[picker.setIndex]?.spell_ids[picker.slot] ?? -1
    : -1
  const pickerSetName = picker && viewedFile
    ? viewedFile.spellsets[picker.setIndex]?.name ?? ''
    : ''

  const originalViewedFile = originalFiles.find((f) => f.character === viewed) ?? null
  const dirty = fileIsDirty(viewedFile, originalViewedFile)

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
        {dirty && (
          <span
            className="text-[11px] px-2 py-0.5 rounded"
            style={{
              backgroundColor: 'var(--color-warning, #f59e0b)',
              color: '#000',
            }}
            title="Unsaved changes to this character's spellsets"
          >
            Unsaved
          </span>
        )}
        <button
          onClick={() => setConfirmAction({ type: 'cancel' })}
          disabled={!dirty || saving}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded disabled:opacity-40"
          style={{
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
          title="Discard unsaved changes"
        >
          <Undo2 size={12} /> Cancel
        </button>
        <button
          onClick={() => setConfirmAction({ type: 'save' })}
          disabled={!dirty || saving}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded font-medium disabled:opacity-40"
          style={{
            backgroundColor: 'var(--color-primary)',
            color: 'var(--color-primary-foreground, #fff)',
          }}
          title="Write changes to the .ini file"
        >
          <Save size={12} className={saving ? 'animate-pulse' : ''} /> Save
        </button>
        <button
          onClick={load}
          disabled={saving}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded disabled:opacity-40"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Reload spellset files from disk"
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
          <div
            style={{
              display: 'grid',
              gap: 12,
              gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
            }}
          >
            {viewedFile.spellsets.map((s, i) => {
              const siblingNames = viewedFile.spellsets
                .filter((_, j) => j !== i)
                .map((other) => other.name)
              return (
                <SpellsetCard
                  key={i}
                  spellset={s}
                  spellsById={spellsById}
                  siblingNames={siblingNames}
                  defaultEditing={i === justAddedIdx}
                  onSlotClick={(slot) => handleSlotClick(i, slot)}
                  onRename={(next) => handleRename(i, next)}
                  onRemove={() => handleRequestRemove(i)}
                />
              )
            })}
            <AddSpellsetTile onClick={handleAddSpellset} />
          </div>
        )}
      </div>

      {picker && viewedFile && (
        <SlotPicker
          classIndex={classIndex}
          characterLevel={characterLevel}
          knownIds={knownIds}
          allSpells={classSpells}
          referencedSpells={referenced}
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
                The change stays local until you click <strong>Save</strong> — until then
                nothing is written to <code>{viewedFile.character}_spellsets.ini</code>.
              </p>
            </div>
          }
        />
      )}

      {confirmAction?.type === 'save' && viewedFile && (
        <ConfirmModal
          title="Save spellsets to disk?"
          confirmLabel="Save"
          onConfirm={handleSave}
          onCancel={() => setConfirmAction(null)}
          message={
            <div className="space-y-2">
              <p>
                Overwrite <code>{viewedFile.character}_spellsets.ini</code> with the current
                changes? This file controls the in-game spell-set dropdown, so {viewedFile.character} should
                be camped out of the game before saving.
              </p>
            </div>
          }
        />
      )}

      {confirmAction?.type === 'cancel' && viewedFile && (
        <ConfirmModal
          title="Discard changes?"
          confirmLabel="Discard"
          onConfirm={handleCancel}
          onCancel={() => setConfirmAction(null)}
          message={
            <p>
              Revert unsaved changes to <code>{viewedFile.character}</code>'s spellsets? The .ini
              file on disk is unaffected.
            </p>
          }
        />
      )}

      {confirmAction?.type === 'remove' && viewedFile && (
        <ConfirmModal
          title="Delete spellset?"
          confirmLabel="Delete"
          onConfirm={handleConfirmRemove}
          onCancel={() => setConfirmAction(null)}
          message={
            <div className="space-y-2">
              <p>
                Remove <strong>{confirmAction.setName}</strong> from {viewedFile.character}'s
                spellsets? The change stays local until you click Save.
              </p>
            </div>
          }
        />
      )}
    </div>
  )
}
