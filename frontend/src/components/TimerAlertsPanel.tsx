/**
 * TimerAlertsPanel — configuration panel for Timer Audio Alerts.
 *
 * Rendered inside the Triggers tab's Alerts sub-section. Supports an
 * `inline` prop that drops the slide-over chrome for embedding in a tab.
 * Changes are saved to localStorage immediately on every edit via
 * saveTimerAlertConfig().
 */
import React, { useEffect, useState } from 'react'
import { X, Plus, Trash2, Volume2 } from 'lucide-react'
import { getAvailableVoices } from '../services/audio'
import { loadTimerAlertConfig, saveTimerAlertConfig } from '../services/timerAlertStore'
import type { TimerAlertConfig, TimerAlertThreshold, TimerAlertType } from '../types/timerAlerts'

function newThreshold(): TimerAlertThreshold {
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

interface ThresholdRowProps {
  threshold: TimerAlertThreshold
  voices: string[]
  onChange: (t: TimerAlertThreshold) => void
  onRemove: () => void
}

function ThresholdRow({ threshold, voices, onChange, onRemove }: ThresholdRowProps): React.ReactElement {
  const inputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
    borderRadius: 4,
    padding: '3px 7px',
    fontSize: 12,
    outline: 'none',
  }

  const selectStyle: React.CSSProperties = { ...inputStyle, appearance: 'none' as const }

  return (
    <div
      className="rounded p-3 space-y-2"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      {/* Row 1: threshold seconds + type */}
      <div className="flex items-center gap-2 flex-wrap">
        <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Alert at
          <input
            type="number"
            min={1}
            max={3600}
            value={threshold.seconds}
            onChange={(e) => onChange({ ...threshold, seconds: Math.max(1, parseInt(e.target.value) || 1) })}
            style={{ ...inputStyle, width: 54 }}
          />
          s remaining
        </label>

        <label className="flex items-center gap-1.5 text-xs ml-auto" style={{ color: 'var(--color-muted-foreground)' }}>
          Type
          <select
            value={threshold.type}
            onChange={(e) => onChange({ ...threshold, type: e.target.value as TimerAlertType })}
            style={{ ...selectStyle, paddingRight: 24 }}
          >
            <option value="text_to_speech">Text to Speech</option>
            <option value="play_sound">Sound File</option>
          </select>
        </label>

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

      {/* Row 2: type-specific fields */}
      {threshold.type === 'text_to_speech' && (
        <div className="flex flex-col gap-2">
          <label className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span className="shrink-0" style={{ minWidth: 48 }}>Message</span>
            <input
              type="text"
              value={threshold.tts_template}
              onChange={(e) => onChange({ ...threshold, tts_template: e.target.value })}
              placeholder="{spell} expiring soon"
              style={{ ...inputStyle, flex: 1 }}
            />
          </label>
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-xs flex-1 min-w-0" style={{ color: 'var(--color-muted-foreground)' }}>
              <span className="shrink-0" style={{ minWidth: 48 }}>Voice</span>
              <select
                value={threshold.voice}
                onChange={(e) => onChange({ ...threshold, voice: e.target.value })}
                style={{ ...selectStyle, flex: 1, minWidth: 0, paddingRight: 24 }}
              >
                <option value="">System default</option>
                {voices.map((v) => (
                  <option key={v} value={v}>{v}</option>
                ))}
              </select>
            </label>
            <label className="flex items-center gap-1.5 text-xs shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
              Vol
              <input
                type="number"
                min={0}
                max={100}
                value={threshold.tts_volume}
                onChange={(e) => onChange({ ...threshold, tts_volume: Math.min(100, Math.max(0, parseInt(e.target.value) || 0)) })}
                style={{ ...inputStyle, width: 48 }}
              />
              %
            </label>
          </div>
        </div>
      )}

      {threshold.type === 'play_sound' && (
        <div className="flex flex-col gap-2">
          <label className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span className="shrink-0" style={{ minWidth: 48 }}>File</span>
            <input
              type="text"
              value={threshold.sound_path}
              onChange={(e) => onChange({ ...threshold, sound_path: e.target.value })}
              placeholder="C:\sounds\alert.wav"
              style={{ ...inputStyle, flex: 1 }}
            />
          </label>
          <label className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span className="shrink-0" style={{ minWidth: 48 }}>Volume</span>
            <input
              type="number"
              min={0}
              max={100}
              value={threshold.volume}
              onChange={(e) => onChange({ ...threshold, volume: Math.min(100, Math.max(0, parseInt(e.target.value) || 0)) })}
              style={{ ...inputStyle, width: 48 }}
            />
            %
          </label>
        </div>
      )}
    </div>
  )
}

interface Props {
  /** When true, renders inline (no slide-over wrapper or close button). */
  inline?: boolean
  onClose?: () => void
}

export default function TimerAlertsPanel({ inline = false, onClose }: Props): React.ReactElement {
  const [cfg, setCfg] = useState<TimerAlertConfig>(() => loadTimerAlertConfig())
  const [voices, setVoices] = useState<string[]>([])

  // Voices are populated asynchronously by the browser.
  useEffect(() => {
    function loadVoices() {
      const list = window.speechSynthesis?.getVoices().map((v) => v.name).sort() ?? []
      if (list.length > 0) setVoices(list)
    }
    loadVoices()
    window.speechSynthesis?.addEventListener('voiceschanged', loadVoices)
    return () => window.speechSynthesis?.removeEventListener('voiceschanged', loadVoices)
  }, [])

  function update(next: TimerAlertConfig) {
    setCfg(next)
    saveTimerAlertConfig(next)
  }

  function handleToggleEnabled(v: boolean) {
    update({ ...cfg, enabled: v })
  }

  function handleThresholdChange(index: number, t: TimerAlertThreshold) {
    const next = cfg.thresholds.slice()
    next[index] = t
    update({ ...cfg, thresholds: next })
  }

  function handleRemove(index: number) {
    const next = cfg.thresholds.filter((_, i) => i !== index)
    update({ ...cfg, thresholds: next })
  }

  function handleAdd() {
    update({ ...cfg, thresholds: [...cfg.thresholds, newThreshold()] })
  }

  const body = (
    <>
      {/* Enable toggle */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '10px 14px',
          borderBottom: '1px solid var(--color-border)',
          flexShrink: 0,
        }}
      >
        <div>
          <p style={{ fontSize: 13, color: 'var(--color-foreground)', margin: 0 }}>Enable timer alerts</p>
          <p style={{ fontSize: 11, color: 'var(--color-muted-foreground)', margin: '2px 0 0' }}>
            Play audio when any timer crosses a threshold.
          </p>
        </div>
        <Toggle checked={cfg.enabled} onChange={handleToggleEnabled} />
      </div>

      {/* Threshold list */}
      <div style={{ flex: 1, overflow: 'auto', padding: 14, display: 'flex', flexDirection: 'column', gap: 10 }}>
        <p style={{ fontSize: 11, color: 'var(--color-muted)', margin: 0 }}>
          Use <code style={{ color: 'var(--color-foreground)' }}>{'{spell}'}</code> in the message to insert the spell name.
        </p>

        {cfg.thresholds.length === 0 && (
          <p style={{ fontSize: 12, color: 'var(--color-muted)', textAlign: 'center', marginTop: 24 }}>
            No alert thresholds configured.
          </p>
        )}

        {cfg.thresholds.map((t, i) => (
          <ThresholdRow
            key={t.id}
            threshold={t}
            voices={voices}
            onChange={(next) => handleThresholdChange(i, next)}
            onRemove={() => handleRemove(i)}
          />
        ))}

        <button
          type="button"
          onClick={handleAdd}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 6,
            padding: '7px 12px',
            borderRadius: 6,
            border: '1px dashed var(--color-border)',
            backgroundColor: 'transparent',
            color: 'var(--color-muted-foreground)',
            cursor: 'pointer',
            fontSize: 12,
          }}
        >
          <Plus size={13} />
          Add threshold
        </button>
      </div>
    </>
  )

  if (inline) {
    return <div style={{ display: 'flex', flexDirection: 'column', flex: 1 }}>{body}</div>
  }

  return (
    <div
      style={{
        position: 'absolute',
        top: 0,
        right: 0,
        bottom: 0,
        width: 380,
        backgroundColor: 'var(--color-surface)',
        borderLeft: '1px solid var(--color-border)',
        display: 'flex',
        flexDirection: 'column',
        zIndex: 10,
        boxShadow: '-4px 0 16px rgba(0,0,0,0.4)',
      }}
    >
      {/* Header */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '10px 14px',
          borderBottom: '1px solid var(--color-border)',
          flexShrink: 0,
        }}
      >
        <span style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, fontWeight: 600, color: 'var(--color-foreground)' }}>
          <Volume2 size={14} style={{ color: 'var(--color-primary)' }} />
          Timer Audio Alerts
        </span>
        <button
          type="button"
          onClick={onClose}
          style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
        >
          <X size={15} />
        </button>
      </div>
      {body}
    </div>
  )
}
