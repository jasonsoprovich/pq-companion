/**
 * Collapse when idle: which popouts offer it and the delays to pick from.
 * The setting lives in config.preferences.overlay_idle_collapse (seconds,
 * keyed by window key; missing/0 = off). See hooks/useIdleCollapse.
 */
import type { Config } from '../types/config'
import { instanceKind, isInstanceKey } from './instanceKeys'

// Overlays with a clear "nothing to show" state. Instance windows — named
// custom timer windows (customTimer:<groupId>) and custom overlays
// (customOverlay:<id>) — offer it too.
export const IDLE_COLLAPSE_OVERLAYS: readonly string[] = [
  'buffTimer',
  'detrimTimer',
  'filteredTimer',
  'customTimer',
  'respawnTimer',
  'dps',
  'threat',
  'npc',
  'pet',
  'chChain',
  'chMetronome',
  'rollTracker',
  'deathRecap',
]

export const DEFAULT_IDLE_SECS = 10

export const IDLE_DELAY_OPTIONS: ReadonlyArray<{ secs: number; label: string }> = [
  { secs: 0, label: 'Off' },
  { secs: 5, label: 'After 5s' },
  { secs: 10, label: 'After 10s' },
  { secs: 30, label: 'After 30s' },
  { secs: 60, label: 'After 1 min' },
  { secs: 300, label: 'After 5 min' },
]

export function supportsIdleCollapse(key: string): boolean {
  // Chat windows keep their scrollback, so they're never "empty".
  if (instanceKind(key) === 'chatWindow') return false
  return isInstanceKey(key) || IDLE_COLLAPSE_OVERLAYS.includes(key)
}

export function idleCollapseSecs(config: Config | null | undefined, key: string): number {
  const v = config?.preferences.overlay_idle_collapse?.[key] ?? 0
  return v > 0 ? v : 0
}
