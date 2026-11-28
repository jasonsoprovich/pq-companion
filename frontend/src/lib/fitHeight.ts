/**
 * Fit height to rows: which popouts offer it. The setting lives in
 * config.preferences.overlay_fit_height (keyed by window key; missing/false
 * = off). See hooks/useFitHeight and the main process's fit store.
 */
import type { Config } from '../types/config'
import { instanceKind } from './instanceKeys'

// Row lists, where one row in a tall window is mostly empty space. Named
// Custom Timers windows (customTimer:<groupId>) offer it too.
export const FIT_HEIGHT_OVERLAYS: readonly string[] = [
  'buffTimer',
  'detrimTimer',
  'filteredTimer',
  'customTimer',
]

export function supportsFitHeight(key: string): boolean {
  return FIT_HEIGHT_OVERLAYS.includes(key) || instanceKind(key) === 'customTimer'
}

export function fitHeightOn(config: Config | null | undefined, key: string): boolean {
  return supportsFitHeight(key) && config?.preferences.overlay_fit_height?.[key] === true
}
