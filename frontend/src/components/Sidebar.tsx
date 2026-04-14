import React from 'react'
import { NavLink } from 'react-router-dom'
import { Sword, Sparkles, Skull, Map, Settings, Search, Package, BookOpen } from 'lucide-react'

interface NavItem {
  to: string
  label: string
  icon: React.ReactNode
}

const PRIMARY_NAV: NavItem[] = [
  { to: '/items', label: 'Items', icon: <Sword size={16} /> },
  { to: '/spells', label: 'Spells', icon: <Sparkles size={16} /> },
  { to: '/npcs', label: 'NPCs', icon: <Skull size={16} /> },
  { to: '/zones', label: 'Zones', icon: <Map size={16} /> },
]

const ZEAL_NAV: NavItem[] = [
  { to: '/inventory-tracker', label: 'Inventory Tracker', icon: <Package size={16} /> },
  { to: '/spell-checklist', label: 'Spell Checklist', icon: <BookOpen size={16} /> },
]

function SidebarLink({ to, label, icon }: NavItem): React.ReactElement {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        [
          'no-drag flex items-center gap-3 rounded px-3 py-2 text-sm transition-colors',
          isActive
            ? 'bg-(--color-surface-2) text-(--color-primary) font-medium'
            : 'text-(--color-muted-foreground) hover:bg-(--color-surface-2) hover:text-(--color-foreground)',
        ].join(' ')
      }
    >
      {icon}
      {label}
    </NavLink>
  )
}

export default function Sidebar(): React.ReactElement {
  return (
    <aside
      className="drag-region flex w-48 shrink-0 flex-col border-r"
      style={{
        backgroundColor: 'var(--color-surface)',
        borderColor: 'var(--color-border)',
      }}
    >
      {/* Global search hint */}
      <div className="no-drag px-2 pb-1 pt-3">
        <div
          className="flex items-center justify-between rounded px-3 py-1.5"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          <div className="flex items-center gap-2">
            <Search size={12} style={{ color: 'var(--color-muted)' }} />
            <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>Search</span>
          </div>
          <kbd
            className="text-[10px]"
            style={{ color: 'var(--color-muted)' }}
          >
            ⌘K
          </kbd>
        </div>
      </div>

      {/* Section header */}
      <div className="px-4 pb-1 pt-3">
        <span
          className="text-[10px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Database
        </span>
      </div>

      {/* Primary nav */}
      <nav className="space-y-0.5 px-2 py-1">
        {PRIMARY_NAV.map((item) => (
          <SidebarLink key={item.to} {...item} />
        ))}
      </nav>

      {/* Zeal section */}
      <div className="px-4 pb-1 pt-3">
        <span
          className="text-[10px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Zeal
        </span>
      </div>
      <nav className="flex-1 space-y-0.5 px-2 py-1">
        {ZEAL_NAV.map((item) => (
          <SidebarLink key={item.to} {...item} />
        ))}
      </nav>

      {/* Bottom — Settings */}
      <div
        className="border-t px-2 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <SidebarLink
          to="/settings"
          label="Settings"
          icon={<Settings size={16} />}
        />
      </div>
    </aside>
  )
}
