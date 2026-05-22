import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Star,
  Plus,
  RefreshCw,
  GripVertical,
  Trash2,
  AlertCircle,
  ChevronDown,
  ChevronRight,
  LayoutGrid,
  List as ListIcon,
  ChevronsDownUp,
  ChevronsUpDown,
} from 'lucide-react'
import {
  listCharacters,
  listWishlist,
  addWishlistEntries,
  deleteWishlistEntry,
  reorderWishlist,
  updateWishlistSlotLayout,
  getItem,
  getItemSources,
  type Character,
} from '../services/api'
import type { Item, ItemSources } from '../types/item'
import type { WishlistEntry, WishlistSlotLayout } from '../types/wishlist'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import CharacterSubTabs from '../components/CharacterSubTabs'
import ItemSearchModal from '../components/ItemSearchModal'
import WishlistSlotPicker from '../components/WishlistSlotPicker'
import ItemDetailModal from '../components/ItemDetailModal'
import { ConfirmModal } from '../components/ConfirmModal'
import { ItemIcon } from '../components/Icon'
import { WISHLIST_SLOT_ORDER, GENERAL_BUCKET, validSlotsForItem, isMultiSlotItem } from '../lib/wishlistSlots'

type ViewMode = 'category' | 'all'

// MIME-ish keys we set on dataTransfer to distinguish drag kinds. Browsers
// lowercase types, so check via includes() at the destination.
const DRAG_TYPE_ITEM = 'application/x-wishlist-item'
const DRAG_TYPE_CARD = 'application/x-wishlist-card'

// ── Source line ───────────────────────────────────────────────────────────────

interface SourceLineProps {
  sources: ItemSources | null
}

function SourceLine({ sources }: SourceLineProps): React.ReactElement {
  const navigate = useNavigate()
  if (!sources) {
    return (
      <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
        Loading source…
      </span>
    )
  }
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
  showSlotBadge?: boolean
  onOpenItem: (item: { id: number; name: string; icon: number }) => void
  onDelete: () => void
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
  showSlotBadge,
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
      <span className="cursor-grab" style={{ color: 'var(--color-muted)' }} title="Drag to reorder">
        <GripVertical size={14} />
      </span>
      {item && <ItemIcon id={item.icon} name={item.name} size={28} />}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <button
            onClick={() => item && onOpenItem({ id: item.id, name: item.name, icon: item.icon })}
            className="block max-w-full truncate text-left text-sm underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
            title={item?.name}
          >
            {item?.name ?? `Item #${entry.item_id}`}
          </button>
          {showSlotBadge && (
            <span
              className="shrink-0 rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
              title={`Slot: ${entry.slot_bucket}`}
            >
              {entry.slot_bucket === GENERAL_BUCKET ? 'General' : entry.slot_bucket}
            </span>
          )}
        </div>
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

// ── Helpers ───────────────────────────────────────────────────────────────────

// computeBucketOrder returns the slot buckets to display in category view, in
// the order the user has saved them — falling back to canonical order for any
// bucket the user hasn't explicitly placed. Only buckets with at least one
// entry are returned (empty cards aren't useful).
function computeBucketOrder(entries: WishlistEntry[], layout: WishlistSlotLayout[]): string[] {
  const present = new Set(entries.map((e) => e.slot_bucket))
  const ordered: string[] = []
  const seen = new Set<string>()
  const sortedLayout = [...layout].sort(
    (a, b) => a.position - b.position || a.slot_bucket.localeCompare(b.slot_bucket),
  )
  for (const l of sortedLayout) {
    if (present.has(l.slot_bucket) && !seen.has(l.slot_bucket)) {
      ordered.push(l.slot_bucket)
      seen.add(l.slot_bucket)
    }
  }
  for (const b of WISHLIST_SLOT_ORDER) {
    if (present.has(b) && !seen.has(b)) {
      ordered.push(b)
      seen.add(b)
    }
  }
  return ordered
}

// reorderItemsWithinBucket produces a new global entry list where two entries
// in the same bucket are swapped to a new relative position, while every
// entry of any other bucket keeps its existing global index. This preserves
// the user's mental model: dragging boots inside the boots card only moves
// boots — helmets stay where they were.
function reorderItemsWithinBucket(
  entries: WishlistEntry[],
  bucket: string,
  sourceID: number,
  targetID: number,
): WishlistEntry[] {
  const bucketEntries = entries.filter((e) => e.slot_bucket === bucket)
  const ids = bucketEntries.map((e) => e.id)
  const from = ids.indexOf(sourceID)
  const to = ids.indexOf(targetID)
  if (from === -1 || to === -1 || from === to) return entries
  const moved = bucketEntries.splice(from, 1)[0]
  bucketEntries.splice(to, 0, moved)
  let cursor = 0
  return entries.map((e) => {
    if (e.slot_bucket !== bucket) return e
    return bucketEntries[cursor++]
  })
}

// reorderItemsGlobal moves one entry to immediately before another in the
// global list (used by the All Items view, which allows cross-bucket drag).
function reorderItemsGlobal(
  entries: WishlistEntry[],
  sourceID: number,
  targetID: number,
): WishlistEntry[] {
  const ids = entries.map((e) => e.id)
  const from = ids.indexOf(sourceID)
  const to = ids.indexOf(targetID)
  if (from === -1 || to === -1 || from === to) return entries
  const next = [...entries]
  const moved = next.splice(from, 1)[0]
  next.splice(to, 0, moved)
  return next
}

// buildLayoutForOrder produces a full WishlistSlotLayout list from a displayed
// bucket order and a collapsed-state lookup. Buckets not in the displayed
// order but already saved in `existing` are kept (appended after visible ones)
// so collapse state survives temporarily-empty cards.
function buildLayoutForOrder(
  displayedOrder: string[],
  existing: WishlistSlotLayout[],
  collapsedByBucket: Map<string, boolean>,
): WishlistSlotLayout[] {
  const out: WishlistSlotLayout[] = []
  const seen = new Set<string>()
  displayedOrder.forEach((bucket, i) => {
    out.push({
      slot_bucket: bucket,
      position: i,
      collapsed: collapsedByBucket.get(bucket) ?? false,
    })
    seen.add(bucket)
  })
  let pos = displayedOrder.length
  for (const l of existing) {
    if (seen.has(l.slot_bucket)) continue
    out.push({
      slot_bucket: l.slot_bucket,
      position: pos++,
      collapsed: collapsedByBucket.get(l.slot_bucket) ?? l.collapsed,
    })
    seen.add(l.slot_bucket)
  }
  return out
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function WishlistPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewedCharacter, setViewedCharacter] = useState('')
  const [characters, setCharacters] = useState<Character[]>([])
  const [entries, setEntries] = useState<WishlistEntry[]>([])
  const [slotLayout, setSlotLayout] = useState<WishlistSlotLayout[]>([])
  const [viewMode, setViewMode] = useState<ViewMode>('category')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [sourcesCache, setSourcesCache] = useState<Map<number, ItemSources>>(new Map())

  // Item picker state
  const [searchOpen, setSearchOpen] = useState(false)
  const [picker, setPicker] = useState<{ item: Item; currentSlots: string[] } | null>(null)
  const [detailItem, setDetailItem] = useState<Item | null>(null)
  const [pendingDelete, setPendingDelete] = useState<WishlistEntry | null>(null)

  // Drag state — separate refs so item and card drags don't collide.
  const itemDragSrc = useRef<{ id: number; slot: string } | null>(null)
  const cardDragSrc = useRef<string | null>(null)
  const [dragOverItemID, setDragOverItemID] = useState<number | null>(null)
  const [dragOverCardBucket, setDragOverCardBucket] = useState<string | null>(null)

  useEffect(() => {
    listCharacters().then((r) => setCharacters(r.characters)).catch(() => setCharacters([]))
  }, [])
  const viewedCharID = useMemo(() => {
    return characters.find((c) => c.name.toLowerCase() === viewedCharacter.toLowerCase())?.id ?? 0
  }, [characters, viewedCharacter])

  useEffect(() => {
    if (!viewedCharacter && active) setViewedCharacter(active)
  }, [active, viewedCharacter])

  const load = useCallback(() => {
    if (!viewedCharID) {
      setEntries([])
      setSlotLayout([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    listWishlist(viewedCharID)
      .then((r) => {
        setEntries(r.entries ?? [])
        setSlotLayout(r.slot_layout ?? [])
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [viewedCharID])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    const ids = new Set(entries.map((e) => e.item_id))
    for (const id of ids) {
      if (sourcesCache.has(id)) continue
      getItemSources(id)
        .then((s) => setSourcesCache((prev) => new Map(prev).set(id, s)))
        .catch(() => undefined)
    }
  }, [entries, sourcesCache])

  // ── Derived view state ──────────────────────────────────────────────────────

  const bucketOrder = useMemo(
    () => computeBucketOrder(entries, slotLayout),
    [entries, slotLayout],
  )

  const collapsedByBucket = useMemo(() => {
    const m = new Map<string, boolean>()
    for (const l of slotLayout) m.set(l.slot_bucket, l.collapsed)
    return m
  }, [slotLayout])

  const entriesByBucket = useMemo(() => {
    const m = new Map<string, WishlistEntry[]>()
    for (const e of entries) {
      const list = m.get(e.slot_bucket) ?? []
      list.push(e)
      m.set(e.slot_bucket, list)
    }
    return m
  }, [entries])

  // ── Add / remove handlers ───────────────────────────────────────────────────

  function handleSearchSelect(item: Item) {
    setSearchOpen(false)
    if (!viewedCharID) return
    if (isMultiSlotItem(item.slots)) {
      const currentSlots = entries
        .filter((e) => e.item_id === item.id)
        .map((e) => e.slot_bucket)
      setPicker({ item, currentSlots })
      return
    }
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

  // ── Layout persistence ──────────────────────────────────────────────────────

  const persistLayout = useCallback(
    (next: WishlistSlotLayout[]) => {
      if (!viewedCharID) return
      setSlotLayout(next)
      updateWishlistSlotLayout(viewedCharID, next).catch((err: Error) => {
        setError(err.message)
        load()
      })
    },
    [viewedCharID, load],
  )

  function setBucketCollapsed(bucket: string, collapsed: boolean) {
    const nextMap = new Map(collapsedByBucket)
    nextMap.set(bucket, collapsed)
    persistLayout(buildLayoutForOrder(bucketOrder, slotLayout, nextMap))
  }

  function setAllCollapsed(collapsed: boolean) {
    const nextMap = new Map(collapsedByBucket)
    for (const b of bucketOrder) nextMap.set(b, collapsed)
    persistLayout(buildLayoutForOrder(bucketOrder, slotLayout, nextMap))
  }

  function reorderCards(sourceBucket: string, targetBucket: string) {
    const from = bucketOrder.indexOf(sourceBucket)
    const to = bucketOrder.indexOf(targetBucket)
    if (from === -1 || to === -1 || from === to) return
    const next = [...bucketOrder]
    const moved = next.splice(from, 1)[0]
    next.splice(to, 0, moved)
    persistLayout(buildLayoutForOrder(next, slotLayout, collapsedByBucket))
  }

  // ── Item reorder ────────────────────────────────────────────────────────────

  function commitEntryOrder(next: WishlistEntry[]) {
    setEntries(next.map((e, i) => ({ ...e, sort_order: i })))
    if (!viewedCharID) return
    reorderWishlist(
      viewedCharID,
      next.map((e) => e.id),
    ).catch((err: Error) => {
      setError(err.message)
      load()
    })
  }

  // ── Item drag handlers ──────────────────────────────────────────────────────

  function onItemDragStart(entry: WishlistEntry) {
    return (e: React.DragEvent) => {
      itemDragSrc.current = { id: entry.id, slot: entry.slot_bucket }
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData(DRAG_TYPE_ITEM, String(entry.id))
      // Firefox requires text/plain for any drag.
      e.dataTransfer.setData('text/plain', String(entry.id))
    }
  }
  function onItemDragOver(entry: WishlistEntry) {
    return (e: React.DragEvent) => {
      const src = itemDragSrc.current
      if (!src) return
      // In category view, only allow drop on same-bucket targets.
      if (viewMode === 'category' && src.slot !== entry.slot_bucket) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'move'
      setDragOverItemID(entry.id)
    }
  }
  function onItemDragLeave() {
    setDragOverItemID(null)
  }
  function onItemDrop(target: WishlistEntry) {
    return (e: React.DragEvent) => {
      e.preventDefault()
      const src = itemDragSrc.current
      setDragOverItemID(null)
      itemDragSrc.current = null
      if (!src || src.id === target.id) return
      if (viewMode === 'category' && src.slot !== target.slot_bucket) return
      const next =
        viewMode === 'category'
          ? reorderItemsWithinBucket(entries, target.slot_bucket, src.id, target.id)
          : reorderItemsGlobal(entries, src.id, target.id)
      commitEntryOrder(next)
    }
  }
  function onItemDragEnd() {
    itemDragSrc.current = null
    setDragOverItemID(null)
  }

  // ── Card drag handlers ──────────────────────────────────────────────────────

  function onCardDragStart(bucket: string) {
    return (e: React.DragEvent) => {
      cardDragSrc.current = bucket
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData(DRAG_TYPE_CARD, bucket)
      e.dataTransfer.setData('text/plain', bucket)
    }
  }
  function onCardDragOver(bucket: string) {
    return (e: React.DragEvent) => {
      if (!cardDragSrc.current || cardDragSrc.current === bucket) return
      // Only react to card drags, not item drags drifting onto the header.
      if (!e.dataTransfer.types.includes(DRAG_TYPE_CARD)) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'move'
      setDragOverCardBucket(bucket)
    }
  }
  function onCardDragLeave() {
    setDragOverCardBucket(null)
  }
  function onCardDrop(bucket: string) {
    return (e: React.DragEvent) => {
      e.preventDefault()
      const src = cardDragSrc.current
      setDragOverCardBucket(null)
      cardDragSrc.current = null
      if (!src || src === bucket) return
      reorderCards(src, bucket)
    }
  }
  function onCardDragEnd() {
    cardDragSrc.current = null
    setDragOverCardBucket(null)
  }

  // ── Render ──────────────────────────────────────────────────────────────────

  const anyExpanded = bucketOrder.some((b) => !(collapsedByBucket.get(b) ?? false))
  const isEmpty = entries.length === 0

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

        {/* View toggle */}
        <div
          className="ml-3 flex overflow-hidden rounded"
          style={{ border: '1px solid var(--color-border)' }}
        >
          <button
            onClick={() => setViewMode('category')}
            className="flex items-center gap-1 px-2 py-1 text-xs"
            style={{
              backgroundColor:
                viewMode === 'category' ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color:
                viewMode === 'category'
                  ? 'var(--color-surface)'
                  : 'var(--color-muted-foreground)',
            }}
            title="Group by slot"
          >
            <LayoutGrid size={11} />
            Category
          </button>
          <button
            onClick={() => setViewMode('all')}
            className="flex items-center gap-1 px-2 py-1 text-xs"
            style={{
              backgroundColor:
                viewMode === 'all' ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color:
                viewMode === 'all' ? 'var(--color-surface)' : 'var(--color-muted-foreground)',
            }}
            title="Flat list of all items"
          >
            <ListIcon size={11} />
            All items
          </button>
        </div>

        {viewMode === 'category' && !isEmpty && (
          <button
            onClick={() => setAllCollapsed(anyExpanded)}
            className="flex items-center gap-1 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title={anyExpanded ? 'Collapse all sections' : 'Expand all sections'}
          >
            {anyExpanded ? <ChevronsDownUp size={11} /> : <ChevronsUpDown size={11} />}
            {anyExpanded ? 'Collapse all' : 'Expand all'}
          </button>
        )}

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
            style={{
              backgroundColor: 'var(--color-surface)',
              border: '1px solid var(--color-danger)',
              color: 'var(--color-danger)',
            }}
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
        {!loading && viewedCharID && isEmpty && (
          <div className="flex h-32 items-center justify-center">
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              No items wishlisted yet. Click "Add item" or star an item on its detail page.
            </p>
          </div>
        )}

        {!loading && viewedCharID && !isEmpty && viewMode === 'category' && (
          <>
            {bucketOrder.map((bucket) => {
              const list = entriesByBucket.get(bucket) ?? []
              const collapsed = collapsedByBucket.get(bucket) ?? false
              const isCardDragOver = dragOverCardBucket === bucket
              return (
                <div
                  key={bucket}
                  onDragOver={onCardDragOver(bucket)}
                  onDragLeave={onCardDragLeave}
                  onDrop={onCardDrop(bucket)}
                  className="rounded"
                  style={{
                    border: `1px solid ${isCardDragOver ? 'var(--color-primary)' : 'var(--color-border)'}`,
                    backgroundColor: 'var(--color-surface-2)',
                  }}
                >
                  {/* Card header */}
                  <div
                    className="flex items-center gap-1.5 px-2 py-1.5"
                    style={{
                      borderBottom: collapsed ? 'none' : '1px solid var(--color-border)',
                    }}
                  >
                    <span
                      draggable
                      onDragStart={onCardDragStart(bucket)}
                      onDragEnd={onCardDragEnd}
                      className="cursor-grab shrink-0"
                      style={{ color: 'var(--color-muted)' }}
                      title="Drag to reorder section"
                    >
                      <GripVertical size={14} />
                    </span>
                    <button
                      onClick={() => setBucketCollapsed(bucket, !collapsed)}
                      className="flex flex-1 items-center gap-2 text-left"
                      title={collapsed ? 'Expand section' : 'Collapse section'}
                    >
                      {collapsed ? (
                        <ChevronRight size={12} style={{ color: 'var(--color-muted)' }} />
                      ) : (
                        <ChevronDown size={12} style={{ color: 'var(--color-muted)' }} />
                      )}
                      <span
                        className="text-[10px] font-semibold uppercase tracking-widest"
                        style={{ color: 'var(--color-muted)' }}
                      >
                        {bucket === GENERAL_BUCKET ? 'General' : bucket}
                      </span>
                      <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
                        {list.length}
                      </span>
                    </button>
                    <button
                      onClick={() => setBucketCollapsed(bucket, !collapsed)}
                      className="shrink-0 rounded p-0.5"
                      style={{ color: 'var(--color-muted)' }}
                      title={collapsed ? 'Expand section' : 'Collapse section'}
                    >
                      <span className="text-sm font-mono leading-none">
                        {collapsed ? '+' : '−'}
                      </span>
                    </button>
                  </div>
                  {!collapsed && (
                    <div className="space-y-1.5 p-2">
                      {list.map((entry) => (
                        <WishlistRow
                          key={entry.id}
                          entry={entry}
                          sources={sourcesCache.get(entry.item_id) ?? null}
                          onOpenItem={handleOpenItem}
                          onDelete={() => setPendingDelete(entry)}
                          isDraggedOver={dragOverItemID === entry.id}
                          onDragStart={onItemDragStart(entry)}
                          onDragOver={onItemDragOver(entry)}
                          onDragLeave={onItemDragLeave}
                          onDrop={onItemDrop(entry)}
                          onDragEnd={onItemDragEnd}
                        />
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </>
        )}

        {!loading && viewedCharID && !isEmpty && viewMode === 'all' && (
          <div className="space-y-1.5">
            {entries.map((entry) => (
              <WishlistRow
                key={entry.id}
                entry={entry}
                sources={sourcesCache.get(entry.item_id) ?? null}
                showSlotBadge
                onOpenItem={handleOpenItem}
                onDelete={() => setPendingDelete(entry)}
                isDraggedOver={dragOverItemID === entry.id}
                onDragStart={onItemDragStart(entry)}
                onDragOver={onItemDragOver(entry)}
                onDragLeave={onItemDragLeave}
                onDrop={onItemDrop(entry)}
                onDragEnd={onItemDragEnd}
              />
            ))}
          </div>
        )}
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
