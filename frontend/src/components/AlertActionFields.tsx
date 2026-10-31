/**
 * AlertActionFields — the overlay text / text-to-speech / sound sections of a
 * built-in (non-trigger) alert's settings dialog, shared by
 * WishlistAlertSettings and PopHailReminderSettings. Both back a config
 * struct with the same flat action fields (see AlertActionPrefs), and the
 * alert text lives in the dialog's own template field, so these sections
 * carry no per-action text input.
 */
import React from 'react'
import { Volume2 } from 'lucide-react'
import { voiceLabel } from '../lib/piper'
import { PlaySoundFields } from './NotificationActionEditor'
import DecimalInput from './DecimalInput'

export interface AlertActionPrefs {
  overlay_enabled?: boolean
  overlay_color?: string
  overlay_duration_secs?: number
  tts_enabled?: boolean
  tts_voice?: string
  tts_volume?: number
  sound_enabled?: boolean
  sound_path?: string
  sound_volume?: number
}

interface AlertActionFieldsProps {
  prefs: AlertActionPrefs
  patch: (p: Partial<AlertActionPrefs>) => void
  voices: string[]
}

const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

export default function AlertActionFields({
  prefs,
  patch,
  voices,
}: AlertActionFieldsProps): React.ReactElement {
  return (
    <>
      {/* Overlay text */}
      <div className="space-y-2 pt-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={prefs.overlay_enabled ?? false}
            onChange={(e) => patch({ overlay_enabled: e.target.checked })}
          />
          <span className="text-xs font-semibold">Overlay text</span>
        </label>
        {prefs.overlay_enabled && (
          <div className="flex gap-3 items-center flex-wrap pl-5">
            <div className="flex items-center gap-1.5">
              <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                Duration (s)
              </label>
              <DecimalInput
                min={1}
                max={30}
                fallback={5}
                value={prefs.overlay_duration_secs || 5}
                onValue={(v) => patch({ overlay_duration_secs: v })}
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
                value={prefs.overlay_color || '#fbbf24'}
                onChange={(e) => patch({ overlay_color: e.target.value })}
                className="w-8 h-6 rounded cursor-pointer"
                style={{ border: '1px solid var(--color-border)', padding: 1 }}
              />
            </div>
          </div>
        )}
      </div>

      {/* Text to speech */}
      <div className="space-y-2 pt-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={prefs.tts_enabled ?? false}
            onChange={(e) => patch({ tts_enabled: e.target.checked })}
          />
          <span className="text-xs font-semibold">Text to speech</span>
        </label>
        {prefs.tts_enabled && (
          <div className="flex gap-3 pl-5 min-w-0">
            <div className="flex items-center gap-1.5 flex-1 min-w-0">
              <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
                Voice
              </label>
              {voices.length > 0 ? (
                <select
                  value={prefs.tts_voice ?? ''}
                  onChange={(e) => patch({ tts_voice: e.target.value })}
                  className="rounded px-2 py-0.5 text-xs outline-none flex-1 min-w-0"
                  style={{ ...inputStyle, appearance: 'none' }}
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
                  value={prefs.tts_voice ?? ''}
                  onChange={(e) => patch({ tts_voice: e.target.value })}
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
                value={prefs.tts_volume ?? 100}
                onChange={(e) => patch({ tts_volume: parseInt(e.target.value) || 0 })}
                className="w-20"
              />
              <span className="text-[11px] w-8 text-right font-mono" style={{ color: 'var(--color-muted)' }}>
                {prefs.tts_volume ?? 100}%
              </span>
            </div>
          </div>
        )}
      </div>

      {/* Sound */}
      <div className="space-y-2 pt-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={prefs.sound_enabled ?? false}
            onChange={(e) => patch({ sound_enabled: e.target.checked })}
          />
          <span className="text-xs font-semibold">Play sound</span>
        </label>
        {prefs.sound_enabled && (
          <div className="pl-5">
            <PlaySoundFields
              soundPath={prefs.sound_path ?? ''}
              onSoundPathChange={(v) => patch({ sound_path: v })}
              volume={prefs.sound_volume ?? 100}
              onVolumeChange={(v) => patch({ sound_volume: v })}
            />
          </div>
        )}
      </div>
    </>
  )
}
