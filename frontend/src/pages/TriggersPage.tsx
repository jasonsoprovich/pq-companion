import React, { useCallback, useEffect, useRef, useState } from 'react'
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
  MonitorPlay,
  Bell,
} from 'lucide-react'
import { useVoices } from '../hooks/useVoices'
import EventAlertsPanel from '../components/EventAlertsPanel'
import TimerAlertsPanel from '../components/TimerAlertsPanel'
import NotificationActionEditor, { NotificationTypeSelect } from '../components/NotificationActionEditor'
import {
  listTriggers,
  createTrigger,
  updateTrigger,
  deleteTrigger,
  getTriggerHistory,
  getBuiltinPacks,
  installBuiltinPack,
  importTriggerPack,
  exportTriggerPack,
  importGINAxml,
  type CreateTriggerRequest,
} from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import type { Trigger, TriggerFired, TriggerPack, Action, TimerType } from '../types/trigger'

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
              onChange(index, {
                type: t,
                text: action.text,
                duration_secs: action.duration_secs || 5,
                color: action.color || '#ffffff',
                sound_path: action.sound_path || '',
                volume: action.volume || 0,
                voice: action.voice || '',
              })
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
        color={action.color || '#ffffff'}
        onColorChange={(v) => onChange(index, { ...action, color: v })}
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
  onSaved: (t: Trigger) => void
  onCancel: () => void
}

function TriggerForm({ initial, onSaved, onCancel }: TriggerFormProps): React.ReactElement {
  const [name, setName] = useState(initial?.name ?? '')
  const [pattern, setPattern] = useState(initial?.pattern ?? '')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [actions, setActions] = useState<Action[]>(
    initial?.actions ?? [{ type: 'overlay_text', text: '', duration_secs: 5, color: '#ffffff', sound_path: '', volume: 0, voice: '' }],
  )
  const [timerType, setTimerType] = useState<TimerType>(initial?.timer_type ?? 'none')
  const [timerDuration, setTimerDuration] = useState(initial?.timer_duration_secs ?? 0)
  const [wornOffPattern, setWornOffPattern] = useState(initial?.worn_off_pattern ?? '')
  const [displayThreshold, setDisplayThreshold] = useState(initial?.display_threshold_secs ?? 0)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [patternError, setPatternError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => { nameRef.current?.focus() }, [])

  const validatePattern = (p: string) => {
    try {
      new RegExp(p)
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
    setActions((prev) => [...prev, { type: 'overlay_text', text: '', duration_secs: 5, color: '#ffffff', sound_path: '', volume: 0, voice: '' }])
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !pattern.trim()) return
    if (!validatePattern(pattern)) return

    const req: CreateTriggerRequest = {
      name: name.trim(),
      enabled,
      pattern: pattern.trim(),
      actions,
      timer_type: timerType,
      timer_duration_secs: timerType === 'none' ? 0 : Math.max(0, timerDuration),
      worn_off_pattern: timerType === 'none' ? '' : wornOffPattern.trim(),
      spell_id: initial?.spell_id ?? 0,
      display_threshold_secs: timerType === 'none' ? 0 : Math.max(0, displayThreshold),
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

      {/* Pattern */}
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
      </div>

      {/* Timer */}
      <div className="space-y-2">
        <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
          Spell timer
        </label>
        <div className="flex gap-1">
          {(['none', 'buff', 'detrimental'] as TimerType[]).map((tt) => {
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
              <input
                type="text"
                placeholder="worn-off regex (optional)"
                value={wornOffPattern}
                onChange={(e) => setWornOffPattern(e.target.value)}
                className="flex-1 rounded px-2 py-0.5 text-xs outline-none font-mono"
                style={inputStyle}
                disabled={submitting}
              />
            </div>
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

interface TriggerRowProps {
  trigger: Trigger
  onEdit: (t: Trigger) => void
  onDeleted: (id: string) => void
  onToggled: (t: Trigger) => void
}

function TriggerRow({ trigger, onEdit, onDeleted, onToggled }: TriggerRowProps): React.ReactElement {
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [toggling, setToggling] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState(false)
  const [shared, setShared] = useState(false)

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
    const req: CreateTriggerRequest = {
      name: trigger.name,
      enabled: v,
      pattern: trigger.pattern,
      actions: trigger.actions,
    }
    updateTrigger(trigger.id, req)
      .then((updated) => {
        onToggled(updated)
        setToggling(false)
      })
      .catch((err: Error) => {
        setError(err.message)
        setToggling(false)
      })
  }

  return (
    <div
      className="rounded-lg"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${trigger.enabled ? 'var(--color-border)' : 'var(--color-surface-3)'}`,
        opacity: trigger.enabled ? 1 : 0.65,
      }}
    >
      <div className="flex items-center gap-3 px-3 py-2.5">
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
            {trigger.pack_name && (
              <span
                className="text-[10px] px-1.5 py-0.5 rounded shrink-0"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {trigger.pack_name}
              </span>
            )}
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
            onClick={() => onEdit(trigger)}
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
    if (msg.type === 'trigger:fired') {
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
  onInstalled: () => void
}

function PacksTab({ onInstalled }: PacksTabProps): React.ReactElement {
  const [packs, setPacks] = useState<TriggerPack[]>([])
  const [loading, setLoading] = useState(true)
  const [installing, setInstalling] = useState<string | null>(null)
  const [installed, setInstalled] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
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
    setInstalling(packName)
    setError(null)
    installBuiltinPack(packName)
      .then(() => {
        setInstalled(packName)
        onInstalled()
        setTimeout(() => setInstalled(null), 3000)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setInstalling(null))
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
            "{installed}" installed successfully.
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
                      handleInstall(pack.pack_name)
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        e.stopPropagation()
                        handleInstall(pack.pack_name)
                      }
                    }}
                    aria-disabled={installing === pack.pack_name}
                    className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium shrink-0 cursor-pointer"
                    style={{
                      backgroundColor: installed === pack.pack_name
                        ? 'var(--color-surface-2)'
                        : 'var(--color-primary)',
                      color: installed === pack.pack_name
                        ? 'var(--color-success)'
                        : 'var(--color-background)',
                      border: '1px solid transparent',
                      opacity: installing === pack.pack_name ? 0.6 : 1,
                    }}
                  >
                    {installing === pack.pack_name ? (
                      <RefreshCw size={11} className="animate-spin" />
                    ) : installed === pack.pack_name ? (
                      <CheckCircle2 size={11} />
                    ) : (
                      <Download size={11} />
                    )}
                    {installed === pack.pack_name ? 'Installed' : 'Install'}
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
    </div>
  )
}

// ── Alerts tab ────────────────────────────────────────────────────────────────

type AlertSubTab = 'events' | 'timers'

function AlertsTab(): React.ReactElement {
  const [sub, setSub] = useState<AlertSubTab>('events')

  const pillStyle = (active: boolean): React.CSSProperties => ({
    padding: '4px 12px',
    fontSize: 12,
    fontWeight: 500,
    borderRadius: 4,
    border: '1px solid transparent',
    backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
    color: active ? 'var(--color-background)' : 'var(--color-muted-foreground)',
    cursor: 'pointer',
  })

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <div
        className="flex items-center gap-2 px-4 py-2 shrink-0"
        style={{ borderBottom: '1px solid var(--color-border)' }}
      >
        <button onClick={() => setSub('events')} style={pillStyle(sub === 'events')}>
          Event Alerts
        </button>
        <button onClick={() => setSub('timers')} style={pillStyle(sub === 'timers')}>
          Timer Alerts
        </button>
      </div>
      {sub === 'events' && <EventAlertsPanel inline />}
      {sub === 'timers' && <TimerAlertsPanel inline />}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

type Tab = 'triggers' | 'history' | 'packs' | 'alerts'

export default function TriggersPage(): React.ReactElement {
  const [tab, setTab] = useState<Tab>('triggers')
  const [triggers, setTriggers] = useState<Trigger[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [editing, setEditing] = useState<Trigger | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listTriggers()
      .then(setTriggers)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleSaved = (t: Trigger) => {
    if (editing) {
      setTriggers((prev) => prev.map((x) => (x.id === t.id ? t : x)))
      setEditing(null)
    } else {
      setTriggers((prev) => [...prev, t])
      setShowCreate(false)
    }
  }

  const handleDeleted = (id: string) => {
    setTriggers((prev) => prev.filter((t) => t.id !== id))
  }

  const handleToggled = (updated: Trigger) => {
    setTriggers((prev) => prev.map((t) => (t.id === updated.id ? updated : t)))
  }

  const handleEdit = (t: Trigger) => {
    setEditing(t)
    setShowCreate(false)
  }

  const handleCancelForm = () => {
    setEditing(null)
    setShowCreate(false)
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
          Custom Triggers
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => window.electron?.overlay?.toggleTrigger()}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Toggle trigger overlay window"
          >
            <MonitorPlay size={11} />
            Overlay
          </button>
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
                onClick={() => { setShowCreate((v) => !v); setEditing(null) }}
                className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
                style={{
                  backgroundColor: showCreate ? 'var(--color-surface-2)' : 'var(--color-primary)',
                  color: showCreate ? 'var(--color-muted-foreground)' : 'var(--color-background)',
                  border: `1px solid ${showCreate ? 'var(--color-border)' : 'transparent'}`,
                }}
              >
                <Plus size={11} />
                New Trigger
              </button>
            </>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div
        className="flex gap-0 border-b shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {(['triggers', 'history', 'packs', 'alerts'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className="px-4 py-2 text-xs font-medium capitalize transition-colors"
            style={tabStyle(t)}
          >
            {t === 'triggers' && <span>Triggers ({triggers.length})</span>}
            {t === 'history' && <span>History</span>}
            {t === 'packs' && <span>Packs</span>}
            {t === 'alerts' && (
              <span className="flex items-center gap-1">
                <Bell size={10} />
                Alerts
              </span>
            )}
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
            <div className="flex-1 overflow-y-auto p-4 space-y-3">
              {/* Create form */}
              {showCreate && !editing && (
                <TriggerForm onSaved={handleSaved} onCancel={handleCancelForm} />
              )}

              {/* Edit form */}
              {editing && (
                <TriggerForm initial={editing} onSaved={handleSaved} onCancel={handleCancelForm} />
              )}

              {/* Empty state */}
              {triggers.length === 0 && !showCreate && (
                <div className="flex h-full flex-col items-center justify-center gap-3 py-16">
                  <Zap size={32} style={{ color: 'var(--color-muted)' }} />
                  <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                    No triggers yet.
                  </p>
                  <div className="flex gap-2">
                    <button
                      onClick={() => setShowCreate(true)}
                      className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
                      style={{
                        backgroundColor: 'var(--color-primary)',
                        color: 'var(--color-background)',
                        border: '1px solid transparent',
                      }}
                    >
                      <Plus size={11} /> Create a trigger
                    </button>
                    <button
                      onClick={() => setTab('packs')}
                      className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        color: 'var(--color-foreground)',
                        border: '1px solid var(--color-border)',
                      }}
                    >
                      <Package size={11} /> Install a pack
                    </button>
                  </div>
                </div>
              )}

              {/* Trigger list */}
              {triggers.map((t) => (
                <TriggerRow
                  key={t.id}
                  trigger={t}
                  onEdit={handleEdit}
                  onDeleted={handleDeleted}
                  onToggled={handleToggled}
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* Tab: History */}
      {tab === 'history' && <HistoryTab />}

      {/* Tab: Packs */}
      {tab === 'packs' && <PacksTab onInstalled={load} />}

      {/* Tab: Alerts */}
      {tab === 'alerts' && <AlertsTab />}
    </div>
  )
}
