// TimerAlertPrefEditor — settings control for a global TimerAlertPref (the
// default fading-soon alert for the Custom Timer and Respawn overlays). It
// reuses the trigger editor's NotificationActionEditor so the sound/TTS fields
// (file picker, voice list, volume sliders, test buttons) match the per-trigger
// experience exactly.

import React from 'react'
import NotificationActionEditor, {
  NotificationTypeSelect,
  type NotificationActionType,
} from '../NotificationActionEditor'
import { useVoices } from '../../hooks/useVoices'
import { useTTSVoices } from '../../hooks/usePiperStatus'
import type { TimerAlertPref } from '../../types/config'

interface TimerAlertPrefEditorProps {
  value: TimerAlertPref
  onChange: (next: TimerAlertPref) => void
  /** Label + unit + hint for the threshold input, since "remaining" means
   *  different things for a countdown vs a respawn. Omit alongside
   *  showSeconds=false for alert kinds with no threshold concept. */
  secondsLabel?: string
  secondsUnit?: string
  secondsHint?: string
  /** Placeholder shown in the TTS text box (documents the supported token). */
  ttsPlaceholder: string
  /** Set false to hide the threshold input entirely (default true) — for
   *  alert kinds, like the CH Metronome's, that fire on a state edge rather
   *  than a remaining-time crossing. */
  showSeconds?: boolean
}

export default function TimerAlertPrefEditor({
  value,
  onChange,
  secondsLabel,
  secondsUnit,
  secondsHint,
  ttsPlaceholder,
  showSeconds = true,
}: TimerAlertPrefEditorProps): React.ReactElement {
  const voices = useTTSVoices(useVoices())

  const inputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface-2)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
    borderRadius: 4,
    padding: '3px 7px',
    fontSize: 12,
    outline: 'none',
  }

  return (
    <div className="space-y-3">
      <label className="flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={value.enabled}
          onChange={(e) => onChange({ ...value, enabled: e.target.checked })}
          style={{ marginTop: 3 }}
        />
        <span>
          <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Enable alert
          </span>
          {secondsHint && (
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {secondsHint}
            </span>
          )}
        </span>
      </label>

      {value.enabled && (
        <div
          className="rounded p-3 space-y-2"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        >
          <div className="flex items-center gap-2 flex-wrap">
            {showSeconds && (
              <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {secondsLabel}
                <input
                  type="number"
                  min={0}
                  max={3600}
                  value={value.seconds}
                  onChange={(e) =>
                    onChange({ ...value, seconds: Math.max(0, parseInt(e.target.value) || 0) })
                  }
                  style={{ ...inputStyle, width: 64 }}
                />
                {secondsUnit}
              </label>
            )}

            <div className="flex items-center gap-1.5 text-xs ml-auto" style={{ color: 'var(--color-muted-foreground)' }}>
              <span>Type</span>
              <NotificationTypeSelect
                value={value.type}
                onChange={(t) => onChange({ ...value, type: t as TimerAlertPref['type'] })}
                allowedTypes={['text_to_speech', 'play_sound'] as NotificationActionType[]}
                className="rounded px-2 py-0.5 text-xs outline-none"
              />
            </div>
          </div>

          <NotificationActionEditor
            type={value.type}
            voices={voices}
            ttsText={value.tts_template}
            onTtsTextChange={(v) => onChange({ ...value, tts_template: v })}
            ttsTextPlaceholder={ttsPlaceholder}
            voice={value.voice}
            onVoiceChange={(v) => onChange({ ...value, voice: v })}
            ttsVolume={value.tts_volume}
            onTtsVolumeChange={(v) => onChange({ ...value, tts_volume: v })}
            soundPath={value.sound_path}
            onSoundPathChange={(v) => onChange({ ...value, sound_path: v })}
            soundVolume={value.volume}
            onSoundVolumeChange={(v) => onChange({ ...value, volume: v })}
          />
        </div>
      )}
    </div>
  )
}
