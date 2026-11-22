/**
 * ChatLinesView — a chat window's pinned strip and scrollback, shared by the
 * popout window and the editor's preview.
 *
 * Follows the newest line while scrolled to the bottom; scrolling up pauses
 * that and shows a "jump to newest" button. Text is selectable. Right-click
 * a line (when onLineMenu is given) for its menu.
 */
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ArrowDown, Pin, X } from 'lucide-react'
import type { ChatLine, ChatPin, ChatWindowDef } from '../../types/chatWindow'
import { channelTag, formatTime, lineColor } from '../../lib/chatWindow'

const BOTTOM_SLACK_PX = 24
const BASE_FONT_PX = 12

export default function ChatLinesView({
  def,
  lines,
  pins,
  nowMs,
  onDismissPin,
  onLineMenu,
  emptyText,
  interactive = true,
}: {
  def: ChatWindowDef
  lines: ChatLine[]
  pins: ChatPin[]
  nowMs: number
  onDismissPin?: (seq: number) => void
  onLineMenu?: (line: ChatLine, x: number, y: number) => void
  emptyText?: string
  // False while the window passes clicks through (no selection, no menu).
  interactive?: boolean
}): React.ReactElement {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [follow, setFollow] = useState(true)
  const followRef = useRef(true)
  followRef.current = follow

  const fontPx = BASE_FONT_PX * (def.appearance.font_scale || 1)
  const livePins = pins.filter((p) => p.until > nowMs)
  const last = lines.length ? lines[lines.length - 1].seq : 0

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el && followRef.current) el.scrollTop = el.scrollHeight
  }, [last, lines.length, fontPx])

  // A resize (or the strip growing) shouldn't drop us off the bottom.
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const ro = new ResizeObserver(() => {
      if (followRef.current) el.scrollTop = el.scrollHeight
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const onScroll = (): void => {
    const el = scrollRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_SLACK_PX
    if (atBottom !== followRef.current) setFollow(atBottom)
  }

  const jump = (): void => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
    setFollow(true)
  }

  const renderLine = (l: ChatLine, pinned: boolean): React.ReactNode => {
    const color = lineColor(def, l)
    const tag = def.appearance.show_channel ? channelTag(l.channel) : ''
    return (
      <>
        {def.appearance.show_timestamps && (
          <span style={{ color: 'rgba(255,255,255,0.35)', marginRight: 5, fontVariantNumeric: 'tabular-nums' }}>
            {formatTime(l.ts)}
          </span>
        )}
        {tag && (
          <span style={{ color, opacity: 0.7, marginRight: 5, fontWeight: 600 }}>[{tag}]</span>
        )}
        <span style={{ color, fontWeight: l.color || pinned ? 600 : undefined }}>{l.text}</span>
      </>
    )
  }

  const menu = (l: ChatLine) =>
    onLineMenu && interactive
      ? (e: React.MouseEvent): void => {
          e.preventDefault()
          onLineMenu(l, e.clientX, e.clientY)
        }
      : undefined

  return (
    <div
      style={{
        position: 'relative',
        flex: 1,
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        fontSize: fontPx,
        lineHeight: 1.35,
        userSelect: interactive ? 'text' : 'none',
      }}
    >
      {livePins.length > 0 && (
        <div
          style={{
            flexShrink: 0,
            maxHeight: '45%',
            overflowY: 'auto',
            borderBottom: '1px solid rgba(255,255,255,0.12)',
            backgroundColor: 'rgba(250,204,21,0.07)',
            padding: '3px 6px',
          }}
        >
          {livePins.map((p) => (
            <div
              key={p.seq}
              onContextMenu={menu(p.line)}
              style={{ display: 'flex', alignItems: 'flex-start', gap: 4, padding: '1px 0' }}
            >
              <Pin size={fontPx * 0.8} style={{ color: lineColor(def, p.line), flexShrink: 0, marginTop: fontPx * 0.25 }} />
              <div style={{ flex: 1, minWidth: 0, overflowWrap: 'anywhere' }}>{renderLine(p.line, true)}</div>
              {onDismissPin && interactive && (
                <button
                  onClick={() => onDismissPin(p.seq)}
                  title="Unpin"
                  aria-label="Unpin"
                  style={{
                    flexShrink: 0,
                    border: 'none',
                    background: 'transparent',
                    color: 'rgba(255,255,255,0.4)',
                    cursor: 'pointer',
                    padding: 0,
                    lineHeight: 1,
                  }}
                >
                  <X size={fontPx * 0.9} />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        style={{
          flex: 1,
          minHeight: 0,
          overflowY: interactive ? 'auto' : 'hidden',
          padding: '3px 6px',
        }}
      >
        {lines.length === 0 && emptyText && (
          <div style={{ padding: '10px 4px', fontSize: 11, color: 'rgba(255,255,255,0.4)', textAlign: 'center' }}>
            {emptyText}
          </div>
        )}
        {lines.map((l) => (
          <div
            key={l.seq}
            onContextMenu={menu(l)}
            style={{
              padding: '1px 0',
              overflowWrap: 'anywhere',
              backgroundColor: l.color ? `${l.color}14` : undefined,
              borderLeft: l.color ? `2px solid ${l.color}` : '2px solid transparent',
              paddingLeft: 4,
            }}
          >
            {renderLine(l, false)}
          </div>
        ))}
      </div>
      {!follow && (
        <button
          onClick={jump}
          style={{
            position: 'absolute',
            right: 8,
            bottom: 8,
            display: 'flex',
            alignItems: 'center',
            gap: 3,
            padding: '2px 8px',
            borderRadius: 10,
            border: '1px solid rgba(255,255,255,0.2)',
            backgroundColor: 'rgba(20,20,24,0.9)',
            color: 'rgba(255,255,255,0.8)',
            fontSize: 10,
            cursor: 'pointer',
          }}
        >
          <ArrowDown size={10} /> Newest
        </button>
      )}
    </div>
  )
}
