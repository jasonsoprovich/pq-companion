import React, { useEffect, useRef, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Sword, Sparkles, Skull, Map, Settings, Search, Activity, Layers, ScrollText, Archive, Zap, Users, ChevronLeft, ChevronRight, Dice5, UserSearch } from 'lucide-react'
import { getLogStatus } from '../services/api'
import CharacterSwitcher from './CharacterSwitcher'
import { useHistoryNav } from '../hooks/useHistoryNav'

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
  { to: '/characters', label: 'Characters', icon: <Users size={16} /> },
]

const PARSING_NAV: NavItem[] = [
  { to: '/log-feed', label: 'Log Feed', icon: <Activity size={16} /> },
  { to: '/overlays', label: 'Overlays', icon: <Layers size={16} /> },
  { to: '/combat-log', label: 'Combat Log', icon: <ScrollText size={16} /> },
  { to: '/combat-history', label: 'Combat History', icon: <Archive size={16} /> },
  { to: '/triggers', label: 'Triggers', icon: <Zap size={16} /> },
  { to: '/rolls', label: 'Roll Tracker', icon: <Dice5 size={16} /> },
  { to: '/players', label: 'Players', icon: <UserSearch size={16} /> },
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

interface SidebarProps {
  onSearchClick: () => void
}

export default function Sidebar({ onSearchClick }: SidebarProps): React.ReactElement {
  const [logLargeFile, setLogLargeFile] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const { canGoBack, canGoForward, goBack, goForward } = useHistoryNav()

  useEffect(() => {
    const poll = () => {
      getLogStatus()
        .then((s) => setLogLargeFile(s.large_file))
        .catch(() => null)
    }
    poll()
    pollRef.current = setInterval(poll, 10 * 60 * 1000)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [])

  return (
    <aside
      className="drag-region flex w-48 shrink-0 flex-col overflow-hidden border-r"
      style={{
        backgroundColor: 'var(--color-surface)',
        borderColor: 'var(--color-border)',
      }}
    >
      {/* Sticky top — nav buttons, search, character switcher always visible */}
      <div className="shrink-0">
        {/* Nav buttons + global search hint */}
        <div className="no-drag flex items-center gap-1 px-2 pb-1 pt-3">
          <button
            onClick={goBack}
            disabled={!canGoBack}
            className="flex h-[30px] w-7 shrink-0 items-center justify-center rounded transition-colors hover:bg-(--color-surface-3) disabled:opacity-30 disabled:cursor-not-allowed"
            style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface-2)' }}
            title="Go back"
          >
            <ChevronLeft size={13} style={{ color: 'var(--color-muted)' }} />
          </button>
          <button
            onClick={goForward}
            disabled={!canGoForward}
            className="flex h-[30px] w-7 shrink-0 items-center justify-center rounded transition-colors hover:bg-(--color-surface-3) disabled:opacity-30 disabled:cursor-not-allowed"
            style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface-2)' }}
            title="Go forward"
          >
            <ChevronRight size={13} style={{ color: 'var(--color-muted)' }} />
          </button>
          <button
            onClick={onSearchClick}
            className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-(--color-surface-3)"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
            }}
          >
            <Search size={12} style={{ color: 'var(--color-muted)' }} />
            <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>Search</span>
          </button>
        </div>

        {/* Character switcher */}
        <CharacterSwitcher />
      </div>

      {/* Scrollable nav — hidden scrollbar */}
      <div className="scrollbar-hidden flex-1 overflow-y-auto">
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
        <nav className="space-y-0.5 px-2 py-1">
          {ZEAL_NAV.map((item) => (
            <SidebarLink key={item.to} {...item} />
          ))}
        </nav>

        {/* Parsing section */}
        <div className="px-4 pb-1 pt-3">
          <span
            className="text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Parsing
          </span>
        </div>
        <nav className="space-y-0.5 px-2 py-1">
          {PARSING_NAV.map((item) => (
            <SidebarLink key={item.to} {...item} />
          ))}
        </nav>
      </div>

      {/* Bottom — Settings (always visible) */}
      <div
        className="shrink-0 border-t px-2 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <NavLink
          to="/settings"
          className={({ isActive }) =>
            [
              'no-drag flex items-center gap-3 rounded px-3 py-2 text-sm transition-colors',
              isActive
                ? 'bg-(--color-surface-2) text-(--color-primary) font-medium'
                : 'text-(--color-muted-foreground) hover:bg-(--color-surface-2) hover:text-(--color-foreground)',
            ].join(' ')
          }
        >
          <span className="relative">
            <Settings size={16} />
            {logLargeFile && (
              <span
                className="absolute -top-1 -right-1 h-2 w-2 rounded-full"
                style={{ backgroundColor: '#f97316' }}
                title="Log file is large — cleanup recommended"
              />
            )}
          </span>
          Settings
        </NavLink>
      </div>
    </aside>
  )
}
