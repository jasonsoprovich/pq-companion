/**
 * NotificationActionEditor — shared editor for the three notification action
 * types used by triggers, event alerts, and timer alerts:
 *
 *   - overlay_text     on-screen popup text
 *   - play_sound       a local audio file
 *   - text_to_speech   spoken text via the Web Speech API
 *
 * Pure controlled component: the parent owns all state and supplies field
 * values + onChange callbacks. Volume props are always 0–100 (the parent
 * scales to whatever its data shape uses).
 *
 * Test buttons (file browse / play / position) are left as no-op slots in
 * this initial extraction and wired up in subsequent tasks.
 */
import React, { useEffect, useState } from 'react'
import { Volume2, FolderOpen, Play, Square, Crosshair, Check, X as XIcon } from 'lucide-react'
import { playSoundForTest, speakTextForTest, stopTestPlayback } from '../services/audio'
import { fireTriggerTestOverlay, endTriggerTestSession } from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export type NotificationActionType = 'overlay_text' | 'play_sound' | 'text_to_speech'

const TYPE_LABEL: Record<NotificationActionType, string> = {
  overlay_text: 'Overlay Text',
  play_sound: 'Play Sound',
  text_to_speech: 'Text to Speech',
}

const ALL_TYPES: readonly NotificationActionType[] = ['overlay_text', 'play_sound', 'text_to_speech']

const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}
const selectStyle: React.CSSProperties = { ...inputStyle, appearance: 'none' }

// ── Sub-editors (one per action type) ─────────────────────────────────────────

interface OverlayTextFieldsProps {
  text: string
  onTextChange: (v: string) => void
  durationSecs: number
  onDurationSecsChange: (v: number) => void
  color: string
  onColorChange: (v: string) => void
  textPlaceholder?: string
  position?: { x: number; y: number } | null
  onPositionChange?: (p: { x: number; y: number } | null) => void
}

export function OverlayTextFields({
  text,
  onTextChange,
  durationSecs,
  onDurationSecsChange,
  color,
  onColorChange,
  textPlaceholder = 'Display text (e.g. MEZ BROKE!)',
  position,
  onPositionChange,
}: OverlayTextFieldsProps): React.ReactElement {
  // A per-editor session id is round-tripped through the test endpoints so
  // simultaneous editors don't clobber each other's position updates.
  const [testId] = useState(() => `test-${Math.random().toString(36).slice(2)}-${Date.now()}`)
  const [positioning, setPositioning] = useState(false)

  useWebSocket((msg) => {
    if (msg.type === WSEvent.TriggerTestPosition) {
      const data = msg.data as { test_id: string; position: { x: number; y: number } }
      if (data.test_id !== testId) return
      onPositionChange?.(data.position)
      return
    }
    if (msg.type === WSEvent.TriggerTestSessionEnded) {
      const data = msg.data as { test_id: string }
      if (data.test_id !== testId) return
      setPositioning(false)
      return
    }
  })

  // End any open positioning session if this editor unmounts.
  useEffect(() => {
    return () => {
      if (positioning) void endTriggerTestSession(testId).catch(() => {})
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function handleTogglePositioning() {
    if (positioning) {
      // Second click confirms / saves: drop position is already auto-saved on
      // each drag-release; this just dismisses the positioning UI.
      setPositioning(false)
      void endTriggerTestSession(testId).catch(() => {})
      return
    }
    setPositioning(true)
    void window.electron?.overlay?.openTrigger?.()
    void fireTriggerTestOverlay({
      test_id: testId,
      text: text || 'Test alert',
      color: color || '#ffffff',
      // duration_secs is informational only — sticky session, no auto-dismiss.
      duration_secs: Math.max(8, durationSecs || 5),
      position: position ?? null,
    }).catch(() => {
      // If we can't open the session, roll the toggle back so the button
      // doesn't get stuck in the "Done" state.
      setPositioning(false)
    })
  }

  function handleClearPosition() {
    onPositionChange?.(null)
  }

  return (
    <>
      <input
        type="text"
        placeholder={textPlaceholder}
        value={text}
        onChange={(e) => onTextChange(e.target.value)}
        className="w-full rounded px-2 py-1 text-xs outline-none font-mono"
        style={inputStyle}
      />
      <div className="flex gap-2 items-center flex-wrap">
        <div className="flex items-center gap-1.5">
          <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            Duration (s)
          </label>
          <input
            type="number"
            min={1}
            max={30}
            value={durationSecs}
            onChange={(e) => onDurationSecsChange(Math.max(1, parseInt(e.target.value) || 5))}
            className="w-14 rounded px-2 py-0.5 text-xs outline-none text-center"
            style={inputStyle}
          />
        </div>
        <div className="flex items-center gap-1.5">
          <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            Color
          </label>
          <input
            type="color"
            value={color}
            onChange={(e) => onColorChange(e.target.value)}
            className="w-8 h-6 rounded cursor-pointer"
            style={{ border: '1px solid var(--color-border)', padding: '1px' }}
          />
          <span className="text-[11px] font-mono" style={{ color: 'var(--color-muted)' }}>
            {color}
          </span>
        </div>
        <button
          type="button"
          onClick={handleTogglePositioning}
          className="flex items-center gap-1 rounded px-2 py-1 text-[11px] ml-auto"
          style={{
            backgroundColor: positioning ? 'var(--color-success, #16a34a)' : 'var(--color-primary)',
            color: 'var(--color-background)',
            border: '1px solid transparent',
            cursor: 'pointer',
          }}
          title={
            positioning
              ? 'Lock in the current position and dismiss the positioning overlay'
              : 'Pop up the alert in the overlay so you can drag it into position'
          }
        >
          {positioning ? <Check size={11} /> : <Crosshair size={11} />}
          {positioning ? 'Done — Save Position' : 'Set Trigger Position'}
        </button>
      </div>
      {position && (
        <div
          className="flex items-center gap-1.5 text-[10px] rounded px-2 py-1"
          style={{
            color: 'var(--color-muted-foreground)',
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            fontFamily: 'monospace',
          }}
        >
          <span>Pinned at x={position.x}, y={position.y}</span>
          <button
            type="button"
            onClick={handleClearPosition}
            className="ml-auto flex items-center gap-1 px-1 py-0.5 rounded"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted)',
              border: '1px solid var(--color-border)',
              cursor: 'pointer',
              fontFamily: 'inherit',
            }}
            title="Clear pinned position (use default stacking)"
          >
            <XIcon size={9} />
            Reset
          </button>
        </div>
      )}
    </>
  )
}

interface PlaySoundFieldsProps {
  soundPath: string
  onSoundPathChange: (v: string) => void
  /** 0–100. */
  volume: number
  onVolumeChange: (v: number) => void
}

export function PlaySoundFields({
  soundPath,
  onSoundPathChange,
  volume,
  onVolumeChange,
}: PlaySoundFieldsProps): React.ReactElement {
  const canBrowse = typeof window !== 'undefined' && !!window.electron?.dialog?.selectSoundFile
  const canTest = soundPath.trim().length > 0
  const [playing, setPlaying] = useState(false)

  // If this component unmounts while playback is ours, stop it.
  useEffect(() => () => { if (playing) stopTestPlayback() }, [playing])

  async function handleBrowse() {
    const picked = await window.electron?.dialog?.selectSoundFile()
    if (picked) onSoundPathChange(picked)
  }

  function handleTest() {
    if (playing) {
      stopTestPlayback()
      return
    }
    setPlaying(true)
    playSoundForTest(soundPath, volume / 100, () => setPlaying(false))
  }

  return (
    <>
      <div className="flex items-center gap-1.5">
        <input
          type="text"
          placeholder="Sound file path (e.g. C:\sounds\alert.wav)"
          value={soundPath}
          onChange={(e) => onSoundPathChange(e.target.value)}
          className="flex-1 min-w-0 rounded px-2 py-1 text-xs outline-none font-mono"
          style={inputStyle}
        />
        {canBrowse && (
          <button
            type="button"
            onClick={handleBrowse}
            className="shrink-0 flex items-center justify-center rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-muted-foreground)',
              cursor: 'pointer',
            }}
            title="Browse for sound file"
          >
            <FolderOpen size={12} />
          </button>
        )}
        <button
          type="button"
          onClick={handleTest}
          disabled={!canTest}
          className="shrink-0 flex items-center justify-center rounded px-2 py-1 text-xs"
          style={{
            backgroundColor: canTest ? 'var(--color-primary)' : 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            color: canTest ? 'var(--color-background)' : 'var(--color-muted)',
            cursor: canTest ? 'pointer' : 'not-allowed',
          }}
          title={
            !canTest ? 'Enter a sound file path to test'
              : playing ? 'Stop playback'
              : 'Play sound at the configured volume'
          }
        >
          {playing ? <Square size={12} /> : <Play size={12} />}
        </button>
      </div>
      <div className="flex items-center gap-1.5">
        <label className="text-[11px] shrink-0 mr-auto" style={{ color: 'var(--color-muted-foreground)' }}>
          Volume
        </label>
        <Volume2 size={12} style={{ color: 'var(--color-muted-foreground)' }} />
        <input
          type="range"
          min={0}
          max={100}
          value={volume}
          onChange={(e) => onVolumeChange(parseInt(e.target.value) || 0)}
          className="w-32"
        />
        <span className="text-[11px] w-8 text-right font-mono" style={{ color: 'var(--color-muted)' }}>
          {volume}%
        </span>
      </div>
    </>
  )
}

interface TextToSpeechFieldsProps {
  text: string
  onTextChange: (v: string) => void
  textPlaceholder?: string
  voice: string
  onVoiceChange: (v: string) => void
  voices: string[]
  /** 0–100. */
  volume: number
  onVolumeChange: (v: number) => void
}

export function TextToSpeechFields({
  text,
  onTextChange,
  textPlaceholder = 'Text to speak (e.g. Mez broke)',
  voice,
  onVoiceChange,
  voices,
  volume,
  onVolumeChange,
}: TextToSpeechFieldsProps): React.ReactElement {
  // Strip simple {placeholder} tokens for the test playback so the user
  // hears something like "Mez broke" instead of literal "{spell} broke".
  const testText = text.replace(/\{[^}]+\}/g, '').replace(/\s+/g, ' ').trim()
  const canTest = testText.length > 0
  const [playing, setPlaying] = useState(false)

  useEffect(() => () => { if (playing) stopTestPlayback() }, [playing])

  function handleTest() {
    if (playing) {
      stopTestPlayback()
      return
    }
    setPlaying(true)
    speakTextForTest(testText, voice, volume / 100, () => setPlaying(false))
  }

  return (
    <>
      <div className="flex items-center gap-1.5">
        <input
          type="text"
          placeholder={textPlaceholder}
          value={text}
          onChange={(e) => onTextChange(e.target.value)}
          className="flex-1 min-w-0 rounded px-2 py-1 text-xs outline-none font-mono"
          style={inputStyle}
        />
        <button
          type="button"
          onClick={handleTest}
          disabled={!canTest}
          className="shrink-0 flex items-center justify-center rounded px-2 py-1 text-xs"
          style={{
            backgroundColor: canTest ? 'var(--color-primary)' : 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            color: canTest ? 'var(--color-background)' : 'var(--color-muted)',
            cursor: canTest ? 'pointer' : 'not-allowed',
          }}
          title={
            !canTest ? 'Enter text to test'
              : playing ? 'Stop playback'
              : 'Speak text with the configured voice and volume'
          }
        >
          {playing ? <Square size={12} /> : <Play size={12} />}
        </button>
      </div>
      <div className="flex gap-2 min-w-0">
        <div className="flex items-center gap-1.5 flex-1 min-w-0">
          <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            Voice
          </label>
          {voices.length > 0 ? (
            <select
              value={voice}
              onChange={(e) => onVoiceChange(e.target.value)}
              className="rounded px-2 py-0.5 text-xs outline-none flex-1 min-w-0"
              style={selectStyle}
            >
              <option value="">System default</option>
              {voices.map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </select>
          ) : (
            <input
              type="text"
              placeholder="Voice name (leave blank for default)"
              value={voice}
              onChange={(e) => onVoiceChange(e.target.value)}
              className="rounded px-2 py-0.5 text-xs outline-none flex-1 font-mono"
              style={inputStyle}
            />
          )}
        </div>
        <div className="flex items-center gap-1.5">
          <Volume2 size={12} style={{ color: 'var(--color-muted-foreground)' }} />
          <input
            type="range"
            min={0}
            max={100}
            value={volume}
            onChange={(e) => onVolumeChange(parseInt(e.target.value) || 0)}
            className="w-20"
          />
          <span className="text-[11px] w-8 text-right font-mono" style={{ color: 'var(--color-muted)' }}>
            {volume}%
          </span>
        </div>
      </div>
    </>
  )
}

// ── Type selector ─────────────────────────────────────────────────────────────

interface TypeSelectorProps {
  value: NotificationActionType
  onChange: (t: NotificationActionType) => void
  allowedTypes?: readonly NotificationActionType[]
  className?: string
}

export function NotificationTypeSelect({
  value,
  onChange,
  allowedTypes = ALL_TYPES,
  className,
}: TypeSelectorProps): React.ReactElement {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as NotificationActionType)}
      className={className ?? 'rounded px-2 py-0.5 text-xs outline-none flex-1 min-w-0'}
      style={selectStyle}
    >
      {allowedTypes.map((t) => (
        <option key={t} value={t}>{TYPE_LABEL[t]}</option>
      ))}
    </select>
  )
}

// ── Composite editor ──────────────────────────────────────────────────────────

interface NotificationActionEditorProps {
  type: NotificationActionType
  voices: string[]

  // overlay_text fields
  overlayText?: string
  onOverlayTextChange?: (v: string) => void
  overlayTextPlaceholder?: string
  durationSecs?: number
  onDurationSecsChange?: (v: number) => void
  color?: string
  onColorChange?: (v: string) => void
  position?: { x: number; y: number } | null
  onPositionChange?: (p: { x: number; y: number } | null) => void

  // play_sound fields
  soundPath?: string
  onSoundPathChange?: (v: string) => void
  /** 0–100. */
  soundVolume?: number
  onSoundVolumeChange?: (v: number) => void

  // text_to_speech fields
  ttsText?: string
  onTtsTextChange?: (v: string) => void
  ttsTextPlaceholder?: string
  voice?: string
  onVoiceChange?: (v: string) => void
  /** 0–100. */
  ttsVolume?: number
  onTtsVolumeChange?: (v: number) => void
}

/**
 * Renders the per-type field block for the currently selected action type.
 * The parent is responsible for the surrounding card chrome and the type
 * selector — use {@link NotificationTypeSelect} alongside this component.
 */
export default function NotificationActionEditor(
  props: NotificationActionEditorProps,
): React.ReactElement | null {
  const { type, voices } = props

  if (type === 'overlay_text') {
    return (
      <OverlayTextFields
        text={props.overlayText ?? ''}
        onTextChange={props.onOverlayTextChange ?? noop}
        durationSecs={props.durationSecs ?? 5}
        onDurationSecsChange={props.onDurationSecsChange ?? noop}
        color={props.color ?? '#ffffff'}
        onColorChange={props.onColorChange ?? noop}
        textPlaceholder={props.overlayTextPlaceholder}
        position={props.position}
        onPositionChange={props.onPositionChange}
      />
    )
  }
  if (type === 'play_sound') {
    return (
      <PlaySoundFields
        soundPath={props.soundPath ?? ''}
        onSoundPathChange={props.onSoundPathChange ?? noop}
        volume={props.soundVolume ?? 100}
        onVolumeChange={props.onSoundVolumeChange ?? noop}
      />
    )
  }
  if (type === 'text_to_speech') {
    return (
      <TextToSpeechFields
        text={props.ttsText ?? ''}
        onTextChange={props.onTtsTextChange ?? noop}
        textPlaceholder={props.ttsTextPlaceholder}
        voice={props.voice ?? ''}
        onVoiceChange={props.onVoiceChange ?? noop}
        voices={voices}
        volume={props.ttsVolume ?? 100}
        onVolumeChange={props.onTtsVolumeChange ?? noop}
      />
    )
  }
  return null
}

function noop() {}
