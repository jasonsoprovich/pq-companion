import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown, EyeOff, Layers } from 'lucide-react'
import { useOverlayPopouts } from '../hooks/useOverlayPopouts'
import { useOverlayLayouts } from '../hooks/useOverlayLayouts'
import { PANEL_POPOUT, overlayWindowLabel } from '../lib/overlayCatalog'
import type { DashboardPanelKey } from '../services/dashboardLayout'
import { listTimerGroups, getCustomOverlays } from '../services/api'
import type { TimerGroup } from '../types/trigger'

// Sidebar quick-toggle for overlays, next to the character switcher — that
// area of the sidebar is always visible. One click shows overlays hidden by
// the show/hide-all hotkey, closes open ones (they stay in the layout;
// trigger alerts keep showing), or pops out the active layout when nothing
// is open (see useOverlayPopouts).
//
// The caret beside it opens the quick switcher: the layouts, and the windows
// in the active one with a pop out / show / close each.

type WinState = 'open' | 'hidden' | 'closed'

const POLL_MS = 1000
const GROUP_PREFIX = 'customTimer:'
const CUSTOM_OVERLAY_PREFIX = 'customOverlay:'

// Pops out, shows or closes one window. Main's per-window toggle brings back
// a window hidden by the hotkey rather than closing it.
type Named = { id: string; name: string }

function toggleWindow(key: string, groups: TimerGroup[], customs: Named[]): void {
  if (key.startsWith(CUSTOM_OVERLAY_PREFIX)) {
    const id = key.slice(CUSTOM_OVERLAY_PREFIX.length)
    window.electron?.overlay?.toggleCustomOverlay(id, customs.find((o) => o.id === id)?.name ?? '')
    return
  }
  if (key.startsWith(GROUP_PREFIX)) {
    const id = key.slice(GROUP_PREFIX.length)
    window.electron?.overlay?.toggleCustomTimerGroup(id, groups.find((g) => g.id === id)?.name ?? '')
    return
  }
  const panel = (Object.keys(PANEL_POPOUT) as DashboardPanelKey[]).find((k) => PANEL_POPOUT[k].name === key)
  if (panel) PANEL_POPOUT[panel].toggle()
}

const SWITCHER_WIDTH = 256

// Rendered into document.body at a fixed spot under the button: the sidebar
// clips its overflow, and the switcher is wider than the gap beside it.
function QuickSwitcher({
  anchor,
  panelRef,
  onClose,
}: {
  anchor: HTMLElement
  panelRef: React.RefObject<HTMLDivElement>
  onClose: () => void
}): React.ReactElement {
  const [pos, setPos] = useState<{ top: number; left: number }>({ top: 0, left: 0 })
  useLayoutEffect(() => {
    const place = (): void => {
      const r = anchor.getBoundingClientRect()
      const left = Math.max(8, Math.min(r.left, window.innerWidth - SWITCHER_WIDTH - 8))
      setPos({ top: r.bottom + 6, left })
    }
    place()
    window.addEventListener('resize', place)
    return () => window.removeEventListener('resize', place)
  }, [anchor])
  const { state, switchTo } = useOverlayLayouts()
  const [groups, setGroups] = useState<TimerGroup[]>([])
  const [customs, setCustoms] = useState<Named[]>([])
  const [states, setStates] = useState<Record<string, 'open' | 'hidden'>>({})
  // Closing a window takes it out of the active layout. Rows stay listed
  // until the switcher closes, as "closed", so they can be popped back out.
  const [rows, setRows] = useState<string[]>([])

  const active = state?.layouts.find((l) => l.id === state.activeId)

  useEffect(() => {
    listTimerGroups().then(setGroups).catch(() => {})
    getCustomOverlays()
      .then((s) => setCustoms((s.overlays ?? []).map((o) => ({ id: o.def.id, name: o.def.name }))))
      .catch(() => {})
  }, [])

  const refresh = useCallback(() => {
    window.electron?.overlay?.popoutWindowStates?.().then(setStates).catch(() => {})
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, POLL_MS)
    return () => clearInterval(id)
  }, [refresh])

  useEffect(() => {
    if (!active) return
    setRows((prev) => {
      const next = [...prev]
      for (const key of active.windows) if (!next.includes(key)) next.push(key)
      return next
    })
  }, [active])

  const stateOf = (key: string): WinState => states[key] ?? 'closed'

  const sectionLabel: React.CSSProperties = {
    color: 'var(--color-muted)',
  }

  return createPortal(
    <div
      ref={panelRef}
      className="fixed z-50 flex flex-col rounded-lg p-1.5"
      style={{
        top: pos.top,
        left: pos.left,
        width: SWITCHER_WIDTH,
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
      }}
    >
      <div className="px-2 pb-1 pt-0.5 text-[10px] font-semibold uppercase tracking-wide" style={sectionLabel}>
        Layouts
      </div>
      {state?.layouts.map((l) => {
        const isActive = l.id === state.activeId
        return (
          <button
            key={l.id}
            onClick={() => {
              if (!isActive) switchTo(l.id)
              onClose()
            }}
            className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs hover:bg-(--color-surface-2)"
            style={{ color: 'var(--color-foreground)' }}
          >
            <span className="flex w-3 shrink-0 justify-center">
              {isActive && <Check size={11} style={{ color: 'var(--color-primary)' }} />}
            </span>
            <span className="flex-1 truncate" style={{ fontWeight: isActive ? 600 : 400 }}>{l.name}</span>
            <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>
              {l.windows.length}
            </span>
          </button>
        )
      })}

      <div
        className="mt-1 border-t px-2 pb-1 pt-1.5 text-[10px] font-semibold uppercase tracking-wide"
        style={{ ...sectionLabel, borderColor: 'var(--color-border)' }}
      >
        Windows in {active?.name ?? 'this layout'}
      </div>
      {rows.length === 0 && (
        <p className="px-2 py-1 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          None yet. Pop out overlays from the Overlays page and they join this layout.
        </p>
      )}
      <div className="flex max-h-[50vh] flex-col overflow-y-auto">
        {rows.map((key) => {
          const st = stateOf(key)
          const label = overlayWindowLabel(key, groups, customs)
          const action = st === 'open' ? 'Close' : st === 'hidden' ? 'Show' : 'Pop out'
          return (
            <div key={key} className="flex items-center gap-2 rounded px-2 py-0.5 text-xs">
              <span
                className="h-1.5 w-1.5 shrink-0 rounded-full"
                style={{
                  backgroundColor:
                    st === 'open' ? 'var(--color-success)' : st === 'hidden' ? 'var(--color-primary)' : 'var(--color-border)',
                }}
                title={st === 'open' ? 'Open' : st === 'hidden' ? 'Hidden by the hotkey' : 'Closed'}
              />
              <span
                className="flex-1 truncate"
                style={{ color: st === 'closed' ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}
              >
                {label}
              </span>
              <button
                onClick={() => {
                  toggleWindow(key, groups, customs)
                  setTimeout(refresh, 250)
                  setTimeout(refresh, 900)
                }}
                className="shrink-0 rounded px-1.5 py-0.5 text-[10px]"
                style={{
                  border: '1px solid var(--color-border)',
                  backgroundColor: st === 'closed' ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: st === 'closed' ? 'var(--color-primary-foreground)' : 'var(--color-foreground)',
                }}
                title={`${action} ${label}`}
              >
                {action}
              </button>
            </div>
          )
        })}
      </div>
    </div>,
    document.body,
  )
}

export default function OverlayPopoutToggle(): React.ReactElement | null {
  const { anyPopoutOpen, anyHidden, supported, toggle } = useOverlayPopouts()
  const { state, supported: layoutsSupported } = useOverlayLayouts()
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent): void => {
      const t = e.target as Node
      if (wrapRef.current?.contains(t) || panelRef.current?.contains(t)) return
      setOpen(false)
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  if (!supported) return null

  const showing = anyPopoutOpen && !anyHidden
  const activeName = state?.layouts.find((l) => l.id === state.activeId)?.name
  const action = anyHidden
    ? 'Overlays hidden by the hotkey — click to show them again'
    : anyPopoutOpen
      ? 'Close all overlays (they reopen in place; trigger alerts keep showing)'
      : 'Pop out your overlays'
  const borderColor = anyHidden ? 'var(--color-primary)' : 'var(--color-border)'
  const bg = showing ? 'var(--color-primary)' : 'var(--color-surface-2)'
  const fg = showing
    ? 'var(--color-primary-foreground)'
    : anyHidden
      ? 'var(--color-primary)'
      : 'var(--color-muted)'

  return (
    <div ref={wrapRef} className="relative flex shrink-0">
      <button
        onClick={toggle}
        className={`flex h-[30px] w-7 shrink-0 items-center justify-center transition-colors ${
          layoutsSupported ? 'rounded-l' : 'rounded'
        }`}
        style={{ borderWidth: 1, borderStyle: 'solid', borderColor, backgroundColor: bg, color: fg }}
        title={activeName ? `Layout: ${activeName} — ${action}` : action}
      >
        {anyHidden ? <EyeOff size={13} /> : <Layers size={13} />}
      </button>
      {layoutsSupported && (
        <button
          onClick={() => setOpen((o) => !o)}
          className="flex h-[30px] w-4 shrink-0 items-center justify-center rounded-r transition-colors"
          style={{
            borderWidth: '1px 1px 1px 0',
            borderStyle: 'solid',
            borderColor,
            backgroundColor: open ? 'var(--color-surface)' : 'var(--color-surface-2)',
            color: open ? 'var(--color-foreground)' : 'var(--color-muted)',
          }}
          title="Switch layout, or pop out a window"
          aria-label="Open the overlay quick switcher"
        >
          <ChevronDown size={10} />
        </button>
      )}
      {open && wrapRef.current && (
        <QuickSwitcher anchor={wrapRef.current} panelRef={panelRef} onClose={() => setOpen(false)} />
      )}
    </div>
  )
}
