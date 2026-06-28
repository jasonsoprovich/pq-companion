import React, { useEffect, useMemo, useRef, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Settings, Search, ChevronLeft, ChevronRight, ChevronDown } from 'lucide-react'
import { getLogStatus } from '../services/api'
import CharacterSwitcher from './CharacterSwitcher'
import { useHistoryNav } from '../hooks/useHistoryNav'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { visibleNavSections, orderItems, type NavItem } from '../lib/sidebarNav'
import { useSidebarPrefs } from '../hooks/useSidebarPrefs'

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

// Section collapse state lives in localStorage (a pure UI preference, no need
// to round-trip through the backend config).
const COLLAPSE_KEY = 'sidebar_collapsed_sections'

function loadCollapsed(): Record<string, boolean> {
  try {
    const raw = localStorage.getItem(COLLAPSE_KEY)
    return raw ? (JSON.parse(raw) as Record<string, boolean>) : {}
  } catch {
    return {}
  }
}

interface SidebarProps {
  onSearchClick: () => void
}

export default function Sidebar({ onSearchClick }: SidebarProps): React.ReactElement {
  const [logLargeFile, setLogLargeFile] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const { canGoBack, canGoForward, goBack, goForward } = useHistoryNav()
  const onDragMouseDown = useWindowDrag()
  const { hidden, order, flags } = useSidebarPrefs()
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(loadCollapsed)

  const toggleSection = (id: string): void => {
    setCollapsed((prev) => {
      const next = { ...prev, [id]: !prev[id] }
      try {
        localStorage.setItem(COLLAPSE_KEY, JSON.stringify(next))
      } catch {
        /* ignore quota/availability errors — collapse is best-effort */
      }
      return next
    })
  }

  // Apply the user's hide/order prefs to each section; drop sections that end
  // up empty so their header doesn't dangle.
  const sections = useMemo(() => {
    const hiddenSet = new Set(hidden)
    return visibleNavSections(flags)
      .map((s) => ({
        ...s,
        items: orderItems(s.items, order).filter((i) => !hiddenSet.has(i.to)),
      }))
      .filter((s) => s.items.length > 0)
  }, [hidden, order, flags])

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
      onMouseDown={onDragMouseDown}
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

      {/* Scrollable nav — hidden scrollbar. Sections, their visible items, and
          their order all come from the user's navigation preferences.
          `no-drag` is required: the parent <aside> is a window drag region, and
          a scroll viewport that's also a drag region gets flaky/unresponsive
          wheel scrolling in Chromium. Opting out restores smooth scrolling. */}
      <div className="no-drag scrollbar-hidden flex-1 overflow-y-auto">
        {sections.map((section) => {
          const isCollapsed = collapsed[section.id]
          return (
            <React.Fragment key={section.id}>
              <button
                onClick={() => toggleSection(section.id)}
                className="no-drag flex w-full items-center gap-1 px-3 pb-1 pt-3 text-left transition-colors hover:text-(--color-foreground)"
                style={{ color: 'var(--color-muted)' }}
                title={isCollapsed ? `Expand ${section.label}` : `Collapse ${section.label}`}
              >
                <ChevronDown
                  size={12}
                  style={{
                    transition: 'transform 0.15s',
                    transform: isCollapsed ? 'rotate(-90deg)' : 'none',
                  }}
                />
                <span className="text-[10px] font-semibold uppercase tracking-widest">
                  {section.label}
                </span>
              </button>
              {!isCollapsed && (
                <nav className="space-y-0.5 px-2 py-1">
                  {section.items.map((item) => (
                    <SidebarLink key={item.to} {...item} />
                  ))}
                </nav>
              )}
            </React.Fragment>
          )
        })}
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
