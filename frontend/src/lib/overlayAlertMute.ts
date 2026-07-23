/**
 * Shared localStorage-backed mute flags for overlay header bell toggles.
 * The alert-firing hooks (useTimerAlerts, useRespawnAlerts) are mounted once
 * at the App level, not inside the popout windows, so each window's bell
 * writes its flag here and the hook reads it back — mirrors the CH Metronome
 * bell (lib/chMetronome.ts ALERTS_ENABLED_KEY), generalized to the other
 * overlays that fire audio/TTS alerts. Muting here doesn't touch the
 * underlying alert preferences in Settings, it just silences them.
 */
export const BUFF_TIMER_ALERTS_KEY = 'buffTimer:alertsEnabled'
export const DETRIM_TIMER_ALERTS_KEY = 'detrimTimer:alertsEnabled'
export const CUSTOM_TIMER_ALERTS_KEY = 'customTimer:alertsEnabled'
export const RESPAWN_ALERTS_KEY = 'respawnTimer:alertsEnabled'

// Defaults to true (unmuted) so existing configured alerts keep firing until
// the user explicitly mutes them from an overlay header.
export function loadAlertsEnabled(key: string): boolean {
  return localStorage.getItem(key) !== 'false'
}

export function saveAlertsEnabled(key: string, enabled: boolean): void {
  try {
    localStorage.setItem(key, String(enabled))
  } catch {
    /* noop */
  }
}
