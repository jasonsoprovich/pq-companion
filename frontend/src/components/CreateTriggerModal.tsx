import React, { useEffect, useState } from 'react'
import { X, Zap, RefreshCw, Shield, Skull, Bell as BellIcon } from 'lucide-react'
import { createTrigger, type CreateTriggerRequest } from '../services/api'
import type { Action, TimerType, Trigger } from '../types/trigger'
import NotificationActionEditor, { NotificationTypeSelect } from './NotificationActionEditor'
import { useVoices } from '../hooks/useVoices'

export interface TriggerPrefill {
  name: string
  pattern: string
  wornOffPattern?: string
  timerType?: TimerType
  timerDurationSecs?: number
  spellId?: number
  displayText?: string
  displayColor?: string
  displayThresholdSecs?: number
}

interface CreateTriggerModalProps {
  prefill: TriggerPrefill
  onClose: () => void
  onCreated?: (t: Trigger) => void
}

/**
 * Escapes a string so it can be used as a regex literal. Mirrors Go's
 * regexp.QuoteMeta for the characters that matter to RE2-style engines.
 */
export function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * Modal for creating a trigger or spell-timer trigger, pre-filled from a DB
 * entry (spell, NPC ability, buff checklist, etc.). Renders as a centered
 * overlay dialog; call onClose to dismiss.
 */
function buildInitialAction(prefill: TriggerPrefill): Action {
  return {
    type: 'overlay_text',
    text: prefill.displayText ?? prefill.name,
    duration_secs: 5,
    color: prefill.displayColor ?? '#ffffff',
    sound_path: '',
    volume: 1,
    voice: '',
  }
}

/**
 * Returns true if the configured action has the content it needs to fire.
 * An action with no text (overlay/TTS) or no sound_path (sound) is treated
 * as "history-only" and dropped on save so the trigger logs without firing
 * a visible/audible alert.
 */
function actionHasContent(a: Action): boolean {
  if (a.type === 'overlay_text') return a.text.trim().length > 0
  if (a.type === 'play_sound') return a.sound_path.trim().length > 0
  if (a.type === 'text_to_speech') return a.text.trim().length > 0
  return false
}

export default function CreateTriggerModal({
  prefill,
  onClose,
  onCreated,
}: CreateTriggerModalProps): React.ReactElement {
  const [name, setName] = useState(prefill.name)
  const [pattern, setPattern] = useState(prefill.pattern)
  const [wornOff, setWornOff] = useState(prefill.wornOffPattern ?? '')
  const [timerType, setTimerType] = useState<TimerType>(prefill.timerType ?? 'none')
  const [duration, setDuration] = useState(prefill.timerDurationSecs ?? 0)
  const [displayThreshold, setDisplayThreshold] = useState(prefill.displayThresholdSecs ?? 0)
  const [action, setAction] = useState<Action>(() => buildInitialAction(prefill))
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [patternError, setPatternError] = useState<string | null>(null)
  const voices = useVoices()

  // Reset form whenever prefill changes (e.g. user picks a different spell)
  useEffect(() => {
    setName(prefill.name)
    setPattern(prefill.pattern)
    setWornOff(prefill.wornOffPattern ?? '')
    setTimerType(prefill.timerType ?? 'none')
    setDuration(prefill.timerDurationSecs ?? 0)
    setDisplayThreshold(prefill.displayThresholdSecs ?? 0)
    setAction(buildInitialAction(prefill))
    setError(null)
    setPatternError(null)
  }, [
    prefill.name,
    prefill.pattern,
    prefill.wornOffPattern,
    prefill.timerType,
    prefill.timerDurationSecs,
    prefill.displayThresholdSecs,
    prefill.displayText,
    prefill.displayColor,
  ])

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

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !pattern.trim()) return
    if (!validatePattern(pattern)) return
    if (wornOff && !(() => { try { new RegExp(wornOff); return true } catch { return false } })()) {
      setPatternError('invalid worn-off regex')
      return
    }

    const actions: Action[] = actionHasContent(action) ? [action] : []

    const req: CreateTriggerRequest = {
      name: name.trim(),
      enabled: true,
      pattern: pattern.trim(),
      actions,
      timer_type: timerType,
      timer_duration_secs: timerType === 'none' ? 0 : Math.max(0, duration),
      worn_off_pattern: timerType === 'none' ? '' : wornOff.trim(),
      spell_id: prefill.spellId ?? 0,
      display_threshold_secs: timerType === 'none' ? 0 : Math.max(0, displayThreshold),
    }

    setSubmitting(true)
    setError(null)
    createTrigger(req)
      .then((t) => {
        if (onCreated) onCreated(t)
        onClose()
      })
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
    <div
      onClick={onClose}
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
      <form
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
        className="rounded-lg p-4 space-y-3"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 520,
          maxHeight: '90vh',
          overflowY: 'auto',
        }}
      >
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold flex items-center gap-1.5" style={{ color: 'var(--color-foreground)' }}>
            <Zap size={13} style={{ color: 'var(--color-primary)' }} />
            Create Trigger
          </p>
          <button type="button" onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }}>
            <X size={14} />
          </button>
        </div>

        {/* Name */}
        <div className="space-y-1">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>Name</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded px-3 py-1.5 text-sm outline-none"
            style={inputStyle}
            disabled={submitting}
          />
        </div>

        {/* Pattern */}
        <div className="space-y-1">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>Match pattern (regex)</label>
          <input
            type="text"
            value={pattern}
            onChange={(e) => { setPattern(e.target.value); if (e.target.value) validatePattern(e.target.value) }}
            className="w-full rounded px-3 py-1.5 text-sm outline-none font-mono"
            style={{ ...inputStyle, border: `1px solid ${patternError ? 'var(--color-danger)' : 'var(--color-border)'}` }}
            disabled={submitting}
          />
          {patternError && <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{patternError}</p>}
        </div>

        {/* Timer type */}
        <div className="space-y-1">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>Timer</label>
          <div className="flex gap-1">
            {(['none', 'buff', 'detrimental'] as TimerType[]).map((tt) => {
              const active = timerType === tt
              const icon = tt === 'buff' ? <Shield size={11} /> : tt === 'detrimental' ? <Skull size={11} /> : <BellIcon size={11} />
              return (
                <button
                  key={tt}
                  type="button"
                  onClick={() => setTimerType(tt)}
                  className="flex-1 flex items-center justify-center gap-1 rounded px-2 py-1 text-xs"
                  style={{
                    backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                    color: active ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                    border: '1px solid transparent',
                  }}
                >
                  {icon}
                  {tt === 'none' ? 'No timer' : tt === 'buff' ? 'Buff' : 'Detrimental'}
                </button>
              )
            })}
          </div>
        </div>

        {/* Timer fields (only when a timer is enabled) */}
        {timerType !== 'none' && (
          <>
            <div className="space-y-1">
              <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>Duration (seconds)</label>
              <input
                type="number"
                min={0}
                value={duration}
                onChange={(e) => setDuration(Math.max(0, parseInt(e.target.value) || 0))}
                className="w-full rounded px-3 py-1.5 text-sm outline-none"
                style={inputStyle}
                disabled={submitting}
              />
            </div>
            <div className="space-y-1">
              <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                Worn-off pattern (regex, optional — clears the timer early)
              </label>
              <input
                type="text"
                value={wornOff}
                onChange={(e) => setWornOff(e.target.value)}
                className="w-full rounded px-3 py-1.5 text-sm outline-none font-mono"
                style={inputStyle}
                disabled={submitting}
              />
            </div>
            <div className="space-y-1">
              <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
                Display threshold (seconds, 0 = use global default)
              </label>
              <input
                type="number"
                min={0}
                value={displayThreshold}
                onChange={(e) => setDisplayThreshold(Math.max(0, parseInt(e.target.value) || 0))}
                className="w-full rounded px-3 py-1.5 text-sm outline-none"
                style={inputStyle}
                disabled={submitting}
              />
              <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                Hide this timer until its remaining time is at or below this value. Overrides the global Buff / Detrimental defaults in Settings.
              </p>
            </div>
          </>
        )}

        {/* Notification action — overlay text, sound, or TTS. Leave the
            type-specific field empty to make this a history-only trigger. */}
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
              Action (optional)
            </label>
            <NotificationTypeSelect
              value={action.type}
              onChange={(t) => setAction((prev) => ({ ...prev, type: t }))}
              className="rounded px-2 py-0.5 text-xs outline-none"
            />
          </div>
          <div
            className="rounded p-3 space-y-2"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
          >
            <NotificationActionEditor
              type={action.type}
              voices={voices}
              overlayText={action.text}
              overlayTextPlaceholder="leave blank for history-only trigger"
              onOverlayTextChange={(v) => setAction((prev) => ({ ...prev, text: v }))}
              durationSecs={action.duration_secs || 5}
              onDurationSecsChange={(v) => setAction((prev) => ({ ...prev, duration_secs: v }))}
              color={action.color || '#ffffff'}
              onColorChange={(v) => setAction((prev) => ({ ...prev, color: v }))}
              position={action.position ?? null}
              onPositionChange={(p) => setAction((prev) => ({ ...prev, position: p }))}
              soundPath={action.sound_path}
              onSoundPathChange={(v) => setAction((prev) => ({ ...prev, sound_path: v }))}
              soundVolume={Math.round((action.volume || 1) * 100)}
              onSoundVolumeChange={(v) => setAction((prev) => ({ ...prev, volume: v / 100 }))}
              ttsText={action.text}
              onTtsTextChange={(v) => setAction((prev) => ({ ...prev, text: v }))}
              voice={action.voice}
              onVoiceChange={(v) => setAction((prev) => ({ ...prev, voice: v }))}
              ttsVolume={Math.round((action.volume || 1) * 100)}
              onTtsVolumeChange={(v) => setAction((prev) => ({ ...prev, volume: v / 100 }))}
            />
          </div>
        </div>

        {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}

        <div className="flex items-center justify-end gap-2 pt-1">
          <button
            type="button"
            onClick={onClose}
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
            {submitting ? 'Saving…' : 'Create Trigger'}
          </button>
        </div>
      </form>
    </div>
  )
}
