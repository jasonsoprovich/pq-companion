import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Star, Plus, RefreshCw, GripVertical, Trash2, AlertCircle } from 'lucide-react'
import {
  listCharacters,
  listWishlist,
  addWishlistEntries,
  deleteWishlistEntry,
  reorderWishlistSlot,
  getItem,
  getItemSources,
  type Character,
} from '../services/api'
import type { Item, ItemSources } from '../types/item'
import type { WishlistEntry } from '../types/wishlist'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import CharacterSubTabs from '../components/CharacterSubTabs'
import ItemSearchModal from '../components/ItemSearchModal'
import WishlistSlotPicker from '../components/WishlistSlotPicker'
import ItemDetailModal from '../components/ItemDetailModal'
import { ConfirmModal } from '../components/ConfirmModal'
import { ItemIcon } from '../components/Icon'
import { WISHLIST_SLOT_ORDER, GENERAL_BUCKET, validSlotsForItem, isMultiSlotItem } from '../lib/wishlistSlots'

// ── Source line ───────────────────────────────────────────────────────────────

interface SourceLineProps {
  sources: ItemSources | null
}

/** Picks the most informative single source line for a wishlist row. */
function SourceLine({ sources }: SourceLineProps): React.ReactElement {
  const navigate = useNavigate()
  if (!sources) {
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Loading source…
      </span>
    )
  }
  // Go's nil slices serialise to JSON null, so each source list might come back
  // as null rather than []. Normalise once at the top.
  const drops = sources.drops ?? []
  const merchants = sources.merchants ?? []
  const forageZones = sources.forage_zones ?? []
  const groundSpawns = sources.ground_spawns ?? []
  const tradeskills = sources.tradeskills ?? []
  if (drops.length > 0) {
    const npc = drops[0]
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Drops from{' '}
        <button
          onClick={() => navigate(`/npcs?select=${npc.id}`)}
          className="underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          {npc.name.replace(/_/g, ' ')}
        </button>
        {npc.zone_name && (
          <>
            {' in '}
            <button
              onClick={() => navigate(`/zones?select=${npc.zone_short_name}`)}
              className="underline decoration-dotted"
              style={{ color: 'var(--color-primary)' }}
            >
              {npc.zone_name}
            </button>
          </>
        )}
        {drops.length > 1 && (
          <span style={{ color: 'var(--color-muted)' }}>{` + ${drops.length - 1} more`}</span>
        )}
      </span>
    )
  }
  if (merchants.length > 0) {
    const m = merchants[0]
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Sold by{' '}
        <button
          onClick={() => navigate(`/npcs?select=${m.id}`)}
          className="underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          {m.name.replace(/_/g, ' ')}
        </button>
        {m.zone_name && (
          <>
            {' in '}
            <button
              onClick={() => navigate(`/zones?select=${m.zone_short_name}`)}
              className="underline decoration-dotted"
              style={{ color: 'var(--color-primary)' }}
            >
              {m.zone_name}
            </button>
          </>
        )}
      </span>
    )
  }
  if (forageZones.length > 0) {
    const z = forageZones[0]
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Foraged in{' '}
        <button
          onClick={() => navigate(`/zones?select=${z.zone_short_name}`)}
          className="underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          {z.zone_name || z.zone_short_name}
        </button>
      </span>
    )
  }
  if (groundSpawns.length > 0) {
    const g = groundSpawns[0]
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Ground spawn in{' '}
        <button
          onClick={() => navigate(`/zones?select=${g.zone_short_name}`)}
          className="underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          {g.zone_name || g.zone_short_name}
        </button>
      </span>
    )
  }
  if (tradeskills.length > 0) {
    const t = tradeskills[0]
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        {t.role === 'product' ? 'Combined via ' : 'Used in '}
        <span style={{ color: 'var(--color-foreground)' }}>{t.recipe_name}</span>
      </span>
    )
  }
  return (
    <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
      No known source.
    </span>
  )
}

// ── Wishlist row ──────────────────────────────────────────────────────────────

interface WishlistRowProps {
  entry: WishlistEntry
  sources: ItemSources | null
  onOpenItem: (item: { id: number; name: string; icon: number }) => void
  onDelete: () => void
  // Drag handlers
  isDraggedOver: boolean
  onDragStart: (e: React.DragEvent) => void
  onDragOver: (e: React.DragEvent) => void
  onDragLeave: () => void
  onDrop: (e: React.DragEvent) => void
  onDragEnd: () => void
}

function WishlistRow({
  entry,
  sources,
  onOpenItem,
  onDelete,
  isDraggedOver,
  onDragStart,
  onDragOver,
  onDragLeave,
  onDrop,
  onDragEnd,
}: WishlistRowProps): React.ReactElement {
  const item = entry.item
  return (
    <div
      draggable
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      onDragEnd={onDragEnd}
      className="flex items-center gap-2 rounded px-2 py-2"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${isDraggedOver ? 'var(--color-primary)' : 'var(--color-border)'}`,
      }}
    >
      <span
        className="cursor-grab"
        style={{ color: 'var(--color-muted)' }}
        title="Drag to reorder"
      >
        <GripVertical size={14} />
      </span>
      {item && <ItemIcon id={item.icon} name={item.name} size={28} />}
      <div className="min-w-0 flex-1">
        <button
          onClick={() => item && onOpenItem({ id: item.id, name: item.name, icon: item.icon })}
          className="block max-w-full truncate text-left text-sm underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
          title={item?.name}
        >
          {item?.name ?? `Item #${entry.item_id}`}
        </button>
        <div className="truncate">
          <SourceLine sources={sources} />
        </div>
      </div>
      <button
        onClick={onDelete}
        className="shrink-0 rounded p-1"
        style={{ color: 'var(--color-muted)' }}
        title="Remove from wishlist"
      >
        <Trash2 size={14} />
      </button>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function WishlistPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewedCharacter, setViewedCharacter] = useState('')
  const [characters, setCharacters] = useState<Character[]>([])
  const [entries, setEntries] = useState<WishlistEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [sourcesCache, setSourcesCache] = useState<Map<number, ItemSources>>(new Map())

  // Item picker state
  const [searchOpen, setSearchOpen] = useState(false)
  const [picker, setPicker] = useState<{ item: Item; currentSlots: string[] } | null>(null)
  const [detailItem, setDetailItem] = useState<Item | null>(null)
  // Entry pending removal, waiting on user confirmation.
  const [pendingDelete, setPendingDelete] = useState<WishlistEntry | null>(null)

  // Drag state
  const dragSrc = useRef<{ id: number; slot: string } | null>(null)
  const [dragOverID, setDragOverID] = useState<number | null>(null)

  // Look up the active character's id from name.
  useEffect(() => {
    listCharacters().then((r) => setCharacters(r.characters)).catch(() => setCharacters([]))
  }, [])
  const viewedCharID = useMemo(() => {
    return characters.find((c) => c.name.toLowerCase() === viewedCharacter.toLowerCase())?.id ?? 0
  }, [characters, viewedCharacter])

  // Default the viewed character to the active one.
  useEffect(() => {
    if (!viewedCharacter && active) setViewedCharacter(active)
  }, [active, viewedCharacter])

  const load = useCallback(() => {
    if (!viewedCharID) {
      setEntries([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    listWishlist(viewedCharID)
      .then((r) => {
        setEntries(r.entries)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [viewedCharID])

  useEffect(() => {
    load()
  }, [load])

  // Lazy-load sources for each unique item id once.
  useEffect(() => {
    const ids = new Set(entries.map((e) => e.item_id))
    for (const id of ids) {
      if (sourcesCache.has(id)) continue
      getItemSources(id)
        .then((s) => setSourcesCache((prev) => new Map(prev).set(id, s)))
        .catch(() => undefined)
    }
  }, [entries, sourcesCache])

  // Group entries by bucket and sort by sort_order within each.
  const grouped = useMemo(() => {
    const m = new Map<string, WishlistEntry[]>()
    for (const e of entries) {
      const list = m.get(e.slot_bucket) ?? []
      list.push(e)
      m.set(e.slot_bucket, list)
    }
    for (const list of m.values()) list.sort((a, b) => a.sort_order - b.sort_order || a.id - b.id)
    return m
  }, [entries])

  // ── Add / remove handlers ───────────────────────────────────────────────────

  function handleSearchSelect(item: Item) {
    setSearchOpen(false)
    if (!viewedCharID) return
    if (isMultiSlotItem(item.slots)) {
      // Open the slot picker, prefilled with whatever buckets are already
      // starred for this item.
      const currentSlots = entries
        .filter((e) => e.item_id === item.id)
        .map((e) => e.slot_bucket)
      setPicker({ item, currentSlots })
      return
    }
    // Single slot (or non-equippable → General) — add directly, no prompt.
    const slots = validSlotsForItem(item.slots)
    addWishlistEntries(viewedCharID, item.id, slots)
      .then(() => load())
      .catch((err: Error) => setError(err.message))
  }

  function handlePickerConfirm(selected: string[]) {
    if (!picker || !viewedCharID) return
    const { item, currentSlots } = picker
    const toAdd = selected.filter((s) => !currentSlots.includes(s))
    const toRemove = currentSlots.filter((s) => !selected.includes(s))

    const tasks: Promise<unknown>[] = []
    if (toAdd.length > 0) tasks.push(addWishlistEntries(viewedCharID, item.id, toAdd))
    for (const slot of toRemove) {
      const entry = entries.find((e) => e.item_id === item.id && e.slot_bucket === slot)
      if (entry) tasks.push(deleteWishlistEntry(viewedCharID, entry.id))
    }
    setPicker(null)
    Promise.all(tasks)
      .then(() => load())
      .catch((err: Error) => setError(err.message))
  }

  function confirmDelete() {
    if (!viewedCharID || !pendingDelete) return
    const id = pendingDelete.id
    setPendingDelete(null)
    deleteWishlistEntry(viewedCharID, id)
      .then(() => load())
      .catch((err: Error) => setError(err.message))
  }

  function handleOpenItem(brief: { id: number }) {
    getItem(brief.id).then(setDetailItem).catch(() => undefined)
  }

  // ── Drag/drop reorder ───────────────────────────────────────────────────────

  function onRowDragStart(entry: WishlistEntry) {
    return (e: React.DragEvent) => {
      dragSrc.current = { id: entry.id, slot: entry.slot_bucket }
      e.dataTransfer.effectAllowed = 'move'
      // Required for Firefox to initiate drag.
      e.dataTransfer.setData('text/plain', String(entry.id))
    }
  }
  function onRowDragOver(entry: WishlistEntry) {
    return (e: React.DragEvent) => {
      if (!dragSrc.current || dragSrc.current.slot !== entry.slot_bucket) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'move'
      setDragOverID(entry.id)
    }
  }
  function onRowDragLeave() {
    setDragOverID(null)
  }
  function onRowDrop(targetEntry: WishlistEntry) {
    return (e: React.DragEvent) => {
      e.preventDefault()
      const src = dragSrc.current
      setDragOverID(null)
      dragSrc.current = null
      if (!src || src.slot !== targetEntry.slot_bucket || src.id === targetEntry.id) return
      if (!viewedCharID) return

      const list = grouped.get(targetEntry.slot_bucket) ?? []
      const ids = list.map((e) => e.id)
      const from = ids.indexOf(src.id)
      const to = ids.indexOf(targetEntry.id)
      if (from === -1 || to === -1) return
      ids.splice(to, 0, ids.splice(from, 1)[0])

      // Optimistic local reorder.
      setEntries((prev) =>
        prev.map((e) => {
          if (e.slot_bucket !== targetEntry.slot_bucket) return e
          const idx = ids.indexOf(e.id)
          return idx === -1 ? e : { ...e, sort_order: idx }
        }),
      )
      reorderWishlistSlot(viewedCharID, targetEntry.slot_bucket, ids).catch((err: Error) => {
        setError(err.message)
        load()
      })
    }
  }
  function onRowDragEnd() {
    dragSrc.current = null
    setDragOverID(null)
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  const sectionsToRender = WISHLIST_SLOT_ORDER.filter((s) => (grouped.get(s)?.length ?? 0) > 0)

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Star size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Wishlist
        </span>
        <button
          onClick={() => setSearchOpen(true)}
          disabled={!viewedCharID}
          className="ml-auto flex items-center gap-1.5 text-xs px-2.5 py-1 rounded disabled:opacity-50"
          style={{
            backgroundColor: 'var(--color-primary)',
            color: 'var(--color-surface)',
            border: '1px solid var(--color-primary)',
          }}
          title={viewedCharID ? 'Add item to wishlist' : 'Select a character first'}
        >
          <Plus size={12} />
          Add item
        </button>
        <button
          onClick={load}
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
          title="Refresh"
        >
          <RefreshCw size={11} />
        </button>
      </div>

      <CharacterSubTabs value={viewedCharacter} onChange={setViewedCharacter} />

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {loading && (
          <div className="flex h-32 items-center justify-center">
            <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {error && (
          <div
            className="flex items-center gap-2 rounded px-3 py-2 text-xs"
            style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-danger)', color: 'var(--color-danger)' }}
          >
            <AlertCircle size={14} />
            <span>{error}</span>
          </div>
        )}
        {!loading && !viewedCharID && (
          <div className="flex h-32 items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              Select a character to view their wishlist.
            </p>
          </div>
        )}
        {!loading && viewedCharID && sectionsToRender.length === 0 && (
          <div className="flex h-32 items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              No items wishlisted yet. Click "Add item" or star an item on its detail page.
            </p>
          </div>
        )}
        {sectionsToRender.map((bucket) => {
          const list = grouped.get(bucket) ?? []
          return (
            <div key={bucket}>
              <div
                className="mb-1.5 flex items-baseline justify-between text-[10px] font-semibold uppercase tracking-widest"
                style={{ color: 'var(--color-muted)' }}
              >
                <span>{bucket === GENERAL_BUCKET ? 'General' : bucket}</span>
                <span style={{ color: 'var(--color-muted)' }}>{list.length}</span>
              </div>
              <div className="space-y-1.5">
                {list.map((entry) => (
                  <WishlistRow
                    key={entry.id}
                    entry={entry}
                    sources={sourcesCache.get(entry.item_id) ?? null}
                    onOpenItem={handleOpenItem}
                    onDelete={() => setPendingDelete(entry)}
                    isDraggedOver={dragOverID === entry.id}
                    onDragStart={onRowDragStart(entry)}
                    onDragOver={onRowDragOver(entry)}
                    onDragLeave={onRowDragLeave}
                    onDrop={onRowDrop(entry)}
                    onDragEnd={onRowDragEnd}
                  />
                ))}
              </div>
            </div>
          )
        })}
      </div>

      {/* Modals */}
      <ItemSearchModal
        open={searchOpen}
        title="Add item to wishlist"
        onSelect={handleSearchSelect}
        onClose={() => setSearchOpen(false)}
      />
      {picker && (
        <WishlistSlotPicker
          open
          itemName={picker.item.name}
          itemSlots={picker.item.slots}
          currentSlots={picker.currentSlots}
          onConfirm={handlePickerConfirm}
          onClose={() => setPicker(null)}
        />
      )}
      <ItemDetailModal item={detailItem} open={!!detailItem} onClose={() => setDetailItem(null)} />
      {pendingDelete && (
        <ConfirmModal
          title="Remove from wishlist"
          message={
            <>
              Remove <strong>{pendingDelete.item?.name ?? `item #${pendingDelete.item_id}`}</strong>{' '}
              from the <strong>{pendingDelete.slot_bucket}</strong> section?
            </>
          }
          confirmLabel="Remove"
          tone="danger"
          onConfirm={confirmDelete}
          onCancel={() => setPendingDelete(null)}
        />
      )}
    </div>
  )
}
