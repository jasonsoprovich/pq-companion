import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useAutoTour } from '../components/guide/GuideProvider'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import {
  Zap,
  Plus,
  Trash2,
  Pencil,
  RefreshCw,
  AlertCircle,
  X,
  CheckCircle2,
  Download,
  Upload,
  Package,
  Clock,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Search,
  Users,
  Sparkles,
  Tags,
  GripVertical,
  Check,
  SlidersHorizontal,
  Pin,
  Scissors,
} from 'lucide-react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  useDroppable,
  closestCenter,
  pointerWithin,
  rectIntersection,
  type CollisionDetection,
  type DragStartEvent,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  verticalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
  arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { restrictToVerticalAxis } from '@dnd-kit/modifiers'
import { usePersistentState } from '../hooks/usePersistentState'
import SpellSearchPicker from '../components/SpellSearchPicker'
import ImportTriggersModal from '../components/ImportTriggersModal'
import PackUpdateModal from '../components/PackUpdateModal'
import { refreshPackUpdates, usePackUpdates } from '../hooks/usePackUpdates'
import { NewDot } from '../hooks/useFeatureDots'
import BulkActionsModal from '../components/BulkActionsModal'
import TriggerTesterTab from '../components/TriggerTesterTab'
import { buildSpellTriggerPrefill, secsLabel } from '../lib/spellHelpers'
import type { TriggerPrefill } from '../lib/triggerTemplates'
import TriggerTemplatePicker from '../components/triggers/TriggerTemplatePicker'
import {
  listTriggers,
  updateTrigger,
  deleteTrigger,
  clearAllTriggers,
  getTriggerHistory,
  getBuiltinPacks,
  installBuiltinPack,
  removeTriggerPack,
  exportTriggerPack,
  exportTriggerCategory,
  splitTriggerCategory,
  listCharacters,
  listTriggerCategories,
  createTriggerCategory,
  renameTriggerCategory,
  deleteTriggerCategory,
  reorderTriggers,
  reorderTriggerCategories,
  listTimerGroups,
  type CreateTriggerRequest,
  type Character,
} from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { useActivePlayerName } from '../hooks/useActivePlayerName'
import { WSEvent } from '../lib/wsEvents'
import type {
  Trigger,
  TriggerFired,
  TriggerPack,
  TriggerCategory,
  CategoryPlacement,
  TimerGroup,
  PackUpdateSummary,
} from '../types/trigger'
import { matchesSearch } from '../lib/textSearch'
import TriggerForm from '../components/triggers/TriggerForm'
import { CLASS_NAMES, Toggle, flattenCategoryTree } from '../components/triggers/shared'


// ── Helpers ────────────────────────────────────────────────────────────────────

function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', second: '2-digit' })
}

// downloadTriggerPack saves a fetched TriggerPack as a local .json file.
function downloadTriggerPack(pack: TriggerPack, filename: string): void {
  const blob = new Blob([JSON.stringify(pack, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// slugifyFilename turns a category name into a safe filename fragment.
function slugifyFilename(name: string): string {
  return name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '') || 'category'
}


// categoryLabel is a category's display label — "Parent / Child" for a
// subcategory, so two subcategories with the same name under different
// parents (and the section itself) stay unambiguous.
function categoryLabel(cat: TriggerCategory, byId: Map<string, TriggerCategory>): string {
  if (!cat.parent_id) return cat.name
  const parent = byId.get(cat.parent_id)
  return parent ? `${parent.name} / ${cat.name}` : cat.name
}


// ── Trigger row ───────────────────────────────────────────────────────────────

// Sortable id prefixes. Native HTML5 DnD was unreliable in Electron on Windows
// (the window-drag regions swallowed dragover/drop), so reordering uses dnd-kit
// (pointer events). The prefix distinguishes the three drag kinds — trigger
// rows, category sections (reorder), and section drop zones (move) — in both
// collision detection and the drop handler.
const TRIGGER_PREFIX = 'trigger:'
const CATEGORY_PREFIX = 'category:'
const SECTION_PREFIX = 'section:'

interface TriggerRowProps {
  trigger: Trigger
  categories: TriggerCategory[]
  onCategoriesChanged: () => void
  timerGroups: TimerGroup[]
  onTimerGroupsChanged: () => void
  onDeleted: (id: string) => void
  onUpdated: (t: Trigger) => void
}

function TriggerRow({
  trigger,
  categories,
  onCategoriesChanged,
  timerGroups,
  onTimerGroupsChanged,
  onDeleted,
  onUpdated,
}: TriggerRowProps): React.ReactElement {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  useEscapeToClose(() => setConfirmDelete(false), confirmDelete)
  const [toggling, setToggling] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState(false)
  const [shared, setShared] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  // dnd-kit sortable: the grip handle (below) carries the drag listeners so the
  // row's buttons stay clickable. Reorder within the category happens in the
  // page's drag-end handler; cross-category moves drop onto a section.
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: `${TRIGGER_PREFIX}${trigger.id}`,
  })

  const handleShare = () => {
    const pack: TriggerPack = {
      pack_name: `Share: ${trigger.name}`,
      description: `Single-trigger share from PQ Companion`,
      triggers: [trigger],
    }
    navigator.clipboard.writeText(JSON.stringify(pack, null, 2))
      .then(() => {
        setShared(true)
        setTimeout(() => setShared(false), 2000)
      })
      .catch(() => {
        setError('failed to copy to clipboard')
      })
  }

  const handleDelete = () => {
    setDeleting(true)
    deleteTrigger(trigger.id)
      .then(() => onDeleted(trigger.id))
      .catch((err: Error) => {
        setError(err.message)
        setDeleting(false)
        setConfirmDelete(false)
      })
  }

  const handleToggle = (v: boolean) => {
    setToggling(true)
    // Full update — the backend replaces list fields with what's sent, so
    // every list must be passed through or toggling would wipe it.
    const req: CreateTriggerRequest = {
      name: trigger.name,
      enabled: v,
      pattern: trigger.pattern,
      actions: trigger.actions,
      timer_type: trigger.timer_type,
      timer_duration_secs: trigger.timer_duration_secs,
      timer_duration_capture: trigger.timer_duration_capture ?? '',
      timer_key_capture: trigger.timer_key_capture ?? '',
      timer_target_capture: trigger.timer_target_capture ?? '',
      worn_off_pattern: trigger.worn_off_pattern,
      spell_id: trigger.spell_id,
      display_threshold_secs: trigger.display_threshold_secs,
      bar_color: trigger.bar_color ?? '',
      pinned: trigger.pinned ?? false,
      timer_stack: trigger.timer_stack ?? false,
      timer_repeat: trigger.timer_repeat ?? false,
      timer_repeat_max: trigger.timer_repeat_max ?? 0,
      timer_if_idle: trigger.timer_if_idle,
      timer_clear_on_context_change: trigger.timer_clear_on_context_change,
      custom_group_id: trigger.custom_group_id ?? '',
      characters: trigger.characters,
      timer_alerts: trigger.timer_alerts ?? [],
      exclude_patterns: trigger.exclude_patterns ?? [],
      extra_patterns: trigger.extra_patterns ?? [],
      source: trigger.source,
      pipe_condition: trigger.pipe_condition,
    }
    updateTrigger(trigger.id, req)
      .then((updated) => {
        onUpdated(updated)
        setToggling(false)
      })
      .catch((err: Error) => {
        setError(err.message)
        setToggling(false)
      })
  }

  if (isEditing) {
    return (
      <TriggerForm
        initial={trigger}
        categories={categories}
        onCategoriesChanged={onCategoriesChanged}
        timerGroups={timerGroups}
        onTimerGroupsChanged={onTimerGroupsChanged}
        onSaved={(t) => {
          onUpdated(t)
          setIsEditing(false)
        }}
        onCancel={() => setIsEditing(false)}
      />
    )
  }

  return (
    <div
      ref={setNodeRef}
      className="rounded-lg"
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${trigger.enabled ? 'var(--color-border)' : 'var(--color-surface-3)'}`,
        opacity: isDragging ? 0.4 : trigger.enabled ? 1 : 0.65,
      }}
    >
      <div className="flex items-center gap-3 px-3 py-2.5">
        {/* Drag handle — reorder within or move to another category */}
        <div
          {...attributes}
          {...listeners}
          title="Drag to reorder or move to another category"
          className="shrink-0 cursor-grab touch-none active:cursor-grabbing"
          style={{ color: 'var(--color-muted)' }}
        >
          <GripVertical size={14} />
        </div>

        {/* Enable toggle */}
        {toggling ? (
          <RefreshCw size={14} className="animate-spin shrink-0" style={{ color: 'var(--color-muted)' }} />
        ) : (
          <Toggle checked={trigger.enabled} onChange={handleToggle} />
        )}

        {/* Name + pattern */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium truncate" style={{ color: 'var(--color-foreground)' }}>
              {trigger.name}
            </span>
            {trigger.pinned && trigger.timer_type && trigger.timer_type !== 'none' && (
              <span className="shrink-0 flex items-center" title="Pinned to top of its overlay">
                <Pin size={11} style={{ color: 'var(--color-primary)' }} fill="currentColor" />
              </span>
            )}
            {trigger.timer_type && trigger.timer_type !== 'none' && (
              <span
                className="text-[10px] px-1.5 py-0.5 rounded shrink-0 font-medium capitalize"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: trigger.timer_type === 'buff' ? '#22c55e' : '#ef4444',
                  border: `1px solid ${trigger.timer_type === 'buff' ? '#22c55e' : '#ef4444'}`,
                }}
              >
                {trigger.timer_type} · {secsLabel(trigger.timer_duration_secs)}s
              </span>
            )}
            {trigger.source === 'pipe' && (
              <span
                className="text-[10px] px-1.5 py-0.5 rounded shrink-0 font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-primary)',
                  border: '1px solid var(--color-primary)',
                }}
                title="Zeal pipe event trigger"
              >
                pipe
              </span>
            )}
            {trigger.source_pack && (
              <span
                className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded shrink-0"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
                title={`Installed from the ${trigger.source_pack} pack. Deactivating that pack removes this trigger even if you've moved it to another category.`}
              >
                <Package size={10} />
                {trigger.source_pack}
              </span>
            )}
            {trigger.dedup_key && (
              <span
                className="text-[10px] px-1.5 py-0.5 rounded shrink-0"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-primary)',
                  border: '1px solid var(--color-primary)',
                }}
                title={`Shared across packs (dedup key: ${trigger.dedup_key}). Installing another pack that ships this same entry will skip the duplicate.`}
              >
                shared
              </span>
            )}
            <span
              className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded shrink-0"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
              title={
                trigger.characters && trigger.characters.length > 0
                  ? `Active on: ${trigger.characters.join(', ')}`
                  : 'Active on: any character'
              }
            >
              <Users size={10} />
              {trigger.characters && trigger.characters.length > 0
                ? trigger.characters.length
                : 'all'}
            </span>
          </div>
          <p className="text-[11px] mt-0.5 truncate font-mono" style={{ color: 'var(--color-muted)' }}>
            {trigger.pattern}
          </p>
        </div>

        {/* Actions count */}
        <span className="text-[11px] shrink-0" style={{ color: 'var(--color-muted)' }}>
          {trigger.actions.length} action{trigger.actions.length !== 1 ? 's' : ''}
        </span>

        {/* Expand / share / edit / delete */}
        <div className="flex items-center gap-1 shrink-0">
          <button
            onClick={() => setExpanded((v) => !v)}
            className="p-1 rounded"
            style={{ color: 'var(--color-muted-foreground)' }}
            title={expanded ? 'Collapse' : 'Expand'}
          >
            {expanded ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
          </button>
          <button
            onClick={handleShare}
            className="p-1 rounded"
            style={{ color: shared ? 'var(--color-success)' : 'var(--color-muted-foreground)' }}
            title="Copy quick-share JSON to clipboard"
          >
            {shared ? <CheckCircle2 size={13} /> : <Upload size={13} />}
          </button>
          <button
            onClick={() => setIsEditing(true)}
            className="p-1 rounded"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Edit"
          >
            <Pencil size={13} />
          </button>
          <button
            onClick={() => setConfirmDelete(true)}
            className="p-1 rounded"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Delete"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>

      {/* Expanded detail */}
      {expanded && trigger.actions.length > 0 && (
        <div
          className="border-t px-3 py-2 space-y-1.5"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {trigger.actions.map((action, i) => (
            <div key={i} className="flex items-center gap-3 text-[11px]">
              <span
                className="w-2 h-2 rounded-full shrink-0"
                style={{ backgroundColor: action.color || '#ffffff' }}
              />
              <span className="font-mono" style={{ color: 'var(--color-foreground)' }}>
                "{action.text}"
              </span>
              <span style={{ color: 'var(--color-muted)' }}>
                {action.duration_secs || 5}s
              </span>
            </div>
          ))}
        </div>
      )}

      {/* Delete confirmation */}
      {confirmDelete && (
        <div
          className="border-t flex items-center gap-2 px-3 py-2"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <AlertCircle size={13} style={{ color: 'var(--color-danger)' }} />
          <span className="flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
            Delete "{trigger.name}"?
          </span>
          <button
            onClick={handleDelete}
            disabled={deleting}
            className="text-xs px-2 py-0.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-danger)', color: '#fff' }}
          >
            {deleting ? <RefreshCw size={11} className="animate-spin" /> : 'Delete'}
          </button>
          <button
            onClick={() => setConfirmDelete(false)}
            className="p-0.5"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            <X size={13} />
          </button>
        </div>
      )}

      {error && (
        <p className="px-3 pb-2 text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}
    </div>
  )
}

// ── Category section ──────────────────────────────────────────────────────────

interface CategorySectionProps {
  group: { categoryId: string; items: Trigger[] }
  categories: TriggerCategory[]
  collapsed: boolean
  // True when a movable trigger is mid-drag (i.e. one whose current category is
  // not this one) — drives the dashed "drop allowed" border.
  canDrop: boolean
  isRenaming: boolean
  renameValue: string
  onRenameValueChange: (v: string) => void
  onToggleCollapsed: () => void
  onStartRename: () => void
  onCommitRename: () => void
  onCancelRename: () => void
  onDeleteCategory: (cat: TriggerCategory) => void
  onExportCategory: (id: string) => void
  onAddSubcategory: (parentId: string) => void
  onReparentCategory: (cat: TriggerCategory, newParentId: string) => void
  onSplitCategory: (cat: TriggerCategory) => void
  onTriggerDeleted: (id: string) => void
  onTriggerUpdated: (t: Trigger) => void
  onCategoriesChanged: () => void
  timerGroups: TimerGroup[]
  onTimerGroupsChanged: () => void
}

function CategorySection({
  group,
  categories,
  collapsed,
  canDrop,
  isRenaming,
  renameValue,
  onRenameValueChange,
  onToggleCollapsed,
  onStartRename,
  onCommitRename,
  onCancelRename,
  onDeleteCategory,
  onExportCategory,
  onAddSubcategory,
  onReparentCategory,
  onSplitCategory,
  onTriggerDeleted,
  onTriggerUpdated,
  onCategoriesChanged,
  timerGroups,
  onTimerGroupsChanged,
}: CategorySectionProps): React.ReactElement {
  const categoryId = group.categoryId
  const isUncategorized = categoryId === '__uncategorized__'
  const reorderableSection = !isUncategorized
  const cat = categories.find((c) => c.id === categoryId)
  const isCustom = !!cat?.custom
  const isChild = !!cat?.parent_id
  const hasChildren = !isChild && categories.some((c) => c.parent_id === categoryId)
  const categoriesById = new Map(categories.map((c) => [c.id, c]))
  const label = isUncategorized ? 'Uncategorized' : cat ? categoryLabel(cat, categoriesById) : categoryId
  // Reparent target options: other top-level custom categories. The dropdown
  // itself is only rendered when this section has no children of its own —
  // see hasChildren below — a category that itself has children can't become
  // a child (the backend's ReorderCategories rejects it).
  const parentOptions = categories.filter((c) => c.custom && !c.parent_id && c.id !== categoryId)

  // Category reorder: dragging the header grip moves the whole section.
  // Uncategorized is pinned last, so its sortable is disabled.
  const {
    setNodeRef: setSortRef,
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: `${CATEGORY_PREFIX}${categoryId}`, disabled: !reorderableSection })

  // Trigger move: dropping a trigger anywhere on this section reassigns it here.
  const { setNodeRef: setDropRef, isOver, active } = useDroppable({
    id: `${SECTION_PREFIX}${categoryId}`,
  })

  const triggerBeingDragged = !!active && String(active.id).startsWith(TRIGGER_PREFIX)
  const moveTarget = triggerBeingDragged && canDrop
  const isDropTarget = isOver && moveTarget

  return (
    <div
      ref={setSortRef}
      className="space-y-2 rounded"
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : 1,
        marginLeft: isChild ? 20 : 0,
      }}
    >
      <div ref={setDropRef} className="space-y-2">
        <div
          className="flex w-full items-center gap-2 rounded px-2 py-1.5"
          style={{
            backgroundColor: isDropTarget ? 'var(--color-surface-3)' : 'var(--color-surface-2)',
            border: `1px ${moveTarget ? 'dashed' : 'solid'} ${
              isDropTarget || moveTarget ? 'var(--color-primary)' : 'var(--color-border)'
            }`,
          }}
        >
          {reorderableSection && !isRenaming && (
            <div
              {...attributes}
              {...listeners}
              title="Drag to reorder category"
              className="shrink-0 cursor-grab touch-none active:cursor-grabbing"
              style={{ color: 'var(--color-muted)' }}
            >
              <GripVertical size={13} />
            </div>
          )}
          <button
            type="button"
            onClick={onToggleCollapsed}
            className={`flex items-center gap-2 text-left ${
              isRenaming ? 'shrink-0' : 'flex-1 min-w-0'
            }`}
            style={{ background: 'transparent', border: 'none', cursor: 'pointer' }}
          >
            {collapsed ? (
              <ChevronRight size={13} style={{ color: 'var(--color-muted)' }} />
            ) : (
              <ChevronDown size={13} style={{ color: 'var(--color-muted)' }} />
            )}
            {!isRenaming && (
              <>
                <span
                  className="text-xs font-semibold truncate"
                  style={{ color: 'var(--color-foreground)' }}
                >
                  {label}
                </span>
                <span
                  className="text-[11px] shrink-0"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  {group.items.length}
                </span>
              </>
            )}
          </button>
          {isRenaming && (
            <>
              <input
                type="text"
                autoFocus
                value={renameValue}
                onChange={(e) => onRenameValueChange(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    onCommitRename()
                  } else if (e.key === 'Escape') {
                    e.preventDefault()
                    e.stopPropagation()
                    onCancelRename()
                  }
                }}
                onBlur={onCommitRename}
                className="flex-1 rounded px-2 py-0.5 text-xs outline-none min-w-0"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
              {/* onMouseDown preventDefault keeps the input focused so its
                  onBlur doesn't fire (and re-commit) before the click runs. */}
              <button
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={onCommitRename}
                className="p-0.5 rounded shrink-0"
                title="Save"
                style={{ color: 'var(--color-primary)', cursor: 'pointer' }}
              >
                <Check size={14} />
              </button>
              <button
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={onCancelRename}
                className="p-0.5 rounded shrink-0"
                title="Cancel"
                style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
              >
                <X size={14} />
              </button>
            </>
          )}
          {isDropTarget && (
            <span
              className="text-[11px] font-medium shrink-0"
              style={{ color: 'var(--color-primary)' }}
            >
              Move here
            </span>
          )}
          {isCustom && !isRenaming && (
            <div className="flex items-center gap-1 shrink-0">
              {!hasChildren && parentOptions.length > 0 && (
                <select
                  value={cat?.parent_id ?? ''}
                  onChange={(e) => cat && onReparentCategory(cat, e.target.value)}
                  className="rounded px-1 py-0.5 text-[10px] outline-none"
                  title="Move this category to the top level or nest it under another"
                  style={{
                    backgroundColor: 'var(--color-surface)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-muted-foreground)',
                    maxWidth: 110,
                  }}
                >
                  <option value="">Top level</option>
                  {parentOptions.map((p) => (
                    <option key={p.id} value={p.id}>↳ {p.name}</option>
                  ))}
                </select>
              )}
              {!isChild && (
                <button
                  type="button"
                  onClick={() => onAddSubcategory(categoryId)}
                  className="p-0.5 rounded"
                  title="Add a subcategory"
                  style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
                >
                  <Plus size={12} />
                </button>
              )}
              {!isChild && !hasChildren && cat?.name.includes('/') && (
                <button
                  type="button"
                  onClick={() => cat && onSplitCategory(cat)}
                  className="p-0.5 rounded"
                  title={`Split into "${cat.name.split('/')[0].trim()}" → "${cat.name.split('/').slice(1).join('/').trim()}"`}
                  style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
                >
                  <Scissors size={12} />
                </button>
              )}
              {group.items.length > 0 && (
                <button
                  type="button"
                  onClick={() => onExportCategory(categoryId)}
                  className="p-0.5 rounded"
                  title="Export category as a trigger pack"
                  style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
                >
                  <Download size={12} />
                </button>
              )}
              <button
                type="button"
                onClick={onStartRename}
                className="p-0.5 rounded"
                title="Rename category"
                style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
              >
                <Pencil size={12} />
              </button>
              <button
                type="button"
                onClick={() => cat && onDeleteCategory(cat)}
                className="p-0.5 rounded"
                title="Delete category"
                style={{ color: 'var(--color-destructive)', cursor: 'pointer' }}
              >
                <Trash2 size={12} />
              </button>
            </div>
          )}
        </div>
        {!collapsed && (
          <SortableContext
            items={group.items.map((t) => `${TRIGGER_PREFIX}${t.id}`)}
            strategy={verticalListSortingStrategy}
          >
            {group.items.map((t) => (
              <TriggerRow
                key={t.id}
                trigger={t}
                categories={categories}
                onCategoriesChanged={onCategoriesChanged}
                timerGroups={timerGroups}
                onTimerGroupsChanged={onTimerGroupsChanged}
                onDeleted={onTriggerDeleted}
                onUpdated={onTriggerUpdated}
              />
            ))}
          </SortableContext>
        )}
      </div>
    </div>
  )
}

// Lightweight previews rendered in the DragOverlay so the dragged item follows
// the cursor (the cross-category move in particular reads much better this way).
function TriggerDragPreview({ trigger }: { trigger: Trigger }): React.ReactElement {
  return (
    <div
      className="flex items-center gap-2 rounded-lg px-3 py-2.5 shadow-lg"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-primary)' }}
    >
      <GripVertical size={14} style={{ color: 'var(--color-muted)' }} />
      <span className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
        {trigger.name}
      </span>
    </div>
  )
}

function CategoryDragPreview({ label }: { label: string }): React.ReactElement {
  return (
    <div
      className="flex items-center gap-2 rounded px-2 py-1.5 shadow-lg"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-primary)' }}
    >
      <GripVertical size={13} style={{ color: 'var(--color-muted)' }} />
      <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
        {label}
      </span>
    </div>
  )
}

// ── History tab ───────────────────────────────────────────────────────────────

function HistoryTab(): React.ReactElement {
  const [history, setHistory] = useState<TriggerFired[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getTriggerHistory()
      .then((h) => setHistory(h.slice().reverse())) // newest first
      .finally(() => setLoading(false))
  }, [])

  useWebSocket((msg) => {
    if (msg.type === WSEvent.TriggerFired) {
      const event = msg.data as TriggerFired
      // Trigger Tester fires (see TriggerTesterTab) broadcast the same event
      // so the overlay/audio can preview them, but they're not real matches
      // — keep them out of history.
      if (event.test) return
      setHistory((prev) => [event, ...prev].slice(0, 200))
    }
  })

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  if (history.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2">
        <Clock size={28} style={{ color: 'var(--color-muted)' }} />
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          No triggers have fired yet.
        </p>
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          History updates live as triggers match log lines.
        </p>
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto p-4 space-y-1.5">
      {history.map((event, i) => (
        <div
          key={i}
          className="flex items-start gap-3 rounded px-3 py-2"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
          }}
        >
          <div className="shrink-0 mt-0.5">
            <Zap size={13} style={{ color: 'var(--color-primary)' }} />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
                {event.trigger_name}
              </span>
              {event.actions.map((a, ai) => (
                <span
                  key={ai}
                  className="text-[11px] px-1.5 py-0.5 rounded font-medium"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    color: a.color || 'var(--color-foreground)',
                    border: '1px solid var(--color-border)',
                  }}
                >
                  {a.text}
                </span>
              ))}
            </div>
            <p
              className="text-[11px] mt-0.5 truncate font-mono"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              {event.matched_line}
            </p>
          </div>
          <span className="text-[11px] shrink-0" style={{ color: 'var(--color-muted)' }}>
            {formatTime(event.fired_at)}
          </span>
        </div>
      ))}
    </div>
  )
}

// ── Packs tab ─────────────────────────────────────────────────────────────────

interface PacksTabProps {
  installedPacks: Set<string>
  onInstalled: () => void
}

type PackConfirm = { packName: string; kind: 'deactivate' | 'reactivate' }

function PacksTab({ installedPacks, onInstalled }: PacksTabProps): React.ReactElement {
  const [packs, setPacks] = useState<TriggerPack[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)
  const [installed, setInstalled] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [confirm, setConfirm] = useState<PackConfirm | null>(null)
  useEscapeToClose(() => setConfirm(null), !!confirm)
  const importInputRef = useRef<HTMLInputElement>(null)
  const [importFile, setImportFile] = useState<File | null>(null)
  // Pending pack updates (this build's definitions differ from what the
  // user installed), keyed by pack name; drives the banner + badges.
  const [updates, setUpdates] = useState<Map<string, PackUpdateSummary>>(new Map())
  const [updateModal, setUpdateModal] = useState<string | null>(null)
  const [updateApplied, setUpdateApplied] = useState<string | null>(null)

  // Shared with the sidebar and tab dots, so they clear as packs update.
  const refreshUpdates = useCallback(() => {
    refreshPackUpdates()
      .then((list) => setUpdates(new Map(list.map((u) => [u.pack_name, u]))))
  }, [])

  const toggleExpanded = (packName: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(packName)) next.delete(packName)
      else next.add(packName)
      return next
    })
  }

  useEffect(() => {
    getBuiltinPacks()
      .then(setPacks)
      .finally(() => setLoading(false))
    refreshUpdates()
  }, [refreshUpdates])

  const handleInstall = (packName: string) => {
    if (installedPacks.has(packName)) {
      setConfirm({ packName, kind: 'reactivate' })
      return
    }
    runInstall(packName)
  }

  const runInstall = (packName: string) => {
    setBusy(packName)
    setError(null)
    installBuiltinPack(packName)
      .then(() => {
        setInstalled(packName)
        onInstalled()
        refreshUpdates()
        setTimeout(() => setInstalled(null), 3000)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(null))
  }

  const handleRemove = (packName: string) => {
    setConfirm({ packName, kind: 'deactivate' })
  }

  const runRemove = (packName: string) => {
    setBusy(packName)
    setError(null)
    removeTriggerPack(packName)
      .then(() => {
        onInstalled()
        refreshUpdates()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(null))
  }

  const handleConfirm = () => {
    if (!confirm) return
    const { packName, kind } = confirm
    setConfirm(null)
    if (kind === 'reactivate') runInstall(packName)
    else runRemove(packName)
  }

  const handleExport = () => {
    exportTriggerPack()
      .then((pack) => downloadTriggerPack(pack, 'pq-triggers.json'))
      .catch((err: Error) => setError(err.message))
  }

  // Open the import wizard with the picked file. Detection + parsing happen in
  // the modal; this just hands off the raw File (which may be a binary .gtp /
  // .zip archive, so it must not be read as text here).
  const handleImportFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) setImportFile(file)
    // Reset so the same file can be re-picked.
    e.target.value = ''
  }

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-y-auto p-4 space-y-4">
      {/* Pack updates banner */}
      {updates.size > 0 && (
        <div
          className="rounded-lg p-3 flex items-center gap-2"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-warning, #f59e0b)',
          }}
        >
          <RefreshCw size={14} style={{ color: 'var(--color-warning, #f59e0b)' }} />
          <p className="text-xs flex-1" style={{ color: 'var(--color-foreground)' }}>
            Trigger updates available for {updates.size} installed pack
            {updates.size !== 1 ? 's' : ''}:{' '}
            {Array.from(updates.keys()).join(', ')}. Your customizations can
            be kept — review each pack below.
          </p>
        </div>
      )}
      {updateApplied && (
        <div className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-success)' }}>
          <CheckCircle2 size={13} />
          {updateApplied}
        </div>
      )}

      {/* Import / Export */}
      <div
        className="rounded-lg p-3 space-y-2"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Import / Export
        </p>
        <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Import triggers from PQ Companion, GINA, EQNag, or EQLogParser exports,
          or back up your triggers as JSON.
        </p>
        <div className="flex gap-2">
          <button
            onClick={handleExport}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <Download size={12} /> Export All
          </button>
          <button
            onClick={() => importInputRef.current?.click()}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Import triggers from PQ Companion, GINA, EQNag, or EQLogParser"
          >
            <Upload size={12} /> Import Triggers
          </button>
          <input
            ref={importInputRef}
            type="file"
            accept=".json,.gtp,.xml,.zip,.tgf,application/json,application/xml,text/xml,application/zip"
            onChange={handleImportFile}
            className="hidden"
          />
        </div>
        {installed && (
          <div className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-success)' }}>
            <CheckCircle2 size={13} />
            "{installed}" activated successfully.
          </div>
        )}
        {error && (
          <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
            {error}
          </p>
        )}
      </div>

      {/* Built-in packs */}
      <div>
        <p className="text-[11px] font-semibold uppercase tracking-widest mb-2" style={{ color: 'var(--color-muted)' }}>
          Built-in Packs
        </p>
        <div className="space-y-3">
          {packs.map((pack) => {
            const isOpen = expanded.has(pack.pack_name)
            const isInstalled = installedPacks.has(pack.pack_name)
            const isBusy = busy === pack.pack_name
            const pending = isInstalled ? updates.get(pack.pack_name) : undefined
            const pendingCount = pending
              ? pending.changed + pending.added + pending.removed
              : 0
            const handleAction = () => {
              if (isInstalled) handleRemove(pack.pack_name)
              else handleInstall(pack.pack_name)
            }
            return (
              <div
                key={pack.pack_name}
                className="rounded-lg"
                style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
              >
                <button
                  type="button"
                  onClick={() => toggleExpanded(pack.pack_name)}
                  aria-expanded={isOpen}
                  className="w-full flex items-start justify-between gap-3 p-3 text-left"
                >
                  <div className="flex-1 min-w-0 flex items-start gap-2">
                    {isOpen ? (
                      <ChevronDown size={14} className="mt-0.5 shrink-0" style={{ color: 'var(--color-muted)' }} />
                    ) : (
                      <ChevronRight size={14} className="mt-0.5 shrink-0" style={{ color: 'var(--color-muted)' }} />
                    )}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <Package size={14} style={{ color: 'var(--color-primary)' }} />
                        <span className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
                          {pack.pack_name}
                        </span>
                        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                          {pack.triggers.length} trigger{pack.triggers.length !== 1 ? 's' : ''}
                        </span>
                        {isInstalled && (
                          <span
                            className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded"
                            style={{
                              backgroundColor: 'var(--color-surface-2)',
                              color: 'var(--color-success)',
                            }}
                          >
                            <CheckCircle2 size={10} />
                            Active
                          </span>
                        )}
                        {pendingCount > 0 && (
                          <span
                            className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded"
                            style={{
                              backgroundColor: 'var(--color-surface-2)',
                              color: 'var(--color-warning, #f59e0b)',
                            }}
                          >
                            <RefreshCw size={10} />
                            {pendingCount} update{pendingCount !== 1 ? 's' : ''}
                          </span>
                        )}
                      </div>
                      <p className="text-[11px] mt-1" style={{ color: 'var(--color-muted-foreground)' }}>
                        {pack.description}
                      </p>
                    </div>
                  </div>
                  {pending && pendingCount > 0 && (
                    <span
                      role="button"
                      tabIndex={0}
                      onClick={(e) => {
                        e.stopPropagation()
                        setUpdateModal(pack.pack_name)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          e.stopPropagation()
                          setUpdateModal(pack.pack_name)
                        }
                      }}
                      className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium shrink-0 cursor-pointer"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        color: 'var(--color-warning, #f59e0b)',
                        border: '1px solid var(--color-warning, #f59e0b)',
                      }}
                    >
                      <RefreshCw size={11} />
                      Review updates
                    </span>
                  )}
                  <span
                    role="button"
                    tabIndex={0}
                    onClick={(e) => {
                      e.stopPropagation()
                      handleAction()
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        e.stopPropagation()
                        handleAction()
                      }
                    }}
                    aria-disabled={isBusy}
                    className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium shrink-0 cursor-pointer"
                    style={{
                      backgroundColor: isInstalled
                        ? 'var(--color-surface-2)'
                        : 'var(--color-primary)',
                      color: isInstalled
                        ? 'var(--color-danger)'
                        : 'var(--color-background)',
                      border: isInstalled
                        ? '1px solid var(--color-border)'
                        : '1px solid transparent',
                      opacity: isBusy ? 0.6 : 1,
                    }}
                  >
                    {isBusy ? (
                      <RefreshCw size={11} className="animate-spin" />
                    ) : isInstalled ? (
                      <Trash2 size={11} />
                    ) : (
                      <Download size={11} />
                    )}
                    {isInstalled ? 'Deactivate' : 'Activate'}
                  </span>
                </button>

                {isOpen && (
                  <div
                    className="px-3 pb-3 pt-0 space-y-1"
                    style={{ borderTop: '1px solid var(--color-border)' }}
                  >
                    {pack.triggers.map((t, i) => (
                      <div key={i} className="flex items-center gap-2 text-[11px] pt-2">
                        <Zap size={10} style={{ color: 'var(--color-muted)' }} />
                        <span style={{ color: 'var(--color-muted-foreground)' }}>{t.name}</span>
                        <span className="font-mono truncate" style={{ color: 'var(--color-muted)' }}>
                          {t.pattern}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {confirm && (
        <div
          onClick={() => setConfirm(null)}
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
            className="rounded-lg p-4 space-y-3"
            style={{
              backgroundColor: 'var(--color-surface)',
              border: '1px solid var(--color-border)',
              width: '100%',
              maxWidth: 420,
            }}
          >
            <div className="flex items-center gap-2">
              <AlertCircle size={16} style={{ color: 'var(--color-danger)' }} />
              <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
                {confirm.kind === 'deactivate' ? 'Deactivate pack?' : 'Re-activate pack?'}
              </p>
            </div>
            <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
              {confirm.kind === 'deactivate'
                ? `Deactivate the "${confirm.packName}" pack? This deletes all triggers belonging to this pack, including any customizations you made.`
                : `"${confirm.packName}" is already active. Re-activating will replace any customizations you made to its triggers. Continue?`}
            </p>
            <div className="flex justify-end gap-2 pt-1">
              <button
                onClick={() => setConfirm(null)}
                className="text-xs px-3 py-1.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleConfirm}
                className="text-xs px-3 py-1.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-danger)',
                  color: '#fff',
                  border: '1px solid transparent',
                }}
              >
                {confirm.kind === 'deactivate' ? 'Deactivate' : 'Re-activate'}
              </button>
            </div>
          </div>
        </div>
      )}
      {importFile && (
        <ImportTriggersModal
          file={importFile}
          onClose={() => setImportFile(null)}
          onImported={(category, count) => {
            setImportFile(null)
            onInstalled()
            setInstalled(`${category} (${count})`)
            setTimeout(() => setInstalled(null), 3000)
          }}
        />
      )}
      {updateModal && (
        <PackUpdateModal
          packName={updateModal}
          onClose={() => setUpdateModal(null)}
          onApplied={(res) => {
            const name = updateModal
            setUpdateModal(null)
            onInstalled()
            refreshUpdates()
            const parts: string[] = []
            if (res.updated) parts.push(`${res.updated} updated`)
            if (res.added) parts.push(`${res.added} added`)
            if (res.removed) parts.push(`${res.removed} removed`)
            if (res.moved) parts.push(`${res.moved} moved`)
            setUpdateApplied(
              `"${name}" pack updated (${parts.join(', ') || 'no changes'}).`,
            )
            setTimeout(() => setUpdateApplied(null), 5000)
          }}
        />
      )}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

type Tab = 'triggers' | 'history' | 'packs' | 'tester'

// ── Delete category modal ───────────────────────────────────────────────────

interface DeleteCategoryModalProps {
  category: TriggerCategory
  // True when this category has subcategories — they're promoted to the top
  // level (never deleted) regardless of which choice below is made.
  hasChildren: boolean
  onClose: () => void
  // Reload triggers + categories after the delete (it refreshes the
  // pack_name display cache too).
  onChanged: () => void
}

function DeleteCategoryModal({
  category,
  hasChildren,
  onClose,
  onChanged,
}: DeleteCategoryModalProps): React.ReactElement {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useEscapeToClose(onClose, true)

  const run = (deleteTriggers: boolean) => {
    setBusy(true)
    setError(null)
    deleteTriggerCategory(category.id, deleteTriggers)
      .then(() => onChanged())
      .catch((e: Error) => {
        setError(e.message)
        setBusy(false)
      })
  }

  const n = category.count
  const plural = n === 1 ? '' : 's'
  const choiceBtn = {
    border: '1px solid var(--color-border)',
    backgroundColor: 'var(--color-surface-2)',
    cursor: 'pointer' as const,
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-sm rounded-lg"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          className="flex items-center gap-2 border-b px-4 py-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <Trash2 size={15} style={{ color: 'var(--color-destructive)' }} />
          <span className="text-sm font-semibold truncate" style={{ color: 'var(--color-foreground)' }}>
            Delete “{category.name}”
          </span>
          <button
            onClick={onClose}
            className="ml-auto shrink-0"
            style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
            aria-label="Close"
          >
            <X size={16} />
          </button>
        </div>

        <div className="p-4 space-y-3">
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {n === 0
              ? 'This category is empty and will be removed.'
              : `This category has ${n} trigger${plural}. What should happen to ${n === 1 ? 'it' : 'them'}?`}
          </p>
          {hasChildren && (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Its subcategories will be kept and moved to the top level.
            </p>
          )}
          {error && (
            <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{error}</p>
          )}
          <div className="flex flex-col gap-2">
            {n > 0 ? (
              <>
                <button
                  onClick={() => run(false)}
                  disabled={busy}
                  className="rounded px-3 py-2 text-xs font-medium text-left"
                  style={{ ...choiceBtn, color: 'var(--color-foreground)' }}
                >
                  Move {n} trigger{plural} to Uncategorized
                </button>
                <button
                  onClick={() => run(true)}
                  disabled={busy}
                  className="rounded px-3 py-2 text-xs font-medium text-left"
                  style={{ ...choiceBtn, color: 'var(--color-destructive)' }}
                >
                  Delete category and all {n} trigger{plural}
                </button>
              </>
            ) : (
              <button
                onClick={() => run(false)}
                disabled={busy}
                className="rounded px-3 py-2 text-xs font-medium text-left"
                style={{ ...choiceBtn, color: 'var(--color-destructive)' }}
              >
                Delete category
              </button>
            )}
            <button
              onClick={onClose}
              disabled={busy}
              className="rounded px-3 py-2 text-xs"
              style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
            >
              Cancel
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export default function TriggersPage(): React.ReactElement {
  useAutoTour('triggers')
  const activePlayer = useActivePlayerName()
  const packUpdates = usePackUpdates()
  const [tab, setTab] = usePersistentState<Tab>('pq-tab:triggers', 'triggers', ['triggers', 'history', 'packs', 'tester'])
  const [triggers, setTriggers] = useState<Trigger[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createPrefill, setCreatePrefill] = useState<TriggerPrefill | undefined>(undefined)
  const [showTemplates, setShowTemplates] = useState(false)
  const [showSpellPicker, setShowSpellPicker] = useState(false)
  const [newMenuOpen, setNewMenuOpen] = useState(false)
  const [emptyNewMenuOpen, setEmptyNewMenuOpen] = useState(false)
  const [showClearAll, setShowClearAll] = useState(false)
  const [clearingAll, setClearingAll] = useState(false)
  const [showBulkEdit, setShowBulkEdit] = useState(false)
  const [bulkEditNotice, setBulkEditNotice] = useState<string | null>(null)
  useEscapeToClose(() => {
    if (!clearingAll) setShowClearAll(false)
  }, showClearAll)
  const [search, setSearch] = useState('')
  const [classFilter, setClassFilter] = useState<number | null>(null)
  const [charFilter, setCharFilter] = useState<string>('')
  const [packFilter, setPackFilter] = useState<string>('')
  const [sortMode, setSortMode] = useState<'name' | 'recent' | 'manual'>('name')
  const [chars, setChars] = useState<Character[]>([])
  const [packClassByName, setPackClassByName] = useState<Map<string, number>>(new Map())
  // Tracks which category sections in the grouped trigger list are collapsed,
  // by category id. Default: all expanded. Persists across refreshes via
  // localStorage. Keyed separately from the pre-nesting name-keyed storage
  // (triggers.collapsedPacks) so stale name entries can never phantom-match
  // an id.
  const [collapsedPacks, setCollapsedPacks] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem('triggers.collapsedCategories')
      if (raw) return new Set(JSON.parse(raw))
    } catch {}
    return new Set()
  })
  const [categories, setCategories] = useState<TriggerCategory[]>([])
  // Timer groups are managed from the Overlays dashboard's "Custom Timers"
  // button (TimerGroupsModal) — fetched here only to populate the trigger
  // editor's window picker.
  const [timerGroups, setTimerGroups] = useState<TimerGroup[]>([])
  // Inline category management on the section headers: which category is being
  // renamed (+ its edit-box value), which is pending delete (opens the modal),
  // and whether a New Category create is in flight.
  const [renamingCategory, setRenamingCategory] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [deletingCategory, setDeletingCategory] = useState<TriggerCategory | null>(null)
  const [creatingCategory, setCreatingCategory] = useState(false)
  // Latches a rename commit so the input's unmount-blur doesn't fire twice
  // (and so Escape skips the rename). See commitRenameCategory.
  const cancelRenameRef = useRef(false)
  // dnd-kit drag state. activeTrigger is the trigger being dragged (drives the
  // per-section "drop allowed" hint + overlay preview); activeCategory is the
  // section being dragged for reorder (drives its overlay preview).
  const [activeTrigger, setActiveTrigger] = useState<Trigger | null>(null)
  const [activeCategory, setActiveCategory] = useState<string | null>(null)
  // A small activation distance lets the grip be clicked without starting a
  // drag; the keyboard sensor brings arrow-key reordering for free.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  // Categories are partly derived from in-use pack_name values, so refresh
  // them whenever triggers change (create/move/delete) as well as after
  // explicit category edits.
  const reloadCategories = useCallback(() => {
    listTriggerCategories()
      .then(setCategories)
      .catch(() => {})
  }, [])

  const reloadTimerGroups = useCallback(() => {
    listTimerGroups()
      .then(setTimerGroups)
      .catch(() => {})
  }, [])

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listTriggers()
      .then(setTriggers)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
    reloadCategories()
    reloadTimerGroups()
  }, [reloadCategories, reloadTimerGroups])

  useEffect(() => { load() }, [load])

  // Characters populate the character filter dropdown.
  useEffect(() => {
    listCharacters()
      .then((resp) => setChars(resp.characters))
      .catch(() => {})
  }, [])

  // Pack → class map drives the class filter — selecting "Cleric" surfaces
  // pack's class, so e.g. selecting "Cleric" surfaces an installed Cleric
  // pack even when no Cleric character has been added to the roster yet.
  useEffect(() => {
    getBuiltinPacks()
      .then((all) => {
        const m = new Map<string, number>()
        for (const p of all) {
          if (typeof p.class === 'number' && p.class >= 0) m.set(p.pack_name, p.class)
        }
        setPackClassByName(m)
      })
      .catch(() => {})
  }, [])

  const filteredTriggers = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q && classFilter === null && !charFilter && !packFilter) return triggers
    return triggers.filter((t) => {
      if (q) {
        const haystack = `${t.name}\n${t.pattern}\n${t.pack_name}\n${t.dedup_key ?? ''}`.toLowerCase()
        if (!matchesSearch(haystack, q)) return false
      }
      // Character filter: empty Characters list = fires for any character
      // (engine's legacy semantic), so universal triggers pass — global
      // alerts (e.g. General Triggers death notifications) stay visible
      // no matter which character you narrow to.
      const universal = !t.characters || t.characters.length === 0
      if (charFilter && !universal) {
        if (!t.characters!.includes(charFilter)) return false
      }
      if (classFilter !== null) {
        // Class filter is a spell-type filter — match strictly on the
        // trigger's pack class, ignoring character assignments. Otherwise
        // an Enchanter-pack trigger that a Wizard character also has
        // active would leak into the Wizard view.
        if (!t.pack_name) return false
        if (packClassByName.get(t.pack_name) !== classFilter) return false
      }
      if (packFilter) {
        // packFilter === "__uncategorized__" picks user-authored triggers
        // (no category); any other value is a category id, matched exactly —
        // ids, unlike names, are never ambiguous between two subcategories
        // that happen to share a name.
        if (packFilter === '__uncategorized__') {
          if (t.category_id) return false
        } else if ((t.category_id ?? '') !== packFilter) {
          return false
        }
      }
      return true
    })
  }, [triggers, search, classFilter, charFilter, packFilter, packClassByName])

  // Categories currently represented in the user's triggers, for the
  // pack-filter dropdown — tree order, so a subcategory sorts right after its
  // parent. The "Uncategorized" bucket (user-authored) is pinned to the end.
  const packsInUse = useMemo(() => {
    const idsInUse = new Set<string>()
    let hasUncategorized = false
    for (const t of triggers) {
      if (t.category_id) idsInUse.add(t.category_id)
      else hasUncategorized = true
    }
    const byId = new Map(categories.map((c) => [c.id, c]))
    const entries = flattenCategoryTree(categories)
      .filter((c) => idsInUse.has(c.id))
      .map((c) => ({ id: c.id, label: categoryLabel(c, byId) }))
    if (hasUncategorized) entries.push({ id: '__uncategorized__', label: 'Uncategorized' })
    return entries
  }, [triggers, categories])

  const hasActiveFilter = !!(search.trim() || classFilter !== null || charFilter || packFilter)

  // Group + sort the filtered triggers for display. Sections follow the
  // backend category order (tree-flattened, so a subcategory follows its
  // parent); Uncategorized pins last. Empty custom categories are shown (so
  // they can be drag targets) when no filter is narrowing the view. Each
  // section's entries are sorted per sortMode.
  const groupedTriggers = useMemo(() => {
    const groups = new Map<string, Trigger[]>()
    for (const t of filteredTriggers) {
      const key = t.category_id || '__uncategorized__'
      if (!groups.has(key)) groups.set(key, [])
      groups.get(key)!.push(t)
    }
    if (!hasActiveFilter) {
      for (const c of categories) {
        if (c.custom && !groups.has(c.id)) groups.set(c.id, [])
      }
    }
    const orderIndex = new Map<string, number>()
    flattenCategoryTree(categories).forEach((c, i) => orderIndex.set(c.id, i))
    const orderOf = (key: string): number => {
      if (key === '__uncategorized__') return Number.MAX_SAFE_INTEGER
      const idx = orderIndex.get(key)
      return idx === undefined ? Number.MAX_SAFE_INTEGER - 1 : idx
    }
    const sortItems = (items: Trigger[]) => {
      if (sortMode === 'manual') {
        items.sort(
          (a, b) =>
            (a.sort_order ?? 0) - (b.sort_order ?? 0) ||
            (a.created_at ? new Date(a.created_at).getTime() : 0) -
              (b.created_at ? new Date(b.created_at).getTime() : 0),
        )
      } else if (sortMode === 'recent') {
        items.sort((a, b) => {
          const aT = a.created_at ? new Date(a.created_at).getTime() : 0
          const bT = b.created_at ? new Date(b.created_at).getTime() : 0
          return bT - aT
        })
      } else {
        items.sort((a, b) => a.name.localeCompare(b.name))
      }
    }
    const ordered = Array.from(groups.entries()).map(([categoryId, items]) => ({
      categoryId,
      items,
    }))
    ordered.sort((a, b) => orderOf(a.categoryId) - orderOf(b.categoryId))
    for (const g of ordered) sortItems(g.items)
    return ordered
  }, [filteredTriggers, categories, sortMode, hasActiveFilter])

  // Set of pack ids the user already has installed, for the Packs tab. Memoized
  // so the PacksTab prop identity is stable across unrelated re-renders.
  const installedPacks = useMemo(
    () => new Set(triggers.map((t) => t.source_pack).filter((n): n is string => !!n)),
    [triggers],
  )

  const togglePackCollapsed = (categoryId: string) => {
    setCollapsedPacks((prev) => {
      const next = new Set(prev)
      if (next.has(categoryId)) next.delete(categoryId)
      else next.add(categoryId)
      try {
        localStorage.setItem('triggers.collapsedCategories', JSON.stringify(Array.from(next)))
      } catch {}
      return next
    })
  }

  // Always offer all 15 EQ classes in the filter so the user can narrow to
  // any installed class pack, even before they have a character of that
  // class in the roster.
  const availableClasses = CLASS_NAMES.map((_, idx) => idx)

  const handleCreated = (t: Trigger) => {
    setTriggers((prev) => [...prev, t])
    setShowCreate(false)
    setCreatePrefill(undefined)
    reloadCategories()
  }

  const handleDeleted = (id: string) => {
    setTriggers((prev) => prev.filter((t) => t.id !== id))
    reloadCategories()
  }

  const handleUpdated = (updated: Trigger) => {
    setTriggers((prev) => prev.map((t) => (t.id === updated.id ? updated : t)))
    reloadCategories()
  }

  // ── Category management (inline, on the section headers) ──
  // Create a category with a unique default name, then immediately drop its
  // header into rename mode so the user can type the real name.
  // Create a category with a unique (among its siblings) default name, then
  // immediately drop its header into rename mode so the user can type the
  // real name. parentId='' creates a top-level category (the toolbar button);
  // a non-empty parentId creates a subcategory (a section header's + button).
  const createCategoryFlow = (parentId: string) => {
    if (creatingCategory) return
    const siblingNames = new Set(
      categories.filter((c) => (c.parent_id || '') === parentId).map((c) => c.name),
    )
    let name = 'New Category'
    for (let i = 2; siblingNames.has(name); i++) name = `New Category ${i}`
    setCreatingCategory(true)
    createTriggerCategory(name, parentId)
      .then((cat) => {
        reloadCategories()
        cancelRenameRef.current = false
        setRenamingCategory(cat.id)
        setRenameValue(cat.name)
      })
      .catch(() => {})
      .finally(() => setCreatingCategory(false))
  }
  const handleNewCategory = () => createCategoryFlow('')
  const handleAddSubcategory = (parentId: string) => createCategoryFlow(parentId)

  const startRenameCategory = (id: string, currentName: string) => {
    cancelRenameRef.current = false
    setRenamingCategory(id)
    setRenameValue(currentName)
  }

  const cancelRenameCategory = () => {
    cancelRenameRef.current = true
    setRenamingCategory(null)
  }

  // Commit the inline rename. Reentrant-safe: the input's onBlur fires again
  // when Enter/Escape unmounts it, so the first call latches cancelRenameRef
  // to make the second a no-op. Escape sets the latch up front to skip the
  // rename entirely.
  const commitRenameCategory = (id: string) => {
    if (cancelRenameRef.current) {
      cancelRenameRef.current = false
      setRenamingCategory(null)
      return
    }
    cancelRenameRef.current = true
    const trimmed = renameValue.trim()
    const current = categories.find((c) => c.id === id)
    setRenamingCategory(null)
    if (!trimmed || trimmed === current?.name) return
    renameTriggerCategory(id, trimmed)
      .then(() => load()) // refreshes the pack_name display cache too
      .catch(() => {})
  }

  // Reparent a category — the section header's "Parent" picker, since
  // spatial drag-and-drop only reorders siblings (see reorderCategoryTo).
  // newParentId='' un-nests a child back to the top level.
  const handleReparentCategory = (cat: TriggerCategory, newParentId: string) => {
    if ((cat.parent_id || '') === newParentId) return
    const sortOrder = categories.filter(
      (c) => (c.parent_id || '') === newParentId && c.id !== cat.id,
    ).length
    setCategories((prev) =>
      prev.map((c) => (c.id === cat.id ? { ...c, parent_id: newParentId, sort_order: sortOrder } : c)),
    )
    reorderTriggerCategories([{ id: cat.id, parent_id: newParentId, sort_order: sortOrder }])
      .catch(() => load())
  }

  const handleExportCategory = (id: string) => {
    const cat = categories.find((c) => c.id === id)
    exportTriggerCategory(id)
      .then((pack) => downloadTriggerPack(pack, `pq-triggers-${slugifyFilename(cat?.name ?? id)}.json`))
      .catch((err: Error) => setError(err.message))
  }

  // Split a slash-named category (left over from a GINA import, or from
  // before nested categories existed) into a real parent/child pair.
  const handleSplitCategory = (cat: TriggerCategory) => {
    splitTriggerCategory(cat.id)
      .then(() => load())
      .catch((err: Error) => setError(err.message))
  }

  // ── Drag-and-drop (dnd-kit) ──
  // A section keyed by categoryKey ('__uncategorized__' or a category id)
  // accepts the active trigger when it isn't already filed there. Pack
  // sections are valid targets too — origin is tracked by source_pack, so
  // moving a trigger into/out of a pack category doesn't change its pack.
  const canDropTriggerOn = (categoryKey: string): boolean => {
    if (!activeTrigger) return false
    const target = categoryKey === '__uncategorized__' ? '' : categoryKey
    return (activeTrigger.category_id ?? '') !== target
  }

  // Move a trigger to another category by reassigning its category_id.
  const moveTriggerToCategory = (t: Trigger, categoryKey: string) => {
    const target = categoryKey === '__uncategorized__' ? '' : categoryKey
    if ((t.category_id ?? '') === target) return
    // Re-serialize the trigger with the new category. Sending the full request
    // (incl. source/pipe_condition) keeps pipe triggers valid; fields not in
    // the request (cooldown_secs, dedup_key) are preserved server-side.
    const req: CreateTriggerRequest = {
      name: t.name,
      enabled: t.enabled,
      pattern: t.pattern,
      actions: t.actions,
      timer_type: t.timer_type,
      timer_duration_secs: t.timer_duration_secs,
      timer_duration_capture: t.timer_duration_capture ?? '',
      timer_key_capture: t.timer_key_capture ?? '',
      timer_target_capture: t.timer_target_capture ?? '',
      worn_off_pattern: t.worn_off_pattern,
      spell_id: t.spell_id,
      display_threshold_secs: t.display_threshold_secs,
      bar_color: t.bar_color ?? '',
      pinned: t.pinned ?? false,
      timer_stack: t.timer_stack ?? false,
      timer_repeat: t.timer_repeat ?? false,
      timer_repeat_max: t.timer_repeat_max ?? 0,
      timer_if_idle: t.timer_if_idle,
      timer_clear_on_context_change: t.timer_clear_on_context_change,
      custom_group_id: t.custom_group_id ?? '',
      characters: t.characters,
      timer_alerts: t.timer_alerts ?? [],
      exclude_patterns: t.exclude_patterns ?? [],
      extra_patterns: t.extra_patterns ?? [],
      source: t.source,
      pipe_condition: t.pipe_condition,
      category_id: target,
    }
    // A failed move (rare local sqlite write) just leaves the list unchanged.
    updateTrigger(t.id, req).then(handleUpdated).catch(() => {})
  }

  // Reorder a trigger within its category by dropping it onto a sibling row.
  // Optimistically rewrites local sort_order so the row jumps immediately;
  // a failed write resyncs from the server.
  const reorderTriggerWithin = (dragged: Trigger, overId: string) => {
    if (dragged.id === overId) return
    // Seed the new manual order from the CURRENT displayed order of the whole
    // category (name/recent/manual) so a drag in any sort mode reorders from
    // what the user sees. Uses all category triggers (not the filtered view)
    // so reordering while searching can't drop hidden rows.
    const byMode = (a: Trigger, b: Trigger): number => {
      if (sortMode === 'recent') {
        const aT = a.created_at ? new Date(a.created_at).getTime() : 0
        const bT = b.created_at ? new Date(b.created_at).getTime() : 0
        return bT - aT
      }
      if (sortMode === 'name') return a.name.localeCompare(b.name)
      return (
        (a.sort_order ?? 0) - (b.sort_order ?? 0) ||
        (a.created_at ? new Date(a.created_at).getTime() : 0) -
          (b.created_at ? new Date(b.created_at).getTime() : 0)
      )
    }
    const key = dragged.category_id || ''
    const ids = triggers
      .filter((t) => (t.category_id || '') === key)
      .sort(byMode)
      .map((t) => t.id)
    const from = ids.indexOf(dragged.id)
    const to = ids.indexOf(overId)
    if (from === -1 || to === -1 || from === to) return
    const next = arrayMove(ids, from, to)
    const orderMap = new Map(next.map((id, i) => [id, i]))
    setTriggers((prev) =>
      prev.map((t) =>
        orderMap.has(t.id) ? { ...t, sort_order: orderMap.get(t.id)! } : t,
      ),
    )
    // Reordering manually implies Manual sort — switch so the change sticks
    // visibly instead of being re-sorted away by Name/Recent.
    if (sortMode !== 'manual') setSortMode('manual')
    reorderTriggers(next).catch(() => load())
  }

  // Reorder category sections by dragging their header grip onto another
  // section. Uncategorized is pinned last and can't be a drag source or
  // target. A drag only ever reorders among siblings sharing the same
  // parent — dropping onto a section at a different level is a no-op;
  // moving a category between levels is the header's "Parent" picker
  // (handleReparentCategory), not a spatial gesture.
  const reorderCategoryTo = (draggedId: string, targetId: string) => {
    if (!draggedId || draggedId === targetId || targetId === '__uncategorized__') return
    const dragged = categories.find((c) => c.id === draggedId)
    const target = categories.find((c) => c.id === targetId)
    if (!dragged || !target) return
    const parentId = dragged.parent_id || ''
    if ((target.parent_id || '') !== parentId) return
    // Order over ALL siblings (already display-sorted) so reordering while
    // filtered doesn't drop hidden ones. Move dragged to target's slot.
    const siblings = categories.filter((c) => (c.parent_id || '') === parentId)
    const order = siblings.map((c) => c.id)
    const from = order.indexOf(draggedId)
    const to = order.indexOf(targetId)
    if (from === -1 || to === -1 || from === to) return
    const next = arrayMove(order, from, to)
    const nextSortOrder = new Map(next.map((id, i) => [id, i]))
    // Optimistic: rebuild categories with the new order so sections reflow.
    setCategories((prev) =>
      prev.map((c) => (nextSortOrder.has(c.id) ? { ...c, sort_order: nextSortOrder.get(c.id)! } : c)),
    )
    const items: CategoryPlacement[] = next.map((id, i) => ({ id, parent_id: parentId, sort_order: i }))
    reorderTriggerCategories(items).catch(() => load())
  }

  // Scope collision detection by drag kind so the nested sortables don't
  // cross-talk. A category drag only sees other category sortables. A trigger
  // drag prefers the sibling row under the pointer (reorder); when the pointer
  // isn't over a row it falls back to the section drop zone (move to another
  // category, including empty ones) — a plain closestCenter can't do this
  // because a tall section's centre often beats the row actually under the
  // cursor.
  const collisionDetection = useCallback<CollisionDetection>((args) => {
    const only = (pred: (id: string) => boolean) => ({
      ...args,
      droppableContainers: args.droppableContainers.filter((c) => pred(String(c.id))),
    })
    if (String(args.active.id).startsWith(CATEGORY_PREFIX)) {
      return closestCenter(only((id) => id.startsWith(CATEGORY_PREFIX)))
    }
    const scoped = only((id) => id.startsWith(TRIGGER_PREFIX) || id.startsWith(SECTION_PREFIX))
    // pointerWithin is precise for the pointer sensor; rectIntersection covers
    // the keyboard sensor (no pointer) and drags past the list edges.
    let hits = pointerWithin(scoped)
    if (hits.length === 0) hits = rectIntersection(scoped)
    const rows = hits.filter((h) => String(h.id).startsWith(TRIGGER_PREFIX))
    if (rows.length) return rows
    const sections = hits.filter((h) => String(h.id).startsWith(SECTION_PREFIX))
    if (sections.length) return sections
    return closestCenter(only((id) => id.startsWith(SECTION_PREFIX)))
  }, [])

  const handleDragStart = (e: DragStartEvent) => {
    const id = String(e.active.id)
    if (id.startsWith(TRIGGER_PREFIX)) {
      const tid = id.slice(TRIGGER_PREFIX.length)
      setActiveTrigger(triggers.find((t) => t.id === tid) ?? null)
    } else if (id.startsWith(CATEGORY_PREFIX)) {
      setActiveCategory(id.slice(CATEGORY_PREFIX.length))
    }
  }

  const handleDragEnd = (e: DragEndEvent) => {
    const aId = String(e.active.id)
    setActiveTrigger(null)
    setActiveCategory(null)
    if (!e.over) return
    const oId = String(e.over.id)
    if (aId === oId) return

    if (aId.startsWith(CATEGORY_PREFIX)) {
      if (!oId.startsWith(CATEGORY_PREFIX)) return
      reorderCategoryTo(aId.slice(CATEGORY_PREFIX.length), oId.slice(CATEGORY_PREFIX.length))
      return
    }

    if (aId.startsWith(TRIGGER_PREFIX)) {
      const dragged = triggers.find((t) => t.id === aId.slice(TRIGGER_PREFIX.length))
      if (!dragged) return
      const srcKey = dragged.category_id || '__uncategorized__'
      if (oId.startsWith(TRIGGER_PREFIX)) {
        const overT = triggers.find((t) => t.id === oId.slice(TRIGGER_PREFIX.length))
        if (!overT) return
        const overKey = overT.category_id || '__uncategorized__'
        // Same category → reorder; different category → move (dropping onto a
        // foreign row reassigns the category, matching the old behaviour).
        if (overKey === srcKey) reorderTriggerWithin(dragged, overT.id)
        else moveTriggerToCategory(dragged, overKey)
      } else if (oId.startsWith(SECTION_PREFIX)) {
        const overKey = oId.slice(SECTION_PREFIX.length)
        if (overKey !== srcKey) moveTriggerToCategory(dragged, overKey)
      }
    }
  }

  const handleCancelCreate = () => {
    setShowCreate(false)
    setCreatePrefill(undefined)
  }

  const handleClearAll = async () => {
    setClearingAll(true)
    try {
      await clearAllTriggers()
      setTriggers([])
      setShowClearAll(false)
    } catch {
      // Surface failures by leaving the modal open; the user can retry.
    } finally {
      setClearingAll(false)
    }
  }

  const tabStyle = (t: Tab) => ({
    color: tab === t ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
    borderBottom: tab === t ? '2px solid var(--color-primary)' : '2px solid transparent',
    backgroundColor: 'transparent',
  })

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Zap size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Triggers / Timers
        </span>
        <div className="ml-auto flex items-center gap-2">
          {tab === 'triggers' && (
            <>
              <button
                onClick={load}
                className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <RefreshCw size={11} />
                Refresh
              </button>
              <button
                onClick={handleNewCategory}
                disabled={creatingCategory}
                className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
                title="Add a new empty category, then rename it inline"
              >
                <Tags size={11} />
                New Category
              </button>
              {triggers.length > 0 && (
                <button
                  onClick={() => setShowBulkEdit(true)}
                  className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    color: 'var(--color-muted-foreground)',
                    border: '1px solid var(--color-border)',
                  }}
                  title="Change actions on many triggers at once (e.g. convert TTS alerts to a sound file)"
                >
                  <SlidersHorizontal size={11} />
                  Bulk Edit
                </button>
              )}
              {triggers.length > 0 && (
                <button
                  onClick={() => setShowClearAll(true)}
                  className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    color: 'var(--color-destructive)',
                    border: '1px solid var(--color-border)',
                  }}
                  title="Remove all triggers and timers"
                >
                  <Trash2 size={11} />
                  Clear All
                </button>
              )}
              <div className="relative flex" data-tour="triggers-new">
                <button
                  onClick={() => {
                    setNewMenuOpen(false)
                    setShowTemplates(true)
                  }}
                  className="flex items-center gap-1.5 text-xs pl-3 pr-2.5 py-1.5 rounded-l font-medium"
                  style={{
                    backgroundColor: 'var(--color-primary)',
                    color: 'var(--color-background)',
                    border: '1px solid transparent',
                    borderRight: 'none',
                  }}
                >
                  <Sparkles size={11} />
                  New Trigger
                </button>
                <button
                  onClick={() => setNewMenuOpen((v) => !v)}
                  aria-label="More create options"
                  className="flex items-center justify-center px-1.5 py-1.5 rounded-r"
                  style={{
                    backgroundColor: 'var(--color-primary)',
                    color: 'var(--color-background)',
                    border: '1px solid transparent',
                    borderLeft: '1px solid rgba(0,0,0,0.2)',
                  }}
                >
                  <ChevronDown size={11} />
                </button>
                {newMenuOpen && (
                  <>
                    <div
                      onClick={() => setNewMenuOpen(false)}
                      style={{ position: 'fixed', inset: 0, zIndex: 40 }}
                    />
                    <div
                      className="absolute right-0 top-full mt-1 rounded shadow-lg overflow-hidden"
                      style={{
                        backgroundColor: 'var(--color-surface)',
                        border: '1px solid var(--color-border)',
                        zIndex: 50,
                        minWidth: 200,
                      }}
                    >
                      <button
                        onClick={() => {
                          setNewMenuOpen(false)
                          setShowSpellPicker(true)
                        }}
                        className="flex w-full items-center gap-2 px-3 py-2 text-xs text-left"
                        style={{ color: 'var(--color-foreground)' }}
                      >
                        <Sparkles size={12} style={{ color: 'var(--color-primary)' }} />
                        From spell…
                      </button>
                      <button
                        onClick={() => {
                          setNewMenuOpen(false)
                          setCreatePrefill(undefined)
                          setShowCreate(true)
                        }}
                        className="flex w-full items-center gap-2 px-3 py-2 text-xs text-left border-t"
                        style={{
                          color: 'var(--color-foreground)',
                          borderColor: 'var(--color-border)',
                        }}
                      >
                        <Plus size={12} style={{ color: 'var(--color-muted)' }} />
                        Custom trigger
                      </button>
                    </div>
                  </>
                )}
              </div>
            </>
          )}
        </div>
      </div>

      {bulkEditNotice && (
        <div
          className="flex items-center gap-1.5 px-4 py-1.5 text-xs shrink-0"
          style={{ color: 'var(--color-success)' }}
        >
          <CheckCircle2 size={13} />
          {bulkEditNotice}
        </div>
      )}

      {/* Tabs */}
      <div
        className="flex gap-0 border-b shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
        data-tour="triggers-tabs"
      >
        {(['triggers', 'history', 'packs', 'tester'] as Tab[]).map((t) => (
          <button
            key={t}
            data-tour={`triggers-tab-${t}`}
            onClick={() => setTab(t)}
            className="px-4 py-2 text-xs font-medium capitalize transition-colors"
            style={tabStyle(t)}
          >
            {t === 'triggers' && <span>Triggers ({triggers.length})</span>}
            {t === 'history' && <span>History</span>}
            {t === 'packs' && (
              <span className="inline-flex items-center gap-1.5">
                Packs
                {packUpdates.length > 0 && (
                  <NewDot title="Updates available for your installed trigger packs" />
                )}
              </span>
            )}
            {t === 'tester' && <span>Tester</span>}
          </button>
        ))}
      </div>

      {/* Tab: Triggers */}
      {tab === 'triggers' && (
        <>
          {loading ? (
            <div className="flex h-full items-center justify-center">
              <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          ) : error ? (
            <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
              <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
              <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>
                {error}
              </p>
              <button
                onClick={load}
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
          ) : (
            <>
              {/* Search + filters — pinned above the scrolling list (matches
                  the Inventory/Keys layout) so the controls stay visible while
                  the list scrolls. */}
              {triggers.length > 0 && (
                <div
                  className="flex items-center gap-2 border-b px-4 py-3 shrink-0"
                  style={{ borderColor: 'var(--color-border)' }}
                >
                  <div className="relative flex-1">
                    <Search
                      size={12}
                      className="absolute left-2.5 top-1/2 -translate-y-1/2"
                      style={{ color: 'var(--color-muted)' }}
                    />
                    <input
                      type="text"
                      placeholder="Search triggers by name, pattern, pack, or dedup key…"
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      className="w-full rounded pl-7 pr-3 py-1.5 text-xs outline-none"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                    />
                  </div>
                  {chars.length > 0 && (
                    <select
                      value={charFilter}
                      onChange={(e) => setCharFilter(e.target.value)}
                      className="rounded px-2 py-1.5 text-xs outline-none"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                      title="Show only triggers active for this character"
                    >
                      <option value="">All characters</option>
                      {chars.map((c) => (
                        <option key={c.id} value={c.name}>
                          {c.name}
                        </option>
                      ))}
                    </select>
                  )}
                  {availableClasses.length > 0 && (
                    <select
                      value={classFilter === null ? '' : String(classFilter)}
                      onChange={(e) =>
                        setClassFilter(e.target.value === '' ? null : Number(e.target.value))
                      }
                      className="rounded px-2 py-1.5 text-xs outline-none"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                      title="Filter by class of assigned characters"
                    >
                      <option value="">All classes</option>
                      {availableClasses.map((idx) => (
                        <option key={idx} value={idx}>
                          {CLASS_NAMES[idx]}
                        </option>
                      ))}
                    </select>
                  )}
                  {packsInUse.length > 0 && (
                    <select
                      value={packFilter}
                      onChange={(e) => setPackFilter(e.target.value)}
                      className="rounded px-2 py-1.5 text-xs outline-none"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                      title="Filter by trigger pack"
                    >
                      <option value="">All packs</option>
                      {packsInUse.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.label}
                        </option>
                      ))}
                    </select>
                  )}
                  <select
                    value={sortMode}
                    onChange={(e) =>
                      setSortMode(e.target.value as 'name' | 'recent' | 'manual')
                    }
                    className="rounded px-2 py-1.5 text-xs outline-none"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--color-foreground)',
                    }}
                    title="Sort triggers within each category. Manual lets you drag rows into a custom order."
                  >
                    <option value="name">Sort: Name</option>
                    <option value="recent">Sort: Recent</option>
                    <option value="manual">Sort: Manual</option>
                  </select>
                  {(search || classFilter !== null || charFilter || packFilter) && (
                    <button
                      type="button"
                      onClick={() => {
                        setSearch('')
                        setClassFilter(null)
                        setCharFilter('')
                        setPackFilter('')
                      }}
                      className="text-[11px] px-2 py-1.5 rounded"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        color: 'var(--color-muted-foreground)',
                        border: '1px solid var(--color-border)',
                      }}
                    >
                      Clear
                    </button>
                  )}
                </div>
              )}

              <div className="flex-1 overflow-y-auto p-4 space-y-3">
                {/* Create form */}
                {showCreate && (
                  <TriggerForm
                    prefill={createPrefill}
                    categories={categories}
                    onCategoriesChanged={reloadCategories}
                    timerGroups={timerGroups}
                    onTimerGroupsChanged={reloadTimerGroups}
                    onSaved={handleCreated}
                    onCancel={handleCancelCreate}
                  />
                )}

              {/* No-match state */}
              {triggers.length > 0 && filteredTriggers.length === 0 && (
                <p
                  className="text-xs italic px-1 py-2"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  No triggers match the current filters.
                </p>
              )}

              {/* Empty state */}
              {triggers.length === 0 && !showCreate && (
                <div className="flex h-full flex-col items-center justify-center gap-3 py-16 text-center">
                  <Zap size={32} style={{ color: 'var(--color-muted)' }} />
                  <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
                    No triggers yet
                  </p>
                  <p className="max-w-sm text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    Triggers watch your log and speak, show a message or start a timer.
                    The quickest start is a pack for your class; or make one from a
                    template in a few clicks.
                  </p>
                  <div className="flex gap-2">
                    <div className="relative flex">
                      <button
                        onClick={() => {
                          setEmptyNewMenuOpen(false)
                          setShowTemplates(true)
                        }}
                        className="flex items-center gap-1.5 text-xs pl-3 pr-2.5 py-1.5 rounded-l font-medium"
                        style={{
                          backgroundColor: 'var(--color-primary)',
                          color: 'var(--color-background)',
                          border: '1px solid transparent',
                          borderRight: 'none',
                        }}
                      >
                        <Sparkles size={11} />
                        New Trigger
                      </button>
                      <button
                        onClick={() => setEmptyNewMenuOpen((v) => !v)}
                        aria-label="More create options"
                        className="flex items-center justify-center px-1.5 py-1.5 rounded-r"
                        style={{
                          backgroundColor: 'var(--color-primary)',
                          color: 'var(--color-background)',
                          border: '1px solid transparent',
                          borderLeft: '1px solid rgba(0,0,0,0.2)',
                        }}
                      >
                        <ChevronDown size={11} />
                      </button>
                      {emptyNewMenuOpen && (
                        <>
                          <div
                            onClick={() => setEmptyNewMenuOpen(false)}
                            style={{ position: 'fixed', inset: 0, zIndex: 40 }}
                          />
                          <div
                            className="absolute left-0 top-full mt-1 rounded shadow-lg overflow-hidden"
                            style={{
                              backgroundColor: 'var(--color-surface)',
                              border: '1px solid var(--color-border)',
                              zIndex: 50,
                              minWidth: 200,
                            }}
                          >
                            <button
                              onClick={() => {
                                setEmptyNewMenuOpen(false)
                                setShowSpellPicker(true)
                              }}
                              className="flex w-full items-center gap-2 px-3 py-2 text-xs text-left"
                              style={{ color: 'var(--color-foreground)' }}
                            >
                              <Sparkles size={12} style={{ color: 'var(--color-primary)' }} />
                              From spell…
                            </button>
                            <button
                              onClick={() => {
                                setEmptyNewMenuOpen(false)
                                setCreatePrefill(undefined)
                                setShowCreate(true)
                              }}
                              className="flex w-full items-center gap-2 px-3 py-2 text-xs text-left border-t"
                              style={{
                                color: 'var(--color-foreground)',
                                borderColor: 'var(--color-border)',
                              }}
                            >
                              <Plus size={12} style={{ color: 'var(--color-muted)' }} />
                              Custom trigger
                            </button>
                          </div>
                        </>
                      )}
                    </div>
                    <button
                      onClick={() => setTab('packs')}
                      className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        color: 'var(--color-foreground)',
                        border: '1px solid var(--color-border)',
                      }}
                    >
                      <Package size={11} /> Activate a pack
                    </button>
                  </div>
                </div>
              )}

              {/* Trigger list — grouped by pack with collapsible sections.
                  Pointer-based DnD (dnd-kit) replaces native HTML5 DnD, which
                  was unreliable in Electron on Windows: reorder triggers within
                  a category, move a trigger to another category by dropping on
                  its section, and reorder the sections themselves. */}
              <DndContext
                sensors={sensors}
                collisionDetection={collisionDetection}
                modifiers={[restrictToVerticalAxis]}
                onDragStart={handleDragStart}
                onDragEnd={handleDragEnd}
              >
                <SortableContext
                  items={groupedTriggers.map((g) => `${CATEGORY_PREFIX}${g.categoryId}`)}
                  strategy={verticalListSortingStrategy}
                >
                  {groupedTriggers.map((group) => (
                    <CategorySection
                      key={group.categoryId}
                      group={group}
                      categories={categories}
                      collapsed={collapsedPacks.has(group.categoryId)}
                      canDrop={canDropTriggerOn(group.categoryId)}
                      isRenaming={renamingCategory === group.categoryId}
                      renameValue={renameValue}
                      onRenameValueChange={setRenameValue}
                      onToggleCollapsed={() => togglePackCollapsed(group.categoryId)}
                      onStartRename={() => {
                        const c = categories.find((x) => x.id === group.categoryId)
                        startRenameCategory(group.categoryId, c?.name ?? '')
                      }}
                      onCommitRename={() => commitRenameCategory(group.categoryId)}
                      onCancelRename={cancelRenameCategory}
                      onDeleteCategory={(c) => setDeletingCategory(c)}
                      onExportCategory={handleExportCategory}
                      onAddSubcategory={handleAddSubcategory}
                      onReparentCategory={handleReparentCategory}
                      onSplitCategory={handleSplitCategory}
                      onTriggerDeleted={handleDeleted}
                      onTriggerUpdated={handleUpdated}
                      onCategoriesChanged={reloadCategories}
                      timerGroups={timerGroups}
                      onTimerGroupsChanged={reloadTimerGroups}
                    />
                  ))}
                </SortableContext>
                <DragOverlay>
                  {activeTrigger ? (
                    <TriggerDragPreview trigger={activeTrigger} />
                  ) : activeCategory ? (
                    <CategoryDragPreview
                      label={
                        activeCategory === '__uncategorized__'
                          ? 'Uncategorized'
                          : categories.find((c) => c.id === activeCategory)?.name ?? activeCategory
                      }
                    />
                  ) : null}
                </DragOverlay>
              </DndContext>
            </div>
            </>
          )}
        </>
      )}

      {/* Tab: History */}
      {tab === 'history' && <HistoryTab />}

      {/* Tab: Packs */}
      {tab === 'packs' && (
        <PacksTab installedPacks={installedPacks} onInstalled={load} />
      )}

      {/* Tab: Tester */}
      {tab === 'tester' && (
        <TriggerTesterTab
          chars={chars}
          activeCharacter={activePlayer}
          onOpenTrigger={(name) => {
            setSearch(name)
            setTab('triggers')
          }}
        />
      )}

      {deletingCategory && (
        <DeleteCategoryModal
          category={deletingCategory}
          hasChildren={categories.some((c) => c.parent_id === deletingCategory.id)}
          onClose={() => setDeletingCategory(null)}
          onChanged={() => {
            load()
            setDeletingCategory(null)
          }}
        />
      )}

      {showClearAll && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center p-4"
          style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
          onClick={() => !clearingAll && setShowClearAll(false)}
        >
          <div
            className="flex flex-col rounded-lg shadow-2xl w-full max-w-md"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div
              className="flex items-center gap-2 border-b px-4 py-3"
              style={{ borderColor: 'var(--color-border)' }}
            >
              <AlertCircle size={16} style={{ color: 'var(--color-destructive)' }} />
              <span
                className="text-sm font-semibold"
                style={{ color: 'var(--color-foreground)' }}
              >
                Clear all triggers?
              </span>
            </div>
            <div className="px-4 py-4 space-y-2">
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                This will permanently remove all{' '}
                <span style={{ color: 'var(--color-foreground)' }}>{triggers.length}</span>{' '}
                trigger{triggers.length === 1 ? '' : 's'} and timer
                {triggers.length === 1 ? '' : 's'}, including any installed from packs.
                This cannot be undone.
              </p>
            </div>
            <div
              className="flex items-center justify-end gap-2 border-t px-4 py-3"
              style={{ borderColor: 'var(--color-border)' }}
            >
              <button
                onClick={() => setShowClearAll(false)}
                disabled={clearingAll}
                className="text-xs px-3 py-1.5 rounded"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                Cancel
              </button>
              <button
                onClick={handleClearAll}
                disabled={clearingAll}
                className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
                style={{
                  backgroundColor: 'var(--color-destructive)',
                  color: '#fff',
                  border: '1px solid transparent',
                }}
              >
                {clearingAll ? (
                  <RefreshCw size={11} className="animate-spin" />
                ) : (
                  <Trash2 size={11} />
                )}
                {clearingAll ? 'Clearing…' : 'Clear all'}
              </button>
            </div>
          </div>
        </div>
      )}

      {showTemplates && (
        <TriggerTemplatePicker
          onClose={() => setShowTemplates(false)}
          onCreated={(t) => {
            handleCreated(t)
            setShowTemplates(false)
            setTab('triggers')
          }}
          onOpenEditor={(p) => {
            setCreatePrefill(p)
            setShowCreate(true)
            setShowTemplates(false)
            setTab('triggers')
          }}
          onSpellTimer={() => {
            setShowTemplates(false)
            setShowSpellPicker(true)
          }}
          onBlank={() => {
            setCreatePrefill(undefined)
            setShowCreate(true)
            setShowTemplates(false)
            setTab('triggers')
          }}
          onPacks={() => {
            setShowTemplates(false)
            setTab('packs')
          }}
        />
      )}

      {showSpellPicker && (
        <SpellSearchPicker
          onClose={() => setShowSpellPicker(false)}
          onPick={(spell) => {
            setCreatePrefill(buildSpellTriggerPrefill(spell))
            setShowCreate(true)
            setShowSpellPicker(false)
            setTab('triggers')
          }}
        />
      )}

      {showBulkEdit && (
        <BulkActionsModal
          triggers={triggers}
          categories={categories}
          onClose={() => setShowBulkEdit(false)}
          onApplied={(res) => {
            setShowBulkEdit(false)
            load()
            setBulkEditNotice(
              `Bulk edit applied: ${res.updated} trigger${res.updated !== 1 ? 's' : ''} updated` +
                (res.skipped ? `, ${res.skipped} unchanged` : '') + '.',
            )
            setTimeout(() => setBulkEditNotice(null), 5000)
          }}
        />
      )}
    </div>
  )
}
