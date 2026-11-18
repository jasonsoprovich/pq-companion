import React, { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { Settings, Search, ChevronLeft, ChevronRight, ChevronDown, Star, LifeBuoy, Pencil, EyeOff, Rocket } from 'lucide-react'
import { getLogStatus } from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import CharacterSwitcher from './CharacterSwitcher'
import { useHistoryNav } from '../hooks/useHistoryNav'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { visibleNavSections, orderItems, favoriteItems, type NavItem } from '../lib/sidebarNav'
import { useSidebarPrefs } from '../hooks/useSidebarPrefs'
import { resolveRememberedRoute } from '../lib/lastSubRoute'
import { NewDot, useFeatureDots } from '../hooks/useFeatureDots'
import { usePackUpdates } from '../hooks/usePackUpdates'
import { hideSidebarItem, toggleSidebarFavorite } from '../lib/sidebarPrefsSave'
import { useGettingStarted } from '../hooks/useGettingStarted'

// Edit mode pulls in dnd-kit; keep it off the startup chunk.
const SidebarEditor = lazy(() => import('./SidebarEditor'))

const PACK_UPDATE_TITLE = 'Updates available for your installed trigger packs'

// useNavAttention is the dot for a nav route that needs a look: pending
// trigger pack updates on Triggers. Unlike feature dots, it stays until
// the packs are updated, not until the page is visited.
function useNavAttention(): (route: string) => boolean {
  const packUpdates = usePackUpdates()
  return (route) => route === '/triggers' && packUpdates.length > 0
}

function SidebarLink({
  to,
  label,
  icon,
  end,
  onContextMenu,
}: NavItem & { onContextMenu?: (e: React.MouseEvent) => void }): React.ReactElement {
  // Subscribe to navigation so the remembered sub-page (Combat/Raids) is
  // re-resolved after every route change, not just when the sidebar re-renders.
  useLocation()
  const { navDot } = useFeatureDots()
  const attention = useNavAttention()
  return (
    <NavLink
      to={resolveRememberedRoute(to)}
      end={end}
      onContextMenu={onContextMenu}
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
      {attention(to) ? (
        <NewDot className="ml-auto" title={PACK_UPDATE_TITLE} />
      ) : (
        navDot(to) && <NewDot className="ml-auto" />
      )}
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
  const { hidden, order, favorites, flags } = useSidebarPrefs()
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(loadCollapsed)
  const { navDot } = useFeatureDots()
  const attention = useNavAttention()
  const [editing, setEditing] = useState(false)
  const gettingStarted = useGettingStarted({ delayMs: 4000, skipWhenDismissed: true })
  const showChecklistPill =
    gettingStarted.loaded && !gettingStarted.dismissed && gettingStarted.doneCount < gettingStarted.total
  // Right-click menu on a nav item: { route, x, y }.
  const [menu, setMenu] = useState<{ route: string; x: number; y: number } | null>(null)
  const editSections = useMemo(() => visibleNavSections(flags), [flags])

  useEffect(() => {
    if (!menu) return
    const close = (): void => setMenu(null)
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') close()
    }
    window.addEventListener('mousedown', close)
    window.addEventListener('blur', close)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('blur', close)
      window.removeEventListener('keydown', onKey)
    }
  }, [menu])

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
  // up empty so their header doesn't dangle. Favorited items also appear in a
  // pinned "Favorites" section prepended at the top — a shortcut, not a move,
  // so the item stays put in its normal section too.
  const sections = useMemo(() => {
    const hiddenSet = new Set(hidden)
    const visible = visibleNavSections(flags)
    const base = visible
      .map((s) => ({
        ...s,
        items: orderItems(s.items, order).filter((i) => !hiddenSet.has(i.to)),
      }))
      .filter((s) => s.items.length > 0)
    const favs = favoriteItems(visible, favorites, order).filter((i) => !hiddenSet.has(i.to))
    return favs.length > 0 ? [{ id: 'favorites', label: 'Favorites', items: favs }, ...base] : base
  }, [hidden, order, favorites, flags])

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

  // The large-log threshold is configurable (Settings > Logs), so re-check
  // when config changes rather than waiting for the next 10-minute poll.
  useWebSocket((msg) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getLogStatus()
      .then((s) => setLogLargeFile(s.large_file))
      .catch(() => null)
  })

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
          <button
            onClick={() => setEditing((v) => !v)}
            data-tour="sidebar-edit"
            className="flex h-[30px] w-7 shrink-0 items-center justify-center rounded transition-colors hover:bg-(--color-surface-3)"
            style={{
              border: '1px solid var(--color-border)',
              backgroundColor: editing ? 'var(--color-surface-3)' : 'var(--color-surface-2)',
            }}
            title={editing ? 'Done editing the menu' : 'Edit menu: reorder, favorite or hide items'}
          >
            <Pencil size={12} style={{ color: editing ? 'var(--color-primary)' : 'var(--color-muted)' }} />
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
        {editing ? (
          <Suspense fallback={null}>
            <SidebarEditor
              sections={editSections}
              order={order}
              hidden={hidden}
              favorites={favorites}
              onDone={() => setEditing(false)}
            />
          </Suspense>
        ) : sections.map((section) => {
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
                <span className="flex items-center gap-1 text-[10px] font-semibold uppercase tracking-widest">
                  {section.id === 'favorites' && (
                    <Star size={10} fill="currentColor" style={{ color: 'var(--color-primary)' }} />
                  )}
                  {section.label}
                </span>
                {/* A collapsed section would hide its items' dots, so the
                    header carries one instead. */}
                {isCollapsed && section.items.some((i) => attention(i.to)) ? (
                  <NewDot className="ml-auto" title={PACK_UPDATE_TITLE} />
                ) : (
                  isCollapsed && section.items.some((i) => navDot(i.to)) && (
                    <NewDot className="ml-auto" />
                  )
                )}
              </button>
              {!isCollapsed && (
                <nav className="space-y-0.5 px-2 py-1">
                  {section.items.map((item) => (
                    <SidebarLink
                      key={item.to}
                      {...item}
                      onContextMenu={(e) => {
                        e.preventDefault()
                        setMenu({ route: item.to, x: e.clientX, y: e.clientY })
                      }}
                    />
                  ))}
                </nav>
              )}
            </React.Fragment>
          )
        })}
      </div>

      {menu && (
        <div
          className="no-drag fixed z-50 min-w-40 rounded py-1 text-xs shadow-lg"
          style={{
            left: menu.x,
            top: Math.min(menu.y, window.innerHeight - 110),
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          {[
            {
              icon: <Star size={12} />,
              label: favorites.includes(menu.route) ? 'Remove from favorites' : 'Add to favorites',
              run: () => void toggleSidebarFavorite(menu.route),
            },
            {
              icon: <EyeOff size={12} />,
              label: 'Hide from menu',
              run: () => void hideSidebarItem(menu.route),
            },
            {
              icon: <Pencil size={12} />,
              label: 'Edit menu…',
              run: () => setEditing(true),
            },
          ].map((a) => (
            <button
              key={a.label}
              onClick={() => {
                a.run()
                setMenu(null)
              }}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-(--color-surface-3)"
              style={{ color: 'var(--color-foreground)', cursor: 'pointer' }}
            >
              {a.icon}
              {a.label}
            </button>
          ))}
        </div>
      )}

      {/* Bottom — Help and Settings (always visible) */}
      <div
        className="shrink-0 border-t px-2 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {showChecklistPill && (
          <NavLink
            to="/help?tab=quickstart"
            className="no-drag mb-1 flex items-center gap-2 rounded px-3 py-1.5 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-primary)',
            }}
            title="Finish setting up PQ Companion"
          >
            <Rocket size={13} />
            <span className="flex-1">Getting started</span>
            <span className="tabular-nums" style={{ color: 'var(--color-muted-foreground)' }}>
              {gettingStarted.doneCount}/{gettingStarted.total}
            </span>
          </NavLink>
        )}
        <NavLink
          to="/help"
          className={({ isActive }) =>
            [
              'no-drag flex items-center gap-3 rounded px-3 py-2 text-sm transition-colors',
              isActive
                ? 'bg-(--color-surface-2) text-(--color-primary) font-medium'
                : 'text-(--color-muted-foreground) hover:bg-(--color-surface-2) hover:text-(--color-foreground)',
            ].join(' ')
          }
        >
          <LifeBuoy size={16} />
          Help
        </NavLink>
        <NavLink
          // With a large log, jump straight to the tab that explains and
          // fixes it — otherwise Settings reopens wherever the user left off.
          to={logLargeFile ? '/settings?tab=logs' : '/settings'}
          title={
            logLargeFile
              ? 'Your EverQuest log file is large — click to archive & trim it (Settings › Logs)'
              : undefined
          }
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
            {logLargeFile ? (
              <span
                className="absolute -top-1 -right-1 h-2 w-2 rounded-full"
                style={{ backgroundColor: '#f97316' }}
              />
            ) : (
              // The orange warning dot wins; otherwise green for new
              // Settings features.
              navDot('/settings') && (
                <span
                  className="absolute -top-1 -right-1 h-2 w-2 rounded-full"
                  style={{ backgroundColor: 'var(--color-success)' }}
                  title="New since your last update"
                />
              )
            )}
          </span>
          Settings
        </NavLink>
      </div>
    </aside>
  )
}
