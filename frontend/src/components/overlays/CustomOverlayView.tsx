/**
 * CustomOverlayView — body of a Custom Overlay, shared by the dashboard panel
 * (CustomOverlayPanel) and the popout window (CustomOverlayWindowPage).
 * Renders the overlay's widgets top to bottom: small uppercase titles as
 * section headers (none when a widget is untitled), tight rows like the
 * timer overlays. Tables and feeds take the spare height and scroll.
 *
 * Contents come pre-shaped from the backend (rows sorted, feed newest first),
 * so this only formats. Right-clicking a table row offers "Remove" when
 * onRemoveRow is set (unlocked or interactive windows, and the dashboard).
 *
 * The `theme` prop picks the colour set: 'panel' uses the app's CSS tokens,
 * 'overlay' uses the translucent-white palette every popout overlay uses.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNowMs } from '../../hooks/useCustomOverlays'
import {
  fieldColor,
  fmtCountdown,
  fmtFeedTime,
  nextCountdownEnd,
  widgetState,
} from '../../lib/customOverlay'
import type { CustomOverlay, CustomWidget } from '../../types/customOverlay'

export type CustomOverlayTheme = 'panel' | 'overlay'

interface Palette {
  fg: string
  muted: string
  faint: string
  border: string
}

const PALETTES: Record<CustomOverlayTheme, Palette> = {
  panel: {
    fg: 'var(--color-foreground)',
    muted: 'var(--color-muted)',
    faint: 'var(--color-muted)',
    border: 'var(--color-border)',
  },
  overlay: {
    fg: 'rgba(255,255,255,0.92)',
    muted: 'rgba(255,255,255,0.55)',
    faint: 'rgba(255,255,255,0.35)',
    border: 'rgba(255,255,255,0.08)',
  },
}

const ACCENT = '#a78bfa'
const URGENT = '#f87171'

interface CustomOverlayViewProps {
  theme: CustomOverlayTheme
  overlay: CustomOverlay
  skewMs: number
  // When set, right-clicking a table row offers to remove it.
  onRemoveRow?: (widgetId: string, key: string) => void
  hideEmptyState?: boolean
}

type RowMenu = { widget: string; key: string; x: number; y: number }

export default function CustomOverlayView({
  theme,
  overlay,
  skewMs,
  onRemoveRow,
  hideEmptyState,
}: CustomOverlayViewProps): React.ReactElement {
  const pal = PALETTES[theme]
  const now = useNowMs(nextCountdownEnd(overlay, Date.now() + skewMs) > 0, skewMs)
  const [menu, setMenu] = useState<RowMenu | null>(null)
  const closeMenu = useCallback(() => setMenu(null), [])

  const empty = overlay.def.widgets.every((w) => {
    const s = widgetState(overlay, w)
    return !Object.values(s.fields ?? {}).some(Boolean) && !(s.rows?.length) &&
      !(s.lines?.length) && !s.count && !((s.ends_at ?? 0) > now)
  })

  if (empty && !hideEmptyState) {
    return (
      <div style={{ padding: '14px 10px', fontSize: 11, color: pal.faint, textAlign: 'center' }}>
        {overlay.def.enabled ? 'Waiting for matching log lines…' : 'Rules are turned off for this overlay.'}
      </div>
    )
  }

  return (
    <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      {overlay.def.widgets.map((w) => (
        <WidgetBlock
          key={w.id}
          widget={w}
          overlay={overlay}
          pal={pal}
          now={now}
          onRowMenu={onRemoveRow ? (key, x, y) => setMenu({ widget: w.id, key, x, y }) : undefined}
        />
      ))}
      {menu && onRemoveRow && (
        <RowMenuPopup
          menu={menu}
          onRemove={() => { onRemoveRow(menu.widget, menu.key); closeMenu() }}
          onClose={closeMenu}
        />
      )}
    </div>
  )
}

function grows(w: CustomWidget): boolean {
  return w.type === 'table' || w.type === 'feed'
}

function WidgetBlock({
  widget: w,
  overlay,
  pal,
  now,
  onRowMenu,
}: {
  widget: CustomWidget
  overlay: CustomOverlay
  pal: Palette
  now: number
  onRowMenu?: (key: string, x: number, y: number) => void
}): React.ReactElement {
  const s = widgetState(overlay, w)
  const header = w.title ? (
    <div
      style={{
        fontSize: 9,
        fontWeight: 700,
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        color: pal.faint,
        padding: '5px 8px 2px',
        flexShrink: 0,
      }}
    >
      {w.title}
    </div>
  ) : null

  const wrap = (body: React.ReactNode): React.ReactElement => (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        // Lists start at their content height and share any spare height or
        // shortfall in proportion, so a short feed doesn't squeeze the table.
        flex: grows(w) ? '1 1 auto' : '0 0 auto',
        minHeight: grows(w) ? 44 : undefined,
        borderBottom: `1px solid ${pal.border}`,
      }}
    >
      {header}
      {body}
    </div>
  )

  switch (w.type) {
    case 'fields':
      return wrap(
        <div style={{ padding: '3px 8px 5px', display: 'grid', gridTemplateColumns: 'auto 1fr', columnGap: 10, rowGap: 2 }}>
          {(w.fields ?? []).map((f) => {
            const v = s.fields?.[f.key] ?? ''
            return (
              <React.Fragment key={f.key}>
                <span style={{ fontSize: 11, color: pal.muted, whiteSpace: 'nowrap' }}>{f.label || f.key}</span>
                <span
                  title={v}
                  style={{
                    fontSize: 12,
                    fontWeight: 600,
                    color: v ? fieldColor(f, v) ?? pal.fg : pal.faint,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    minWidth: 0,
                  }}
                >
                  {v || '—'}
                </span>
              </React.Fragment>
            )
          })}
        </div>,
      )

    case 'table': {
      const cols = w.columns ?? []
      const rows = s.rows ?? []
      const labelled = cols.length > 2 || cols.some((c, i) => i > 0 && c.label)
      const gridCols = cols.map((_, i) => (i === 0 ? 'minmax(0,1fr)' : 'auto')).join(' ')
      return wrap(
        <div style={{ flex: '1 1 auto', minHeight: 0, overflowY: 'auto', padding: '1px 0 3px' }}>
          {labelled && rows.length > 0 && (
            <div style={{ display: 'grid', gridTemplateColumns: gridCols, columnGap: 10, padding: '0 8px 1px' }}>
              {cols.map((c, i) => (
                <span key={c.key} style={{ fontSize: 9, color: pal.faint, textAlign: i === 0 ? 'left' : 'right' }}>
                  {c.label}
                </span>
              ))}
            </div>
          )}
          {rows.length === 0 && (
            <div style={{ fontSize: 11, color: pal.faint, padding: '2px 8px' }}>—</div>
          )}
          {rows.map((r) => (
            <div
              key={r.key}
              onContextMenu={onRowMenu ? (e) => { e.preventDefault(); onRowMenu(r.key, e.clientX, e.clientY) } : undefined}
              style={{
                display: 'grid',
                gridTemplateColumns: gridCols,
                columnGap: 10,
                padding: '2px 8px',
                alignItems: 'baseline',
              }}
            >
              {cols.map((c, i) => (
                <span
                  key={c.key}
                  title={r.cells[c.key]}
                  style={{
                    fontSize: 12,
                    fontWeight: i === 0 ? 500 : 600,
                    color: pal.fg,
                    textAlign: i === 0 ? 'left' : 'right',
                    fontVariantNumeric: 'tabular-nums',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    minWidth: 0,
                  }}
                >
                  {r.cells[c.key] ?? ''}
                </span>
              ))}
            </div>
          ))}
        </div>,
      )
    }

    case 'feed': {
      const lines = s.lines ?? []
      return wrap(
        <div style={{ flex: '1 1 auto', minHeight: 0, overflowY: 'auto', padding: '1px 0 3px' }}>
          {lines.length === 0 && (
            <div style={{ fontSize: 11, color: pal.faint, padding: '2px 8px' }}>—</div>
          )}
          {lines.map((l, i) => (
            <div key={`${l.ts}-${i}`} style={{ display: 'flex', gap: 6, padding: '1px 8px', alignItems: 'baseline' }}>
              <span style={{ fontSize: 10, color: pal.faint, fontVariantNumeric: 'tabular-nums', flexShrink: 0 }}>
                {fmtFeedTime(l.ts)}
              </span>
              <span style={{ fontSize: 11, color: pal.fg, overflowWrap: 'anywhere', minWidth: 0 }}>{l.text}</span>
            </div>
          ))}
        </div>,
      )
    }

    case 'counter':
      return wrap(
        <div style={{ padding: '2px 8px 5px', fontSize: 18, fontWeight: 700, color: pal.fg, fontVariantNumeric: 'tabular-nums' }}>
          {s.count ?? 0}
        </div>,
      )

    case 'countdown': {
      const left = (s.ends_at ?? 0) - now
      const running = left > 0
      return wrap(
        <div
          style={{
            padding: '2px 8px 5px',
            fontSize: 18,
            fontWeight: 700,
            fontVariantNumeric: 'tabular-nums',
            color: !running ? pal.faint : left <= 10_000 ? URGENT : ACCENT,
          }}
        >
          {running ? fmtCountdown(left) : '—'}
        </div>,
      )
    }
  }
}

const MENU_WIDTH = 160

function RowMenuPopup({
  menu,
  onRemove,
  onClose,
}: {
  menu: RowMenu
  onRemove: () => void
  onClose: () => void
}): React.ReactElement {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const onDown = (e: MouseEvent): void => {
      if (!ref.current?.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    window.addEventListener('blur', onClose)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('blur', onClose)
    }
  }, [onClose])
  return (
    <div
      ref={ref}
      className="no-drag"
      role="menu"
      style={{
        position: 'fixed',
        top: Math.min(menu.y, window.innerHeight - 36),
        left: Math.max(4, Math.min(menu.x, window.innerWidth - MENU_WIDTH - 4)),
        width: MENU_WIDTH,
        zIndex: 1000,
        padding: 3,
        borderRadius: 6,
        backgroundColor: 'rgba(16,16,20,0.97)',
        border: '1px solid rgba(255,255,255,0.15)',
        boxShadow: '0 6px 18px rgba(0,0,0,0.5)',
      }}
    >
      <button
        role="menuitem"
        onClick={onRemove}
        style={{
          display: 'block',
          width: '100%',
          textAlign: 'left',
          padding: '5px 10px',
          fontSize: 11,
          background: 'none',
          border: 'none',
          color: 'rgba(255,255,255,0.85)',
          cursor: 'pointer',
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
        }}
      >
        Remove {menu.key}
      </button>
    </div>
  )
}
