/**
 * featureRegistry — the list behind the green "new feature" dots.
 *
 * Each entry points at where a feature lives: a sidebar route (`nav`), and
 * optionally something deeper on that page (a Settings tab or an overlay
 * row). The nav dot clears when the route is visited; the deeper dot clears
 * when that tab is opened or that overlay row is used. An entry with no
 * deeper target is fully cleared by visiting its route.
 *
 * Who sees what (state persisted in config.feature_dots):
 * - Fresh installs see nothing: baseline = the installed version.
 * - Upgraders see entries whose `since` is newer than the version they had
 *   before dots were tracked, plus the one-time `launch` set.
 * - Every dot expires once two minor/major releases have shipped after its
 *   `since` (launch entries: after the version that activated the dots),
 *   seen or not, so stale dots never pile up.
 *
 * Adding an entry: give it a stable `id`, the release version it ships in as
 * `since`, and the most specific target that exists. /newrelease reminds
 * you to add entries for headline features.
 */
import type { FeatureDotsState } from '../types/config'

export interface FeatureEntry {
  id: string
  // Release version (no leading "v") the feature first shipped in. Omitted
  // only on launch entries.
  since?: string
  // One-time launch set: shown to every upgrader the first time the dot
  // system runs, regardless of version.
  launch?: boolean
  // Sidebar route that gets the dot ('/settings' for Settings tabs).
  nav: string
  settingsTab?: string
  // DashboardPanelKey of an overlay row in Manage overlays.
  overlay?: string
}

export const FEATURES: readonly FeatureEntry[] = [
  // ── Launch set: shipped before the dot system existed ──
  { id: 'pet-overlay', launch: true, nav: '/overlays', overlay: 'pet' },
  { id: 'item-info-overlay', launch: true, nav: '/overlays', overlay: 'itemInfo' },
  { id: 'death-recap-overlay', launch: true, nav: '/overlays', overlay: 'deathRecap' },
  { id: 'kill-counters-overlay', launch: true, nav: '/overlays', overlay: 'kills' },
  { id: 'filtered-timers-overlay', launch: true, nav: '/overlays', overlay: 'filtered' },
  { id: 'backup-restore', launch: true, nav: '/settings', settingsTab: 'backups' },
  { id: 'raid-replay', launch: true, nav: '/raid-replay' },
  { id: 'live-map-3d', launch: true, nav: '/live-map' },

  // ── Overlay overhaul (feat/overlay-hub). `since` is a placeholder for the
  // release this branch ships in — update it at merge time. ──
  { id: 'overlay-hotkeys', since: '0.34.0', nav: '/settings', settingsTab: 'hotkeys' },
]

// ── Version helpers ─────────────────────────────────────────────────────────

function parse(v: string): [number, number, number] {
  const [a, b, c] = v.replace(/^v/, '').split(/[.-]/).map((n) => parseInt(n, 10))
  return [a || 0, b || 0, c || 0]
}

export function compareVersions(a: string, b: string): number {
  const pa = parse(a)
  const pb = parse(b)
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] - pb[i]
  }
  return 0
}

// Number of minor/major releases (x.y.0) in the changelog after `start`, up
// to and including the running version. Patch releases don't count.
function releasesSince(start: string, app: string, changelog: string[]): number {
  return changelog.filter(
    (v) =>
      parse(v)[2] === 0 &&
      compareVersions(v, start) > 0 &&
      compareVersions(v, app) <= 0,
  ).length
}

const EXPIRE_AFTER_RELEASES = 2

// ── Visibility ──────────────────────────────────────────────────────────────

export const navSeenKey = (id: string): string => `${id}@nav`

function hasDeepTarget(e: FeatureEntry): boolean {
  return Boolean(e.settingsTab || e.overlay)
}

/**
 * liveFeatures returns the entries that should currently show a dot
 * somewhere (eligible, not expired, deep target not yet seen). Empty until
 * the state has been initialized.
 */
export function liveFeatures(
  state: FeatureDotsState | undefined,
  appVersion: string,
  changelog: string[],
): FeatureEntry[] {
  if (!state?.baseline || !appVersion) return []
  const seen = new Set(state.seen ?? [])
  return FEATURES.filter((e) => {
    if (seen.has(e.id)) return false
    const eligible = e.launch
      ? state.launch_eligible
      : Boolean(e.since) && compareVersions(e.since as string, state.baseline) > 0
    if (!eligible) return false
    const start = e.launch ? state.activated || state.baseline : (e.since as string)
    return releasesSince(start, appVersion, changelog) < EXPIRE_AFTER_RELEASES
  })
}

export function navHasDot(live: FeatureEntry[], seen: string[], route: string): boolean {
  const s = new Set(seen)
  return live.some((e) => e.nav === route && !s.has(navSeenKey(e.id)))
}

// Keys to mark seen when the user lands on `pathname`.
export function keysSeenOnVisit(live: FeatureEntry[], pathname: string): string[] {
  const keys: string[] = []
  for (const e of live) {
    if (pathname !== e.nav && !pathname.startsWith(`${e.nav}/`)) continue
    keys.push(navSeenKey(e.id))
    if (!hasDeepTarget(e)) keys.push(e.id)
  }
  return keys
}

/**
 * initialFeatureDots decides the starting state the first time the dot
 * system runs. Must be computed from the config as loaded at startup —
 * before the What's New logic stamps last_seen_changelog_version — since an
 * empty last-seen version is how a fresh install is recognised.
 */
export function initialFeatureDots(
  lastSeenChangelogVersion: string,
  appVersion: string,
): FeatureDotsState {
  const fresh = !lastSeenChangelogVersion
  return {
    baseline: fresh ? appVersion : lastSeenChangelogVersion,
    activated: appVersion,
    launch_eligible: !fresh,
    seen: [],
  }
}
