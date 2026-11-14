/**
 * overlayCatalog — every popout overlay as the Overlays hub presents it:
 * category, icon, colour, a one-line blurb and a longer description, plus the
 * IPC toggle that opens or closes its window. The dashboard reuses the
 * panel → window mapping (PANEL_POPOUT).
 *
 * Icons and colours match each overlay window's own title bar.
 */
import type { LucideIcon } from 'lucide-react'
import {
  Crosshair,
  Filter,
  Gauge,
  Headphones,
  HeartPulse,
  Hourglass,
  Info,
  Lock,
  Map as MapIcon,
  PawPrint,
  Shield,
  ShieldAlert,
  Skull,
  Swords,
  Target,
  Trophy,
} from 'lucide-react'
import type { OverlayName } from './overlays'
import { DASHBOARD_PANEL_LABELS, type DashboardPanelKey } from '../services/dashboardLayout'

export type OverlayCategory = 'timers' | 'combat' | 'targets' | 'alerts'

export const OVERLAY_CATEGORIES: Array<{ id: OverlayCategory; label: string }> = [
  { id: 'timers', label: 'Timers' },
  { id: 'combat', label: 'Combat' },
  { id: 'targets', label: 'Targets & info' },
  { id: 'alerts', label: 'Alerts & social' },
]

export interface OverlayCatalogEntry {
  panel: DashboardPanelKey
  name: OverlayName
  label: string
  category: OverlayCategory
  icon: LucideIcon
  color: string
  blurb: string
  details: string
}

const o = (): Window['electron']['overlay'] | undefined => window.electron?.overlay

// Each dashboard panel's standalone window: its overlay name (open-state
// polling, position reset, hotkeys) and the IPC toggle for it.
export const PANEL_POPOUT: Record<DashboardPanelKey, { name: OverlayName; toggle: () => void }> = {
  buff:          { name: 'buffTimer',     toggle: () => { o()?.toggleBuffTimer() } },
  detrim:        { name: 'detrimTimer',   toggle: () => { o()?.toggleDetrimTimer() } },
  dps:           { name: 'dps',           toggle: () => { o()?.toggleDPS() } },
  npc:           { name: 'npc',           toggle: () => { o()?.toggleNPC() } },
  threat:        { name: 'threat',        toggle: () => { o()?.toggleThreat() } },
  hps:           { name: 'hps',           toggle: () => { o()?.toggleHPS() } },
  rolls:         { name: 'rollTracker',   toggle: () => { o()?.toggleRollTracker() } },
  respawn:       { name: 'respawnTimer',  toggle: () => { o()?.toggleRespawnTimer() } },
  chChain:       { name: 'chChain',       toggle: () => { o()?.toggleCHChain() } },
  chMetronome:   { name: 'chMetronome',   toggle: () => { o()?.toggleCHMetronome() } },
  custom:        { name: 'customTimer',   toggle: () => { o()?.toggleCustomTimer() } },
  discordVoice:  { name: 'discordVoice',  toggle: () => { o()?.toggleDiscordVoice() } },
  liveMap:       { name: 'liveMap',       toggle: () => { o()?.toggleLiveMap() } },
  zoneLockouts:  { name: 'zoneLockouts',  toggle: () => { o()?.toggleZoneLockouts() } },
  raidReadiness: { name: 'raidReadiness', toggle: () => { o()?.toggleRaidReadiness() } },
  filtered:      { name: 'filteredTimer', toggle: () => { o()?.toggleFilteredTimer() } },
  kills:         { name: 'killCounter',   toggle: () => { o()?.toggleKillCounter() } },
  itemInfo:      { name: 'itemInfo',      toggle: () => { o()?.toggleItemInfo() } },
  pet:           { name: 'pet',           toggle: () => { o()?.togglePet() } },
  deathRecap:    { name: 'deathRecap',    toggle: () => { o()?.toggleDeathRecap() } },
}

type Entry = Omit<OverlayCatalogEntry, 'name'>

const ENTRIES: Entry[] = [
  // ── Timers ──
  {
    panel: 'buff', label: 'Buff Timers', category: 'timers', icon: Shield, color: '#22c55e',
    blurb: 'Your buffs counting down, with a warning before they fade.',
    details:
      'Tracks the buffs on you from your log, with durations scaled to the caster’s level. ' +
      'Choose warning times and which buffs to show in the timer settings.',
  },
  {
    panel: 'filtered', label: 'Filtered Timers', category: 'timers', icon: Filter, color: '#38bdf8',
    blurb: 'Only the timers you pick, by spell, player or mob.',
    details:
      'A second timer window next to Buffs and Detrimentals. Filter it down to what matters ' +
      'for a fight, such as one debuff or one player’s buffs, so it never gets buried.',
  },
  {
    panel: 'detrim', label: 'Detrimental Timers', category: 'timers', icon: Skull, color: '#ef4444',
    blurb: 'Your debuffs, DoTs, mez and slows on each mob.',
    details:
      'Grouped by target, so you can see which mez is about to break and which DoT needs ' +
      'a refresh. Timers clear when the mob dies.',
  },
  {
    panel: 'custom', label: 'Custom Timers', category: 'timers', icon: Hourglass, color: '#38bdf8',
    blurb: 'Timers started by your triggers.',
    details:
      'Any trigger with a timer action counts down here. For raid or boss timers you want ' +
      'kept apart, make extra named windows under Your windows.',
  },
  {
    panel: 'respawn', label: 'Respawn Timers', category: 'timers', icon: Hourglass, color: '#a855f7',
    blurb: 'Counts down to each NPC’s respawn after it dies.',
    details:
      'Uses the zone’s spawn data. Pin the camps you care about, and switch between fast ' +
      'and normal timing when you’re in an instance.',
  },
  {
    panel: 'zoneLockouts', label: 'Zone Lockouts', category: 'timers', icon: Lock, color: '#a855f7',
    blurb: 'Your loot lockouts for the raid bosses in this zone.',
    details:
      'Shows each raid target in your current zone and whether the active character is ' +
      'locked out, with time left.',
  },
  // ── Combat ──
  {
    panel: 'dps', label: 'DPS Meter', category: 'combat', icon: Swords, color: '#818cf8',
    blurb: 'Damage per second for everyone in the fight.',
    details:
      'Parses each fight from your log and ranks everyone by damage, so you can compare ' +
      'yourself with the rest of the group or raid.',
  },
  {
    panel: 'threat', label: 'Threat Meter', category: 'combat', icon: Gauge, color: '#c9a84c',
    blurb: 'Your estimated hate on each mob.',
    details:
      'An estimate built from your damage, heals and hate spells, so you can see when to ' +
      'ease off before you pull aggro.',
  },
  {
    panel: 'kills', label: 'Kill Counters', category: 'combat', icon: Crosshair, color: '#f59e0b',
    blurb: 'Counts kills of the mobs you choose.',
    details:
      'Set up counters for events and quests that need a number of kills. Event presets ' +
      'are included, and the counts can be adjusted by hand.',
  },
  {
    panel: 'deathRecap', label: 'Death Recap', category: 'combat', icon: Skull, color: '#ef4444',
    blurb: 'What killed you, and how.',
    details:
      'After a death: the killer, damage and healing taken, Complete Heals that landed, ' +
      'and the last 20 seconds of incoming hits.',
  },
  {
    panel: 'chChain', label: 'CH Chain', category: 'combat', icon: HeartPulse, color: '#3b82f6',
    blurb: 'The Complete Heal rotation, one bar per position.',
    details:
      'Follows the chain from cleric calls in your log and shows who cast last and who is ' +
      'next. Can follow a second chain at the same time.',
  },
  {
    panel: 'chMetronome', label: 'CH Metronome', category: 'combat', icon: Gauge, color: '#22c55e',
    blurb: 'Counts down to your turn in the CH chain.',
    details:
      'Watches the cleric before you and flashes CAST NOW when it’s time to press your ' +
      'heal key, with your own delay.',
  },
  // ── Targets & info ──
  {
    panel: 'npc', label: 'NPC Overlay', category: 'targets', icon: Target, color: '#c9a84c',
    blurb: 'Facts about the mob you’re fighting.',
    details:
      'Level, resists, special abilities, spells and loot for your current target, with ' +
      'tags for things like summon, enrage and immunities.',
  },
  {
    panel: 'pet', label: 'Pet', category: 'targets', icon: PawPrint, color: '#a855f7',
    blurb: 'Your pet, or a charm pet finder when you have none.',
    details:
      'Shows your summoned or charmed pet and warns when charm breaks. With no pet, it ' +
      'turns into a compact charm pet finder.',
  },
  {
    panel: 'itemInfo', label: 'Item Info', category: 'targets', icon: Info, color: '#38bdf8',
    blurb: 'Full stats for items you inspect in game.',
    details:
      'Shows an item’s stats and effects when you inspect it in game, and can pop up on ' +
      'its own each time. Needs Zeal 1.5.0 or newer.',
  },
  {
    panel: 'liveMap', label: 'Live Map', category: 'targets', icon: MapIcon, color: '#22c55e',
    blurb: 'A map of your zone with you on it.',
    details:
      'A lean version of the Live Map page for playing over the game: where you are and ' +
      'what’s near you. Search and details stay on the Live Map page.',
  },
  // ── Alerts & social ──
  {
    panel: 'discordVoice', label: 'Discord Voice', category: 'alerts', icon: Headphones, color: '#5865f2',
    blurb: 'Who’s talking in your Discord voice channel.',
    details:
      'Shows the members of your voice channel and lights up whoever is speaking, so you ' +
      'can follow raid calls in game.',
  },
  {
    panel: 'rolls', label: 'Roll Tracker', category: 'alerts', icon: Trophy, color: '#facc15',
    blurb: 'Collects /random rolls and picks the winner.',
    details:
      'Groups rolls into sessions and shows the winner. Run a timed roll-call or stop it ' +
      'by hand, without leaving the overlay.',
  },
  {
    panel: 'raidReadiness', label: 'Raid Readiness', category: 'alerts', icon: ShieldAlert, color: '#ef4444',
    blurb: 'Which raid roles are still short for this encounter.',
    details:
      'Compares the live raid roster against the minimum roles for the detected ' +
      'encounter. Uses Zeal’s raid roster.',
  },
]

export const OVERLAY_CATALOG: OverlayCatalogEntry[] = ENTRIES.map((e) => ({
  ...e,
  name: PANEL_POPOUT[e.panel].name,
}))

// Trigger Alerts isn't a window you open: it's the always-on layer alert text
// draws into. The hub shows it as an info card.
export const TRIGGER_ALERTS_INFO = {
  label: 'Trigger Alerts',
  blurb: 'Alert text from your triggers, drawn over the game.',
  details:
    'Always on, and never hidden by the show/hide-all hotkey. Set where alert text appears and how it ' +
    'looks in the overlay settings; make and edit triggers on the Triggers page.',
}

export const CUSTOM_GROUP_INFO = {
  blurb: 'A named Custom Timers window.',
  details:
    'Shows only the triggers assigned to this window, kept apart from the default ' +
    'Custom Timers window.',
}

// A window key's display name: an overlay's label, a named timer window's
// name from groups, or a custom overlay's name from customOverlays.
export function overlayWindowLabel(
  key: string,
  groups: Array<{ id: string; name: string }>,
  customOverlays: Array<{ id: string; name: string }> = [],
): string {
  if (key.startsWith('customTimer:')) {
    const id = key.slice('customTimer:'.length)
    return groups.find((g) => g.id === id)?.name ?? 'Timer window'
  }
  if (key.startsWith('customOverlay:')) {
    const id = key.slice('customOverlay:'.length)
    return customOverlays.find((o) => o.id === id)?.name ?? 'Custom overlay'
  }
  const entry = OVERLAY_CATALOG.find((e) => e.name === key)
  if (entry) return entry.label
  const panel = (Object.keys(PANEL_POPOUT) as DashboardPanelKey[]).find((k) => PANEL_POPOUT[k].name === key)
  return panel ? DASHBOARD_PANEL_LABELS[panel] : key
}
