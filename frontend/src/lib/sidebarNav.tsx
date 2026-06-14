import React from 'react'
import {
  Sword, Sparkles, Skull, Map, Hammer, Activity, Layers, ScrollText, Zap,
  Users, Dice5, UserSearch, MessageSquare, Package, TrendingUp, BookOpen,
  Library, KeyRound, Hourglass, Star, Wand2, ListChecks,
} from 'lucide-react'

// A single navigable side-tab. `to` doubles as the stable key used for
// hide/order preferences.
export interface NavItem {
  to: string
  label: string
  icon: React.ReactNode
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
      { to: '/characters/keys', label: 'Keys', icon: <KeyRound size={16} /> },
      { to: '/characters/lockouts', label: 'Lockouts', icon: <Hourglass size={16} /> },
      { to: '/characters/wishlist', label: 'Wishlist', icon: <Star size={16} /> },
      { to: '/characters/upgrades', label: 'Gear Upgrades', icon: <Wand2 size={16} /> },
      { to: '/characters/tasks', label: 'Tasks', icon: <ListChecks size={16} /> },
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
