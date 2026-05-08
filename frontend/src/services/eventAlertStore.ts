/**
 * eventAlertStore.ts — Persist event notification alert config in localStorage.
 *
 * Ships with defaults that cover the four most important game events so the
 * feature works out of the box without any configuration.
 */
import type { EventAlertConfig, EventAlertRule } from '../types/eventAlerts'

const STORAGE_KEY = 'pq-event-alerts'

function rule(
  id: string,
  event_type: EventAlertRule['event_type'],
  tts_template: string,
): EventAlertRule {
  return {
    id,
    event_type,
    enabled: true,
    type: 'text_to_speech',
    sound_path: '',
    volume: 80,
    tts_template,
    voice: '',
    tts_volume: 80,
  }
}

const DEFAULT_CONFIG: EventAlertConfig = {
  enabled: false,
  rules: [
    rule('default-death',     'log:death',          'You have died'),
    rule('default-zone',      'log:zone',            'Entering {zone}'),
    rule('default-resist',    'log:spell_resist',    '{spell} resisted'),
    rule('default-interrupt', 'log:spell_interrupt', 'Spell interrupted'),
  ],
}

export function loadEventAlertConfig(): EventAlertConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return structuredClone(DEFAULT_CONFIG)
    return JSON.parse(raw) as EventAlertConfig
  } catch {
    return structuredClone(DEFAULT_CONFIG)
  }
}

export function saveEventAlertConfig(cfg: EventAlertConfig): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(cfg))
  } catch {
    // Silently ignore (e.g. private browsing quota exceeded)
  }
}
