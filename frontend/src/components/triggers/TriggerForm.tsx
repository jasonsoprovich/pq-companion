/**
 * TriggerForm — the full trigger editor (create and edit), moved out of
 * TriggersPage. Fields are grouped into Basics, Actions, Timer and
 * Advanced sections.
 */
import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import {
  Zap,
  Plus,
  Trash2,
  RefreshCw,
  X,
  ChevronDown,
  ChevronRight,
  Pin,
  HelpCircle,
} from 'lucide-react'
import { useVoices } from '../../hooks/useVoices'
import { useTTSVoices } from '../../hooks/usePiperStatus'
import { useDiscordWebhooks } from '../../hooks/useDiscordWebhooks'
import NotificationActionEditor, { NotificationTypeSelect } from '../NotificationActionEditor'
import DecimalInput from '../DecimalInput'
import ActionTemplatesMenu from '../ActionTemplatesMenu'
import { MatchRow } from '../TriggerTesterTab'
import type { TriggerPrefill } from '../../lib/triggerTemplates'
import {
  createTrigger,
  updateTrigger,
  listActionTemplates,
  listCharacters,
  createTriggerCategory,
  createTimerGroup,
  runTriggerTest,
  type CreateTriggerRequest,
  type Character,
  convertTriggerPattern,
} from '../../services/api'
import { useActivePlayerName } from '../../hooks/useActivePlayerName'
import { compileTriggerRegex } from '../../lib/triggerRegex'
import { specialAbilityName } from '../../lib/enumsCache'
import type {
  Trigger,
  TriggerCategory,
  TimerGroup,
  Action,
  TimerType,
  TimerAlertThreshold,
  TimerAlertType,
  TriggerSource,
  PipeConditionKind,
  PipeCondition,
  ExtraPattern,
  TestLineResult,
} from '../../types/trigger'
import { CLASS_NAMES, Toggle, flattenCategoryTree } from './shared'

// NPC special ability codes (Project Quarm's EQMacEmu numbering) offered as
// the target-HP trigger's ability filter.
const SPECIAL_ABILITY_CODES = Array.from({ length: 54 }, (_, i) => i + 1)

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
          <DecimalInput
            min={0}
            max={3600}
            value={alert.seconds}
            onValue={(v) => onChange({ ...alert, seconds: v })}
            style={{ ...inputStyle, width: 60 }}
          />
          s remaining
        </label>

        <div className="flex items-center gap-1.5 text-xs ml-auto" style={{ color: 'var(--color-muted-foreground)' }}>
          <span>Type</span>
          <NotificationTypeSelect
            value={alert.type}
            onChange={(t) =>
              onChange({
                ...alert,
                type: t as TimerAlertType,
                // Seed the overlay message so a switched-from-TTS alert
                // isn't saved with an empty (silently skipped) text.
                text:
                  t === 'overlay_text' && !alert.text?.trim()
                    ? alert.tts_template || '{spell} expiring soon'
                    : alert.text,
              })
            }
            allowedTypes={['overlay_text', 'text_to_speech', 'play_sound']}
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
        overlayText={alert.text ?? ''}
        overlayTextPlaceholder="{spell} expiring soon"
        onOverlayTextChange={(v) => onChange({ ...alert, text: v })}
        durationSecs={alert.duration_secs || 5}
        onDurationSecsChange={(v) => onChange({ ...alert, duration_secs: v })}
        color={alert.color ?? ''}
        onColorChange={(v) => onChange({ ...alert, color: v })}
        glowColor={alert.glow_color ?? ''}
        onGlowColorChange={(v) => onChange({ ...alert, glow_color: v })}
        fontFamily={alert.font_family ?? ''}
        onFontFamilyChange={(v) => onChange({ ...alert, font_family: v })}
        fontSize={alert.font_size ?? 0}
        onFontSizeChange={(v) => onChange({ ...alert, font_size: v })}
        align={alert.align ?? ''}
        onAlignChange={(v) => onChange({ ...alert, align: v })}
        onStyleReset={() =>
          onChange({ ...alert, color: '', glow_color: '', font_family: '', font_size: 0, align: '' })
        }
        position={alert.position ?? null}
        onPositionChange={(p) => onChange({ ...alert, position: p })}
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
  const voices = useTTSVoices(useVoices())
  const webhooks = useDiscordWebhooks()
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
        align={action.align ?? ''}
        onAlignChange={(v) => onChange(index, { ...action, align: v })}
        onStyleReset={() =>
          onChange(index, {
            ...action,
            color: '',
            glow_color: '',
            font_family: '',
            font_size: 0,
            align: '',
          })
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
        clipboardText={action.text}
        onClipboardTextChange={(v) => onChange(index, { ...action, text: v })}
        webhookText={action.text}
        onWebhookTextChange={(v) => onChange(index, { ...action, text: v })}
        webhookId={action.webhook_id || ''}
        onWebhookIdChange={(v) => onChange(index, { ...action, webhook_id: v })}
        webhooks={webhooks}
      />
    </div>
  )
}

// ── Sample-line tester (inline, in the trigger editor) ──────────────────────
// Lets the user validate an unsaved draft's pattern against one pasted line
// before Save — the trigger editor's slice of the Trigger Tester (see
// TriggerTesterTab for the full paste-and-run tab). buildDraft assembles a
// Trigger-shaped payload from the form's current (possibly invalid/unsaved)
// state at test time.

function SampleLineTester({
  buildDraft,
  disabled,
}: {
  buildDraft: () => Trigger
  disabled: boolean
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [line, setLine] = useState('')
  const [fireEffects, setFireEffects] = useState(false)
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<TestLineResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  const run = () => {
    if (!line.trim()) return
    setRunning(true)
    setError(null)
    setResult(null)
    runTriggerTest({ lines: line, trigger: buildDraft(), fire_effects: fireEffects })
      .then((report) => {
        if (report.errors && report.errors.length > 0) {
          setError(report.errors.join(' '))
          return
        }
        setResult(report.lines[0] ?? null)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setRunning(false))
  }

  return (
    <div
      className="rounded p-2 space-y-1.5"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1.5 text-[11px] font-medium"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        {open ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
        Test against sample line
      </button>
      {open && (
        <div className="space-y-1.5 pt-0.5">
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={line}
              onChange={(e) => setLine(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); run() } }}
              placeholder="Paste a sample log line here (with or without the timestamp)"
              className="flex-1 rounded px-2 py-1 text-xs outline-none font-mono"
              style={{
                backgroundColor: 'var(--color-surface)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
              disabled={disabled}
            />
            <button
              type="button"
              onClick={run}
              disabled={!line.trim() || running || disabled}
              className="text-[11px] px-2 py-1 rounded font-medium shrink-0"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: 'var(--color-background)',
                opacity: !line.trim() || running || disabled ? 0.6 : 1,
              }}
            >
              {running ? 'Testing…' : 'Test'}
            </button>
          </div>
          <label
            className="flex items-center gap-1.5 text-[11px]"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Also start the real timer and preview the overlay/audio — never posts a Discord webhook or writes trigger history"
          >
            <input
              type="checkbox"
              checked={fireEffects}
              onChange={(e) => setFireEffects(e.target.checked)}
            />
            Fire alerts
          </label>
          {error && (
            <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{error}</p>
          )}
          {result && (
            result.matches.length === 0 ? (
              <p className="text-[11px] italic" style={{ color: 'var(--color-muted)' }}>
                No match.
              </p>
            ) : (
              <div className="space-y-1.5">
                {result.matches.map((m, i) => (
                  <MatchRow key={i} match={m} onOpenTrigger={() => {}} />
                ))}
              </div>
            )
          )}
        </div>
      )}
    </div>
  )
}

// ── Trigger form ──────────────────────────────────────────────────────────────

// FormSection is one collapsible group of the trigger editor. The ? opens
// the help drawer (the Triggers article) without leaving the form.
function FormSection({
  title,
  hint,
  summary,
  open,
  onToggle,
  children,
}: {
  title: string
  hint: string
  summary?: string
  open: boolean
  onToggle?: () => void
  children: React.ReactNode
}): React.ReactElement {
  return (
    <section className="rounded-md" style={{ border: '1px solid var(--color-border)' }}>
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          type="button"
          onClick={onToggle}
          disabled={!onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          style={{ cursor: onToggle ? 'pointer' : 'default' }}
        >
          {onToggle && (
            <ChevronDown
              size={12}
              style={{
                color: 'var(--color-muted)',
                transition: 'transform 0.15s',
                transform: open ? 'none' : 'rotate(-90deg)',
              }}
            />
          )}
          <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {title}
          </span>
          <span className="truncate text-[11px]" style={{ color: 'var(--color-muted)' }}>
            {open ? hint : summary}
          </span>
        </button>
        <button
          type="button"
          onClick={() => window.dispatchEvent(new Event('pq:open-help'))}
          className="shrink-0 rounded p-0.5"
          style={{ color: 'var(--color-muted)', cursor: 'pointer' }}
          title="Help for triggers (F1)"
        >
          <HelpCircle size={12} />
        </button>
      </div>
      {open && <div className="space-y-4 px-3 pb-3">{children}</div>}
    </section>
  )
}

export interface TriggerFormProps {
  initial?: Trigger
  prefill?: TriggerPrefill
  /** Categories for the dropdown; built-in entries are shown but flagged. */
  categories: TriggerCategory[]
  /** Called after the form creates a new category inline, so the parent
   *  can refresh its category list. */
  onCategoriesChanged: () => void
  /** Timer groups for the Custom Timers window picker (timer_type='custom'). */
  timerGroups: TimerGroup[]
  /** Called after the form creates a new timer group inline, so the parent
   *  can refresh its group list. */
  onTimerGroupsChanged: () => void
  onSaved: (t: Trigger) => void
  onCancel: () => void
}

export default function TriggerForm({ initial, prefill, categories, onCategoriesChanged, timerGroups, onTimerGroupsChanged, onSaved, onCancel }: TriggerFormProps): React.ReactElement {
  const [name, setName] = useState(initial?.name ?? prefill?.name ?? '')
  // Category, by id. Defaults to the trigger's current category on edit,
  // Uncategorized ('') on create. Falls back to matching by pack_name for
  // older cached data with no category_id — categories is already loaded by
  // the time this form mounts. The "__new__" sentinel opens an inline create
  // input (always a top-level category — nest it afterward from the section
  // header's Parent picker).
  const [categoryId, setCategoryId] = useState(
    initial?.category_id ?? categories.find((c) => c.name === initial?.pack_name)?.id ?? '',
  )
  const categoriesById = useMemo(() => new Map(categories.map((c) => [c.id, c])), [categories])
  const orderedCategories = useMemo(() => flattenCategoryTree(categories), [categories])
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
  const [pipeSpecialAbility, setPipeSpecialAbility] = useState<number>(
    initial?.pipe_condition?.special_ability ?? 0,
  )
  const [pipeError, setPipeError] = useState<string | null>(null)
  const [actions, setActions] = useState<Action[]>(
    initial?.actions ?? prefill?.actions ?? [{ type: 'overlay_text', text: prefill?.name ?? '', duration_secs: 5, color: '', sound_path: '', volume: 0, voice: '' }],
  )
  // True once the user edits actions in this session — the async default-
  // template prefill below must never clobber their work.
  const actionsTouched = useRef(false)
  const [timerType, setTimerType] = useState<TimerType>(initial?.timer_type ?? prefill?.timerType ?? 'none')
  const [timerDuration, setTimerDuration] = useState(initial?.timer_duration_secs ?? prefill?.timerDurationSecs ?? 0)
  const [timerDurationCapture, setTimerDurationCapture] = useState(initial?.timer_duration_capture ?? '')
  const [timerKeyCapture, setTimerKeyCapture] = useState(initial?.timer_key_capture ?? '')
  const [timerTargetCapture, setTimerTargetCapture] = useState(initial?.timer_target_capture ?? '')
  const [wornOffPattern, setWornOffPattern] = useState(initial?.worn_off_pattern ?? prefill?.wornOffPattern ?? '')
  // Anti-spam refire lockout (seconds); 0 = fire on every match (default).
  const [refireCooldown, setRefireCooldown] = useState(initial?.refire_cooldown_secs ?? 0)
  const [repeatReset, setRepeatReset] = useState(initial?.repeat_reset_secs ?? 0)
  const [displayThreshold, setDisplayThreshold] = useState(initial?.display_threshold_secs ?? 0)
  // Optional per-trigger bar color; '' = automatic overlay color.
  const [barColor, setBarColor] = useState(initial?.bar_color ?? '')
  // Pins this trigger's timer(s) to the top of their overlay.
  const [pinned, setPinned] = useState(initial?.pinned ?? false)
  // Stacks each firing as its own timer row instead of restarting/
  // overwriting the existing same-name row. Only meaningful for custom timers.
  const [timerStack, setTimerStack] = useState(initial?.timer_stack ?? false)
  const [timerRepeat, setTimerRepeat] = useState(initial?.timer_repeat ?? false)
  const [timerRepeatMax, setTimerRepeatMax] = useState(initial?.timer_repeat_max ?? 0)
  const [timerIfIdle, setTimerIfIdle] = useState(initial?.timer_if_idle ?? false)
  const [timerClearOnContextChange, setTimerClearOnContextChange] = useState(
    initial?.timer_clear_on_context_change ?? false,
  )
  // Keep first countdown / Clear on zone or character change only apply to
  // a log-source custom timer that isn't stacked or repeating — mirrors
  // Trigger.timerPolicyEligible in the backend.
  const timerPolicyEligible = source === 'log' && timerType === 'custom'
    && !timerStack && !timerRepeat
  // Which Custom Timers window this trigger's timer appears in (only used
  // when timerType === 'custom'). '' = the original/default window. The
  // "__new__" sentinel opens an inline create input, mirroring the category
  // picker above.
  const [customGroupID, setCustomGroupID] = useState(initial?.custom_group_id ?? '')
  const [creatingGroup, setCreatingGroup] = useState(false)
  const [newGroupName, setNewGroupName] = useState('')
  const [addingGroup, setAddingGroup] = useState(false)
  const [groupError, setGroupError] = useState<string | null>(null)
  const [timerAlerts, setTimerAlerts] = useState<TimerAlertThreshold[]>(
    initial?.timer_alerts ?? prefill?.timerAlerts ?? [],
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
  const voices = useTTSVoices(useVoices())
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

  // Assembles the current (possibly unsaved/invalid) form state into a
  // Trigger-shaped payload for the inline sample-line tester — mirrors
  // handleSubmit's request below, minus its validation (an invalid pattern
  // just comes back as a report error from the backend instead of blocking
  // the test).
  const buildTestDraft = useCallback((): Trigger => ({
    id: initial?.id ?? 'draft',
    name: name.trim() || 'Draft',
    enabled: true,
    pattern: pattern.trim(),
    actions,
    pack_name: '',
    category_id: categoryId,
    created_at: initial?.created_at ?? new Date().toISOString(),
    timer_type: timerType,
    timer_duration_secs: timerType === 'none' ? 0 : Math.max(0, timerDuration),
    timer_duration_capture: timerType === 'none' ? '' : timerDurationCapture.trim(),
    timer_key_capture: timerType === 'none' ? '' : timerKeyCapture.trim(),
    timer_target_capture: timerTargetCapture.trim(),
    worn_off_pattern: timerType === 'none' ? '' : wornOffPattern.trim(),
    spell_id: initial?.spell_id ?? prefill?.spellId ?? 0,
    refire_cooldown_secs: Math.max(0, refireCooldown),
    repeat_reset_secs: Math.max(0, repeatReset),
    display_threshold_secs: timerType === 'none' ? 0 : Math.max(0, displayThreshold),
    bar_color: timerType === 'none' ? '' : barColor,
    characters: Array.from(selectedChars),
    timer_if_idle: timerPolicyEligible && timerIfIdle,
    timer_clear_on_context_change: timerPolicyEligible && timerClearOnContextChange,
    timer_alerts: timerType === 'none' ? [] : timerAlerts,
    exclude_patterns: excludePatternsText.split('\n').map((s) => s.trim()).filter((s) => s.length > 0),
    extra_patterns: extraPatterns
      .map((ep) => ({ ...ep, pattern: ep.pattern.trim() }))
      .filter((ep) => ep.pattern.length > 0),
    sort_order: initial?.sort_order ?? 0,
  }), [
    initial, name, pattern, actions, categoryId, timerType, timerDuration,
    timerDurationCapture, timerKeyCapture, timerTargetCapture, wornOffPattern,
    prefill, refireCooldown, repeatReset, displayThreshold, barColor, selectedChars,
    timerAlerts, excludePatternsText, extraPatterns,
    timerPolicyEligible, timerIfIdle, timerClearOnContextChange,
  ])

  // Plain new triggers start from the default action template when one is
  // set. Spell-prefilled and edited triggers keep their own actions.
  useEffect(() => {
    if (initial || prefill) return
    listActionTemplates()
      .then((templates) => {
        const def = templates.find((t) => t.is_default)
        if (!def || def.actions.length === 0) return
        if (actionsTouched.current) return
        setActions(JSON.parse(JSON.stringify(def.actions)) as Action[])
      })
      .catch(() => {})
    // Run once per form mount; initial/prefill never change after mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

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
      // The backend matches with Go/RE2, which accepts named groups and inline
      // flags JS rejects. Translate before validating so documented syntax
      // passes (see lib/triggerRegex).
      compileTriggerRegex(p)
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

  // ── Plain-text pattern mode ──────────────────────────────────────────────
  // The stored pattern is always the regex. Plain mode is an editing view
  // converted by the backend grammar (internal/trigger/plainpattern.go). A
  // form opens in Plain when its pattern is empty or converts back exactly;
  // otherwise Regex, with Plain disabled and the reason shown.
  const [patternMode, setPatternMode] = useState<'plain' | 'regex'>(pattern.trim() ? 'regex' : 'plain')
  const [plainText, setPlainText] = useState('')
  const [plainError, setPlainError] = useState<string | null>(null)
  const [plainPending, setPlainPending] = useState(false)
  const [plainBlocked, setPlainBlocked] = useState<string | null>(null)
  const plainInit = useRef(false)
  // Latest values, so a mode-switch conversion that returns after the user
  // kept typing is ignored instead of overwriting the newer text.
  const plainTextRef = useRef(plainText)
  plainTextRef.current = plainText
  const patternRef = useRef(pattern)
  patternRef.current = pattern

  useEffect(() => {
    if (plainInit.current) return
    plainInit.current = true
    if (!pattern.trim()) return
    convertTriggerPattern('plain', pattern)
      .then((r) => {
        if (r.ok) {
          setPlainText(r.result ?? '')
          setPatternMode('plain')
        } else {
          setPlainBlocked(r.error ?? 'not plain text')
        }
      })
      .catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Plain → regex as the user types.
  useEffect(() => {
    if (patternMode !== 'plain' || !plainInit.current) return
    if (!plainText.trim()) {
      setPlainError(null)
      setPlainPending(false)
      handlePatternChange('')
      return
    }
    let stale = false
    setPlainPending(true)
    const t = window.setTimeout(() => {
      convertTriggerPattern('regex', plainText)
        .then((r) => {
          if (stale) return
          if (r.ok) {
            setPlainError(null)
            handlePatternChange(r.result ?? '')
          } else {
            setPlainError(r.error ?? 'Invalid pattern')
          }
        })
        .catch(() => {
          if (!stale) setPlainError("Couldn't check the pattern")
        })
        .finally(() => {
          if (!stale) setPlainPending(false)
        })
    }, 150)
    return () => {
      stale = true
      window.clearTimeout(t)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [plainText, patternMode])

  // In Regex mode, keep track of whether Plain is available for this regex.
  useEffect(() => {
    if (patternMode !== 'regex') return
    if (!pattern.trim()) {
      setPlainBlocked(null)
      return
    }
    let stale = false
    const t = window.setTimeout(() => {
      convertTriggerPattern('plain', pattern)
        .then((r) => {
          if (stale) return
          setPlainBlocked(r.ok ? null : (r.error ?? 'not plain text'))
          if (r.ok) setPlainText(r.result ?? '')
        })
        .catch(() => undefined)
    }, 300)
    return () => {
      stale = true
      window.clearTimeout(t)
    }
  }, [pattern, patternMode])

  const handleActionChange = (index: number, action: Action) => {
    actionsTouched.current = true
    setActions((prev) => prev.map((a, i) => (i === index ? action : a)))
  }

  const handleActionRemove = (index: number) => {
    actionsTouched.current = true
    setActions((prev) => prev.filter((_, i) => i !== index))
  }

  const handleAddAction = () => {
    actionsTouched.current = true
    setActions((prev) => [...prev, { type: 'overlay_text', text: '', duration_secs: 5, color: '', sound_path: '', volume: 0, voice: '' }])
  }

  const handleCategorySelect = (v: string) => {
    if (v === '__new__') {
      setCreatingCat(true)
      setCatError(null)
      return
    }
    setCategoryId(v)
  }

  const handleAddCategory = () => {
    const trimmed = newCatName.trim()
    if (!trimmed) return
    setAddingCat(true)
    setCatError(null)
    createTriggerCategory(trimmed)
      .then((cat) => {
        setCategoryId(cat.id)
        setCreatingCat(false)
        setNewCatName('')
        onCategoriesChanged()
      })
      .catch((err: Error) => setCatError(err.message))
      .finally(() => setAddingCat(false))
  }

  const handleGroupSelect = (v: string) => {
    if (v === '__new__') {
      setCreatingGroup(true)
      setGroupError(null)
      return
    }
    setCustomGroupID(v)
  }

  const handleAddGroup = () => {
    const trimmed = newGroupName.trim()
    if (!trimmed) return
    setAddingGroup(true)
    setGroupError(null)
    createTimerGroup(trimmed)
      .then((group) => {
        setCustomGroupID(group.id)
        setCreatingGroup(false)
        setNewGroupName('')
        onTimerGroupsChanged()
      })
      .catch((err: Error) => setGroupError(err.message))
      .finally(() => setAddingGroup(false))
  }

  const buildPipeCondition = (): PipeCondition | null => {
    switch (pipeKind) {
      case 'target_hp_below': {
        const t = Math.max(1, Math.min(99, Math.round(pipeHPThreshold)))
        const cond: PipeCondition = { kind: 'target_hp_below', hp_threshold: t }
        if (pipeTargetName.trim()) cond.target_name = pipeTargetName.trim()
        if (pipeSpecialAbility > 0) cond.special_ability = pipeSpecialAbility
        return cond
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
      case 'buff_stripped':
      case 'debuff_stripped': {
        const spell = pipeSpellName.trim()
        return spell ? { kind: pipeKind, spell_name: spell } : { kind: pipeKind }
      }
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
      if (patternMode === 'plain' && (plainPending || plainError)) {
        setPatternError(plainError ?? 'Still checking the pattern, try again.')
        return
      }
      if (!pattern.trim()) return
      if (!validatePattern(pattern)) return

      excludeList = excludePatternsText
        .split('\n')
        .map((s) => s.trim())
        .filter((s) => s.length > 0)
      const errs: string[] = []
      for (const ex of excludeList) {
        try { compileTriggerRegex(ex) } catch (e) { errs.push(`${ex} — ${(e as Error).message}`) }
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
          compileTriggerRegex(ep.pattern)
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
      // Kept for plain alerts too (not just timers): the captured target now
      // also fills {target} in the alert/TTS text, so a proc/groupmate-cast
      // trigger with no timer can still show who it hit.
      timer_target_capture: source === 'pipe' ? '' : timerTargetCapture.trim(),
      worn_off_pattern: source === 'pipe' || timerType === 'none' ? '' : wornOffPattern.trim(),
      spell_id: initial?.spell_id ?? prefill?.spellId ?? 0,
      refire_cooldown_secs: Math.max(0, refireCooldown),
      repeat_reset_secs: Math.max(0, repeatReset),
      display_threshold_secs: timerType === 'none' ? 0 : Math.max(0, displayThreshold),
      bar_color: timerType === 'none' ? '' : barColor,
      pinned: timerType === 'none' ? false : pinned,
      timer_stack: timerType === 'custom' ? timerStack : false,
      timer_repeat: timerType === 'custom' && !timerStack ? timerRepeat : false,
      timer_repeat_max: Math.max(0, Math.floor(timerRepeatMax)),
      timer_if_idle: timerPolicyEligible && timerIfIdle,
      timer_clear_on_context_change: timerPolicyEligible && timerClearOnContextChange,
      custom_group_id: timerType === 'custom' ? customGroupID : '',
      characters: Array.from(selectedChars),
      timer_alerts: timerType === 'none' ? [] : timerAlerts,
      exclude_patterns: source === 'pipe' ? [] : excludeList,
      extra_patterns: source === 'pipe' ? [] : extraList,
      source,
      pipe_condition: pipeCondition,
      category_id: categoryId,
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

  // Pipe triggers have no regex; their pipe condition is checked on submit.
  const canSubmit =
    name.trim() &&
    (source === 'pipe' || (pattern.trim() && !patternError)) &&
    !submitting

  // Sections: Basics is always open; Actions and Timer start open when
  // they hold something; Advanced opens only when it has non-default values.
  const advancedSet =
    excludePatternsText.trim() !== '' || refireCooldown > 0 || repeatReset > 0
  const [openSections, setOpenSections] = useState(() => ({
    actions: true,
    timer: timerType !== 'none',
    advanced: advancedSet,
  }))
  const toggleSection = (k: 'actions' | 'timer' | 'advanced'): void =>
    setOpenSections((o) => ({ ...o, [k]: !o[k] }))
  const advancedSummary = advancedSet
    ? [
        excludePatternsText.trim() ? 'exclusions' : '',
        refireCooldown > 0 ? `${refireCooldown}s cooldown` : '',
        repeatReset > 0 ? 'repeat count' : '',
      ]
        .filter(Boolean)
        .join(', ')
    : 'Defaults'

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

      <FormSection title="Basics" hint="What to watch for, and for which characters." open>
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
              value={categoryId}
              onChange={(e) => handleCategorySelect(e.target.value)}
              className="w-full rounded px-3 py-1.5 text-sm outline-none"
              style={inputStyle}
              disabled={submitting}
            >
              <option value="">Uncategorized</option>
              {orderedCategories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.parent_id ? `  ↳ ${c.name}` : c.name}{c.is_builtin ? ' (pack)' : ''}
                </option>
              ))}
              {/* Defensive: keep the current value selectable even if the
                  category list hasn't loaded or doesn't include it yet. */}
              {categoryId && !categoriesById.has(categoryId) && (
                <option value={categoryId}>{initial?.pack_name || categoryId}</option>
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
              Log line
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
            <div className="flex items-center gap-2">
              <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                Pattern
              </label>
              <div className="ml-auto flex overflow-hidden rounded" style={{ border: '1px solid var(--color-border)' }}>
                {(['plain', 'regex'] as const).map((m) => {
                  const active = patternMode === m
                  const blocked = m === 'plain' && patternMode === 'regex' && !!plainBlocked
                  return (
                    <button
                      key={m}
                      type="button"
                      disabled={blocked || submitting}
                      onClick={() => {
                        if (m === 'regex') {
                          if (patternMode === 'regex') return
                          // Convert the latest plain text first: a pending
                          // debounced conversion would otherwise be dropped
                          // and Regex would show an older pattern. Invalid
                          // plain text stays in Plain so it can be fixed.
                          if (!plainText.trim()) {
                            setPatternMode('regex')
                            return
                          }
                          const sent = plainText
                          convertTriggerPattern('regex', sent)
                            .then((r) => {
                              if (plainTextRef.current !== sent) return
                              if (r.ok) {
                                setPlainError(null)
                                handlePatternChange(r.result ?? '')
                                setPatternMode('regex')
                              } else {
                                setPlainError(r.error ?? 'Invalid pattern')
                              }
                            })
                            .catch(() => {
                              if (plainTextRef.current === sent) setPlainError("Couldn't check the pattern")
                            })
                          return
                        }
                        if (patternMode === 'plain') return
                        // Convert the current regex fresh: the background
                        // check may lag behind the last keystroke.
                        if (!pattern.trim()) {
                          setPlainText('')
                          setPatternMode('plain')
                          return
                        }
                        const sentRegex = pattern
                        convertTriggerPattern('plain', sentRegex)
                          .then((r) => {
                            if (patternRef.current !== sentRegex) return
                            if (r.ok) {
                              setPlainText(r.result ?? '')
                              setPatternMode('plain')
                            } else {
                              setPlainBlocked(r.error ?? 'not plain text')
                            }
                          })
                          .catch(() => undefined)
                      }}
                      title={
                        blocked
                          ? `This pattern ${plainBlocked}, so it can only be edited as regex.`
                          : m === 'plain'
                            ? 'Type the line as it appears; {name} captures any text'
                            : 'Edit the regular expression directly'
                      }
                      className="px-2 py-0.5 text-[11px] font-medium disabled:opacity-40"
                      style={{
                        backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                        color: active ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                      }}
                    >
                      {m === 'plain' ? 'Plain text' : 'Regex'}
                    </button>
                  )
                })}
              </div>
            </div>
            {patternMode === 'plain' ? (
              <input
                type="text"
                placeholder="e.g. {name} has been slain by {C}"
                value={plainText}
                onChange={(e) => {
                  setPlainText(e.target.value)
                  if (patternError) setPatternError(null)
                }}
                className="w-full rounded px-3 py-1.5 text-sm outline-none font-mono"
                style={{
                  ...inputStyle,
                  border: `1px solid ${plainError || patternError ? 'var(--color-danger)' : 'var(--color-border)'}`,
                }}
                disabled={submitting}
              />
            ) : (
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
            )}
            {(patternMode === 'plain' ? plainError || patternError : patternError) && (
              <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>
                {patternMode === 'plain' ? plainError || patternError : patternError}
              </p>
            )}
            {patternMode === 'regex' && plainBlocked && pattern.trim() && (
              <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Plain text is off: this pattern {plainBlocked}.
              </p>
            )}
            {patternMode === 'plain' ? (
              <p className="text-[11px] leading-snug" style={{ color: 'var(--color-muted)' }}>
                Type the line as it appears in your log; it can match anywhere in the
                line. <span className="font-mono">{'{name}'}</span> captures any text,
                and you can use it in alert text the same way.{' '}
                <span className="font-mono">{'{C}'}</span> is your character,{' '}
                <span className="font-mono">{'{N}'}</span> a number,{' '}
                <span className="font-mono">{'{{ }}'}</span> literal braces.
                {pattern && (
                  <>
                    {' '}Regex: <span className="font-mono">{pattern}</span>
                  </>
                )}
              </p>
            ) : (
              <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Matched against the log message text (after the timestamp).
              </p>
            )}
            {patternMode === 'regex' && (
            <p className="text-[10px] leading-snug" style={{ color: 'var(--color-muted)' }}>
              Reuse capture groups in the alert/TTS text: <span className="font-mono">{'{1}'}</span>,{' '}
              <span className="font-mono">{'{2}'}</span> (or <span className="font-mono">$1</span>,{' '}
              GINA-style <span className="font-mono">{'{S1}'}</span>) for numbered groups,{' '}
              <span className="font-mono">{'{name}'}</span> for named groups like{' '}
              <span className="font-mono">(?P&lt;name&gt;…)</span>. Built-ins:{' '}
              <span className="font-mono">{'{c}'}</span> = your character (works in the
              pattern too), <span className="font-mono">{'{repeated}'}</span> = match count
              within the repeat window, <span className="font-mono">{'{target}'}</span> = the mob you’re
              fighting — or set “Target from capture” below to pull it from the matched line
              (a groupmate’s slow victim, a proc target).
            </p>
            )}
            {source === 'log' && (
              <div className="flex items-center gap-1.5 flex-wrap">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Target from capture
                </label>
                <input
                  type="text"
                  value={timerTargetCapture}
                  onChange={(e) => setTimerTargetCapture(e.target.value)}
                  placeholder="e.g. target"
                  className="w-24 rounded px-2 py-0.5 text-xs outline-none text-center font-mono"
                  style={inputStyle}
                  disabled={submitting}
                  title="Name (or number) of a capture group holding who the spell/proc hit. Its text fills {target} in the alert & TTS text AND shows as the grey 'on <target>' suffix on timers. Capture it from the line itself, e.g. (?P<target>[A-Za-z' ]+) is ensnared. Empty (or an unmatched self-cast branch) = fall back to the mob you're currently fighting."
                />
                <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                  fills <span className="font-mono">{'{target}'}</span> &amp; the “on &lt;name&gt;” timer
                  suffix; capture like <span className="font-mono">(?P&lt;target&gt;[A-Za-z&#39; ]+)</span>
                </span>
              </div>
            )}

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
                    <DecimalInput
                      min={0}
                      value={ep.timer_duration_secs ?? 0}
                      onValue={(v) => {
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
              <option value="buff_stripped">My buff is stripped</option>
              <option value="debuff_stripped">My debuff is dispelled off a mob</option>
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
            {pipeKind === 'target_hp_below' && (
              <div className="flex gap-2">
                <input
                  type="text"
                  placeholder="Only this mob (optional)"
                  value={pipeTargetName}
                  onChange={(e) => setPipeTargetName(e.target.value)}
                  className="min-w-0 flex-1 rounded px-3 py-1.5 text-sm outline-none"
                  style={inputStyle}
                  disabled={submitting}
                />
                <select
                  value={pipeSpecialAbility}
                  onChange={(e) => setPipeSpecialAbility(Number(e.target.value))}
                  title="Only fire when the target's NPC data has this special ability"
                  className="min-w-0 flex-1 rounded px-3 py-1.5 text-sm outline-none"
                  style={inputStyle}
                  disabled={submitting}
                >
                  <option value={0}>Any mob</option>
                  {SPECIAL_ABILITY_CODES.map((c) => (
                    <option key={c} value={c}>Has {specialAbilityName(c)}</option>
                  ))}
                </select>
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
            {(pipeKind === 'buff_stripped' || pipeKind === 'debuff_stripped') && (
              <div className="space-y-1">
                <input
                  type="text"
                  placeholder="Spell name (optional, blank = any)"
                  value={pipeSpellName}
                  onChange={(e) => setPipeSpellName(e.target.value)}
                  className="w-full rounded px-3 py-1.5 text-sm outline-none"
                  style={inputStyle}
                  disabled={submitting}
                />
                <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {pipeKind === 'buff_stripped'
                    ? 'Fires when one of your buffs leaves the buff window with '
                      + 'more than 12 s left and nothing replaced it. Clicking '
                      + 'a buff off also counts.'
                    : 'Fires when one of your detrimentals wears off with more '
                      + 'than 12 s left and a dispel lands on that mob, or two '
                      + 'of them fade at once. Works without Zeal.'}
                  {' '}Use <code>{'{spell}'}</code>
                  {pipeKind === 'debuff_stripped' && (
                    <> and <code>{'{target}'}</code> (the mob)</>
                  )}
                  {' '}in the action text.
                </p>
              </div>
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

        {/* Sample-line tester — log source only (pipe conditions aren't
            driven by pasted log lines). */}
        {source === 'log' && (
          <SampleLineTester buildDraft={buildTestDraft} disabled={submitting} />
        )}

      </FormSection>

      <FormSection
        title="Actions"
        hint="What happens when it fires: text, speech, a sound, clipboard or Discord."
        summary={`${actions.length} action${actions.length === 1 ? '' : 's'}`}
        open={openSections.actions}
        onToggle={() => toggleSection('actions')}
      >
        {/* Actions */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
              Actions
            </label>
            <div className="flex items-center gap-1.5">
              <ActionTemplatesMenu
                currentActions={actions}
                onApply={(a) => {
                  actionsTouched.current = true
                  setActions(a)
                }}
              />
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

      </FormSection>

      <FormSection
        title="Timer"
        hint="Optionally start a timer bar on a timer overlay."
        summary={timerType === 'none' ? 'No timer' : `${timerType} timer`}
        open={openSections.timer}
        onToggle={() => toggleSection('timer')}
      >
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
                  <DecimalInput
                    min={0}
                    value={timerDuration}
                    onValue={setTimerDuration}
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
                    placeholder="1 or {1} {2}"
                    className="w-28 rounded px-2 py-0.5 text-xs outline-none text-center font-mono"
                    style={inputStyle}
                    disabled={submitting}
                    title="Capture group number or name whose text names the timer (e.g. the spell name), or a template combining groups such as {1} {2} or {S1}: {S2}. Each distinct name runs its own countdown. The worn-off pattern must capture the same value. Empty = use the trigger name."
                  />
                  <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    group # / name, or a template like {'{1} {2}'}; empty = trigger name
                  </span>
                </div>
              )}
              {source === 'log' && timerType === 'custom' && (
                <div className="flex items-center gap-1.5">
                  <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                    Stack timers
                  </label>
                  <Toggle checked={timerStack} onChange={setTimerStack} />
                  <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    start a new bar each time this fires instead of restarting the existing one — e.g. one row per mob killed
                  </span>
                </div>
              )}
              {source === 'log' && timerType === 'custom' && !timerStack && (
                <div className="flex items-center gap-1.5">
                  <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                    Repeat timer
                  </label>
                  <Toggle checked={timerRepeat} onChange={setTimerRepeat} />
                  {timerRepeat && (
                    <>
                      <label className="text-[11px] shrink-0 ml-1" style={{ color: 'var(--color-muted-foreground)' }}>
                        max cycles
                      </label>
                      <DecimalInput
                        min={0}
                        value={timerRepeatMax}
                        onValue={(v) => setTimerRepeatMax(Math.max(0, Math.floor(v)))}
                        className="w-16 rounded px-2 py-0.5 text-xs outline-none text-center"
                        style={inputStyle}
                        disabled={submitting}
                        title="Stop after this many restarts. 0 = until the worn-off pattern clears it (with a safety cap of 200)."
                      />
                    </>
                  )}
                  <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    restart each time it runs out — for waves on a fixed cadence; the worn-off pattern stops it
                  </span>
                </div>
              )}
              {timerPolicyEligible && (
                <div className="flex items-center gap-1.5">
                  <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                    Keep first countdown
                  </label>
                  <Toggle checked={timerIfIdle} onChange={setTimerIfIdle} />
                  <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    ignore this trigger while its timer is running — no restart, no alerts; the worn-off pattern or expiry allows a new start
                  </span>
                </div>
              )}
              {timerPolicyEligible && (
                <div className="flex items-center gap-1.5">
                  <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                    Clear on zone or character change
                  </label>
                  <Toggle checked={timerClearOnContextChange} onChange={setTimerClearOnContextChange} />
                  <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    remove this timer when you zone, switch characters or start a log replay
                  </span>
                </div>
              )}
              <div className="flex items-center gap-1.5">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Display threshold (s)
                </label>
                <DecimalInput
                  min={0}
                  value={displayThreshold}
                  onValue={setDisplayThreshold}
                  className="w-20 rounded px-2 py-0.5 text-xs outline-none text-center"
                  style={inputStyle}
                  disabled={submitting}
                  title="Hide this timer until remaining ≤ this value. 0 uses the global default."
                />
                <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                  0 = use global default
                </span>
              </div>

              <div className="flex items-center gap-1.5">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Bar color
                </label>
                <input
                  type="color"
                  value={barColor || '#38bdf8'}
                  onChange={(e) => setBarColor(e.target.value)}
                  className="h-6 w-10 rounded cursor-pointer p-0"
                  style={{ border: '1px solid var(--color-border)', background: 'transparent' }}
                  disabled={submitting}
                  title="Custom color for this trigger's timer bar (overrides the automatic color)"
                />
                {barColor ? (
                  <button
                    type="button"
                    onClick={() => setBarColor('')}
                    className="text-[10px] underline"
                    style={{ color: 'var(--color-muted)' }}
                  >
                    reset to automatic
                  </button>
                ) : (
                  <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    automatic (click swatch to set)
                  </span>
                )}
              </div>

              <div className="flex items-center gap-1.5">
                <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                  Pin to top
                </label>
                <Toggle checked={pinned} onChange={setPinned} />
                <span className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                  keep this timer above unpinned timers in its overlay
                </span>
              </div>

              {timerType === 'custom' && (
                <div className="space-y-1">
                  <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                    Custom Timers window
                  </label>
                  {creatingGroup ? (
                    <div className="space-y-1">
                      <div className="flex gap-2">
                        <input
                          type="text"
                          autoFocus
                          placeholder="New window name"
                          value={newGroupName}
                          onChange={(e) => setNewGroupName(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') { e.preventDefault(); handleAddGroup() }
                            else if (e.key === 'Escape') { e.preventDefault(); setCreatingGroup(false); setGroupError(null) }
                          }}
                          className="flex-1 rounded px-3 py-1.5 text-sm outline-none"
                          style={inputStyle}
                          disabled={addingGroup}
                        />
                        <button
                          type="button"
                          onClick={handleAddGroup}
                          disabled={addingGroup || !newGroupName.trim()}
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
                          onClick={() => { setCreatingGroup(false); setGroupError(null); setNewGroupName('') }}
                          disabled={addingGroup}
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
                      {groupError && (
                        <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{groupError}</p>
                      )}
                    </div>
                  ) : (
                    <select
                      value={customGroupID}
                      onChange={(e) => handleGroupSelect(e.target.value)}
                      className="w-full rounded px-3 py-1.5 text-sm outline-none"
                      style={inputStyle}
                      disabled={submitting}
                    >
                      <option value="">Custom Timers (default window)</option>
                      {timerGroups.map((g) => (
                        <option key={g.id} value={g.id}>{g.name}</option>
                      ))}
                      {/* Defensive: keep the current value selectable even if the
                          group list hasn't loaded or doesn't include it yet
                          (e.g. the group was just deleted elsewhere). */}
                      {customGroupID && !timerGroups.some((g) => g.id === customGroupID) && (
                        <option value={customGroupID}>(unknown window)</option>
                      )}
                      <option value="__new__">+ New window…</option>
                    </select>
                  )}
                  <p className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
                    Split raid/boss timers into their own popout window — manage windows from
                    Custom Timers in the Overlays dashboard.
                  </p>
                </div>
              )}

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

      </FormSection>

      <FormSection
        title="Advanced"
        hint="Exclusions, cooldowns and repeat counting."
        summary={advancedSummary}
        open={openSections.advanced}
        onToggle={() => toggleSection('advanced')}
      >
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

        {/* Refire cooldown — applies to any trigger (log or pipe) */}
        <div className="space-y-1">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Refire cooldown (seconds)
          </label>
          <DecimalInput
            min={0}
            value={refireCooldown}
            onValue={setRefireCooldown}
            className="w-28 rounded px-3 py-1.5 text-xs outline-none"
            style={{ ...inputStyle, border: '1px solid var(--color-border)' }}
            disabled={submitting}
          />
          <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            After firing, ignore further matches for this many seconds. 0 = fire on every match.
            Useful for spammy lines (rampage, resists) without flooding alerts.
          </p>
        </div>

        {/* Repeat count window — log triggers only (pipe fires have no line run) */}
        {source === 'log' && (
          <div className="space-y-1">
            <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
              Repeat count window (seconds)
            </label>
            <DecimalInput
              min={0}
              value={repeatReset}
              onValue={setRepeatReset}
              className="w-28 rounded px-3 py-1.5 text-xs outline-none"
              style={{ ...inputStyle, border: '1px solid var(--color-border)' }}
              disabled={submitting}
            />
            <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              Counts matches that land within this many seconds of each other. Use{' '}
              <span className="font-mono">{'{repeated}'}</span> in the alert text (e.g.
              “PBAE hit {'{repeated}'}”) and the one alert updates in place instead of
              stacking. Sound and TTS play once per burst. 0 = off.
            </p>
          </div>
        )}

      </FormSection>

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
