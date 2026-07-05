import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import {
  Check,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Search,
  Swords,
  Trash2,
  Undo2,
  X,
} from 'lucide-react'
import {
  getAllBandoliers,
  getAllInventories,
  getBandolierSlotItems,
  getItem,
  listCharacters,
  updateBandolier,
  type Character,
} from '../services/api'
import type { BandolierFile, BandolierItem } from '../types/zeal'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { ConfirmModal } from '../components/ConfirmModal'
import { ItemIcon } from '../components/Icon'
import ItemHoverCard from '../components/ItemHoverCard'

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

// Slot order mirrors backend zeal.BandolierSlotCount: 0=Primary … 3=Ammo.
const SLOT_LABELS = ['Primary', 'Secondary', 'Range', 'Ammo']
const BANDOLIER_SLOT_COUNT = 4
const EMPTY = 0

// Lightweight item display info, keyed by item id, used to render the icon and
// name of items already referenced in a set (the slot picker supplies these for
// newly chosen items; existing ids are resolved via getItem on load).
type ItemInfo = { name: string; icon: number }

function deepCloneFiles(files: BandolierFile[]): BandolierFile[] {
  return files.map((f) => ({ ...f, sets: f.sets.map((s) => ({ ...s, item_ids: s.item_ids.slice() })) }))
}

function fileIsDirty(current: BandolierFile | null, original: BandolierFile | null): boolean {
  if (!current || !original) return false
  if (current.sets.length !== original.sets.length) return true
  for (let i = 0; i < current.sets.length; i++) {
    const a = current.sets[i]
    const b = original.sets[i]
    if (a.name !== b.name) return true
    if (a.item_ids.length !== b.item_ids.length) return true
    for (let j = 0; j < a.item_ids.length; j++) {
      if (a.item_ids[j] !== b.item_ids[j]) return true
    }
  }
  return false
}

// ── Slot picker modal ────────────────────────────────────────────────────────

interface SlotPickerProps {
  character: string
  slot: number
  currentItemId: number
  setName: string
  equip: { class?: number; race?: number; level?: number }
  onPick: (item: BandolierItem | null) => void // null = clear the slot
  onClose: () => void
}

function SlotPicker({
  character,
  slot,
  currentItemId,
  setName,
  equip,
  onPick,
  onClose,
}: SlotPickerProps): React.ReactElement {
  useEscapeToClose(onClose)
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<BandolierItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Debounce the search so each keystroke doesn't fire a request.
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    const t = setTimeout(() => {
      getBandolierSlotItems(character, slot, query.trim(), equip)
        .then((res) => {
          if (cancelled) return
          setItems(res.items ?? [])
          setError(null)
        })
        .catch((err: Error) => !cancelled && setError(err.message))
        .finally(() => !cancelled && setLoading(false))
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
  }, [character, slot, query])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.5)' }}
      onClick={onClose}
    >
      <div
        className="flex max-h-[80vh] w-full max-w-md flex-col rounded-lg border"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="flex items-center gap-2 border-b px-3 py-2"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <div className="flex-1 text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            {SLOT_LABELS[slot]} — <span style={{ color: 'var(--color-muted)' }}>{setName}</span>
          </div>
          <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }} title="Close">
            <X size={16} />
          </button>
        </div>

        <div className="px-3 py-2">
          <div
            className="flex items-center gap-2 rounded px-2 py-1.5"
            style={{ border: '1px solid var(--color-border)' }}
          >
            <Search size={14} style={{ color: 'var(--color-muted)' }} />
            <input
              autoFocus
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={`Search ${character}'s ${SLOT_LABELS[slot].toLowerCase()} items…`}
              className="flex-1 bg-transparent text-sm outline-none"
              style={{ color: 'var(--color-foreground)' }}
            />
          </div>
          <p className="mt-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
            Only items in {character}'s inventory that this character can equip in
            this slot (class, race, and level permitting) are shown.
          </p>
        </div>

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {/* Clear-slot option */}
          <button
            onClick={() => onPick(null)}
            className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-(--color-surface-2)"
            style={{ border: '1px solid var(--color-border)', marginBottom: 4 }}
          >
            <span
              className="flex items-center justify-center"
              style={{ width: 20, height: 20, color: 'var(--color-muted)' }}
            >
              <X size={14} />
            </span>
            <span className="text-xs italic" style={{ color: 'var(--color-muted)' }}>
              Empty (no item)
            </span>
            {currentItemId === EMPTY && (
              <Check size={14} className="ml-auto" style={{ color: 'var(--color-primary)' }} />
            )}
          </button>

          {loading && (
            <div className="flex items-center justify-center py-8">
              <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          )}
          {!loading && error && (
            <p className="py-8 text-center text-xs" style={{ color: 'var(--color-danger, #ef4444)' }}>
              {error}
            </p>
          )}
          {!loading && !error && items.length === 0 && (
            <p className="py-8 text-center text-xs" style={{ color: 'var(--color-muted)' }}>
              No matching {SLOT_LABELS[slot].toLowerCase()} items in inventory.
            </p>
          )}
          {!loading &&
            !error &&
            items.map((it) => (
              <ItemHoverCard key={it.id} itemId={it.id} clickHint="Click to select for this slot">
                <button
                  onClick={() => onPick(it)}
                  className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-(--color-surface-2)"
                >
                  <ItemIcon id={it.icon} name={it.name} size={20} />
                  <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                    {it.name}
                  </span>
                  {it.id === currentItemId && (
                    <Check size={14} className="ml-auto" style={{ color: 'var(--color-primary)' }} />
                  )}
                </button>
              </ItemHoverCard>
            ))}
        </div>
      </div>
    </div>
  )
}

// ── Set card ─────────────────────────────────────────────────────────────────

interface BandolierCardProps {
  set: { name: string; item_ids: number[] }
  itemsById: Map<number, ItemInfo>
  siblingNames: string[]
  defaultEditing?: boolean
  onSlotClick: (slot: number) => void
  onRename: (next: string) => void
  onRemove: () => void
}

function BandolierCard({
  set,
  itemsById,
  siblingNames,
  defaultEditing = false,
  onSlotClick,
  onRename,
  onRemove,
}: BandolierCardProps): React.ReactElement {
  const [editing, setEditing] = useState(defaultEditing)
  const [draft, setDraft] = useState(set.name)
  const inputRef = React.useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) {
      setDraft(set.name)
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.select()
      })
    }
  }, [editing, set.name])

  const trimmed = draft.trim()
  const isDuplicate = trimmed !== set.name && siblingNames.includes(trimmed)
  const hasBracket = trimmed.includes(']') || trimmed.includes('[')
  const tooLong = trimmed.length > 32
  const isInvalid = trimmed === '' || isDuplicate || hasBracket || tooLong
  const validationMsg = trimmed === ''
    ? 'Name required'
    : hasBracket
      ? 'Brackets not allowed'
      : tooLong
        ? 'Max 32 characters'
        : isDuplicate
          ? 'Duplicate name'
          : ''

  function commit(): void {
    if (isInvalid) return
    if (trimmed !== set.name) onRename(trimmed)
    setEditing(false)
  }

  function cancel(): void {
    setDraft(set.name)
    setEditing(false)
  }

  return (
    <div
      className="rounded-lg border"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
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
              className="min-w-0 flex-1 border-b bg-transparent text-sm font-medium outline-none"
              style={{
                color: 'var(--color-foreground)',
                borderColor: isInvalid ? 'var(--color-danger, #ef4444)' : 'var(--color-primary)',
              }}
              maxLength={32}
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
              className="min-w-0 flex-1 truncate text-sm font-medium"
              style={{ color: 'var(--color-foreground)' }}
              title={set.name}
            >
              {set.name}
            </div>
            <button
              onClick={() => setEditing(true)}
              className="shrink-0 opacity-60 hover:opacity-100"
              style={{ color: 'var(--color-muted-foreground)' }}
              title="Rename set"
            >
              <Pencil size={12} />
            </button>
            <button
              onClick={onRemove}
              className="shrink-0 opacity-60 hover:opacity-100"
              style={{ color: 'var(--color-danger, #ef4444)' }}
              title="Delete set"
            >
              <Trash2 size={12} />
            </button>
          </>
        )}
      </div>
      <div className="flex flex-col gap-1 p-2">
        {set.item_ids.map((id, idx) => {
          const info = id > 0 ? itemsById.get(id) : undefined
          const isEmpty = id <= 0
          const button = (
            <button
              onClick={() => onSlotClick(idx)}
              className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left transition-colors hover:bg-(--color-surface-2)"
              style={{ border: '1px solid var(--color-border)' }}
            >
              <span
                className="shrink-0 text-[10px] font-medium uppercase tracking-wide"
                style={{ color: 'var(--color-muted)', width: 64 }}
              >
                {SLOT_LABELS[idx]}
              </span>
              {!isEmpty && info ? (
                <>
                  <ItemIcon id={info.icon} name={info.name} size={20} />
                  <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                    {info.name}
                  </span>
                </>
              ) : !isEmpty ? (
                <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                  Item #{id}
                </span>
              ) : (
                <span className="text-xs italic" style={{ color: 'var(--color-muted)' }}>
                  Empty
                </span>
              )}
            </button>
          )
          return isEmpty ? (
            <div key={idx}>{button}</div>
          ) : (
            <ItemHoverCard key={idx} itemId={id} clickHint="Click to change this slot">
              {button}
            </ItemHoverCard>
          )
        })}
      </div>
    </div>
  )
}

function AddSetTile({ onClick }: { onClick: () => void }): React.ReactElement {
  return (
    <button
      onClick={onClick}
      className="flex items-center justify-center gap-2 rounded-lg text-sm"
      style={{
        border: '2px dashed var(--color-border)',
        backgroundColor: 'transparent',
        color: 'var(--color-muted-foreground)',
        minHeight: 80,
      }}
      title="Add a new bandolier set"
    >
      <Plus size={16} /> Add Set
    </button>
  )
}

// ── Character sub-tabs ───────────────────────────────────────────────────────

function CharacterTabs({
  value,
  onChange,
  characters,
  active,
}: {
  value: string
  onChange: (next: string) => void
  characters: string[]
  active: string
}): React.ReactElement {
  return (
    <div
      className="flex shrink-0 items-center gap-1 overflow-x-auto border-b px-4"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      {characters.map((name) => {
        const isActive = name === value
        const isLogged = name === active
        return (
          <button
            key={name}
            onClick={() => onChange(name)}
            className="whitespace-nowrap px-3 py-2 text-xs font-medium transition-colors"
            style={{
              color: isActive ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              borderBottom: isActive ? '2px solid var(--color-primary)' : '2px solid transparent',
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

// ── Main page ────────────────────────────────────────────────────────────────

export default function CharacterBandolierPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [files, setFiles] = useState<BandolierFile[]>([])
  const [originalFiles, setOriginalFiles] = useState<BandolierFile[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [itemsById, setItemsById] = useState<Map<number, ItemInfo>>(new Map())
  const [invExportedAt, setInvExportedAt] = useState<Map<string, string>>(new Map())
  const [viewed, setViewed] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [picker, setPicker] = useState<{ setIndex: number; slot: number } | null>(null)
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
    Promise.all([getAllBandoliers(), listCharacters(), getAllInventories()])
      .then(([bands, chars, invs]) => {
        const fresh = bands.characters ?? []
        setFiles(fresh)
        setOriginalFiles(deepCloneFiles(fresh))
        setCharacters(chars.characters ?? [])
        const stamps = new Map<string, string>()
        for (const inv of invs.characters ?? []) stamps.set(inv.character, inv.exported_at)
        setInvExportedAt(stamps)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  // Resolve display info (name/icon) for every item id referenced across all
  // bandolier files, so existing sets render with icons. Best-effort per id.
  useEffect(() => {
    const needed = new Set<number>()
    for (const f of files) {
      for (const s of f.sets) {
        for (const id of s.item_ids) {
          if (id > 0 && !itemsById.has(id)) needed.add(id)
        }
      }
    }
    if (needed.size === 0) return
    let cancelled = false
    Promise.all(
      Array.from(needed).map((id) =>
        getItem(id)
          .then((it) => ({ id, info: { name: it.name, icon: it.icon } as ItemInfo }))
          .catch(() => null),
      ),
    ).then((results) => {
      if (cancelled) return
      setItemsById((prev) => {
        const next = new Map(prev)
        for (const r of results) if (r) next.set(r.id, r.info)
        return next
      })
    })
    return () => {
      cancelled = true
    }
  }, [files, itemsById])

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
    setViewed(names.find((n) => n === active) ?? names[0])
  }, [files, active, viewed])

  const viewedFile = files.find((f) => f.character === viewed) ?? null
  const viewedChar = characters.find((c) => c.name === viewed) ?? null
  const classIndex = viewedChar?.class ?? -1
  const filenames = useMemo(() => files.map((f) => f.character), [files])

  const originalViewedFile = originalFiles.find((f) => f.character === viewed) ?? null
  const dirty = fileIsDirty(viewedFile, originalViewedFile)

  // mutateViewed applies a change to the viewed character's sets immutably.
  const mutateViewed = useCallback(
    (fn: (sets: { name: string; item_ids: number[] }[]) => { name: string; item_ids: number[] }[]) => {
      setFiles((prev) =>
        prev.map((f) => (f.character === viewed ? { ...f, sets: fn(f.sets.map((s) => ({ ...s, item_ids: s.item_ids.slice() }))) } : f)),
      )
    },
    [viewed],
  )

  function handleSlotClick(setIndex: number, slot: number): void {
    setPicker({ setIndex, slot })
  }

  function handlePick(item: BandolierItem | null): void {
    if (!picker) return
    const { setIndex, slot } = picker
    if (item) {
      setItemsById((prev) => {
        const next = new Map(prev)
        next.set(item.id, { name: item.name, icon: item.icon })
        return next
      })
    }
    mutateViewed((sets) => {
      if (sets[setIndex]) sets[setIndex].item_ids[slot] = item ? item.id : EMPTY
      return sets
    })
    setPicker(null)
  }

  function handleRename(setIndex: number, next: string): void {
    mutateViewed((sets) => {
      if (sets[setIndex]) sets[setIndex].name = next
      return sets
    })
  }

  function handleAddSet(): void {
    if (!viewedFile) return
    const existing = new Set(viewedFile.sets.map((s) => s.name))
    let n = viewedFile.sets.length + 1
    let name = `Set ${n}`
    while (existing.has(name)) name = `Set ${++n}`
    mutateViewed((sets) => {
      sets.push({ name, item_ids: new Array(BANDOLIER_SLOT_COUNT).fill(EMPTY) })
      return sets
    })
    setJustAddedIdx(viewedFile.sets.length)
  }

  function handleConfirmRemove(): void {
    if (confirmAction?.type !== 'remove') return
    const idx = confirmAction.setIndex
    mutateViewed((sets) => sets.filter((_, i) => i !== idx))
    setConfirmAction(null)
  }

  function handleSave(): void {
    if (!viewedFile) return
    setSaving(true)
    setError(null)
    updateBandolier(viewedFile.character, viewedFile.sets)
      .then((res) => {
        if (!res.bandolier) return
        const saved = res.bandolier
        setFiles((prev) => prev.map((f) => (f.character === saved.character ? saved : f)))
        setOriginalFiles((prev) => {
          const without = prev.filter((f) => f.character !== saved.character)
          return deepCloneFiles([...without, saved])
        })
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => {
        setSaving(false)
        setConfirmAction(null)
      })
  }

  function handleCancel(): void {
    if (!originalViewedFile) return
    const reverted = deepCloneFiles([originalViewedFile])[0]
    setFiles((prev) => prev.map((f) => (f.character === viewed ? reverted : f)))
    setConfirmAction(null)
  }

  const pickerSet = picker && viewedFile ? viewedFile.sets[picker.setIndex] : null
  const invStamp = invExportedAt.get(viewed)

  return (
    <div className="flex h-full flex-col" style={{ backgroundColor: 'var(--color-background)' }}>
      {filenames.length > 0 && (
        <CharacterTabs value={viewed} onChange={setViewed} characters={filenames} active={active} />
      )}

      <div
        className="flex shrink-0 items-center gap-3 border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Swords size={18} style={{ color: 'var(--color-primary)' }} />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Bandolier
          </div>
          {viewedChar && (
            <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {viewedChar.name} · Level {viewedChar.level}{' '}
              {classIndex >= 0 ? CLASS_NAMES[classIndex] : ''}
            </div>
          )}
        </div>
        {dirty && (
          <span
            className="rounded px-2 py-0.5 text-[11px]"
            style={{ backgroundColor: 'var(--color-warning, #f59e0b)', color: '#000' }}
            title="Unsaved changes to this character's bandolier"
          >
            Unsaved
          </span>
        )}
        <button
          onClick={() => setConfirmAction({ type: 'cancel' })}
          disabled={!dirty || saving}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs disabled:opacity-40"
          style={{ color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          title="Discard unsaved changes"
        >
          <Undo2 size={12} /> Cancel
        </button>
        <button
          onClick={() => setConfirmAction({ type: 'save' })}
          disabled={!dirty || saving}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium disabled:opacity-40"
          style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground, #fff)' }}
          title="Write changes to the .ini file"
        >
          <Save size={12} className={saving ? 'animate-pulse' : ''} /> Save
        </button>
        <button
          onClick={load}
          disabled={saving}
          className="flex items-center gap-1 rounded px-2 py-1 text-xs disabled:opacity-40"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Reload bandolier files from disk"
        >
          <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Refresh
        </button>
      </div>

      {viewedFile && invStamp && (
        <div
          className="shrink-0 border-b px-4 py-1.5 text-[11px]"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
        >
          Item choices reflect {viewed}'s inventory as of{' '}
          {new Date(invStamp).toLocaleString()}. Re-export from Zeal (camp or{' '}
          <code>/outputfile inventory</code>) if it's stale.
        </div>
      )}

      <div className="flex-1 overflow-y-auto px-4 py-4">
        {loading && (
          <div className="flex items-center justify-center py-12">
            <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {!loading && error && (
          <p className="py-12 text-center text-sm" style={{ color: 'var(--color-danger, #ef4444)' }}>
            {error}
          </p>
        )}
        {!loading && !error && filenames.length === 0 && (
          <div className="py-12 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
            No bandolier files found. In-game with Zeal, use{' '}
            <code>/bandolier save &lt;name&gt;</code> to create{' '}
            <code>&lt;CharName&gt;_bandolier.ini</code> in your EQ folder.
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
            {viewedFile.sets.map((s, i) => {
              const siblingNames = viewedFile.sets.filter((_, j) => j !== i).map((o) => o.name)
              return (
                <BandolierCard
                  key={s.name}
                  set={s}
                  itemsById={itemsById}
                  siblingNames={siblingNames}
                  defaultEditing={i === justAddedIdx}
                  onSlotClick={(slot) => handleSlotClick(i, slot)}
                  onRename={(next) => handleRename(i, next)}
                  onRemove={() => setConfirmAction({ type: 'remove', setIndex: i, setName: s.name })}
                />
              )
            })}
            <AddSetTile onClick={handleAddSet} />
          </div>
        )}
      </div>

      {picker && viewedFile && pickerSet && (
        <SlotPicker
          character={viewedFile.character}
          slot={picker.slot}
          currentItemId={pickerSet.item_ids[picker.slot] ?? EMPTY}
          setName={pickerSet.name}
          equip={{
            class: viewedChar?.class,
            race: viewedChar?.race,
            level: viewedChar?.level,
          }}
          onPick={handlePick}
          onClose={() => setPicker(null)}
        />
      )}

      {confirmAction?.type === 'save' && viewedFile && (
        <ConfirmModal
          title="Save bandolier to disk?"
          confirmLabel="Save"
          onConfirm={handleSave}
          message={
            <p>
              Overwrite <code>{viewedFile.character}_bandolier.ini</code> with the current
              changes? Zeal reads this file for the in-game <code>/bandolier</code> sets, so{' '}
              {viewedFile.character} should be camped out of the game before saving.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}

      {confirmAction?.type === 'cancel' && viewedFile && (
        <ConfirmModal
          title="Discard changes?"
          confirmLabel="Discard"
          tone="danger"
          onConfirm={handleCancel}
          message={
            <p>
              Revert unsaved changes to <code>{viewedFile.character}</code>'s bandolier? The
              .ini file on disk is unaffected.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}

      {confirmAction?.type === 'remove' && viewedFile && (
        <ConfirmModal
          title="Delete set?"
          confirmLabel="Delete"
          tone="danger"
          onConfirm={handleConfirmRemove}
          message={
            <p>
              Remove <strong>{confirmAction.setName}</strong> from {viewedFile.character}'s
              bandolier? The change stays local until you click Save.
            </p>
          }
          onCancel={() => setConfirmAction(null)}
        />
      )}
    </div>
  )
}
