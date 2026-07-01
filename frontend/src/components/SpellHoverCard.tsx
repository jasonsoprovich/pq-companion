/**
 * SpellHoverCard — hover tooltip for item effect links (click / proc / worn /
 * focus). Hovering the wrapped trigger for a beat fetches the spell and shows
 * a floating card with the same CASTING / TARGETING / EFFECTS info (in the
 * same order) as the Spells explorer, so a quick mouse-over answers "what
 * does this do" without leaving the item. The trigger's own click behavior
 * (navigate to the spell) is untouched.
 *
 * No CLASSES section on purpose: proc/worn spells commonly read "NPC Only"
 * there, which is noise in this context. `effectsOnly` (worn/focus effects)
 * drops CASTING and TARGETING too — cast time and targeting are meaningless
 * for an always-on worn effect.
 *
 * Wraps exactly one element child via cloneElement so no wrapper DOM is
 * added (triggers live inside tight flex/truncate layouts). The card portals
 * to <body> (item detail modal clips overflow) and is pointer-events: none
 * so it can never trap the cursor. Spells are cached module-level for the
 * session, one fetch per spell id.
 */
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { getSpell } from '../services/api'
import { loadEnums } from '../lib/enumsCache'
import {
  durationLabel,
  durationScales,
  effectDescription,
  msLabel,
  resistLabel,
  skillLabel,
  targetLabel,
} from '../lib/spellHelpers'
import { usePoPEnabled } from '../hooks/usePoPEnabled'
import { maxLevel as eraMaxLevel } from '../lib/era'
import type { Spell } from '../types/spell'

const OPEN_DELAY_MS = 300
const CURSOR_OFFSET = 12
const EDGE_PAD = 8

const spellCache = new Map<number, Spell>()
const inFlight = new Map<number, Promise<Spell>>()

function fetchSpellCached(id: number): Promise<Spell> {
  const hit = spellCache.get(id)
  if (hit) return Promise.resolve(hit)
  let p = inFlight.get(id)
  if (!p) {
    // Enum labels (target/resist/skill) read a module-level catalog loaded at
    // app boot; await it alongside the spell so a cold hover never renders
    // "Unknown (n)" placeholders.
    p = Promise.all([getSpell(id), loadEnums()])
      .then(([s]) => {
        spellCache.set(id, s)
        inFlight.delete(id)
        return s
      })
      .catch((err) => {
        inFlight.delete(id)
        throw err
      })
    inFlight.set(id, p)
  }
  return p
}

interface SpellHoverCardProps {
  spellId: number
  /** Worn/focus effects: show only the EFFECTS section. */
  effectsOnly?: boolean
  /** A single element; mouse handlers are attached to it directly. */
  children: React.ReactElement
}

export default function SpellHoverCard({
  spellId,
  effectsOnly,
  children,
}: SpellHoverCardProps): React.ReactElement {
  const [spell, setSpell] = useState<Spell | null>(null)
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
    hovered.current = true
    timer.current = window.setTimeout(() => {
      timer.current = null
      fetchSpellCached(spellId)
        .then((s) => {
          if (hovered.current) {
            setSpell(s)
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
      {anchor && spell && createPortal(
        <CardBox x={anchor.x} y={anchor.y}>
          <CardBody spell={spell} effectsOnly={effectsOnly} />
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
        maxWidth: 320,
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

function CardBody({
  spell, effectsOnly,
}: {
  spell: Spell
  effectsOnly?: boolean
}): React.ReactElement {
  const levelCap = eraMaxLevel(usePoPEnabled())

  // Same effect-slot walk as the Spells explorer detail panel.
  const effects = spell.effect_ids
    .map((id, i) => effectDescription(
      id,
      spell.effect_base_values[i] ?? 0,
      spell.buff_duration,
      spell.effect_max_values[i] ?? 0,
      spell.effect_formulas?.[i] ?? 0,
      spell.class_levels,
      levelCap,
    ))
    .filter((d) => d !== '')

  return (
    <div className="flex flex-col gap-1.5">
      <div className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>
        {spell.name}
      </div>
      {!effectsOnly && (
        <>
          <CardSection title="Casting">
            {skillLabel(spell.skill) && <CardRow label="Skill" value={skillLabel(spell.skill)} />}
            <CardRow label="Mana Cost" value={spell.mana > 0 ? spell.mana : 'None'} />
            <CardRow label="Cast Time" value={msLabel(spell.cast_time)} />
            {spell.recast_time > 0 && <CardRow label="Recast Time" value={msLabel(spell.recast_time)} />}
            {spell.buff_duration > 0 && (
              <CardRow
                label={durationScales(spell.buff_duration_formula, spell.buff_duration) ? 'Max Duration' : 'Duration'}
                value={durationLabel(spell.buff_duration_formula, spell.buff_duration)}
              />
            )}
          </CardSection>
          <CardSection title="Targeting">
            <CardRow label="Target" value={targetLabel(spell.target_type)} />
            <CardRow label="Resist" value={resistLabel(spell.resist_type)} />
            {spell.range > 0 && <CardRow label="Range" value={`${spell.range} units`} />}
            {spell.aoe_range > 0 && <CardRow label="AoE Range" value={`${spell.aoe_range} units`} />}
          </CardSection>
        </>
      )}
      {effects.length > 0 && (
        <CardSection title="Effects">
          {effects.map((d, i) => (
            <div key={i} className="text-xs" style={{ color: 'var(--color-foreground)' }}>
              {d}
            </div>
          ))}
        </CardSection>
      )}
      <div className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
        Click to open in the spell explorer
      </div>
    </div>
  )
}
