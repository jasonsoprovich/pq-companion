import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
import { useVoices } from '../hooks/useVoices'
import NotificationActionEditor, { NotificationTypeSelect } from '../components/NotificationActionEditor'
import SpellSearchPicker from '../components/SpellSearchPicker'
import { buildSpellTriggerPrefill, type SpellTimerTriggerPrefill } from '../lib/spellHelpers'
import {
  listTriggers,
  createTrigger,
  updateTrigger,
  deleteTrigger,
  clearAllTriggers,
  getTriggerHistory,
  getBuiltinPacks,
  installBuiltinPack,
  removeTriggerPack,
  importTriggerPack,
  exportTriggerPack,
  importGINAxml,
  listCharacters,
  listTriggerCategories,
  createTriggerCategory,
  renameTriggerCategory,
  deleteTriggerCategory,
  reorderTriggers,
  reorderTriggerCategories,
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
  Action,
  TimerType,
  TimerAlertThreshold,
  TimerAlertType,
  TriggerSource,
  PipeConditionKind,
  PipeCondition,
  ExtraPattern,
} from '../types/trigger'

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight',
  'Druid', 'Monk', 'Bard', 'Rogue', 'Shaman',
  'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

// ── Helpers ────────────────────────────────────────────────────────────────────

function formatTime(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit', second: '2-digit' })
}

function Toggle({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (v: boolean) => void
}): React.ReactElement {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className="relative inline-flex h-4 w-7 items-center rounded-full transition-colors shrink-0"
      style={{
        backgroundColor: checked ? 'var(--color-primary)' : 'var(--color-surface-3)',
      }}
    >
      <span
        className="inline-block h-3 w-3 rounded-full bg-white shadow transition-transform"
        style={{ transform: checked ? 'translateX(14px)' : 'translateX(2px)' }}
      />
    </button>
  )
}

// ── Fading-alert threshold editor ─────────────────────────────────────────────

function newTimerAlert(): TimerAlertThreshold {
  return {
    id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
    seconds: 30,
    type: 'text_to_speech',
    sound_path: '',
    volume: 80,
    tts_template: '{spell} expiring soon',
    voice: '',
    tts_volume: 80,
  }
}

interface TimerAlertRowProps {
  alert: TimerAlertThreshold
  voices: string[]
  onChange: (a: TimerAlertThreshold) => void
  onRemove: () => void
}

function TimerAlertRow({ alert, voices, onChange, onRemove }: TimerAlertRowProps): React.ReactElement {
  const inputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
    borderRadius: 4,
    padding: '3px 7px',
    fontSize: 12,
    outline: 'none',
  }

  return (
    <div
      className="rounded p-3 space-y-2"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center gap-2 flex-wrap">
        <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Alert at
          <input
            type="number"
            min={1}
            max={3600}
            value={alert.seconds}
            onChange={(e) => onChange({ ...alert, seconds: Math.max(1, parseInt(e.target.value) || 1) })}
            style={{ ...inputStyle, width: 60 }}
          />
          s remaining
        </label>

        <div className="flex items-center gap-1.5 text-xs ml-auto" style={{ color: 'var(--color-muted-foreground)' }}>
          <span>Type</span>
          <NotificationTypeSelect
            value={alert.type}
            onChange={(t) => onChange({ ...alert, type: t as TimerAlertType })}
            allowedTypes={['text_to_speech', 'play_sound']}
            className="rounded px-2 py-0.5 text-xs outline-none"
          />
        </div>

        <button
          type="button"
          onClick={onRemove}
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'var(--color-muted)',
            padding: 2,
            display: 'flex',
            alignItems: 'center',
          }}
          title="Remove alert"
        >
          <Trash2 size={13} />
        </button>
      </div>

      <NotificationActionEditor
        type={alert.type}
        voices={voices}
        ttsText={alert.tts_template}
        onTtsTextChange={(v) => onChange({ ...alert, tts_template: v })}
        ttsTextPlaceholder="{spell} expiring soon"
        voice={alert.voice}
        onVoiceChange={(v) => onChange({ ...alert, voice: v })}
        ttsVolume={alert.tts_volume}
        onTtsVolumeChange={(v) => onChange({ ...alert, tts_volume: v })}
        soundPath={alert.sound_path}
        onSoundPathChange={(v) => onChange({ ...alert, sound_path: v })}
        soundVolume={alert.volume}
        onSoundVolumeChange={(v) => onChange({ ...alert, volume: v })}
      />
    </div>
  )
}

// ── Action editor ─────────────────────────────────────────────────────────────

interface ActionEditorProps {
  action: Action
  index: number
  onChange: (index: number, action: Action) => void
  onRemove: (index: number) => void
}

function ActionEditor({ action, index, onChange, onRemove }: ActionEditorProps): React.ReactElement {
  const voices = useVoices()
  const volume0to100 = Math.round((action.volume || 1.0) * 100)

  return (
    <div
      className="rounded p-3 space-y-2"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      {/* Header row: type selector + remove */}
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-1.5 flex-1 min-w-0">
          <span className="text-xs font-medium shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            Action {index + 1}:
          </span>
          <NotificationTypeSelect
            value={action.type}
            onChange={(t) =>
              // Spread keeps type-agnostic fields (position, style overrides)
              // intact when the user flips the action type back and forth.
              onChange(index, { ...action, type: t, duration_secs: action.duration_secs || 5 })
            }
          />
        </div>
        <button
          type="button"
          onClick={() => onRemove(index)}
          className="text-xs px-1.5 py-0.5 rounded shrink-0"
          style={{ color: 'var(--color-danger)' }}
        >
          <X size={12} />
        </button>
      </div>

      <NotificationActionEditor
        type={action.type}
        voices={voices}
        overlayText={action.text}
        onOverlayTextChange={(v) => onChange(index, { ...action, text: v })}
        durationSecs={action.duration_secs || 5}
        onDurationSecsChange={(v) => onChange(index, { ...action, duration_secs: v })}
        color={action.color ?? ''}
        onColorChange={(v) => onChange(index, { ...action, color: v })}
        glowColor={action.glow_color ?? ''}
        onGlowColorChange={(v) => onChange(index, { ...action, glow_color: v })}
        fontFamily={action.font_family ?? ''}
        onFontFamilyChange={(v) => onChange(index, { ...action, font_family: v })}
        fontSize={action.font_size ?? 0}
        onFontSizeChange={(v) => onChange(index, { ...action, font_size: v })}
        onStyleReset={() =>
          onChange(index, { ...action, color: '', glow_color: '', font_family: '', font_size: 0 })
        }
        position={action.position ?? null}
        onPositionChange={(p) => onChange(index, { ...action, position: p })}
        soundPath={action.sound_path || ''}
        onSoundPathChange={(v) => onChange(index, { ...action, sound_path: v })}
        soundVolume={volume0to100}
        onSoundVolumeChange={(v) => onChange(index, { ...action, volume: v / 100 })}
        ttsText={action.text}
        onTtsTextChange={(v) => onChange(index, { ...action, text: v })}
        voice={action.voice || ''}
        onVoiceChange={(v) => onChange(index, { ...action, voice: v })}
        ttsVolume={volume0to100}
        onTtsVolumeChange={(v) => onChange(index, { ...action, volume: v / 100 })}
      />
    </div>
  )
}

// ── Trigger form ──────────────────────────────────────────────────────────────

interface TriggerFormProps {
  initial?: Trigger
  prefill?: SpellTimerTriggerPrefill
  /** Categories for the dropdown; built-in entries are shown but flagged. */
  categories: TriggerCategory[]
  /** Called after the form creates a new category inline, so the parent
   *  can refresh its category list. */
  onCategoriesChanged: () => void
  onSaved: (t: Trigger) => void
  onCancel: () => void
}

function TriggerForm({ initial, prefill, categories, onCategoriesChanged, onSaved, onCancel }: TriggerFormProps): React.ReactElement {
  const [name, setName] = useState(initial?.name ?? prefill?.name ?? '')
  // Category (pack_name). Defaults to the trigger's current category on edit,
  // Uncategorized ('') on create. The "__new__" sentinel opens an inline
  // create input.
  const [packName, setPackName] = useState(initial?.pack_name ?? '')
  const [creatingCat, setCreatingCat] = useState(false)
  const [newCatName, setNewCatName] = useState('')
  const [addingCat, setAddingCat] = useState(false)
  const [catError, setCatError] = useState<string | null>(null)
  const [pattern, setPattern] = useState(initial?.pattern ?? prefill?.pattern ?? '')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [source, setSource] = useState<TriggerSource>(initial?.source === 'pipe' ? 'pipe' : 'log')
  const [pipeKind, setPipeKind] = useState<PipeConditionKind>(initial?.pipe_condition?.kind ?? 'target_hp_below')
  const [pipeHPThreshold, setPipeHPThreshold] = useState<number>(initial?.pipe_condition?.hp_threshold ?? 20)
  const [pipeTargetName, setPipeTargetName] = useState<string>(initial?.pipe_condition?.target_name ?? '')
  const [pipeSpellName, setPipeSpellName] = useState<string>(initial?.pipe_condition?.spell_name ?? '')
  const [pipeCommandText, setPipeCommandText] = useState<string>(initial?.pipe_condition?.text ?? '')
  const [pipeError, setPipeError] = useState<string | null>(null)
  const [actions, setActions] = useState<Action[]>(
    initial?.actions ?? [{ type: 'overlay_text', text: prefill?.name ?? '', duration_secs: 5, color: '', sound_path: '', volume: 0, voice: '' }],
  )
  const [timerType, setTimerType] = useState<TimerType>(initial?.timer_type ?? prefill?.timerType ?? 'none')
  const [timerDuration, setTimerDuration] = useState(initial?.timer_duration_secs ?? prefill?.timerDurationSecs ?? 0)
  const [timerDurationCapture, setTimerDurationCapture] = useState(initial?.timer_duration_capture ?? '')
  const [timerKeyCapture, setTimerKeyCapture] = useState(initial?.timer_key_capture ?? '')
  const [timerTargetCapture, setTimerTargetCapture] = useState(initial?.timer_target_capture ?? '')
  const [wornOffPattern, setWornOffPattern] = useState(initial?.worn_off_pattern ?? prefill?.wornOffPattern ?? '')
  const [displayThreshold, setDisplayThreshold] = useState(initial?.display_threshold_secs ?? 0)
  const [timerAlerts, setTimerAlerts] = useState<TimerAlertThreshold[]>(
    initial?.timer_alerts ?? [],
  )
  // Exclude patterns are stored as one regex per line in the textarea so the
  // user can paste a curated list without juggling per-row controls. Empty
  // lines are dropped on save.
  const [excludePatternsText, setExcludePatternsText] = useState<string>(
    (initial?.exclude_patterns ?? []).join('\n'),
  )
  const [excludeErrors, setExcludeErrors] = useState<string[]>([])
  // Additional match patterns — per-row controls (unlike excludes) because
  // each carries its own enabled toggle. Empty rows are dropped on save.
  const [extraPatterns, setExtraPatterns] = useState<ExtraPattern[]>(
    initial?.extra_patterns ?? [],
  )
  const [extraErrors, setExtraErrors] = useState<string[]>([])
  const voices = useVoices()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [patternError, setPatternError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  const activePlayer = useActivePlayerName()
  const [allChars, setAllChars] = useState<Character[]>([])
  const [selectedChars, setSelectedChars] = useState<Set<string>>(
    () => new Set(initial?.characters ?? []),
  )

  // Load known characters once for the chip selector. New triggers default to
  // the active character only; existing triggers preserve their saved set.
  useEffect(() => {
    listCharacters()
      .then((resp) => {
        setAllChars(resp.characters)
        if (!initial) {
          // First mount on a new trigger: pre-select the active char if known,
          // otherwise leave empty (the user can pick before saving).
          const fallback = resp.active || ''
          setSelectedChars((prev) => {
            if (prev.size > 0) return prev
            return fallback ? new Set([fallback]) : prev
          })
        }
      })
      .catch(() => {})
  }, [initial])

  // If activePlayer arrives after the chars list and no chars are selected
  // yet (new trigger, no active at first), pre-select the active player.
  useEffect(() => {
    if (initial) return
    if (!activePlayer) return
    setSelectedChars((prev) => (prev.size > 0 ? prev : new Set([activePlayer])))
  }, [activePlayer, initial])

  useEffect(() => { nameRef.current?.focus() }, [])

  const toggleChar = (charName: string) => {
    setSelectedChars((prev) => {
      const next = new Set(prev)
      if (next.has(charName)) next.delete(charName)
      else next.add(charName)
      return next
    })
  }

  const selectAllChars = () => {
    setSelectedChars(new Set(allChars.map((c) => c.name)))
  }

  const clearAllChars = () => {
    setSelectedChars(new Set())
  }

  const validatePattern = (p: string) => {
    try {
      // The backend accepts Go (?P<name>…) named groups; JS only knows
      // (?<name>…). Normalize before validating so documented syntax passes.
      new RegExp(p.replace(/\(\?P</g, '(?<'))
      setPatternError(null)
      return true
    } catch (e) {
      setPatternError((e as Error).message)
      return false
    }
  }

  const handlePatternChange = (v: string) => {
    setPattern(v)
    if (v) validatePattern(v)
    else setPatternError(null)
  }

  const handleActionChange = (index: number, action: Action) => {
    setActions((prev) => prev.map((a, i) => (i === index ? action : a)))
  }

  const handleActionRemove = (index: number) => {
    setActions((prev) => prev.filter((_, i) => i !== index))
  }

  const handleAddAction = () => {
    setActions((prev) => [...prev, { type: 'overlay_text', text: '', duration_secs: 5, color: '', sound_path: '', volume: 0, voice: '' }])
  }

  const handleCategorySelect = (v: string) => {
    if (v === '__new__') {
      setCreatingCat(true)
      setCatError(null)
      return
    }
    setPackName(v)
  }

  const handleAddCategory = () => {
    const trimmed = newCatName.trim()
    if (!trimmed) return
    setAddingCat(true)
    setCatError(null)
    createTriggerCategory(trimmed)
      .then((cat) => {
        setPackName(cat.name)
        setCreatingCat(false)
        setNewCatName('')
        onCategoriesChanged()
      })
      .catch((err: Error) => setCatError(err.message))
      .finally(() => setAddingCat(false))
  }

  const buildPipeCondition = (): PipeCondition | null => {
    switch (pipeKind) {
      case 'target_hp_below': {
        const t = Math.max(1, Math.min(99, Math.round(pipeHPThreshold)))
        return { kind: 'target_hp_below', hp_threshold: t }
      }
      case 'target_name':
        if (!pipeTargetName.trim()) return null
        return { kind: 'target_name', target_name: pipeTargetName.trim() }
      case 'buff_landed':
        if (!pipeSpellName.trim()) return null
        return { kind: 'buff_landed', spell_name: pipeSpellName.trim() }
      case 'buff_faded':
        if (!pipeSpellName.trim()) return null
        return { kind: 'buff_faded', spell_name: pipeSpellName.trim() }
      case 'pipe_command':
        if (!pipeCommandText.trim()) return null
        return { kind: 'pipe_command', text: pipeCommandText.trim() }
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    let pipeCondition: PipeCondition | undefined
    let excludeList: string[] = []
    let extraList: ExtraPattern[] = []
    if (source === 'pipe') {
      const pc = buildPipeCondition()
      if (!pc) {
        setPipeError('Fill in the field for this pipe condition kind.')
        return
      }
      setPipeError(null)
      pipeCondition = pc
    } else {
      if (!pattern.trim()) return
      if (!validatePattern(pattern)) return

      excludeList = excludePatternsText
        .split('\n')
        .map((s) => s.trim())
        .filter((s) => s.length > 0)
      const errs: string[] = []
      for (const ex of excludeList) {
        try { new RegExp(ex) } catch (e) { errs.push(`${ex} — ${(e as Error).message}`) }
      }
      if (errs.length > 0) {
        setExcludeErrors(errs)
        return
      }
      setExcludeErrors([])

      extraList = extraPatterns
        .map((ep) => ({ ...ep, pattern: ep.pattern.trim() }))
        .filter((ep) => ep.pattern.length > 0)
      const extraErrs: string[] = []
      for (const ep of extraList) {
        try {
          new RegExp(ep.pattern.replace(/\(\?P</g, '(?<'))
        } catch (e) {
          extraErrs.push(`${ep.pattern} — ${(e as Error).message}`)
        }
      }
      if (extraErrs.length > 0) {
        setExtraErrors(extraErrs)
        return
      }
      setExtraErrors([])
    }

    const req: CreateTriggerRequest = {
      name: name.trim(),
      enabled,
      // Pipe triggers don't use the regex pattern; send empty so the
      // backend doesn't try to compile one.
      pattern: source === 'pipe' ? '' : pattern.trim(),
      actions,
      timer_type: timerType,
      timer_duration_secs: timerType === 'none' ? 0 : Math.max(0, timerDuration),
      timer_duration_capture:
        source === 'pipe' || timerType === 'none' ? '' : timerDurationCapture.trim(),
      timer_key_capture:
        source === 'pipe' || timerType === 'none' ? '' : timerKeyCapture.trim(),
      timer_target_capture:
        source === 'pipe' || timerType === 'none' ? '' : timerTargetCapture.trim(),
      worn_off_pattern: source === 'pipe' || timerType === 'none' ? '' : wornOffPattern.trim(),
      spell_id: initial?.spell_id ?? prefill?.spellId ?? 0,
      display_threshold_secs: timerType === 'none' ? 0 : Math.max(0, displayThreshold),
      characters: Array.from(selectedChars),
      timer_alerts: timerType === 'none' ? [] : timerAlerts,
      exclude_patterns: source === 'pipe' ? [] : excludeList,
      extra_patterns: source === 'pipe' ? [] : extraList,
      source,
      pipe_condition: pipeCondition,
      pack_name: packName,
    }

    setSubmitting(true)
    setError(null)

    const promise = initial
      ? updateTrigger(initial.id, req)
      : createTrigger(req)

    promise
      .then((t) => onSaved(t))
      .catch((err: Error) => {
        setError(err.message)
        setSubmitting(false)
      })
  }

  const inputStyle = {
    backgroundColor: 'var(--color-surface-2)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
  }

  const canSubmit = name.trim() && pattern.trim() && !patternError && !submitting

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-lg p-4 space-y-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-primary)' }}
    >
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {initial ? 'Edit Trigger' : 'New Trigger'}
        </p>
        <div className="flex items-center gap-2">
          <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Enabled</span>
          <Toggle checked={enabled} onChange={setEnabled} />
        </div>
      </div>

      {/* Name */}
      <div className="space-y-1">
        <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
          Name
        </label>
        <input
          ref={nameRef}
          type="text"
          placeholder="e.g. Mez Wore Off"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded px-3 py-1.5 text-sm outline-none"
          style={inputStyle}
          disabled={submitting}
        />
      </div>

      {/* Category */}
      <div className="space-y-1">
        <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
          Category
        </label>
        {creatingCat ? (
          <div className="space-y-1">
            <div className="flex gap-2">
              <input
                type="text"
                autoFocus
                placeholder="New category name"
                value={newCatName}
                onChange={(e) => setNewCatName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') { e.preventDefault(); handleAddCategory() }
                  else if (e.key === 'Escape') { e.preventDefault(); setCreatingCat(false); setCatError(null) }
                }}
                className="flex-1 rounded px-3 py-1.5 text-sm outline-none"
                style={inputStyle}
                disabled={addingCat}
              />
              <button
                type="button"
                onClick={handleAddCategory}
                disabled={addingCat || !newCatName.trim()}
                className="rounded px-3 py-1.5 text-xs font-semibold"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-background)',
                  border: '1px solid transparent',
                  cursor: 'pointer',
                }}
              >
                Add
              </button>
              <button
                type="button"
                onClick={() => { setCreatingCat(false); setCatError(null); setNewCatName('') }}
                disabled={addingCat}
                className="rounded px-3 py-1.5 text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                  cursor: 'pointer',
                }}
              >
                Cancel
              </button>
            </div>
            {catError && (
              <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{catError}</p>
            )}
          </div>
        ) : (
          <select
            value={packName}
            onChange={(e) => handleCategorySelect(e.target.value)}
            className="w-full rounded px-3 py-1.5 text-sm outline-none"
            style={inputStyle}
            disabled={submitting}
          >
            <option value="">Uncategorized</option>
            {categories.map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}{c.is_builtin ? ' (pack)' : ''}
              </option>
            ))}
            {/* Defensive: keep the current value selectable even if the
                category list hasn't loaded or doesn't include it yet. */}
            {packName && !categories.some((c) => c.name === packName) && (
              <option value={packName}>{packName}</option>
            )}
            <option value="__new__">+ New category…</option>
          </select>
        )}
      </div>

      {/* Source toggle */}
      <div className="space-y-1">
        <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
          Match source
        </label>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={() => setSource('log')}
            className="flex-1 rounded px-3 py-1.5 text-xs font-semibold"
            style={{
              backgroundColor: source === 'log' ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: source === 'log' ? '#fff' : 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
              cursor: 'pointer',
            }}
          >
            Log line (regex)
          </button>
          <button
            type="button"
            onClick={() => setSource('pipe')}
            className="flex-1 rounded px-3 py-1.5 text-xs font-semibold"
            style={{
              backgroundColor: source === 'pipe' ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: source === 'pipe' ? '#fff' : 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
              cursor: 'pointer',
            }}
          >
            Zeal pipe event
          </button>
        </div>
        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Pipe triggers fire on live game state (target HP, buff changes, /pipe commands) and require Zeal running.
        </p>
      </div>

      {/* Pattern — log source only */}
      {source === 'log' && (
        <div className="space-y-1">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Pattern (regex)
          </label>
          <input
            type="text"
            placeholder="e.g. Your .+ spell has worn off\."
            value={pattern}
            onChange={(e) => handlePatternChange(e.target.value)}
            className="w-full rounded px-3 py-1.5 text-sm outline-none font-mono"
            style={{
              ...inputStyle,
              border: `1px solid ${patternError ? 'var(--color-danger)' : 'var(--color-border)'}`,
            }}
            disabled={submitting}
          />
          {patternError && (
            <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>
              {patternError}
            </p>
          )}
          <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            Matched against the log message text (after the timestamp).
          </p>
          <p className="text-[10px] leading-snug" style={{ color: 'var(--color-muted)' }}>
            Reuse capture groups in the alert/TTS text: <span className="font-mono">{'{1}'}</span>,{' '}
            <span className="font-mono">{'{2}'}</span> (or <span className="font-mono">$1</span>,{' '}
            GINA-style <span className="font-mono">{'{S1}'}</span>) for numbered groups,{' '}
            <span className="font-mono">{'{name}'}</span> for named groups like{' '}
            <span className="font-mono">(?P&lt;name&gt;…)</span>. Built-ins:{' '}
            <span className="font-mono">{'{c}'}</span> = your character (works in the
            pattern too), <span className="font-mono">{'{target}'}</span> = current target.
          </p>

          {/* Additional patterns — any enabled pattern fires the trigger. */}
          <div className="space-y-1 pt-1">
            <div className="flex items-center justify-between">
              <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                Additional patterns
              </label>
              <button
                type="button"
                onClick={() => setExtraPatterns((prev) => [...prev, { pattern: '', enabled: true }])}
                className="text-[11px] rounded px-2 py-0.5"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)' }}
                disabled={submitting}
              >
                + Add pattern
              </button>
            </div>
            {extraPatterns.length > 0 && (
              <p className="text-[10px] leading-snug" style={{ color: 'var(--color-muted)' }}>
                The trigger fires when the main pattern <em>or</em> any enabled pattern below
                matches; the matching pattern's capture groups fill the alert text. Uncheck a
                row to disable it without deleting.
              </p>
            )}
            {extraPatterns.map((ep, i) => (
              <div key={i} className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={ep.enabled}
                  onChange={(e) => {
                    const v = e.target.checked
                    setExtraPatterns((prev) => prev.map((p, j) => (j === i ? { ...p, enabled: v } : p)))
                  }}
                  disabled={submitting}
                />
                <input
                  type="text"
                  value={ep.pattern}
                  onChange={(e) => {
                    const v = e.target.value
                    setExtraPatterns((prev) => prev.map((p, j) => (j === i ? { ...p, pattern: v } : p)))
                    if (extraErrors.length > 0) setExtraErrors([])
                  }}
                  className="flex-1 rounded px-3 py-1.5 text-sm outline-none font-mono"
                  style={{ ...inputStyle, opacity: ep.enabled ? 1 : 0.5 }}
                  placeholder="^(.+) is behind you\.$"
                  disabled={submitting}
                />
                {timerType !== 'none' && (
                  <input
                    type="number"
                    min={0}
                    value={ep.timer_duration_secs ?? 0}
                    onChange={(e) => {
                      const v = Math.max(0, parseInt(e.target.value) || 0)
                      setExtraPatterns((prev) => prev.map((p, j) => (j === i ? { ...p, timer_duration_secs: v } : p)))
                    }}
                    className="w-16 rounded px-2 py-1.5 text-xs outline-none text-center"
                    style={{ ...inputStyle, opacity: ep.enabled ? 1 : 0.5 }}
                    disabled={submitting}
                    title="Timer duration override (seconds) when THIS pattern matches. 0 = use the trigger's duration above."
                  />
                )}
                <button
                  type="button"
                  onClick={() => setExtraPatterns((prev) => prev.filter((_, j) => j !== i))}
                  style={{ color: 'var(--color-muted-foreground)' }}
                  disabled={submitting}
                >
                  <X size={13} />
                </button>
              </div>
            ))}
            {extraErrors.map((msg, i) => (
              <p key={i} className="text-[11px]" style={{ color: 'var(--color-danger)' }}>
                {msg}
              </p>
            ))}
          </div>
        </div>
      )}

      {/* Pipe condition — pipe source only */}
      {source === 'pipe' && (
        <div className="space-y-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}>
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Pipe condition
          </label>
          <select
            value={pipeKind}
            onChange={(e) => { setPipeKind(e.target.value as PipeConditionKind); setPipeError(null) }}
            className="w-full rounded px-3 py-1.5 text-sm outline-none"
            style={inputStyle}
            disabled={submitting}
          >
            <option value="target_hp_below">Target HP drops below…</option>
            <option value="target_name">Target name becomes…</option>
            <option value="buff_landed">Buff lands on me</option>
            <option value="buff_faded">Buff fades from me</option>
            <option value="pipe_command">In-game /pipe text</option>
          </select>

          {pipeKind === 'target_hp_below' && (
            <div className="flex items-center gap-3">
              <input
                type="range"
                min={1}
                max={99}
                value={pipeHPThreshold}
                onChange={(e) => setPipeHPThreshold(Number(e.target.value))}
                style={{ flex: 1 }}
                disabled={submitting}
              />
              <span
                className="tabular-nums text-xs"
                style={{ color: 'var(--color-foreground)', minWidth: '3.5rem' }}
              >
                {pipeHPThreshold}% HP
              </span>
            </div>
          )}
          {pipeKind === 'target_name' && (
            <input
              type="text"
              placeholder="e.g. Vulak`Aerr"
              value={pipeTargetName}
              onChange={(e) => setPipeTargetName(e.target.value)}
              className="w-full rounded px-3 py-1.5 text-sm outline-none"
              style={inputStyle}
              disabled={submitting}
            />
          )}
          {(pipeKind === 'buff_landed' || pipeKind === 'buff_faded') && (
            <input
              type="text"
              placeholder="e.g. Shield of Words"
              value={pipeSpellName}
              onChange={(e) => setPipeSpellName(e.target.value)}
              className="w-full rounded px-3 py-1.5 text-sm outline-none"
              style={inputStyle}
              disabled={submitting}
            />
          )}
          {pipeKind === 'pipe_command' && (
            <div className="space-y-1">
              <input
                type="text"
                placeholder="e.g. pull"
                value={pipeCommandText}
                onChange={(e) => setPipeCommandText(e.target.value)}
                className="w-full rounded px-3 py-1.5 text-sm outline-none"
                style={inputStyle}
                disabled={submitting}
              />
              <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Triggers when you type <code>/pipe {pipeCommandText || '<text>'}</code> in-game (exact, case-sensitive).
              </p>
            </div>
          )}

          {pipeError && (
            <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>
              {pipeError}
            </p>
          )}
        </div>
      )}

      {/* Exclude patterns — log source only (pipe matches are typed, not regex) */}
      {source === 'log' && (
      <div className="space-y-1">
        <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
          Exclude patterns (one regex per line, optional)
        </label>
        <textarea
          value={excludePatternsText}
          onChange={(e) => { setExcludePatternsText(e.target.value); if (excludeErrors.length > 0) setExcludeErrors([]) }}
          rows={Math.min(8, Math.max(2, excludePatternsText.split('\n').length))}
          className="w-full rounded px-3 py-1.5 text-xs outline-none font-mono"
          style={{
            ...inputStyle,
            border: `1px solid ${excludeErrors.length > 0 ? 'var(--color-danger)' : 'var(--color-border)'}`,
            resize: 'vertical',
          }}
          disabled={submitting}
          placeholder={`\\b[Mm]aster[.!]\ntells you, '[Tt]hat'll be `}
        />
        {excludeErrors.length > 0 ? (
          <ul className="text-[11px]" style={{ color: 'var(--color-danger)' }}>
            {excludeErrors.map((m, i) => (<li key={i}>{m}</li>))}
          </ul>
        ) : (
          <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            Suppresses this trigger when any of these regexes also matches the same line — useful for filtering
            pet/merchant tells out of a broad pattern, or silencing specific bazaar-trader names.
          </p>
        )}
      </div>
      )}

      {/* Characters */}
      <div className="space-y-1.5">
        <div className="flex items-center justify-between">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Active for characters
          </label>
          {allChars.length > 0 && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={selectAllChars}
                className="text-[10px]"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                All
              </button>
              <span style={{ color: 'var(--color-border)' }}>·</span>
              <button
                type="button"
                onClick={clearAllChars}
                className="text-[10px]"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                None
              </button>
            </div>
          )}
        </div>
        {allChars.length === 0 ? (
          <p className="text-[11px] italic" style={{ color: 'var(--color-muted)' }}>
            No characters discovered yet — this trigger will fire for any active character.
          </p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {allChars.map((c) => {
              const sel = selectedChars.has(c.name)
              const className = c.class >= 0 ? CLASS_NAMES[c.class] : null
              return (
                <button
                  key={c.id}
                  type="button"
                  onClick={() => toggleChar(c.name)}
                  className="text-[11px] px-2 py-0.5 rounded font-medium"
                  title={className ? `${c.name} — ${className}` : c.name}
                  style={{
                    backgroundColor: sel ? 'var(--color-primary)' : 'var(--color-surface-2)',
                    color: sel ? 'var(--color-primary-foreground)' : 'var(--color-muted-foreground)',
                    border: `1px solid ${sel ? 'var(--color-primary)' : 'var(--color-border)'}`,
                    opacity: sel ? 1 : 0.75,
                  }}
                >
                  {c.name}
                </button>
              )
            })}
          </div>
        )}
        {allChars.length > 0 && selectedChars.size === 0 && (
          <p className="text-[11px] italic" style={{ color: 'var(--color-warning)' }}>
            No characters selected — trigger will fire for any active character.
          </p>
        )}
      </div>

      {/* Timer */}
      <div className="space-y-2">
        <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
          Spell timer
        </label>
        <div className="flex gap-1">
          {(['none', 'buff', 'detrimental', 'custom'] as TimerType[]).map((tt) => {
            const active = timerType === tt
            return (
              <button
                key={tt}
                type="button"
                onClick={() => setTimerType(tt)}
                className="flex-1 rounded px-2 py-1 text-xs font-medium capitalize"
                style={{
                  backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: active ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                  border: '1px solid transparent',
                }}
                title={tt === 'custom' ? 'Counts down on the Custom Timers overlay' : undefined}
              >
                {tt === 'none' ? 'No timer' : tt}
              </button>
            )
          })}
        </div>
        {timerType !== 'none' && (
          <>
            <div className="flex gap-2">
              <div className="flex items-center gap-1.5 flex-1">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Duration (s)
                </label>
                <input
                  type="number"
                  min={0}
                  value={timerDuration}
                  onChange={(e) => setTimerDuration(Math.max(0, parseInt(e.target.value) || 0))}
                  className="w-20 rounded px-2 py-0.5 text-xs outline-none text-center"
                  style={inputStyle}
                  disabled={submitting}
                />
              </div>
              {source === 'log' && (
                <input
                  type="text"
                  placeholder="worn-off regex (optional)"
                  value={wornOffPattern}
                  onChange={(e) => setWornOffPattern(e.target.value)}
                  className="flex-1 rounded px-2 py-0.5 text-xs outline-none font-mono"
                  style={inputStyle}
                  disabled={submitting}
                />
              )}
            </div>
            {source === 'log' && (
              <div className="flex items-center gap-1.5">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Duration from capture
                </label>
                <input
                  type="text"
                  value={timerDurationCapture}
                  onChange={(e) => setTimerDurationCapture(e.target.value)}
                  placeholder="e.g. 2"
                  className="w-20 rounded px-2 py-0.5 text-xs outline-none text-center font-mono"
                  style={inputStyle}
                  disabled={submitting}
                  title="Capture group number or name whose text supplies the duration (400 / 6:40 / 6m40s). Falls back to the fixed duration when it doesn't parse."
                />
                <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                  group # or name; parses 400 / 6:40 / 6m40s
                </span>
              </div>
            )}
            {source === 'log' && (
              <div className="flex items-center gap-1.5">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Timer name from capture
                </label>
                <input
                  type="text"
                  value={timerKeyCapture}
                  onChange={(e) => setTimerKeyCapture(e.target.value)}
                  placeholder="e.g. 1"
                  className="w-20 rounded px-2 py-0.5 text-xs outline-none text-center font-mono"
                  style={inputStyle}
                  disabled={submitting}
                  title="Capture group number or name whose text names the timer (e.g. the spell name) — each captured value runs its own countdown. The worn-off pattern must capture the same value. Empty = use the trigger name."
                />
                <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                  one timer per captured spell name; empty = trigger name
                </span>
              </div>
            )}
            {source === 'log' && (
              <div className="flex items-center gap-1.5">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Target from capture
                </label>
                <input
                  type="text"
                  value={timerTargetCapture}
                  onChange={(e) => setTimerTargetCapture(e.target.value)}
                  placeholder="e.g. target"
                  className="w-20 rounded px-2 py-0.5 text-xs outline-none text-center font-mono"
                  style={inputStyle}
                  disabled={submitting}
                  title="Capture group number or name whose text is the spell's target — shown as the grey 'on <target>' suffix in the buff/detrim overlay. Capture it from a 'lands on other' pattern, e.g. (?P<target>[A-Z][a-zA-Z']{2,14}) experiences visions of grandeur. Empty (or an unmatched group, like a self-cast branch) = no suffix."
                />
                <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                  shows “on &lt;name&gt;”; capture from the lands-on-other line
                </span>
              </div>
            )}
            <div className="flex items-center gap-1.5">
              <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                Display threshold (s)
              </label>
              <input
                type="number"
                min={0}
                value={displayThreshold}
                onChange={(e) => setDisplayThreshold(Math.max(0, parseInt(e.target.value) || 0))}
                className="w-20 rounded px-2 py-0.5 text-xs outline-none text-center"
                style={inputStyle}
                disabled={submitting}
                title="Hide this timer until remaining ≤ this value. 0 uses the global default."
              />
              <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                0 = use global default
              </span>
            </div>

            {/* Fading-soon alerts */}
            <div className="space-y-2 pt-2" style={{ borderTop: '1px solid var(--color-border)' }}>
              <div className="flex items-center justify-between">
                <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                  Fading-soon alerts
                </label>
                <button
                  type="button"
                  onClick={() => setTimerAlerts((prev) => [...prev, newTimerAlert()])}
                  className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    color: 'var(--color-muted-foreground)',
                    border: '1px solid var(--color-border)',
                  }}
                >
                  <Plus size={10} /> Add
                </button>
              </div>
              {timerAlerts.length === 0 ? (
                <p className="text-[11px] italic" style={{ color: 'var(--color-muted)' }}>
                  No fading alerts — timer counts down silently. Add one to get notified before it expires.
                </p>
              ) : (
                <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  Use <code style={{ color: 'var(--color-foreground)' }}>{'{spell}'}</code> in the message to insert the spell name.
                </p>
              )}
              {timerAlerts.map((alert, i) => (
                <TimerAlertRow
                  key={alert.id}
                  alert={alert}
                  voices={voices}
                  onChange={(next) =>
                    setTimerAlerts((prev) => prev.map((a, idx) => (idx === i ? next : a)))
                  }
                  onRemove={() =>
                    setTimerAlerts((prev) => prev.filter((_, idx) => idx !== i))
                  }
                />
              ))}
            </div>
          </>
        )}
      </div>

      {/* Actions */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Actions
          </label>
          <button
            type="button"
            onClick={handleAddAction}
            className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <Plus size={10} /> Add
          </button>
        </div>
        {actions.length === 0 && (
          <p className="text-[11px] italic" style={{ color: 'var(--color-muted)' }}>
            No actions — trigger will be logged to history only.
          </p>
        )}
        {actions.map((action, i) => (
          <ActionEditor
            key={i}
            action={action}
            index={i}
            onChange={handleActionChange}
            onRemove={handleActionRemove}
          />
        ))}
      </div>

      {error && (
        <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}

      <div className="flex items-center gap-2 justify-end pt-1">
        <button
          type="button"
          onClick={onCancel}
          disabled={submitting}
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
          type="submit"
          disabled={!canSubmit}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: canSubmit ? 'var(--color-primary)' : 'var(--color-surface-2)',
            color: canSubmit ? 'var(--color-background)' : 'var(--color-muted)',
            border: '1px solid transparent',
            cursor: canSubmit ? 'pointer' : 'not-allowed',
          }}
        >
          {submitting ? <RefreshCw size={11} className="animate-spin" /> : <Zap size={11} />}
          {submitting ? 'Saving…' : initial ? 'Save Changes' : 'Create Trigger'}
        </button>
      </div>
    </form>
  )
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
  onDeleted: (id: string) => void
  onUpdated: (t: Trigger) => void
}

function TriggerRow({
  trigger,
  categories,
  onCategoriesChanged,
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
            {trigger.timer_type && trigger.timer_type !== 'none' && (
              <span
                className="text-[10px] px-1.5 py-0.5 rounded shrink-0 font-medium capitalize"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: trigger.timer_type === 'buff' ? '#22c55e' : '#ef4444',
                  border: `1px solid ${trigger.timer_type === 'buff' ? '#22c55e' : '#ef4444'}`,
                }}
              >
                {trigger.timer_type} · {trigger.timer_duration_secs}s
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
  group: { packName: string; items: Trigger[] }
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
  onTriggerDeleted: (id: string) => void
  onTriggerUpdated: (t: Trigger) => void
  onCategoriesChanged: () => void
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
  onTriggerDeleted,
  onTriggerUpdated,
  onCategoriesChanged,
}: CategorySectionProps): React.ReactElement {
  const packName = group.packName
  const isUncategorized = packName === '__uncategorized__'
  const reorderableSection = !isUncategorized
  const label = isUncategorized ? 'Uncategorized' : packName
  const cat = categories.find((c) => c.name === packName)
  const isCustom = !!cat?.custom

  // Category reorder: dragging the header grip moves the whole section.
  // Uncategorized is pinned last, so its sortable is disabled.
  const {
    setNodeRef: setSortRef,
    attributes,
    listeners,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: `${CATEGORY_PREFIX}${packName}`, disabled: !reorderableSection })

  // Trigger move: dropping a trigger anywhere on this section reassigns it here.
  const { setNodeRef: setDropRef, isOver, active } = useDroppable({
    id: `${SECTION_PREFIX}${packName}`,
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
  const fileInputRef = useRef<HTMLInputElement>(null)
  const ginaInputRef = useRef<HTMLInputElement>(null)

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
  }, [])

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
      .then((pack) => {
        const blob = new Blob([JSON.stringify(pack, null, 2)], { type: 'application/json' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = 'pq-triggers.json'
        a.click()
        URL.revokeObjectURL(url)
      })
      .catch((err: Error) => setError(err.message))
  }

  const handleImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const pack = JSON.parse(ev.target?.result as string) as TriggerPack
        importTriggerPack(pack)
          .then(() => {
            onInstalled()
            setInstalled(pack.pack_name)
            setTimeout(() => setInstalled(null), 3000)
          })
          .catch((err: Error) => setError(err.message))
      } catch {
        setError('Invalid JSON file')
      }
    }
    reader.readAsText(file)
    // Reset so same file can be re-imported
    e.target.value = ''
  }

  const handleGINAImport = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      const xml = ev.target?.result as string
      const packName = file.name.replace(/\.(xml|gtp)$/i, '') || 'GINA Import'
      importGINAxml(xml, packName)
        .then((r) => {
          onInstalled()
          setInstalled(`${r.pack_name} (${r.imported})`)
          setTimeout(() => setInstalled(null), 3000)
        })
        .catch((err: Error) => setError(err.message))
    }
    reader.readAsText(file)
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
      {/* Import / Export */}
      <div
        className="rounded-lg p-3 space-y-2"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Import / Export
        </p>
        <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Share trigger packs with other players or back up your triggers as JSON.
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
            onClick={() => fileInputRef.current?.click()}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <Upload size={12} /> Import Pack
          </button>
          <button
            onClick={() => ginaInputRef.current?.click()}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Import a GINA trigger share (.xml / .gtp)"
          >
            <Upload size={12} /> Import GINA
          </button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            onChange={handleImport}
            className="hidden"
          />
          <input
            ref={ginaInputRef}
            type="file"
            accept=".xml,.gtp,application/xml,text/xml"
            onChange={handleGINAImport}
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
                      </div>
                      <p className="text-[11px] mt-1" style={{ color: 'var(--color-muted-foreground)' }}>
                        {pack.description}
                      </p>
                    </div>
                  </div>
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
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

type Tab = 'triggers' | 'history' | 'packs'

// ── Delete category modal ───────────────────────────────────────────────────

interface DeleteCategoryModalProps {
  category: TriggerCategory
  onClose: () => void
  // Reload triggers + categories after the delete (it cascades to pack_name).
  onChanged: () => void
}

function DeleteCategoryModal({
  category,
  onClose,
  onChanged,
}: DeleteCategoryModalProps): React.ReactElement {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useEscapeToClose(onClose, true)

  const run = (deleteTriggers: boolean) => {
    setBusy(true)
    setError(null)
    deleteTriggerCategory(category.name, deleteTriggers)
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
  const [tab, setTab] = useState<Tab>('triggers')
  const [triggers, setTriggers] = useState<Trigger[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createPrefill, setCreatePrefill] = useState<SpellTimerTriggerPrefill | undefined>(undefined)
  const [showSpellPicker, setShowSpellPicker] = useState(false)
  const [newMenuOpen, setNewMenuOpen] = useState(false)
  const [emptyNewMenuOpen, setEmptyNewMenuOpen] = useState(false)
  const [showClearAll, setShowClearAll] = useState(false)
  const [clearingAll, setClearingAll] = useState(false)
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
  // Tracks which pack sections in the grouped trigger list are collapsed.
  // Default: all expanded. Persists across refreshes via localStorage.
  const [collapsedPacks, setCollapsedPacks] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem('triggers.collapsedPacks')
      if (raw) return new Set(JSON.parse(raw))
    } catch {}
    return new Set()
  })
  const [categories, setCategories] = useState<TriggerCategory[]>([])
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

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listTriggers()
      .then(setTriggers)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
    reloadCategories()
  }, [reloadCategories])

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
        if (!haystack.includes(q)) return false
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
        // (empty pack_name); any other value matches the pack name exactly.
        if (packFilter === '__uncategorized__') {
          if (t.pack_name) return false
        } else if (t.pack_name !== packFilter) {
          return false
        }
      }
      return true
    })
  }, [triggers, search, classFilter, charFilter, packFilter, packClassByName])

  // Pack names currently represented in the user's triggers, for the
  // pack-filter dropdown. Sorted alphabetically; the "Uncategorized"
  // bucket (user-authored) is pinned to the end.
  const packsInUse = useMemo(() => {
    const set = new Set<string>()
    let hasUncategorized = false
    for (const t of triggers) {
      if (t.pack_name) set.add(t.pack_name)
      else hasUncategorized = true
    }
    const names = Array.from(set).sort((a, b) => a.localeCompare(b))
    if (hasUncategorized) names.push('__uncategorized__')
    return names
  }, [triggers])

  const hasActiveFilter = !!(search.trim() || classFilter !== null || charFilter || packFilter)

  // Group + sort the filtered triggers for display. Sections follow the
  // backend category order; Uncategorized pins last. Empty custom categories
  // are shown (so they can be drag targets) when no filter is narrowing the
  // view. Each section's entries are sorted per sortMode.
  const groupedTriggers = useMemo(() => {
    const groups = new Map<string, Trigger[]>()
    for (const t of filteredTriggers) {
      const key = t.pack_name || '__uncategorized__'
      if (!groups.has(key)) groups.set(key, [])
      groups.get(key)!.push(t)
    }
    if (!hasActiveFilter) {
      for (const c of categories) {
        if (c.custom && !groups.has(c.name)) groups.set(c.name, [])
      }
    }
    const orderIndex = new Map<string, number>()
    categories.forEach((c, i) => orderIndex.set(c.name, i))
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
    const ordered = Array.from(groups.entries()).map(([packName, items]) => ({
      packName,
      items,
    }))
    ordered.sort((a, b) => {
      const oa = orderOf(a.packName)
      const ob = orderOf(b.packName)
      if (oa !== ob) return oa - ob
      return a.packName.localeCompare(b.packName)
    })
    for (const g of ordered) sortItems(g.items)
    return ordered
  }, [filteredTriggers, categories, sortMode, hasActiveFilter])

  // Set of pack ids the user already has installed, for the Packs tab. Memoized
  // so the PacksTab prop identity is stable across unrelated re-renders.
  const installedPacks = useMemo(
    () => new Set(triggers.map((t) => t.source_pack).filter((n): n is string => !!n)),
    [triggers],
  )

  const togglePackCollapsed = (packName: string) => {
    setCollapsedPacks((prev) => {
      const next = new Set(prev)
      if (next.has(packName)) next.delete(packName)
      else next.add(packName)
      try {
        localStorage.setItem('triggers.collapsedPacks', JSON.stringify(Array.from(next)))
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
  const handleNewCategory = () => {
    if (creatingCategory) return
    const existing = new Set(categories.map((c) => c.name))
    let name = 'New Category'
    for (let i = 2; existing.has(name); i++) name = `New Category ${i}`
    setCreatingCategory(true)
    createTriggerCategory(name)
      .then((cat) => {
        reloadCategories()
        cancelRenameRef.current = false
        setRenamingCategory(cat.name)
        setRenameValue(cat.name)
      })
      .catch(() => {})
      .finally(() => setCreatingCategory(false))
  }

  const startRenameCategory = (name: string) => {
    cancelRenameRef.current = false
    setRenamingCategory(name)
    setRenameValue(name)
  }

  const cancelRenameCategory = () => {
    cancelRenameRef.current = true
    setRenamingCategory(null)
  }

  // Commit the inline rename. Reentrant-safe: the input's onBlur fires again
  // when Enter/Escape unmounts it, so the first call latches cancelRenameRef
  // to make the second a no-op. Escape sets the latch up front to skip the
  // rename entirely.
  const commitRenameCategory = (oldName: string) => {
    if (cancelRenameRef.current) {
      cancelRenameRef.current = false
      setRenamingCategory(null)
      return
    }
    cancelRenameRef.current = true
    const trimmed = renameValue.trim()
    setRenamingCategory(null)
    if (!trimmed || trimmed === oldName) return
    renameTriggerCategory(oldName, trimmed)
      .then(() => load()) // cascades to trigger pack_name → reload everything
      .catch(() => {})
  }

  // ── Drag-and-drop (dnd-kit) ──
  // A section keyed by packKey ('__uncategorized__' or a category name) accepts
  // the active trigger when it isn't already that trigger's category. Pack
  // sections are valid targets too — origin is tracked by source_pack, so
  // moving a trigger into/out of a pack category doesn't change its pack.
  const canDropTriggerOn = (packKey: string): boolean => {
    if (!activeTrigger) return false
    const target = packKey === '__uncategorized__' ? '' : packKey
    return activeTrigger.pack_name !== target
  }

  // Move a trigger to another category by reassigning its pack_name.
  const moveTriggerToCategory = (t: Trigger, packKey: string) => {
    const target = packKey === '__uncategorized__' ? '' : packKey
    if (t.pack_name === target) return
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
      characters: t.characters,
      timer_alerts: t.timer_alerts ?? [],
      exclude_patterns: t.exclude_patterns ?? [],
      extra_patterns: t.extra_patterns ?? [],
      source: t.source,
      pipe_condition: t.pipe_condition,
      pack_name: target,
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
    const key = dragged.pack_name || ''
    const ids = triggers
      .filter((t) => (t.pack_name || '') === key)
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
  // section. Uncategorized is pinned last and can't be a drag source or target.
  const reorderCategoryTo = (dragged: string, targetKey: string) => {
    if (!dragged || dragged === targetKey || targetKey === '__uncategorized__') return
    // Order over ALL categories (already display-sorted) so reordering while
    // filtered doesn't drop hidden ones. Move dragged to target's slot.
    const order = categories.map((c) => c.name)
    const from = order.indexOf(dragged)
    const to = order.indexOf(targetKey)
    if (from === -1 || to === -1 || from === to) return
    const next = arrayMove(order, from, to)
    // Optimistic: rebuild categories in the new order so sections reflow.
    const byName = new Map(categories.map((c) => [c.name, c]))
    setCategories(
      next.map((name, i) => ({ ...(byName.get(name) as TriggerCategory), sort_order: i })),
    )
    reorderTriggerCategories(next).catch(() => load())
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
      const srcKey = dragged.pack_name || '__uncategorized__'
      if (oId.startsWith(TRIGGER_PREFIX)) {
        const overT = triggers.find((t) => t.id === oId.slice(TRIGGER_PREFIX.length))
        if (!overT) return
        const overKey = overT.pack_name || '__uncategorized__'
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
              <div className="relative flex">
                <button
                  onClick={() => {
                    setNewMenuOpen(false)
                    setShowSpellPicker(true)
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

      {/* Tabs */}
      <div
        className="flex gap-0 border-b shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {(['triggers', 'history', 'packs'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className="px-4 py-2 text-xs font-medium capitalize transition-colors"
            style={tabStyle(t)}
          >
            {t === 'triggers' && <span>Triggers ({triggers.length})</span>}
            {t === 'history' && <span>History</span>}
            {t === 'packs' && <span>Packs</span>}
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
                        <option key={p} value={p}>
                          {p === '__uncategorized__' ? 'Uncategorized' : p}
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
                <div className="flex h-full flex-col items-center justify-center gap-3 py-16">
                  <Zap size={32} style={{ color: 'var(--color-muted)' }} />
                  <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                    No triggers yet.
                  </p>
                  <div className="flex gap-2">
                    <div className="relative flex">
                      <button
                        onClick={() => {
                          setEmptyNewMenuOpen(false)
                          setShowSpellPicker(true)
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
                  items={groupedTriggers.map((g) => `${CATEGORY_PREFIX}${g.packName}`)}
                  strategy={verticalListSortingStrategy}
                >
                  {groupedTriggers.map((group) => (
                    <CategorySection
                      key={group.packName}
                      group={group}
                      categories={categories}
                      collapsed={collapsedPacks.has(group.packName)}
                      canDrop={canDropTriggerOn(group.packName)}
                      isRenaming={renamingCategory === group.packName}
                      renameValue={renameValue}
                      onRenameValueChange={setRenameValue}
                      onToggleCollapsed={() => togglePackCollapsed(group.packName)}
                      onStartRename={() => startRenameCategory(group.packName)}
                      onCommitRename={() => commitRenameCategory(group.packName)}
                      onCancelRename={cancelRenameCategory}
                      onDeleteCategory={(c) => setDeletingCategory(c)}
                      onTriggerDeleted={handleDeleted}
                      onTriggerUpdated={handleUpdated}
                      onCategoriesChanged={reloadCategories}
                    />
                  ))}
                </SortableContext>
                <DragOverlay>
                  {activeTrigger ? (
                    <TriggerDragPreview trigger={activeTrigger} />
                  ) : activeCategory ? (
                    <CategoryDragPreview
                      label={
                        activeCategory === '__uncategorized__' ? 'Uncategorized' : activeCategory
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

      {deletingCategory && (
        <DeleteCategoryModal
          category={deletingCategory}
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
    </div>
  )
}
