// Search index of settings for Ctrl+K and the Settings search box. Each
// entry points at where the setting actually lives (Settings tabs, or the
// Overlays hub for overlay and timer settings) and, optionally, an anchor
// (data-setting="<anchor>") to scroll to and highlight on arrival.

import { OVERLAY_CATALOG } from './overlayCatalog'

export interface SettingEntry {
  id: string
  label: string
  // Where it lives, shown under the label ("Settings › Audio").
  where: string
  keywords: string[]
  to: { path: string; search?: string }
  anchor?: string
}

const settings = (tab: string, anchor?: string) => ({
  path: '/settings',
  search: `?tab=${tab}${anchor ? `&focus=${anchor}` : ''}`,
})
const hub = (panel: string) => ({ path: '/overlays', search: `?panel=${panel}` })

const SETTINGS_TAB_ENTRIES: SettingEntry[] = [
  {
    id: 'app',
    label: 'App',
    where: 'Settings › General',
    keywords: ['updates', 'check for updates', 'auto update', 'version', 'install update'],
    to: settings('general', 'app'),
    anchor: 'app',
  },
  {
    id: 'setup-health',
    label: 'Feature health',
    where: 'Settings › General',
    keywords: ['status', 'not working', 'no data', 'log on', 'check', 'health', 'troubleshoot', 'setup health', 'zeal pipe', 'zeal settings', 'zeal commands', 'live data'],
    to: settings('general', 'setup-health'),
    anchor: 'setup-health',
  },
  {
    id: 'eq-install',
    label: 'EverQuest installation folder',
    where: 'Settings › General',
    keywords: ['eq path', 'eq folder', 'eqgame', 'install path', 'directory', 'character', 'quarm files'],
    to: settings('general', 'eq-install'),
    anchor: 'eq-install',
  },
  {
    id: 'eq-logging',
    label: 'EverQuest logging (Log=TRUE)',
    where: 'Settings › General › Feature health',
    keywords: ['log on', '/log', 'logging', 'eqclient.ini', 'export on camp', 'zeal export'],
    to: settings('general', 'eq-logging'),
    anchor: 'eq-logging',
  },
  {
    id: 'zeal',
    label: 'Zeal integration',
    where: 'Settings › General',
    keywords: ['zeal', 'install zeal', 'update zeal', 'pipe', 'zealpipes', 'quarm patcher', 'eqw'],
    to: settings('general', 'zeal'),
    anchor: 'zeal',
  },
  {
    id: 'eq-client',
    label: 'EQ client version',
    where: 'Settings › General',
    keywords: ['client', 'eqgame version', 'version check'],
    to: settings('general', 'eq-client'),
    anchor: 'eq-client',
  },
  {
    id: 'preferences',
    label: 'Preferences',
    where: 'Settings › General',
    keywords: ['parse combat log', 'parser', 'log parsing', 'minimize to tray', 'tray', 'close button'],
    to: settings('general', 'preferences'),
    anchor: 'preferences',
  },
  {
    id: 'audio',
    label: 'Audio & text-to-speech',
    where: 'Settings › Audio',
    keywords: ['tts', 'voice', 'speech', 'voice rate', 'speed', 'volume', 'output device', 'speakers', 'piper', 'kokoro', 'sound'],
    to: settings('audio'),
  },
  {
    id: 'accessibility',
    label: 'Accessibility',
    where: 'Settings › Accessibility',
    keywords: ['zoom', 'font size', 'text size', 'high contrast', 'readability', 'overlay zoom', 'scale'],
    to: settings('accessibility'),
  },
  {
    id: 'navigation',
    label: 'Navigation menu',
    where: 'Settings › Navigation',
    keywords: ['sidebar', 'hide', 'favorites', 'pin', 'reorder', 'menu', 'nav'],
    to: settings('navigation'),
  },
  {
    id: 'hotkeys',
    label: 'Hotkeys',
    where: 'Settings › Hotkeys',
    keywords: ['shortcut', 'keybind', 'key binding', 'show hide overlays', 'toggle overlay', 'layout hotkey'],
    to: settings('hotkeys'),
  },
  {
    id: 'discord',
    label: 'Discord webhooks',
    where: 'Settings › Discord',
    keywords: ['discord', 'webhook', 'post to discord', 'channel'],
    to: settings('discord'),
  },
  {
    id: 'logs',
    label: 'Log files',
    where: 'Settings › Logs',
    keywords: ['log', 'archive', 'trim', 'large log', 'log size', 'parser', 'log on'],
    to: settings('logs'),
  },
  {
    id: 'backfill',
    label: 'Backfill from old logs',
    where: 'Settings › Logs',
    keywords: ['backfill', 'import history', 'chat history', 'loot history', 'players', 'old logs'],
    to: settings('logs'),
  },
  {
    id: 'backups',
    label: 'Backup & restore',
    where: 'Settings › Backup & Restore',
    keywords: ['backup', 'restore', 'snapshot', 'ini', 'move to new pc', 'export', 'reset', 'factory reset'],
    to: settings('backups'),
  },
  {
    id: 'maps',
    label: 'Maps',
    where: 'Settings › Maps',
    keywords: ['map', 'map files', 'brewall', 'good maps', 'export map', '3d'],
    to: settings('maps'),
  },
  {
    id: 'network',
    label: 'Backend network',
    where: 'Settings › Advanced',
    keywords: ['port', 'network', 'lan', 'server address', 'firewall'],
    to: settings('advanced', 'network'),
    anchor: 'network',
  },
  {
    id: 'diagnostics',
    label: 'Diagnostics',
    where: 'Settings › Advanced',
    keywords: ['diagnostics', 'debug', 'logs folder', 'report bug', 'support'],
    to: settings('advanced', 'diagnostics'),
    anchor: 'diagnostics',
  },
]

const HUB_ENTRIES: SettingEntry[] = [
  {
    id: 'overlay-global',
    label: 'Overlay settings (all overlays)',
    where: 'Overlays',
    keywords: ['opacity', 'fade', 'transparency', 'lock', 'click-through', 'clickable links', 'restore on launch', 'alert text', 'monitor', 'display'],
    to: hub('global'),
  },
  {
    id: 'overlay-timers',
    label: 'Timer settings',
    where: 'Overlays › Overlay settings',
    keywords: ['tracking mode', 'scope', 'self only', 'cast by me', 'anyone', 'display threshold', 'seconds', 'row controls', 'bar', 'timer'],
    to: hub('timers'),
  },
  {
    id: 'overlay-trigger',
    label: 'Trigger alert text position',
    where: 'Overlays',
    keywords: ['alert position', 'alert text', 'on-screen text', 'trigger overlay'],
    to: hub('trigger'),
  },
  ...OVERLAY_CATALOG.map<SettingEntry>((e) => ({
    id: `overlay-${e.panel}`,
    label: `${e.label} overlay`,
    where: 'Overlays',
    keywords: ['overlay', 'popout', 'pop out', e.panel],
    to: hub(e.panel),
  })),
]

export const SETTINGS_INDEX: SettingEntry[] = [...SETTINGS_TAB_ENTRIES, ...HUB_ENTRIES]

// searchSettings ranks entries matching every query word in the label,
// location or keywords. Label matches rank first.
export function searchSettings(query: string, limit = 6): SettingEntry[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return []
  const scored: { e: SettingEntry; score: number }[] = []
  for (const e of SETTINGS_INDEX) {
    const label = e.label.toLowerCase()
    const hay = `${label} ${e.where.toLowerCase()} ${e.keywords.join(' ').toLowerCase()}`
    if (!terms.every((t) => hay.includes(t))) continue
    let score = 0
    for (const t of terms) {
      if (label.startsWith(t)) score += 10
      else if (label.includes(t)) score += 6
      if (e.keywords.some((k) => k.toLowerCase() === t)) score += 4
      else score += 1
    }
    scored.push({ e, score })
  }
  return scored.sort((a, b) => b.score - a.score).slice(0, limit).map((x) => x.e)
}
