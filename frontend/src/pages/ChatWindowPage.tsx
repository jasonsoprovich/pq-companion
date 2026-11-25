/**
 * ChatWindowPage — transparent always-on-top popout for one Chat Window
 * (route chat-window-window?id=<windowId>&name=<name>). Main opens one window
 * per chat window, keyed `chatWindow:<id>`.
 *
 * Lock mode and fade are one shared 'chatWindow' setting; opacity and font
 * size live on each window's definition, since each window has its own job
 * (a big raid-callout window, a small tells window). Ctrl+scroll changes the
 * font size and saves it to the window.
 *
 * Right-click a line for Copy, Hide lines like this and Keep lines like this;
 * the last two open the editor in the main window with the suggested rule.
 *
 * In the tabs layout a tab row sits below the title bar (outside it, since
 * the title bar fades and is hidden in display-only mode); see useChatTabs.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { MessagesSquare, Pencil } from 'lucide-react'
import ChatLinesView from '../components/overlays/ChatLinesView'
import ChatTabStrip from '../components/overlays/ChatTabStrip'
import { useChatWindows } from '../hooks/useChatWindows'
import { useChatTabs } from '../hooks/useChatTabs'
import { useNowMs } from '../hooks/useCustomOverlays'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { CHAT_WINDOW_COLOR } from '../lib/chatWindow'
import { dismissChatPin } from '../services/api'
import type { ChatLine } from '../types/chatWindow'

const FONT_MIN = 0.5
const FONT_MAX = 3
const FONT_STEP = 0.1
const FONT_SAVE_MS = 500
// Pins expire server-side too; this tick only hides them on time.
const PIN_TICK_MS = 5000

interface LineMenu {
  line: ChatLine
  x: number
  y: number
  selection: string
}

export default function ChatWindowPage(): React.ReactElement {
  const [searchParams] = useSearchParams()
  const id = searchParams.get('id') ?? ''
  const urlName = searchParams.get('name') ?? ''

  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock('chatWindow')
  const chromeFade = useOverlayChromeFade('chatWindow', mode === 'display-only')
  const { collapsed, toggleCollapsed, headerRef } = useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()

  // Appends go to this window's tabs for unread counts.
  const appendRef = useRef<(lines: ChatLine[], replay: boolean) => void>()
  const cw = useChatWindows((wid, lines, replay) => {
    if (wid === id) appendRef.current?.(lines, replay)
  })
  const win = cw.windows.find((w) => w.def.id === id)
  const now = useNowMs((win?.state.pins.length ?? 0) > 0, cw.skewMs, PIN_TICK_MS)

  // Ctrl+scroll font size: shown at once, saved on a short debounce.
  const [fontOverride, setFontOverride] = useState<number | null>(null)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const winRef = useRef(win)
  winRef.current = win
  const fontScale = fontOverride ?? win?.def.appearance.font_scale ?? 1
  const fontRef = useRef(fontScale)
  fontRef.current = fontScale

  const onWheel = useCallback((e: WheelEvent) => {
    if (!e.ctrlKey || e.deltaY === 0 || !winRef.current) return
    e.preventDefault()
    const next = Math.round(Math.max(FONT_MIN, Math.min(FONT_MAX, fontRef.current + (e.deltaY < 0 ? FONT_STEP : -FONT_STEP))) * 10) / 10
    setFontOverride(next)
    if (saveTimer.current) clearTimeout(saveTimer.current)
    saveTimer.current = setTimeout(() => {
      saveTimer.current = null
      const w = winRef.current
      if (!w) return
      void cw.act(w.def.id, { action: 'appearance', appearance: { ...w.def.appearance, font_scale: next } })
    }, FONT_SAVE_MS)
  }, [cw])

  useEffect(() => {
    window.addEventListener('wheel', onWheel, { passive: false })
    return () => window.removeEventListener('wheel', onWheel)
  }, [onWheel])

  // Drop the local override once the saved value comes back.
  useEffect(() => {
    if (fontOverride !== null && !saveTimer.current && win?.def.appearance.font_scale === fontOverride) {
      setFontOverride(null)
    }
  }, [win?.def.appearance.font_scale, fontOverride])

  const [menu, setMenu] = useState<LineMenu | null>(null)
  useEffect(() => {
    if (!menu) return
    const close = (): void => setMenu(null)
    window.addEventListener('mousedown', close)
    window.addEventListener('blur', close)
    return () => {
      window.removeEventListener('mousedown', close)
      window.removeEventListener('blur', close)
    }
  }, [menu])

  const interactive = rowsInteractive || !locked
  const opacity = win?.def.appearance.opacity ?? 0.85

  const openEditor = (extra = ''): void => {
    void window.electron?.app?.navigateMain?.(`/overlays?panel=chat:${encodeURIComponent(id)}${extra}`)
  }

  const menuItem = (label: string, run: () => void): React.ReactNode => (
    <button
      onMouseDown={(e) => e.stopPropagation()}
      onClick={() => { run(); setMenu(null) }}
      style={{
        display: 'block',
        width: '100%',
        textAlign: 'left',
        padding: '4px 10px',
        border: 'none',
        background: 'transparent',
        color: 'rgba(255,255,255,0.85)',
        fontSize: 11,
        cursor: 'pointer',
        whiteSpace: 'nowrap',
      }}
      onMouseEnter={(e) => { e.currentTarget.style.backgroundColor = 'rgba(255,255,255,0.08)' }}
      onMouseLeave={(e) => { e.currentTarget.style.backgroundColor = 'transparent' }}
    >
      {label}
    </button>
  )

  const headerBtn: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    padding: '1px 5px',
    borderRadius: 3,
    border: '1px solid rgba(255,255,255,0.1)',
    backgroundColor: 'transparent',
    color: 'rgba(255,255,255,0.4)',
    cursor: 'pointer',
    lineHeight: 1,
    fontSize: 11,
  }

  const def = win ? { ...win.def, appearance: { ...win.def.appearance, font_scale: fontScale } } : null
  const tabs = useChatTabs({
    def: win?.def ?? null,
    lines: win?.state.lines ?? [],
    replay: !!win?.state.replay,
    persistKey: id || null,
  })
  appendRef.current = tabs.onAppend
  const tabFontPx = Math.round(Math.max(10, Math.min(16, 11 * fontScale)))
  const pinCount = win?.state.pins.filter((p) => p.until > now).length ?? 0

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100%',
        height: '100%',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        border: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
        transition: 'background-color 0.4s ease, border-color 0.4s ease',
        borderRadius: 8,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        color: 'rgba(255,255,255,0.9)',
      }}
    >
      <div
        ref={headerRef}
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 6,
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0 }}>
          <MessagesSquare size={11} style={{ color: CHAT_WINDOW_COLOR, flexShrink: 0 }} />
          <span
            style={{
              fontSize: 11,
              fontWeight: 700,
              color: 'rgba(255,255,255,0.8)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {win?.def.name || urlName || 'Chat window'}
          </span>
          {win && !win.def.enabled && (
            <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.4)' }}>paused</span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
          <CollapseToggle
            collapsed={collapsed}
            onToggle={toggleCollapsed}
            summary={collapsed && pinCount ? `${pinCount} pinned` : null}
          />
          {win && (
            <button onClick={() => openEditor()} style={headerBtn} title="Edit this chat window">
              <Pencil size={10} />
            </button>
          )}
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeChatWindow(id)}
            style={headerBtn}
            title="Close window"
          >
            ×
          </button>
        </div>
      </div>

      {!collapsed && (win && def ? (
        <ChatLinesView
          def={def}
          lines={tabs.lines}
          pins={win.state.pins}
          nowMs={now}
          interactive={interactive}
          viewKey={tabs.active}
          between={
            tabs.enabled && (
              <ChatTabStrip
                tabs={tabs.tabs}
                active={tabs.active}
                unread={tabs.unread}
                onSelect={tabs.select}
                onClose={tabs.close}
                onCloseAllRead={tabs.closeAllRead}
                fontPx={tabFontPx}
                interactive={mode !== 'display-only' || !locked}
                interactionProps={headerInteractionProps}
              />
            )
          }
          onDismissPin={(seq) => void dismissChatPin(win.def.id, seq).catch(() => {})}
          onLineMenu={(line, x, y) =>
            setMenu({ line, x, y, selection: window.getSelection()?.toString() ?? '' })
          }
          emptyText={chrome ? 'Nothing yet. Matching lines show up here.' : undefined}
        />
      ) : (
        cw.loaded && chrome && (
          <div style={{ padding: '14px 10px', fontSize: 11, color: 'rgba(255,255,255,0.4)', textAlign: 'center' }}>
            This chat window no longer exists.
          </div>
        )
      ))}

      {menu && (
        <div
          onMouseDown={(e) => e.stopPropagation()}
          style={{
            position: 'fixed',
            left: Math.min(menu.x, window.innerWidth - 170),
            top: Math.min(menu.y, window.innerHeight - 90),
            zIndex: 10,
            minWidth: 160,
            padding: '3px 0',
            borderRadius: 4,
            border: '1px solid rgba(255,255,255,0.15)',
            backgroundColor: 'rgba(24,24,28,0.97)',
            boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
          }}
        >
          {menuItem(menu.selection ? 'Copy selection' : 'Copy line', () => {
            void navigator.clipboard?.writeText(menu.selection || menu.line.text)
          })}
          {menuItem('Hide lines like this…', () => openEditor(`&suggest=${menu.line.seq}&mode=hide`))}
          {menuItem('Keep lines like this…', () => openEditor(`&suggest=${menu.line.seq}&mode=keep`))}
        </div>
      )}
    </div>
  )
}
