/**
 * Fit height to rows: which popouts offer it. The setting lives in
 * config.preferences.overlay_fit_height (keyed by window key; missing/false
 * = off). See hooks/useFitHeight and the main process's fit store.
 */
import type { Config } from '../types/config'

// Row lists, where one row in a tall window is mostly empty space.
export const FIT_HEIGHT_OVERLAYS: readonly string[] = ['buffTimer', 'detrimTimer']

export function supportsFitHeight(key: string): boolean {
  return FIT_HEIGHT_OVERLAYS.includes(key)
}

export function fitHeightOn(config: Config | null | undefined, key: string): boolean {
  return supportsFitHeight(key) && config?.preferences.overlay_fit_height?.[key] === true
}
