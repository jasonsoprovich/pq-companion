/**
 * SidebarEditor — the sidebar's in-place edit mode. Every item shows
 * (hidden ones dimmed) with a drag handle to reorder within its section, a
 * star to favorite and an eye to hide. Changes save immediately to the same
 * config fields Settings → Navigation edits.
 */
import React, { useEffect, useMemo, useState } from 'react'
import { Check, Eye, EyeOff, GripVertical, RotateCcw, Star } from 'lucide-react'
import {
  DndContext,
  PointerSensor,
  KeyboardSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { restrictToVerticalAxis } from '@dnd-kit/modifiers'
import { orderItems, type NavItem, type NavSection } from '../lib/sidebarNav'
import { saveSidebarPrefs } from '../lib/sidebarPrefsSave'

function Row({
  item,
  hidden,
  favorite,
  onToggleHidden,
  onToggleFavorite,
}: {
  item: NavItem
  hidden: boolean
  favorite: boolean
  onToggleHidden: () => void
  onToggleFavorite: () => void
}): React.ReactElement {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: item.to,
  })
  return (
    <div
      ref={setNodeRef}
      className="flex items-center gap-1.5 rounded px-1 py-1 text-sm"
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: hidden ? 0.4 : 1,
        backgroundColor: isDragging ? 'var(--color-surface-2)' : 'transparent',
        color: 'var(--color-muted-foreground)',
        position: 'relative',
        zIndex: isDragging ? 1 : undefined,
      }}
    >
      <span
        {...attributes}
        {...listeners}
        className="touch-none cursor-grab rounded p-0.5"
        title="Drag to reorder"
      >
        <GripVertical size={12} />
      </span>
      <span className="shrink-0">{item.icon}</span>
      <span className="min-w-0 flex-1 truncate">{item.label}</span>
      <button
        onClick={onToggleFavorite}
        className="rounded p-0.5"
        title={favorite ? 'Remove from favorites' : 'Add to favorites'}
        style={{ color: favorite ? 'var(--color-primary)' : 'var(--color-muted)', cursor: 'pointer' }}
      >
        <Star size={12} fill={favorite ? 'currentColor' : 'none'} />
      </button>
      <button
        onClick={onToggleHidden}
        className="rounded p-0.5"
        title={hidden ? 'Show in menu' : 'Hide from menu'}
        style={{ color: 'var(--color-muted)', cursor: 'pointer' }}
      >
        {hidden ? <EyeOff size={12} /> : <Eye size={12} />}
      </button>
    </div>
  )
}

export default function SidebarEditor({
  sections,
  order,
  hidden,
  favorites,
  onDone,
}: {
  sections: NavSection[]
  order: string[]
  hidden: string[]
  favorites: string[]
  onDone: () => void
}): React.ReactElement {
  // Working copies so the UI responds before the config round-trip.
  const [secOrder, setSecOrder] = useState<Record<string, string[]>>({})
  const [hiddenSet, setHiddenSet] = useState(() => new Set(hidden))
  const [favSet, setFavSet] = useState(() => new Set(favorites))

  useEffect(() => {
    const next: Record<string, string[]> = {}
    sections.forEach((s) => {
      next[s.id] = orderItems(s.items, order).map((i) => i.to)
    })
    setSecOrder(next)
    // Re-seed only when the section set changes, not on our own saves.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sections])

  const itemByKey = useMemo(() => {
    const m = new Map<string, NavItem>()
    sections.forEach((s) => s.items.forEach((i) => m.set(i.to, i)))
    return m
  }, [sections])

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const flatOrder = (o: Record<string, string[]>): string[] => sections.flatMap((s) => o[s.id] ?? [])

  const onDragEnd = (sectionId: string) => ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const list = secOrder[sectionId] ?? []
    const from = list.indexOf(String(active.id))
    const to = list.indexOf(String(over.id))
    if (from < 0 || to < 0) return
    const next = { ...secOrder, [sectionId]: arrayMove(list, from, to) }
    setSecOrder(next)
    void saveSidebarPrefs({ sidebar_order: flatOrder(next) })
  }

  const toggleHidden = (key: string): void => {
    const next = new Set(hiddenSet)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setHiddenSet(next)
    void saveSidebarPrefs({ sidebar_hidden: Array.from(next) })
  }

  const toggleFavorite = (key: string): void => {
    const next = new Set(favSet)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setFavSet(next)
    void saveSidebarPrefs({ sidebar_favorites: Array.from(next) })
  }

  const reset = (): void => {
    const next: Record<string, string[]> = {}
    sections.forEach((s) => {
      next[s.id] = s.items.map((i) => i.to)
    })
    setSecOrder(next)
    setHiddenSet(new Set())
    setFavSet(new Set())
    void saveSidebarPrefs({ sidebar_order: [], sidebar_hidden: [], sidebar_favorites: [] })
  }

  return (
    <div className="pb-2">
      <div
        className="sticky top-0 z-10 flex items-center gap-1 px-2 py-2"
        style={{ backgroundColor: 'var(--color-surface)', borderBottom: '1px solid var(--color-border)' }}
      >
        <span className="flex-1 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Drag, star or hide
        </span>
        <button
          onClick={reset}
          className="rounded p-1"
          style={{ color: 'var(--color-muted)', cursor: 'pointer' }}
          title="Show every item in the default order, with no favorites"
        >
          <RotateCcw size={12} />
        </button>
        <button
          onClick={onDone}
          className="flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-semibold"
          style={{ backgroundColor: 'var(--color-primary)', color: '#fff', cursor: 'pointer' }}
        >
          <Check size={11} /> Done
        </button>
      </div>
      {sections.map((section) => (
        <div key={section.id}>
          <p
            className="px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            {section.label}
          </p>
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={onDragEnd(section.id)}
          >
            <SortableContext items={secOrder[section.id] ?? []} strategy={verticalListSortingStrategy}>
              <div className="space-y-0.5 px-2">
                {(secOrder[section.id] ?? []).map((key) => {
                  const item = itemByKey.get(key)
                  if (!item) return null
                  return (
                    <Row
                      key={key}
                      item={item}
                      hidden={hiddenSet.has(key)}
                      favorite={favSet.has(key)}
                      onToggleHidden={() => toggleHidden(key)}
                      onToggleFavorite={() => toggleFavorite(key)}
                    />
                  )
                })}
              </div>
            </SortableContext>
          </DndContext>
        </div>
      ))}
    </div>
  )
}
