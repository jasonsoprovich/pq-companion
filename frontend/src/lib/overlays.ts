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
  | 'threat'
  | 'rollTracker'
  | 'respawnTimer'
  | 'chChain'
  | 'chMetronome'
  | 'discordVoice'
  | 'liveMap'
  | 'zoneLockouts'
  | 'raidReadiness'
  | 'filteredTimer'
  | 'killCounter'
  | 'itemInfo'

/**
 * How an overlay behaves while locked.
 *   "interactive"  — hover the overlay to scroll / clear individual rows;
 *                    move off and clicks pass through to the game.
 *   "clickthrough" — only the title-bar buttons are clickable; scrolling and
 *                    clicks everywhere else pass through to the game.
 *   "display-only" — nothing ever captures the mouse (the title bar is hidden
 *                    too); a pure HUD. Reposition it with the global "Position
 *                    overlays" toggle in Settings → Overlays.
 */
export type LockedMode = 'interactive' | 'clickthrough' | 'display-only'

export const DEFAULT_LOCKED_MODE: LockedMode = 'interactive'

/**
 * Per-overlay override of the global "Fade When Inactive" toggle. Absent
 * (shown as "Default") follows the global setting; "on" / "off" force it for
 * that overlay only — e.g. fade the overlays sitting over the game window
 * but keep the ones on a second monitor solid.
 */
export type FadeMode = 'on' | 'off'

// Per-overlay override of the fallback used when the user hasn't picked a
// mode yet (see resolveLockedMode). Discord Voice has no rows to scroll or
// clear — it's a passive roster — so "interactive" (the shared default) would
// just mean "the whole thing captures the mouse on hover for no reason."
// "clickthrough" is a better out-of-the-box default: only the header responds
// to hover, the roster itself never blocks clicks to the game underneath.
// Users can still change it in Settings → Overlays like any other overlay.
const OVERLAY_DEFAULT_LOCKED_MODE: Partial<Record<OverlayName, LockedMode>> = {
  discordVoice: 'clickthrough',
  // The live map is a HUD: it follows the player, so there is nothing to scroll
  // or clear and no reason for the map body to capture the mouse while playing.
  // Only the header responds to hover, as with Discord Voice.
  liveMap: 'clickthrough',
  // Same reasoning as Discord Voice: the gap list is a passive readout with
  // nothing to click or clear, so there's no reason for its body to capture
  // the mouse while playing.
  raidReadiness: 'clickthrough',
  // A HUD of counts — the +/− buttons are for the dashboard panel and an
  // unlocked popout, never for mid-fight clicks.
  killCounter: 'clickthrough',
}

// The Item Info overlay runs on a proposed Zeal item pipe message that no
// released Zeal sends yet, so it's offered only once the backend has seen one
// (zeal_item_display_seen) — or in Developer Mode, to build it against the
// simulate endpoint. Until then it's absent from Settings and the dashboard.
export function itemInfoOffered(prefs: { zeal_item_display_seen?: boolean; developer_mode?: boolean }): boolean {
  return Boolean(prefs.zeal_item_display_seen || prefs.developer_mode)
}

export const OVERLAY_DEFS: { name: OverlayName; label: string }[] = [
  { name: 'dps', label: 'DPS Meter' },
  { name: 'hps', label: 'HPS Meter' },
  { name: 'buffTimer', label: 'Buff Timers' },
  { name: 'detrimTimer', label: 'Detrimental Timers' },
  { name: 'filteredTimer', label: 'Filtered Timers' },
  { name: 'customTimer', label: 'Custom Timers' },
  { name: 'npc', label: 'NPC Info' },
  { name: 'threat', label: 'Threat Meter' },
  { name: 'rollTracker', label: 'Roll Tracker' },
  { name: 'respawnTimer', label: 'Respawn Timers' },
  { name: 'chChain', label: 'CH Chain' },
  { name: 'chMetronome', label: 'CH Metronome' },
  { name: 'discordVoice', label: 'Discord Voice' },
  { name: 'liveMap', label: 'Live Map' },
  { name: 'zoneLockouts', label: 'Zone Lockouts' },
  { name: 'raidReadiness', label: 'Raid Readiness' },
  { name: 'killCounter', label: 'Kill Counters' },
  { name: 'itemInfo', label: 'Item Info' },
]

// Matches a StreamKit voice-overlay URL, e.g.
// https://streamkit.discord.com/overlay/voice/123456789/987654321 — see
// DiscordVoiceOverlaySettings.tsx and issue #150. Shared so the Settings card
// and the overlay window page agree on what "looks valid" means; the Electron
// main process (electron/main/index.ts) has its own copy of this same pattern
// since it can't import frontend code.
const STREAMKIT_VOICE_URL_RE = /^https:\/\/streamkit\.discord\.com\/overlay\/voice\/\d+\/\d+(?:[/?#].*)?$/

export function isValidStreamKitVoiceUrl(url: string): boolean {
  return STREAMKIT_VOICE_URL_RE.test(url.trim())
}

/**
 * Resolve a single overlay's locked mode from the stored preference map,
 * defaulting missing keys to "interactive" so an unset/nil map preserves the
 * original behaviour.
 */
export function resolveLockedMode(
  modes: Partial<Record<string, LockedMode>> | undefined,
  name: OverlayName,
): LockedMode {
  return modes?.[name] ?? OVERLAY_DEFAULT_LOCKED_MODE[name] ?? DEFAULT_LOCKED_MODE
}

/**
 * Resolve whether an overlay's chrome should fade: its own override when set,
 * otherwise the global overlay_fade_enabled toggle.
 */
export function resolveFadeEnabled(
  modes: Partial<Record<string, FadeMode | ''>> | undefined,
  name: OverlayName,
  globalEnabled: boolean,
): boolean {
  const mode = modes?.[name]
  if (mode === 'on') return true
  if (mode === 'off') return false
  return globalEnabled
}

/**
 * Resolve an overlay's background opacity: its own override when set (0–1),
 * otherwise the global overlay_opacity. A negative override means "Default" —
 * see Preferences.overlay_opacities for why it is not a deleted key.
 */
export function resolveOverlayOpacity(
  overrides: Partial<Record<string, number>> | undefined,
  name: OverlayName,
  globalOpacity: number,
): number {
  const v = overrides?.[name]
  return typeof v === 'number' && v >= 0 && v <= 1 ? v : globalOpacity
}
