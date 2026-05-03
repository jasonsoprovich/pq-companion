/**
 * EventAlertsPanel — slide-in configuration panel for Event Notification Alerts.
 *
 * Rendered by LogFeedPage when the user clicks the bell icon.
 * Config is saved to localStorage immediately on every edit.
 *
 * Each row represents one log event type (fixed set). The user can:
 *   - Enable/disable the rule for that event
 *   - Choose TTS or play_sound
 *   - Configure the message/file and volume
 */
import React, { useState } from 'react'
import { X, Volume2 } from 'lucide-react'
import { loadEventAlertConfig, saveEventAlertConfig } from '../services/eventAlertStore'
import type {
  EventAlertConfig,
  EventAlertRule,
  EventAlertType,
  AlertableEventType,
} from '../types/eventAlerts'
import { useVoices } from '../hooks/useVoices'
import NotificationActionEditor, { NotificationTypeSelect } from './NotificationActionEditor'

// ── Metadata for each supported event type ─────────────────────────────────────

const EVENT_META: Record<
  AlertableEventType,
  { label: string; description: string; placeholders: string }
> = {
  'log:death': {
    label: 'Player Death',
    description: 'You are slain.',
    placeholders: '{slain_by}',
  },
  'log:zone': {
    label: 'Zone Change',
    description: 'You enter a new zone.',
    placeholders: '{zone}',
  },
  'log:spell_resist': {
    label: 'Spell Resist',
    description: 'Your target resists your spell.',
    placeholders: '{spell}',
  },
  'log:spell_interrupt': {
    label: 'Spell Interrupt',
    description: 'Your spell cast is interrupted.',
    placeholders: '{spell}',
  },
}

const EVENT_ORDER: AlertableEventType[] = [
  'log:death',
  'log:zone',
  'log:spell_resist',
  'log:spell_interrupt',
]

// ── Sub-components ─────────────────────────────────────────────────────────────

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
      style={{ backgroundColor: checked ? 'var(--color-primary)' : 'var(--color-surface-3)' }}
    >
      <span
        className="inline-block h-3 w-3 rounded-full bg-white shadow transition-transform"
        style={{ transform: checked ? 'translateX(14px)' : 'translateX(2px)' }}
      />
    </button>
  )
}

interface RuleRowProps {
  rule: EventAlertRule
  voices: string[]
  onChange: (r: EventAlertRule) => void
}

function RuleRow({ rule, voices, onChange }: RuleRowProps): React.ReactElement {
  const meta = EVENT_META[rule.event_type]

  return (
    <div
      className="rounded p-3 space-y-2"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        opacity: rule.enabled ? 1 : 0.55,
        transition: 'opacity 0.15s',
      }}
    >
      {/* Header row: event label + enabled toggle */}
      <div className="flex items-start justify-between gap-2">
        <div>
          <p style={{ fontSize: 12, fontWeight: 600, color: 'var(--color-foreground)', margin: 0 }}>
            {meta.label}
          </p>
          <p style={{ fontSize: 11, color: 'var(--color-muted-foreground)', margin: '2px 0 0' }}>
            {meta.description}
          </p>
        </div>
        <Toggle checked={rule.enabled} onChange={(v) => onChange({ ...rule, enabled: v })} />
      </div>

      {/* Alert type selector */}
      <div className="flex items-center gap-2">
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Type</span>
        <NotificationTypeSelect
          value={rule.type}
          onChange={(t) => onChange({ ...rule, type: t as EventAlertType })}
          allowedTypes={['text_to_speech', 'play_sound']}
          className="rounded px-2 py-0.5 text-xs outline-none"
        />
      </div>

      <NotificationActionEditor
        type={rule.type}
        voices={voices}
        ttsText={rule.tts_template}
        onTtsTextChange={(v) => onChange({ ...rule, tts_template: v })}
        ttsTextPlaceholder={`e.g. ${meta.placeholders}`}
        voice={rule.voice}
        onVoiceChange={(v) => onChange({ ...rule, voice: v })}
        ttsVolume={rule.tts_volume}
        onTtsVolumeChange={(v) => onChange({ ...rule, tts_volume: v })}
        soundPath={rule.sound_path}
        onSoundPathChange={(v) => onChange({ ...rule, sound_path: v })}
        soundVolume={rule.volume}
        onSoundVolumeChange={(v) => onChange({ ...rule, volume: v })}
      />

      {rule.type === 'text_to_speech' && (
        <p style={{ fontSize: 10, color: 'var(--color-muted)', margin: 0 }}>
          Use <code style={{ color: 'var(--color-foreground)' }}>{meta.placeholders}</code> to insert context.
        </p>
      )}
    </div>
  )
}

// ── Panel ──────────────────────────────────────────────────────────────────────

interface Props {
  /** When true, renders inline (no slide-over wrapper or close button). */
  inline?: boolean
  onClose?: () => void
}

export default function EventAlertsPanel({ inline = false, onClose }: Props): React.ReactElement {
  const [cfg, setCfg] = useState<EventAlertConfig>(() => loadEventAlertConfig())
  const voices = useVoices()

  function update(next: EventAlertConfig) {
    setCfg(next)
    saveEventAlertConfig(next)
  }

  function handleRuleChange(index: number, rule: EventAlertRule) {
    const rules = cfg.rules.slice()
    rules[index] = rule
    update({ ...cfg, rules })
  }

  // Ensure every supported event type has a rule — fill in any that are missing
  // (e.g. after a schema update adds new event types).
  const rulesById = new Map(cfg.rules.map((r) => [r.event_type, r]))
  const displayRules: EventAlertRule[] = EVENT_ORDER.map((eventType, i) => {
    return rulesById.get(eventType) ?? {
      id: `generated-${i}`,
      event_type: eventType,
      enabled: false,
      type: 'text_to_speech' as EventAlertType,
      sound_path: '',
      volume: 80,
      tts_template: '',
      voice: '',
      tts_volume: 80,
    }
  })

  const ruleList = (
    <>
      {/* Global enable toggle */}
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
          <p style={{ fontSize: 13, color: 'var(--color-foreground)', margin: 0 }}>
            Enable event alerts
          </p>
          <p style={{ fontSize: 11, color: 'var(--color-muted-foreground)', margin: '2px 0 0' }}>
            Play audio when important game events occur.
          </p>
        </div>
        <Toggle
          checked={cfg.enabled}
          onChange={(v) => update({ ...cfg, enabled: v })}
        />
      </div>

      {/* Rule list */}
      <div
        style={{
          flex: 1,
          overflow: 'auto',
          padding: 14,
          display: 'flex',
          flexDirection: 'column',
          gap: 10,
        }}
      >
        {displayRules.map((rule, i) => (
          <RuleRow
            key={rule.event_type}
            rule={rule}
            voices={voices}
            onChange={(next) => {
              const realIndex = cfg.rules.findIndex((r) => r.event_type === rule.event_type)
              if (realIndex >= 0) {
                handleRuleChange(realIndex, next)
              } else {
                update({ ...cfg, rules: [...cfg.rules, next] })
              }
              void i
            }}
          />
        ))}
      </div>
    </>
  )

  if (inline) {
    return <div style={{ display: 'flex', flexDirection: 'column', flex: 1 }}>{ruleList}</div>
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
        <span
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            fontSize: 13,
            fontWeight: 600,
            color: 'var(--color-foreground)',
          }}
        >
          <Volume2 size={14} style={{ color: 'var(--color-primary)' }} />
          Event Audio Alerts
        </span>
        <button
          type="button"
          onClick={onClose}
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: 'var(--color-muted)',
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <X size={15} />
        </button>
      </div>
      {ruleList}
    </div>
  )
}
