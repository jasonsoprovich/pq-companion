/**
 * ItemHoverCard — hover tooltip for an item id. Hovering the wrapped trigger for
 * a beat fetches the item and shows a floating card with the essentials (combat,
 * key stats, resists, effects, and equip restrictions) so a quick mouse-over
 * answers "what is this" without leaving the current view. The trigger's own
 * click behavior is untouched.
 *
 * Modeled on SpellHoverCard: wraps exactly one element child via cloneElement so
 * no wrapper DOM is added (triggers live inside tight flex/truncate layouts),
 * portals to <body> (modals clip overflow), and is pointer-events: none so it
 * can never trap the cursor. Items are cached module-level via itemCache, one
 * fetch per id.
 */
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { fetchItemCached } from '../lib/itemCache'
import {
  classesLabel,
  racesLabel,
  slotsLabel,
  effectiveItemTypeLabel,
  isLoreItem,
} from '../lib/itemHelpers'
import type { Item } from '../types/item'

const OPEN_DELAY_MS = 300
const CURSOR_OFFSET = 12
const EDGE_PAD = 8

interface ItemHoverCardProps {
  itemId: number
  /** Show a "Click to change / open" style footer. */
  clickHint?: string
  /** A single element; mouse handlers are attached to it directly. */
  children: React.ReactElement
}

export default function ItemHoverCard({
  itemId,
  clickHint,
  children,
}: ItemHoverCardProps): React.ReactElement {
  const [item, setItem] = useState<Item | null>(null)
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null)
  const timer = useRef<number | null>(null)
  const hovered = useRef(false)

  const close = (): void => {
    hovered.current = false
    if (timer.current !== null) {
      clearTimeout(timer.current)
      timer.current = null
    }
    setAnchor(null)
  }

  // Clear the pending open timer if we unmount mid-hover.
  useEffect(() => close, [])

  const open = (x: number, y: number): void => {
    if (itemId <= 0) return
    hovered.current = true
    timer.current = window.setTimeout(() => {
      timer.current = null
      fetchItemCached(itemId)
        .then((it) => {
          if (hovered.current) {
            setItem(it)
            setAnchor({ x, y })
          }
        })
        .catch(() => {}) // no card on fetch failure; click still works
    }, OPEN_DELAY_MS)
  }

  const child = React.Children.only(children)
  return (
    <>
      {React.cloneElement(child, {
        onMouseEnter: (e: React.MouseEvent) => open(e.clientX, e.clientY),
        onMouseLeave: close,
      })}
      {anchor && item && createPortal(
        <CardBox x={anchor.x} y={anchor.y}>
          <CardBody item={item} clickHint={clickHint} />
        </CardBox>,
        document.body,
      )}
    </>
  )
}

// CardBox measures itself after render, then flips to the other side of the
// cursor if it would overflow the viewport.
function CardBox({
  x, y, children,
}: {
  x: number
  y: number
  children: React.ReactNode
}): React.ReactElement {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    let left = x + CURSOR_OFFSET
    let top = y + CURSOR_OFFSET
    if (left + el.offsetWidth > window.innerWidth - EDGE_PAD) {
      left = Math.max(EDGE_PAD, x - el.offsetWidth - CURSOR_OFFSET)
    }
    if (top + el.offsetHeight > window.innerHeight - EDGE_PAD) {
      top = Math.max(EDGE_PAD, y - el.offsetHeight - CURSOR_OFFSET)
    }
    setPos({ left, top })
  }, [x, y])

  return (
    <div
      ref={ref}
      className="fixed rounded border px-3 py-2 shadow-lg"
      style={{
        left: pos?.left ?? x + CURSOR_OFFSET,
        top: pos?.top ?? y + CURSOR_OFFSET,
        visibility: pos ? 'visible' : 'hidden',
        pointerEvents: 'none',
        zIndex: 100, // above the item detail modal (z-50)
        maxWidth: 300,
        backgroundColor: 'var(--color-surface)',
        borderColor: 'var(--color-border)',
      }}
    >
      {children}
    </div>
  )
}

function CardSection({
  title, children,
}: {
  title: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <div>
      <div
        className="mb-0.5 text-[9px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      {children}
    </div>
  )
}

function CardRow({
  label, value,
}: {
  label: string
  value: string | number
}): React.ReactElement {
  return (
    <div className="flex justify-between gap-4 text-xs">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span className="text-right" style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

// Nonzero stat pairs rendered as a compact wrapped line.
function statList(item: Item): { label: string; value: number }[] {
  const pairs: [string, number][] = [
    ['STR', item.str], ['STA', item.sta], ['AGI', item.agi], ['DEX', item.dex],
    ['WIS', item.wis], ['INT', item.int], ['CHA', item.cha],
    ['HP', item.hp], ['MANA', item.mana],
  ]
  return pairs.filter(([, v]) => v !== 0).map(([label, value]) => ({ label, value }))
}

function resistList(item: Item): { label: string; value: number }[] {
  const pairs: [string, number][] = [
    ['MR', item.mr], ['CR', item.cr], ['FR', item.fr], ['DR', item.dr], ['PR', item.pr],
  ]
  return pairs.filter(([, v]) => v !== 0).map(([label, value]) => ({ label, value }))
}

function CardBody({
  item, clickHint,
}: {
  item: Item
  clickHint?: string
}): React.ReactElement {
  const isWeapon = item.damage > 0 || item.delay > 0
  const stats = statList(item)
  const resists = resistList(item)
  const effects = [
    item.click_name && `Click: ${item.click_name}`,
    item.proc_name && `Proc: ${item.proc_name}`,
    item.worn_name && `Worn: ${item.worn_name}`,
    item.focus_name && `Focus: ${item.focus_name}`,
  ].filter(Boolean) as string[]

  const fmt = (v: number): string => (v > 0 ? `+${v}` : String(v))

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <div className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>
          {item.name}
        </div>
        <div className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>
          {effectiveItemTypeLabel(item.item_class, item.item_type)}
        </div>
      </div>

      {(isLoreItem(item.lore) || item.magic === 1 || item.nodrop === 0) && (
        <div className="flex flex-wrap gap-1 text-[9px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          {isLoreItem(item.lore) && <span>Lore</span>}
          {item.magic === 1 && <span>Magic</span>}
          {item.nodrop === 0 && <span>No Drop</span>}
        </div>
      )}

      {(isWeapon || item.ac > 0) && (
        <CardSection title="Combat">
          {isWeapon && <CardRow label="Dmg / Delay" value={`${item.damage} / ${item.delay}`} />}
          {item.ac > 0 && <CardRow label="AC" value={item.ac} />}
        </CardSection>
      )}

      {stats.length > 0 && (
        <CardSection title="Stats">
          <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
            {stats.map((s) => (
              <span key={s.label}>
                <span style={{ color: 'var(--color-muted-foreground)' }}>{s.label} </span>{fmt(s.value)}
              </span>
            ))}
          </div>
        </CardSection>
      )}

      {resists.length > 0 && (
        <CardSection title="Resists">
          <div className="flex flex-wrap gap-x-2 gap-y-0.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
            {resists.map((s) => (
              <span key={s.label}>
                <span style={{ color: 'var(--color-muted-foreground)' }}>{s.label} </span>{fmt(s.value)}
              </span>
            ))}
          </div>
        </CardSection>
      )}

      {effects.length > 0 && (
        <CardSection title="Effects">
          {effects.map((e, i) => (
            <div key={i} className="text-xs" style={{ color: 'var(--color-foreground)' }}>{e}</div>
          ))}
        </CardSection>
      )}

      <CardSection title="Restrictions">
        <CardRow label="Slot" value={slotsLabel(item.slots)} />
        <CardRow label="Class" value={classesLabel(item.classes)} />
        <CardRow label="Race" value={racesLabel(item.races)} />
        {item.req_level > 0 && <CardRow label="Req Level" value={item.req_level} />}
      </CardSection>

      {clickHint && (
        <div className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
          {clickHint}
        </div>
      )}
    </div>
  )
}
