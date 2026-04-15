/**
 * timerAlertStore.ts — Persist timer alert configuration in localStorage.
 *
 * Defaults ship with one 30-second TTS threshold so the feature works
 * out of the box without any configuration.
 */
import type { TimerAlertConfig } from '../types/timerAlerts'

const STORAGE_KEY = 'pq-timer-alerts'

const DEFAULT_CONFIG: TimerAlertConfig = {
  enabled: true,
  thresholds: [
    {
      id: 'default-30s',
      seconds: 30,
      type: 'text_to_speech',
      sound_path: '',
      volume: 80,
      tts_template: '{spell} expiring soon',
      voice: '',
      tts_volume: 80,
    },
  ],
}

export function loadTimerAlertConfig(): TimerAlertConfig {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return structuredClone(DEFAULT_CONFIG)
    return JSON.parse(raw) as TimerAlertConfig
  } catch {
    return structuredClone(DEFAULT_CONFIG)
  }
}

export function saveTimerAlertConfig(cfg: TimerAlertConfig): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(cfg))
  } catch {
    // Silently ignore (e.g., private browsing quota exceeded)
  }
}
