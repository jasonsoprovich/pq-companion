import React from 'react'
import {
  Sword, Sparkles, Skull, Map, Hammer, Activity, Layers, ScrollText, Zap,
  Users, Dice5, UserSearch, MessageSquare, Package, TrendingUp, BookOpen,
  Library, KeyRound, Hourglass, Star, Wand2, ListChecks, Percent, Store, PawPrint,
  Flag, Swords, Keyboard,
} from 'lucide-react'
import type { Preferences } from '../types/config'

// A single navigable side-tab. `to` doubles as the stable key used for
// hide/order preferences. `flag`, when set, is a Developer-tab preference key
// that gates the tab: it only appears (in the sidebar and the Navigation
// settings editor) while that flag is enabled.
export interface NavItem {
  to: string
  label: string
  icon: React.ReactNode
  flag?: string
}

// A labeled group of side tabs. Visibility/ordering preferences apply to the
// items within each section; sections themselves and their order are fixed.
export interface NavSection {
  id: string
  label: string
  items: NavItem[]
}

// NAV_SECTIONS is the canonical sidebar tab definition, shared by the Sidebar
// and the Settings → Navigation editor so they never drift. The fixed controls
// (back/forward, search, character switcher, settings) live in the Sidebar
// itself and are intentionally NOT part of this list — they can't be hidden or
// reordered.
export const NAV_SECTIONS: NavSection[] = [
  {
    id: 'database',
    label: 'Database',
    items: [
      { to: '/items', label: 'Items', icon: <Sword size={16} /> },
      { to: '/spells', label: 'Spells', icon: <Sparkles size={16} /> },
      { to: '/npcs', label: 'NPCs', icon: <Skull size={16} /> },
      { to: '/zones', label: 'Zones', icon: <Map size={16} /> },
      { to: '/recipes', label: 'Recipes', icon: <Hammer size={16} /> },
      { to: '/quests', label: 'Quests', icon: <ScrollText size={16} /> },
      { to: '/charm-pet-finder', label: 'Charm Pet Finder', icon: <PawPrint size={16} /> },
      { to: '/resist-calc', label: 'Resist Calculator', icon: <Percent size={16} /> },
    ],
  },
  {
    id: 'characters',
    label: 'Characters',
    items: [
      { to: '/characters/overview', label: 'Active Character', icon: <Users size={16} /> },
      { to: '/characters/progress', label: 'Character Info', icon: <TrendingUp size={16} /> },
      { to: '/characters/inventory', label: 'Inventory', icon: <Package size={16} /> },
      { to: '/characters/spells', label: 'Spells', icon: <BookOpen size={16} /> },
      { to: '/characters/spellsets', label: 'Spellsets', icon: <Library size={16} /> },
      { to: '/characters/bandolier', label: 'Bandolier', icon: <Swords size={16} /> },
      { to: '/characters/macros', label: 'Macros', icon: <Keyboard size={16} /> },
      { to: '/characters/keys', label: 'Keys', icon: <KeyRound size={16} /> },
      { to: '/characters/lockouts', label: 'Lockouts', icon: <Hourglass size={16} /> },
      { to: '/characters/wishlist', label: 'Wishlist', icon: <Star size={16} /> },
      { to: '/characters/upgrades', label: 'Gear Upgrades', icon: <Wand2 size={16} /> },
      { to: '/characters/tasks', label: 'Tasks', icon: <ListChecks size={16} /> },
      { to: '/pop-flags', label: 'PoP Flags', icon: <Flag size={16} />, flag: 'pop_flags_enabled' },
      { to: '/trader-tracker', label: 'Trader Tracker', icon: <Store size={16} /> },
    ],
  },
  {
    id: 'parsing',
    label: 'Parsing',
    items: [
      { to: '/log-feed', label: 'Log Feed', icon: <Activity size={16} /> },
      { to: '/overlays', label: 'Overlays', icon: <Layers size={16} /> },
      { to: '/combat', label: 'Combat Log', icon: <ScrollText size={16} /> },
      { to: '/triggers', label: 'Triggers', icon: <Zap size={16} /> },
      { to: '/rolls', label: 'Roll Tracker', icon: <Dice5 size={16} /> },
      { to: '/players', label: 'Player Tracker', icon: <UserSearch size={16} /> },
      { to: '/chat', label: 'Chat History', icon: <MessageSquare size={16} /> },
      { to: '/loot', label: 'Loot Tracker', icon: <Package size={16} /> },
    ],
  },
]

// navFlags builds the flag map consumed by visibleNavSections from config
// preferences. Centralized here so the live sidebar and the Navigation settings
// editor gate the same dev-preview tabs identically.
export function navFlags(prefs?: Partial<Preferences>): Record<string, boolean> {
  return {
    pop_flags_enabled: Boolean(prefs?.pop_flags_enabled),
  }
}

// visibleNavSections filters out flag-gated items whose flag isn't enabled, then
// drops any section left empty. `flags` maps a NavItem.flag key to its enabled
// state (from config preferences). Shared by the Sidebar and the Navigation
// settings editor so a gated tab appears in both only when its flag is on.
export function visibleNavSections(flags: Record<string, boolean>): NavSection[] {
  return NAV_SECTIONS
    .map((s) => ({
      ...s,
      items: s.items.filter((i) => !i.flag || flags[i.flag]),
    }))
    .filter((s) => s.items.length > 0)
}

// orderItems sorts a section's items by their position in `order`; items absent
// from `order` keep their default relative position after the listed ones.
export function orderItems(items: NavItem[], order: string[]): NavItem[] {
  const rank = (to: string): number => {
    const i = order.indexOf(to)
    return i === -1 ? Number.MAX_SAFE_INTEGER : i
  }
  return items
    .map((item, idx) => ({ item, idx }))
    .sort((a, b) => {
      const ra = rank(a.item.to)
      const rb = rank(b.item.to)
      return ra !== rb ? ra - rb : a.idx - b.idx
    })
    .map((x) => x.item)
}
