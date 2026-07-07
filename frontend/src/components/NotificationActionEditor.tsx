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
import { usePositioningSession } from '../hooks/usePositioningSession'
import { useOverlayTextDefaults } from '../hooks/useOverlayTextDefaults'
import { resolveOverlayTextStyle, WINDOWS_SAFE_FONTS } from '../lib/overlayTextStyle'
import { voiceLabel } from '../lib/piper'
import DecimalInput from './DecimalInput'

export type NotificationActionType =
  | 'overlay_text'
  | 'play_sound'
  | 'text_to_speech'
  | 'clipboard'

const TYPE_LABEL: Record<NotificationActionType, string> = {
  overlay_text: 'Overlay Text',
  play_sound: 'Play Sound',
  text_to_speech: 'Text to Speech',
  clipboard: 'Copy to Clipboard',
}

const ALL_TYPES: readonly NotificationActionType[] = [
  'overlay_text',
  'play_sound',
  'text_to_speech',
  'clipboard',
]

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
  /** Per-action text color; '' = inherit the app-default style. */
  color: string
  onColorChange: (v: string) => void
  /** Per-action glow color; '' = inherit (app default → text color). */
  glowColor?: string
  onGlowColorChange?: (v: string) => void
  /** Per-action font family; '' = inherit the app-default style. */
  fontFamily?: string
  onFontFamilyChange?: (v: string) => void
  /** Per-action font size in px; 0 = inherit the app-default style. */
  fontSize?: number
  onFontSizeChange?: (v: number) => void
  /**
   * Clears color/glow/font/size back to "App default" in ONE state update.
   * A dedicated callback (rather than four onChange calls) because parents
   * that spread a props-captured action object would clobber each other's
   * resets if called sequentially.
   */
  onStyleReset?: () => void
  textPlaceholder?: string
  position?: { x: number; y: number } | null
  onPositionChange?: (p: { x: number; y: number } | null) => void
}

/**
 * Color swatch with override semantics: the swatch always shows the color
 * that will actually render (the resolved value), but the field only stores
 * an override once the user picks something. While inheriting, a muted
 * "default" tag replaces the hex readout; an overridden field gains a tiny
 * reset button back to "App default".
 *
 * Exported for the Settings page's global-default style controls, which use
 * the same inherit-vs-override semantics against the built-in look.
 */
export function ColorOverrideField({
  label,
  value,
  resolved,
  onChange,
  resetTitle,
}: {
  label: string
  value: string
  resolved: string
  onChange: (v: string) => void
  resetTitle: string
}): React.ReactElement {
  return (
    <div className="flex items-center gap-1.5">
      <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
        {label}
      </label>
      <input
        type="color"
        value={resolved}
        onChange={(e) => onChange(e.target.value)}
        className="w-8 h-6 rounded cursor-pointer"
        style={{ border: '1px solid var(--color-border)', padding: '1px' }}
      />
      {value ? (
        <>
          <span className="text-[11px] font-mono" style={{ color: 'var(--color-muted)' }}>
            {value}
          </span>
          <button
            type="button"
            onClick={() => onChange('')}
            className="flex items-center justify-center rounded p-0.5"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted)',
              border: '1px solid var(--color-border)',
              cursor: 'pointer',
            }}
            title={resetTitle}
          >
            <XIcon size={9} />
          </button>
        </>
      ) : (
        <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
          default
        </span>
      )}
    </div>
  )
}

export function OverlayTextFields({
  text,
  onTextChange,
  durationSecs,
  onDurationSecsChange,
  color,
  onColorChange,
  glowColor = '',
  onGlowColorChange,
  fontFamily = '',
  onFontFamilyChange,
  fontSize = 0,
  onFontSizeChange,
  onStyleReset,
  textPlaceholder = 'Display text (e.g. MEZ BROKE!)',
  position,
  onPositionChange,
}: OverlayTextFieldsProps): React.ReactElement {
  const hasStyleOverride = Boolean(color || glowColor || fontFamily || fontSize > 0)
  // App-default style (Settings → Preferences), fetched once so the swatches
  // and size placeholder can show what an inherited field actually renders as.
  const styleDefaults = useOverlayTextDefaults()
  const resolved = resolveOverlayTextStyle(
    { color, glow_color: glowColor, font_family: fontFamily, font_size: fontSize },
    styleDefaults,
  )

  // Session id, live drag updates, Escape/unmount teardown, and confirm /
  // cancel semantics all live in the shared hook — the Settings page's
  // default-position control runs the identical flow. The resolved style
  // rides along so the positioning card doubles as a live preview that
  // restyles as the user edits these fields.
  const { positioning, toggle: handlePositionButton } = usePositioningSession({
    position,
    onPositionChange,
    testText: text,
    testColor: resolved.color,
    testGlowColor: resolved.glowColor,
    testFontFamily: resolved.fontFamily,
    testFontSize: resolved.fontSize,
    testDurationSecs: durationSecs,
  })

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
          <DecimalInput
            min={1}
            max={30}
            fallback={5}
            value={durationSecs}
            onValue={onDurationSecsChange}
            className="w-14 rounded px-2 py-0.5 text-xs outline-none text-center"
            style={inputStyle}
          />
        </div>
        <ColorOverrideField
          label="Color"
          value={color}
          resolved={resolved.color}
          onChange={onColorChange}
          resetTitle="Reset to the app-default text color"
        />
        <button
          type="button"
          onClick={handlePositionButton}
          className="flex items-center gap-1 rounded px-2 py-1 text-[11px] ml-auto"
          style={{
            backgroundColor: positioning ? '#16a34a' : 'var(--color-primary)',
            color: positioning ? '#fff' : 'var(--color-background)',
            border: '1px solid transparent',
            cursor: 'pointer',
          }}
          title={
            positioning
              ? 'Drag the on-screen card to position, then click here (or press Esc) to save and finish'
              : 'Pop up the alert in the overlay so you can drag it into position'
          }
        >
          {positioning ? <Check size={11} /> : <Crosshair size={11} />}
          {positioning ? 'Done — Save Position' : 'Set Trigger Position'}
        </button>
      </div>
      {/* Per-trigger style overrides. Every field starts on "default" (the
          app-wide style from Settings → Triggers & Alerts); setting a value
          here styles just this alert. */}
      <div className="flex gap-2 items-center flex-wrap">
        <ColorOverrideField
          label="Glow"
          value={glowColor}
          resolved={resolved.glowColor}
          onChange={(v) => onGlowColorChange?.(v)}
          resetTitle="Reset to the app-default glow (matches the text color when unset)"
        />
        <div className="flex items-center gap-1.5">
          <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            Font
          </label>
          <select
            value={fontFamily}
            onChange={(e) => onFontFamilyChange?.(e.target.value)}
            className="rounded px-2 py-0.5 text-xs outline-none max-w-36"
            style={{ ...selectStyle, fontFamily: fontFamily ? `'${fontFamily}'` : undefined }}
            title="Overlay font for this alert (fonts that ship with Windows)"
          >
            <option value="">App default</option>
            {WINDOWS_SAFE_FONTS.map((f) => (
              <option key={f} value={f} style={{ fontFamily: `'${f}'` }}>{f}</option>
            ))}
          </select>
        </div>
        <div className="flex items-center gap-1.5">
          <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
            Size
          </label>
          <input
            type="number"
            min={8}
            max={96}
            value={fontSize > 0 ? fontSize : ''}
            placeholder={String(resolved.fontSize)}
            onChange={(e) => onFontSizeChange?.(Math.max(0, parseInt(e.target.value) || 0))}
            className="w-14 rounded px-2 py-0.5 text-xs outline-none text-center"
            style={inputStyle}
            title="Overlay font size in pixels (blank = app default)"
          />
        </div>
        {hasStyleOverride && onStyleReset && (
          <button
            type="button"
            onClick={onStyleReset}
            className="ml-auto flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px]"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted)',
              border: '1px solid var(--color-border)',
              cursor: 'pointer',
            }}
            title="Reset color, glow, font, and size to the app-default style"
          >
            <XIcon size={9} />
            Reset Style
          </button>
        )}
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
            title="Clear pinned position (use the app default position)"
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
  // Strip capture placeholders ({...} and $N) for the test playback so the
  // user hears "Mez broke" rather than the literal "{1} broke" — there's no
  // live regex match behind the test button.
  const testText = text.replace(/\{[^}]+\}/g, '').replace(/\$\d+/g, '').replace(/\s+/g, ' ').trim()
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
              <option value="">App default</option>
              {voices.map((v) => (
                <option key={v} value={v}>{voiceLabel(v)}</option>
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

interface ClipboardFieldsProps {
  text: string
  onTextChange: (v: string) => void
  textPlaceholder?: string
}

/**
 * ClipboardFields — the editor for a "Copy to Clipboard" action. The text is
 * written to the system clipboard verbatim when the trigger fires (after the
 * usual {1}/$1 capture substitution), so the user can paste it straight into
 * EverQuest. Typical use: "/tar {1}" to target whoever the matched line named.
 */
export function ClipboardFields({
  text,
  onTextChange,
  textPlaceholder = 'Clipboard text (e.g. /tar {1})',
}: ClipboardFieldsProps): React.ReactElement {
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
      <p className="text-[10px] leading-snug" style={{ color: 'var(--color-muted)' }}>
        Copied to the clipboard when the trigger fires. Use {'{1}'} / $1 for
        capture groups — e.g. <span className="font-mono">/tar {'{1}'}</span> then
        paste in-game with Ctrl+V.
      </p>
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
  glowColor?: string
  onGlowColorChange?: (v: string) => void
  fontFamily?: string
  onFontFamilyChange?: (v: string) => void
  fontSize?: number
  onFontSizeChange?: (v: number) => void
  onStyleReset?: () => void
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

  // clipboard fields
  clipboardText?: string
  onClipboardTextChange?: (v: string) => void
  clipboardTextPlaceholder?: string
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
        color={props.color ?? ''}
        onColorChange={props.onColorChange ?? noop}
        glowColor={props.glowColor}
        onGlowColorChange={props.onGlowColorChange}
        fontFamily={props.fontFamily}
        onFontFamilyChange={props.onFontFamilyChange}
        fontSize={props.fontSize}
        onFontSizeChange={props.onFontSizeChange}
        onStyleReset={props.onStyleReset}
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
  if (type === 'clipboard') {
    return (
      <ClipboardFields
        text={props.clipboardText ?? ''}
        onTextChange={props.onClipboardTextChange ?? noop}
        textPlaceholder={props.clipboardTextPlaceholder}
      />
    )
  }
  return null
}

function noop() {}
