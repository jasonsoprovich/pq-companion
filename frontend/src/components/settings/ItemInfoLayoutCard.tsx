import React from 'react'
import { GripVertical } from 'lucide-react'
import {
  DndContext,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  verticalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { restrictToVerticalAxis } from '@dnd-kit/modifiers'
import {
  DEFAULT_ITEM_INFO_LIST_LIMIT,
  resolveItemInfoOrder,
  sectionDef,
  type ItemInfoSectionId,
} from '../../lib/itemInfoLayout'
import type { ItemInfoOverlaySettings } from '../../types/config'

interface Props {
  settings: ItemInfoOverlaySettings
  onChange: (next: ItemInfoOverlaySettings) => void
}

function SortableSection({ id, index }: { id: ItemInfoSectionId; index: number }): React.ReactElement {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      className="flex items-center gap-2 rounded px-2 py-1.5"
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        backgroundColor: 'var(--color-surface-2)',
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
      <span className="w-4 text-right text-xs tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {index + 1}
      </span>
      <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
        {sectionDef(id).label}
      </span>
    </div>
  )
}

/**
 * Item Info overlay section order (both surfaces), the global list limit,
 * and the popout's auto-show / auto-hide behavior.
 */
export default function ItemInfoLayoutCard({ settings, onChange }: Props): React.ReactElement {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const sections = resolveItemInfoOrder(settings.section_order)

  const onDragEnd = ({ active, over }: DragEndEvent): void => {
    if (!over || active.id === over.id) return
    const from = sections.indexOf(active.id as ItemInfoSectionId)
    const to = sections.indexOf(over.id as ItemInfoSectionId)
    onChange({ ...settings, section_order: arrayMove(sections, from, to) })
  }

  const muted = { color: 'var(--color-muted-foreground)' }
  const subheading = 'text-xs font-semibold uppercase tracking-wide'
  const inputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface-2)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
  }
  const limit = settings.list_limit && settings.list_limit > 0 ? settings.list_limit : DEFAULT_ITEM_INFO_LIST_LIMIT
  const autoHide = settings.auto_hide_secs ?? 0

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        Item Info Layout &amp; Behavior
      </h2>
      <p className="mb-4 text-xs" style={muted}>
        Drag to reorder the card on both overlay surfaces. Put what you check most first: a
        tradeskiller might lead with Tradeskills and Vendors, a raider with Key Facts.
      </p>

      <div className="grid gap-6 md:grid-cols-[14rem_1fr]">
        <div>
          <div className="mb-2 flex items-center justify-between">
            <h3 className={subheading} style={{ color: 'var(--color-muted)' }}>Sections</h3>
            {(settings.section_order?.length ?? 0) > 0 && (
              <button
                onClick={() => onChange({ ...settings, section_order: [] })}
                className="text-xs"
                style={{ color: 'var(--color-primary)' }}
              >
                Reset
              </button>
            )}
          </div>
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={onDragEnd}
          >
            <SortableContext items={sections} strategy={verticalListSortingStrategy}>
              <div className="flex flex-col gap-1">
                {sections.map((id, i) => (
                  <SortableSection key={id} id={id} index={i} />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        </div>

        <div className="flex flex-col gap-4">
          <div>
            <h3 className={`${subheading} mb-2`} style={{ color: 'var(--color-muted)' }}>Lists</h3>
            <label className="flex items-center gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
              List up to
              <input
                inputMode="numeric"
                value={String(limit)}
                onChange={(e) => {
                  const n = parseInt(e.target.value.replace(/\D/g, ''), 10)
                  onChange({ ...settings, list_limit: Number.isFinite(n) && n > 0 ? Math.min(n, 99) : DEFAULT_ITEM_INFO_LIST_LIMIT })
                }}
                className="w-12 rounded px-1.5 py-0.5 text-sm"
                style={inputStyle}
              />
              entries per section
            </label>
            <p className="mt-1 text-xs" style={muted}>
              Longer lists collapse to a count (&ldquo;14 vendors&rdquo;) that expands when clicked.
              Entries in your current zone stay visible. Each section can override this in the
              Display card.
            </p>
          </div>

          <div>
            <h3 className={`${subheading} mb-2`} style={{ color: 'var(--color-muted)' }}>Popout window</h3>
            <label className="flex cursor-pointer items-center gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
              <input
                type="checkbox"
                checked={Boolean(settings.auto_show)}
                onChange={(e) => onChange({ ...settings, auto_show: e.target.checked })}
              />
              Open the popout when I click an item link in game
            </label>
            <label
              className="mt-2 flex items-center gap-2 pl-6 text-sm"
              style={{ color: 'var(--color-foreground)', opacity: settings.auto_show ? 1 : 0.4 }}
            >
              Close it again after
              <input
                inputMode="numeric"
                disabled={!settings.auto_show}
                value={autoHide > 0 ? String(autoHide) : ''}
                placeholder="never"
                onChange={(e) => {
                  const n = parseInt(e.target.value.replace(/\D/g, ''), 10)
                  onChange({ ...settings, auto_hide_secs: Number.isFinite(n) && n > 0 ? Math.min(n, 3600) : 0 })
                }}
                className="w-16 rounded px-1.5 py-0.5 text-sm"
                style={inputStyle}
              />
              seconds
            </label>
            <p className="mt-1 pl-6 text-xs" style={muted}>
              Blank keeps it open. The timer restarts on each new item and pauses while your
              mouse is over the window.
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
