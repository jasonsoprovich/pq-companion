import React from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { Crosshair, Swords, Timer } from 'lucide-react'

interface OverlaysTab {
  to: string
  label: string
  icon: React.ReactNode
}

const TABS: OverlaysTab[] = [
  { to: '/overlays/npc', label: 'NPC', icon: <Crosshair size={14} /> },
  { to: '/overlays/dps', label: 'DPS', icon: <Swords size={14} /> },
  { to: '/overlays/timers', label: 'Spell Timers', icon: <Timer size={14} /> },
]

export default function OverlaysLayout(): React.ReactElement {
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
