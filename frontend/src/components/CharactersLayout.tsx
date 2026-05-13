import React from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { Users, TrendingUp, Package, BookOpen, Library, KeyRound, ListChecks } from 'lucide-react'

interface CharactersTab {
  to: string
  label: string
  icon: React.ReactNode
}

const TABS: CharactersTab[] = [
  { to: '/characters/overview', label: 'Active Character', icon: <Users size={14} /> },
  { to: '/characters/progress', label: 'Character Info', icon: <TrendingUp size={14} /> },
  { to: '/characters/inventory', label: 'Inventory', icon: <Package size={14} /> },
  { to: '/characters/spells', label: 'Spells', icon: <BookOpen size={14} /> },
  { to: '/characters/spellsets', label: 'Spellsets', icon: <Library size={14} /> },
  { to: '/characters/keys', label: 'Keys', icon: <KeyRound size={14} /> },
  { to: '/characters/tasks', label: 'Tasks', icon: <ListChecks size={14} /> },
]

export default function CharactersLayout(): React.ReactElement {
  return (
    <div className="flex h-full flex-col">
      <div
        className="shrink-0 flex gap-1 px-6 pt-3 border-b"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {TABS.map((tab) => (
          <NavLink
            key={tab.to}
            to={tab.to}
            className={({ isActive }) =>
              [
                'flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-t transition-colors',
                isActive ? '' : 'hover:bg-(--color-surface-2)',
              ].join(' ')
            }
            style={({ isActive }) => ({
              backgroundColor: isActive ? 'var(--color-surface)' : 'transparent',
              borderBottom: isActive ? '2px solid var(--color-primary)' : '2px solid transparent',
              color: isActive ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
            })}
          >
            {tab.icon}
            {tab.label}
          </NavLink>
        ))}
      </div>
      <div className="flex-1 min-h-0">
        <Outlet />
      </div>
    </div>
  )
}
