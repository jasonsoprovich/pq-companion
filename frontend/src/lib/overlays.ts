/**
 * Single source of truth for the popout overlay windows that support a
 * user-selectable "locked mode". Both the settings UI and the per-overlay
 * lock hook read from here, so adding a future overlay is a one-line change:
 * append it to OVERLAY_DEFS and pass its name to useOverlayLock() in its
 * *WindowPage.tsx. It then appears in Settings → Overlays with both mode
 * options automatically.
 *
 * Names match the canonical OverlayName strings used by the Electron main
 * process (electron/main/index.ts) and the per-overlay lock store, minus the
 * screen-spanning "trigger" overlay, which doesn't use the hover toggle.
 */

export type OverlayName =
  | 'dps'
  | 'hps'
  | 'buffTimer'
  | 'detrimTimer'
  | 'customTimer'
  | 'npc'
  | 'rollTracker'
  | 'respawnTimer'
  | 'chChain'
  | 'chMetronome'

/**
 * How an overlay behaves while locked.
 *   "interactive"  — hover the overlay to scroll / clear individual rows;
 *                    move off and clicks pass through to the game.
 *   "clickthrough" — only the title-bar buttons are clickable; scrolling and
 *                    clicks everywhere else pass through to the game.
 */
export type LockedMode = 'interactive' | 'clickthrough'

export const DEFAULT_LOCKED_MODE: LockedMode = 'interactive'

export const OVERLAY_DEFS: { name: OverlayName; label: string }[] = [
  { name: 'dps', label: 'DPS Meter' },
  { name: 'hps', label: 'HPS Meter' },
  { name: 'buffTimer', label: 'Buff Timers' },
  { name: 'detrimTimer', label: 'Detrimental Timers' },
  { name: 'customTimer', label: 'Custom Timers' },
  { name: 'npc', label: 'NPC Info' },
  { name: 'rollTracker', label: 'Roll Tracker' },
  { name: 'respawnTimer', label: 'Respawn Timers' },
  { name: 'chChain', label: 'CH Chain' },
  { name: 'chMetronome', label: 'CH Metronome' },
]

/**
 * Resolve a single overlay's locked mode from the stored preference map,
 * defaulting missing keys to "interactive" so an unset/nil map preserves the
 * original behaviour.
 */
export function resolveLockedMode(
  modes: Partial<Record<string, LockedMode>> | undefined,
  name: OverlayName,
): LockedMode {
  return modes?.[name] ?? DEFAULT_LOCKED_MODE
}
