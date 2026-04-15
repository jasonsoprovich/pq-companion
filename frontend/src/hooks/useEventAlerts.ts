/**
 * useEventAlerts — subscribes to WebSocket log events and fires audio alerts
 * according to the EventAlertConfig stored in localStorage.
 *
 * Mount once at the App level (alongside useAudioEngine and useTimerAlerts)
 * so alerts fire regardless of which page the user is on.
 *
 * Supported events: log:death, log:zone, log:spell_resist, log:spell_interrupt.
 * Combat hit/miss events are intentionally excluded (too frequent to be useful).
 */
import { useCallback } from 'react'
import { useWebSocket } from './useWebSocket'
import { playSound, speakText } from '../services/audio'
import { loadEventAlertConfig } from '../services/eventAlertStore'
import type { AlertableEventType } from '../types/eventAlerts'
import type {
  ZoneData,
  SpellResistData,
  SpellInterruptData,
  DeathData,
} from '../types/logEvent'

const ALERTABLE_EVENTS = new Set<string>([
  'log:death',
  'log:zone',
  'log:spell_resist',
  'log:spell_interrupt',
])

/**
 * Substitute placeholder variables in a TTS template string.
 * Unknown placeholders are left as-is.
 */
function applyTemplate(template: string, vars: Record<string, string>): string {
  return template.replace(/\{(\w+)\}/g, (_, key: string) => vars[key] ?? `{${key}}`)
}

export function useEventAlerts(): void {
  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (!ALERTABLE_EVENTS.has(msg.type)) return

    const cfg = loadEventAlertConfig()
    if (!cfg.enabled) return

    const eventType = msg.type as AlertableEventType
    const rule = cfg.rules.find((r) => r.event_type === eventType && r.enabled)
    if (!rule) return

    // Build template variables from the event payload.
    const vars: Record<string, string> = {}
    switch (eventType) {
      case 'log:death': {
        const d = msg.data as DeathData | undefined
        vars.slain_by = d?.slain_by ?? 'something'
        break
      }
      case 'log:zone': {
        const d = msg.data as ZoneData | undefined
        vars.zone = d?.zone_name ?? 'unknown zone'
        break
      }
      case 'log:spell_resist': {
        const d = msg.data as SpellResistData | undefined
        vars.spell = d?.spell_name ?? 'spell'
        break
      }
      case 'log:spell_interrupt': {
        const d = msg.data as SpellInterruptData | undefined
        vars.spell = d?.spell_name ?? 'spell'
        break
      }
    }

    if (rule.type === 'play_sound' && rule.sound_path) {
      playSound(rule.sound_path, rule.volume / 100)
    } else if (rule.type === 'text_to_speech' && rule.tts_template) {
      const text = applyTemplate(rule.tts_template, vars)
      speakText(text, rule.voice, rule.tts_volume / 100)
    }
  }, [])

  useWebSocket(handleMessage)
}
