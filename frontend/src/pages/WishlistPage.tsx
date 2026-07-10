import React, { useCallback, useEffect, useMemo, useState } from 'react'
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
  Bell,
} from 'lucide-react'
import {
  DndContext,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  closestCenter,
  type CollisionDetection,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  verticalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { restrictToVerticalAxis } from '@dnd-kit/modifiers'
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
import WishlistAlertSettings from '../components/WishlistAlertSettings'
import { ItemIcon } from '../components/Icon'
import { WISHLIST_SLOT_ORDER, GENERAL_BUCKET, validSlotsForItem, isMultiSlotItem } from '../lib/wishlistSlots'

type ViewMode = 'category' | 'all'

// Sortable id prefixes. Native HTML5 DnD was unreliable in Electron on Windows
// (the window-drag regions swallowed dragover/drop), so reordering uses dnd-kit
// (pointer events). Items and cards live in one DndContext; the id prefix tells
// a card drag from an item drag in both collision detection and the drop handler.
const ITEM_PREFIX = 'item:'
const CARD_PREFIX = 'card:'

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
}

function WishlistRow({
  entry,
  sources,
  showSlotBadge,
  onOpenItem,
  onDelete,
}: WishlistRowProps): React.ReactElement {
  const item = entry.item
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: `${ITEM_PREFIX}${entry.id}`,
  })
  return (
    <div
      ref={setNodeRef}
      className="flex items-center gap-2 rounded px-2 py-2"
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        opacity: isDragging ? 0.4 : 1,
      }}
    >
      <span
        {...attributes}
        {...listeners}
        className="cursor-grab touch-none active:cursor-grabbing"
        style={{ color: 'var(--color-muted)' }}
        title="Drag to reorder"
      >
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

// ── Wishlist card (slot bucket section) ─────────────────────────────────────────

interface WishlistCardProps {
  bucket: string
  list: WishlistEntry[]
  collapsed: boolean
  sourcesCache: Map<number, ItemSources>
  onToggleCollapsed: () => void
  onOpenItem: (item: { id: number; name: string; icon: number }) => void
  onDelete: (entry: WishlistEntry) => void
}

function WishlistCard({
  bucket,
  list,
  collapsed,
  sourcesCache,
  onToggleCollapsed,
  onOpenItem,
  onDelete,
}: WishlistCardProps): React.ReactElement {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: `${CARD_PREFIX}${bucket}`,
  })
  return (
    <div
      ref={setNodeRef}
      className="rounded"
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        border: '1px solid var(--color-border)',
        backgroundColor: 'var(--color-surface-2)',
        opacity: isDragging ? 0.5 : 1,
      }}
    >
      {/* Card header */}
      <div
        className="flex items-center gap-1.5 px-2 py-1.5"
        style={{ borderBottom: collapsed ? 'none' : '1px solid var(--color-border)' }}
      >
        <span
          {...attributes}
          {...listeners}
          className="cursor-grab touch-none shrink-0 active:cursor-grabbing"
          style={{ color: 'var(--color-muted)' }}
          title="Drag to reorder section"
        >
          <GripVertical size={14} />
        </span>
        <button
          onClick={onToggleCollapsed}
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
          onClick={onToggleCollapsed}
          className="shrink-0 rounded p-0.5"
          style={{ color: 'var(--color-muted)' }}
          title={collapsed ? 'Expand section' : 'Collapse section'}
        >
          <span className="text-sm font-mono leading-none">{collapsed ? '+' : '−'}</span>
        </button>
      </div>
      {!collapsed && (
        <div className="space-y-1.5 p-2">
          <SortableContext
            items={list.map((e) => `${ITEM_PREFIX}${e.id}`)}
            strategy={verticalListSortingStrategy}
          >
            {list.map((entry) => (
              <WishlistRow
                key={entry.id}
                entry={entry}
                sources={sourcesCache.get(entry.item_id) ?? null}
                onOpenItem={onOpenItem}
                onDelete={() => onDelete(entry)}
              />
            ))}
          </SortableContext>
        </div>
      )}
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
  const [alertsOpen, setAlertsOpen] = useState(false)
  const [picker, setPicker] = useState<{ item: Item; currentSlots: string[] } | null>(null)
  const [detailItem, setDetailItem] = useState<Item | null>(null)
  const [pendingDelete, setPendingDelete] = useState<WishlistEntry | null>(null)

  // dnd-kit sensors: a small activation distance lets the grip be clicked
  // without starting a drag, and keyboard reordering comes for free.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

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

  // ── Drag-and-drop (dnd-kit) ─────────────────────────────────────────────────

  // Scope collision detection by drag kind so a card drag only considers card
  // targets and an item drag only considers item targets. Without this, the
  // nested card/item sortables cross-talk and the wrong thing reorders.
  const collisionDetection = useCallback<CollisionDetection>(
    (args) => {
      const activeId = String(args.active.id)
      if (activeId.startsWith(CARD_PREFIX)) {
        return closestCenter({
          ...args,
          droppableContainers: args.droppableContainers.filter((c) =>
            String(c.id).startsWith(CARD_PREFIX),
          ),
        })
      }
      // Item drag. In category view, confine targets to the same bucket so the
      // live reflow never strays into a bucket the item can't be moved to.
      let allowed: Set<string> | null = null
      if (viewMode === 'category') {
        const srcID = Number(activeId.slice(ITEM_PREFIX.length))
        const bucket = entries.find((x) => x.id === srcID)?.slot_bucket
        if (bucket != null) {
          allowed = new Set(
            entries
              .filter((x) => x.slot_bucket === bucket)
              .map((x) => `${ITEM_PREFIX}${x.id}`),
          )
        }
      }
      return closestCenter({
        ...args,
        droppableContainers: args.droppableContainers.filter((c) => {
          const id = String(c.id)
          if (!id.startsWith(ITEM_PREFIX)) return false
          return allowed ? allowed.has(id) : true
        }),
      })
    },
    [viewMode, entries],
  )

  function handleDragEnd(e: DragEndEvent) {
    const { active, over } = e
    if (!over) return
    const a = String(active.id)
    const o = String(over.id)
    if (a === o) return
    if (a.startsWith(CARD_PREFIX) && o.startsWith(CARD_PREFIX)) {
      reorderCards(a.slice(CARD_PREFIX.length), o.slice(CARD_PREFIX.length))
      return
    }
    if (a.startsWith(ITEM_PREFIX) && o.startsWith(ITEM_PREFIX)) {
      const srcID = Number(a.slice(ITEM_PREFIX.length))
      const tgtID = Number(o.slice(ITEM_PREFIX.length))
      const src = entries.find((x) => x.id === srcID)
      const tgt = entries.find((x) => x.id === tgtID)
      if (!src || !tgt) return
      if (viewMode === 'category') {
        // Category view forbids cross-bucket moves — bucket is fixed by slot.
        if (src.slot_bucket !== tgt.slot_bucket) return
        commitEntryOrder(reorderItemsWithinBucket(entries, tgt.slot_bucket, srcID, tgtID))
      } else {
        commitEntryOrder(reorderItemsGlobal(entries, srcID, tgtID))
      }
    }
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
          onClick={() => setAlertsOpen(true)}
          className="ml-auto flex items-center gap-1.5 text-xs px-2.5 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
          title="Alert when a wishlisted item appears in your log"
        >
          <Bell size={12} />
          Alerts
        </button>
        <button
          onClick={() => setSearchOpen(true)}
          disabled={!viewedCharID}
          className="flex items-center gap-1.5 text-xs px-2.5 py-1 rounded disabled:opacity-50"
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
          <DndContext
            sensors={sensors}
            collisionDetection={collisionDetection}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={handleDragEnd}
          >
            <SortableContext
              items={bucketOrder.map((b) => `${CARD_PREFIX}${b}`)}
              strategy={verticalListSortingStrategy}
            >
              <div className="space-y-4">
                {bucketOrder.map((bucket) => (
                  <WishlistCard
                    key={bucket}
                    bucket={bucket}
                    list={entriesByBucket.get(bucket) ?? []}
                    collapsed={collapsedByBucket.get(bucket) ?? false}
                    sourcesCache={sourcesCache}
                    onToggleCollapsed={() =>
                      setBucketCollapsed(bucket, !(collapsedByBucket.get(bucket) ?? false))
                    }
                    onOpenItem={handleOpenItem}
                    onDelete={(entry) => setPendingDelete(entry)}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}

        {!loading && viewedCharID && !isEmpty && viewMode === 'all' && (
          <DndContext
            sensors={sensors}
            collisionDetection={collisionDetection}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={handleDragEnd}
          >
            <SortableContext
              items={entries.map((e) => `${ITEM_PREFIX}${e.id}`)}
              strategy={verticalListSortingStrategy}
            >
              <div className="space-y-1.5">
                {entries.map((entry) => (
                  <WishlistRow
                    key={entry.id}
                    entry={entry}
                    sources={sourcesCache.get(entry.item_id) ?? null}
                    showSlotBadge
                    onOpenItem={handleOpenItem}
                    onDelete={() => setPendingDelete(entry)}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}
      </div>

      {/* Modals */}
      <WishlistAlertSettings open={alertsOpen} onClose={() => setAlertsOpen(false)} />
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
