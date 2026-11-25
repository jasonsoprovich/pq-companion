/**
 * ChatTabStrip — the tab row of a chat window in the tabs layout.
 *
 * A tab with unread lines fills with its channel color, pulses a few times
 * per new line, and shows a count. Tell conversation tabs have an × (and
 * close on middle-click). The row never wraps: tabs that don't fit go
 * into a "+N" menu at the end, which takes the color of the first unread
 * tab inside it and pulses too, so hidden tabs still notify. The active
 * tab is always kept visible.
 */
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { X } from 'lucide-react'
import type { ChatTab } from '../../lib/chatTabs'
import { isConvTab } from '../../lib/chatTabs'

const OVERFLOW_W = 44
const GAP = 3

function TabBody({
  tab,
  count,
  active,
  fontPx,
  onClose,
}: {
  tab: ChatTab
  count: number
  active: boolean
  fontPx: number
  onClose?: () => void
}): React.ReactElement {
  return (
    <>
      {count > 0 && (
        // Keyed by count so each new line restarts the pulse.
        <span
          key={count}
          aria-hidden
          style={{
            position: 'absolute',
            inset: 0,
            borderRadius: 3,
            backgroundColor: tab.color,
            animation: 'chat-tab-pulse 0.7s ease-in-out 3',
            opacity: 0,
            pointerEvents: 'none',
          }}
        />
      )}
      <span
        style={{
          position: 'relative',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          color: active || count > 0 ? '#fff' : tab.color,
          fontWeight: active || count > 0 ? 700 : 500,
          fontSize: fontPx,
        }}
      >
        {tab.label}
      </span>
      {count > 0 && (
        <span
          style={{
            position: 'relative',
            flexShrink: 0,
            fontSize: Math.max(9, fontPx - 2),
            fontWeight: 700,
            padding: '0 4px',
            borderRadius: 6,
            backgroundColor: 'rgba(0,0,0,0.35)',
            color: '#fff',
          }}
        >
          {count > 99 ? '99+' : count}
        </span>
      )}
      {onClose && (
        <span
          role="button"
          aria-label={`Close ${tab.label}`}
          title={`Close ${tab.label} (lines stay in Tells)`}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => { e.stopPropagation(); onClose() }}
          style={{ position: 'relative', flexShrink: 0, display: 'flex', color: 'rgba(255,255,255,0.55)', cursor: 'pointer' }}
        >
          <X size={Math.max(9, fontPx - 1)} />
        </span>
      )}
    </>
  )
}

function tabStyle(tab: ChatTab, count: number, active: boolean): React.CSSProperties {
  return {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    gap: 4,
    minWidth: 40,
    maxWidth: 140,
    flexShrink: 0,
    padding: '2px 7px',
    borderRadius: 3,
    border: `1px solid ${active || count > 0 ? tab.color : 'rgba(255,255,255,0.1)'}`,
    backgroundColor: count > 0 ? `${tab.color}55` : active ? `${tab.color}26` : 'transparent',
    cursor: 'pointer',
    overflow: 'hidden',
  }
}

export default function ChatTabStrip({
  tabs,
  active,
  unread,
  onSelect,
  onClose,
  onCloseAllRead,
  fontPx,
  interactive = true,
  interactionProps,
}: {
  tabs: ChatTab[]
  active: string
  unread: (id: string) => number
  onSelect: (id: string) => void
  onClose: (id: string) => void
  onCloseAllRead: () => void
  fontPx: number
  interactive?: boolean
  // Hover handlers that make a locked click-through window interactive.
  interactionProps?: { onMouseEnter?: React.MouseEventHandler; onMouseLeave?: React.MouseEventHandler }
}): React.ReactElement | null {
  const rowRef = useRef<HTMLDivElement>(null)
  const measureRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  const [visibleIds, setVisibleIds] = useState<string[]>(() => tabs.map((t) => t.id))
  const [menuOpen, setMenuOpen] = useState(false)

  useEffect(() => {
    const el = rowRef.current
    if (!el) return
    const ro = new ResizeObserver(() => setWidth(el.clientWidth))
    ro.observe(el)
    setWidth(el.clientWidth)
    return () => ro.disconnect()
  }, [])

  const countsKey = tabs.map((t) => `${t.id}:${t.label}:${unread(t.id)}`).join('|')

  // Measure every tab off screen, then fit as many as the row holds.
  useLayoutEffect(() => {
    const m = measureRef.current
    if (!m || width === 0) return
    const widths = Array.from(m.children).map((c) => (c as HTMLElement).offsetWidth)
    const total = widths.reduce((a, w) => a + w + GAP, 0)
    if (total <= width) {
      setVisibleIds(tabs.map((t) => t.id))
      return
    }
    const room = width - OVERFLOW_W
    const fit: number[] = []
    let used = 0
    for (let i = 0; i < tabs.length; i++) {
      if (used + widths[i] + GAP > room) break
      fit.push(i)
      used += widths[i] + GAP
    }
    const ai = tabs.findIndex((t) => t.id === active)
    if (ai >= 0 && !fit.includes(ai)) {
      while (fit.length && used + widths[ai] + GAP > room) {
        const drop = fit.pop() as number
        used -= widths[drop] + GAP
      }
      fit.push(ai)
    }
    setVisibleIds(fit.map((i) => tabs[i].id))
  }, [width, countsKey, active]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!menuOpen) return
    const close = (): void => setMenuOpen(false)
    window.addEventListener('mousedown', close)
    window.addEventListener('blur', close)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('blur', close)
    }
  }, [menuOpen])

  if (tabs.length === 0) return null

  const visible = tabs.filter((t) => visibleIds.includes(t.id))
  const hidden = tabs.filter((t) => !visibleIds.includes(t.id))
  const hiddenUnread = hidden.filter((t) => unread(t.id) > 0)
  const hiddenCount = hiddenUnread.reduce((a, t) => a + unread(t.id), 0)
  const overflowColor = hiddenUnread[0]?.color
  const anyRead = tabs.some((t) => isConvTab(t.id) && t.id !== active && unread(t.id) === 0)

  const renderTab = (t: ChatTab): React.ReactNode => {
    const count = t.id === active ? 0 : unread(t.id)
    const conv = isConvTab(t.id)
    return (
      <button
        key={t.id}
        onClick={() => onSelect(t.id)}
        onAuxClick={(e) => { if (e.button === 1 && conv) { e.preventDefault(); onClose(t.id) } }}
        title={t.label}
        data-chat-tab={t.id}
        style={tabStyle(t, count, t.id === active)}
      >
        <TabBody tab={t} count={count} active={t.id === active} fontPx={fontPx} onClose={conv ? () => onClose(t.id) : undefined} />
      </button>
    )
  }

  return (
    <div
      {...interactionProps}
      data-chat-tabs
      className="no-drag"
      style={{
        position: 'relative',
        flexShrink: 0,
        padding: '3px 5px',
        borderBottom: '1px solid rgba(255,255,255,0.1)',
        pointerEvents: interactive ? 'auto' : 'none',
      }}
    >
      {/* Off-screen copy of every tab, for measuring. */}
      <div
        ref={measureRef}
        aria-hidden
        style={{ position: 'absolute', visibility: 'hidden', pointerEvents: 'none', display: 'flex', gap: GAP, top: 0, left: 0 }}
      >
        {tabs.map((t) => (
          <div key={t.id} style={tabStyle(t, t.id === active ? 0 : unread(t.id), t.id === active)}>
            <TabBody
              tab={t}
              count={t.id === active ? 0 : unread(t.id)}
              active={t.id === active}
              fontPx={fontPx}
              onClose={isConvTab(t.id) ? () => undefined : undefined}
            />
          </div>
        ))}
      </div>
      <div ref={rowRef} style={{ display: 'flex', gap: GAP, alignItems: 'center', overflow: 'hidden' }}>
        {visible.map(renderTab)}
        {hidden.length > 0 && (
          <button
            onMouseDown={(e) => e.stopPropagation()}
            onClick={() => setMenuOpen((o) => !o)}
            title={`${hidden.length} more ${hidden.length === 1 ? 'tab' : 'tabs'}`}
            style={{
              ...tabStyle(
                { id: '+', label: '', color: overflowColor ?? 'rgba(255,255,255,0.4)' },
                hiddenCount,
                false,
              ),
              minWidth: OVERFLOW_W - GAP,
              justifyContent: 'center',
              marginLeft: 'auto',
            }}
          >
            {hiddenCount > 0 && overflowColor && (
              <span
                key={hiddenCount}
                aria-hidden
                style={{
                  position: 'absolute',
                  inset: 0,
                  backgroundColor: overflowColor,
                  animation: 'chat-tab-pulse 0.7s ease-in-out 3',
                  opacity: 0,
                  pointerEvents: 'none',
                }}
              />
            )}
            <span style={{ position: 'relative', fontSize: fontPx, fontWeight: 700, color: '#fff' }}>
              +{hidden.length}
            </span>
          </button>
        )}
      </div>
      {menuOpen && (
        <div
          onMouseDown={(e) => e.stopPropagation()}
          style={{
            position: 'absolute',
            right: 5,
            top: '100%',
            zIndex: 20,
            minWidth: 170,
            maxWidth: 260,
            maxHeight: '60vh',
            overflowY: 'auto',
            padding: '3px 0',
            borderRadius: 4,
            border: '1px solid rgba(255,255,255,0.15)',
            backgroundColor: 'rgba(24,24,28,0.97)',
            boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
          }}
        >
          {[...hiddenUnread, ...hidden.filter((t) => unread(t.id) === 0)].map((t) => {
            const count = unread(t.id)
            return (
              <div
                key={t.id}
                onClick={() => { onSelect(t.id); setMenuOpen(false) }}
                onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = 'rgba(255,255,255,0.08)' }}
                onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent' }}
                style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '4px 10px', cursor: 'pointer', fontSize: 11 }}
              >
                <span style={{ width: 7, height: 7, borderRadius: 4, flexShrink: 0, backgroundColor: t.color }} />
                <span
                  style={{
                    flex: 1,
                    minWidth: 0,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    color: count ? '#fff' : 'rgba(255,255,255,0.75)',
                    fontWeight: count ? 700 : 400,
                  }}
                >
                  {t.label}
                </span>
                {count > 0 && <span style={{ color: t.color, fontWeight: 700 }}>{count}</span>}
                {isConvTab(t.id) && (
                  <span
                    role="button"
                    aria-label={`Close ${t.label}`}
                    onClick={(e) => { e.stopPropagation(); onClose(t.id) }}
                    style={{ display: 'flex', color: 'rgba(255,255,255,0.5)' }}
                  >
                    <X size={11} />
                  </span>
                )}
              </div>
            )
          })}
          {anyRead && (
            <div
              onClick={() => { onCloseAllRead(); setMenuOpen(false) }}
              onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = 'rgba(255,255,255,0.08)' }}
              onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent' }}
              style={{
                padding: '5px 10px',
                marginTop: 3,
                borderTop: '1px solid rgba(255,255,255,0.1)',
                cursor: 'pointer',
                fontSize: 11,
                color: 'rgba(255,255,255,0.65)',
              }}
            >
              Close all read conversations
            </div>
          )}
        </div>
      )}
    </div>
  )
}
