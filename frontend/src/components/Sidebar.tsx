import React from 'react'
import { NavLink } from 'react-router-dom'
import { Sword, Sparkles, Skull, Map, Settings } from 'lucide-react'

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
      {/* Section header */}
      <div className="px-4 pb-1 pt-4">
        <span
          className="text-[10px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Database
        </span>
      </div>

      {/* Primary nav */}
      <nav className="flex-1 space-y-0.5 px-2 py-1">
        {PRIMARY_NAV.map((item) => (
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
