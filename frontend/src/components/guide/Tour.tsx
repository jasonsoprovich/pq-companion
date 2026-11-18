/**
 * Tour — renders one running guided tour: a dimmed backdrop with a
 * spotlight cut out around the current step's anchor, and a popover with
 * the step text and Back / Next / Skip.
 *
 * Steps whose anchor isn't on the page are skipped in the direction of
 * travel. Esc skips the tour; ← / → move between steps.
 */
import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { X } from 'lucide-react'
import type { TourDef } from '../../lib/tours'
import { findAnchor } from './GuideProvider'

const PAD = 6
const POPOVER_W = 320
const GAP = 12

interface Rect {
  top: number
  left: number
  width: number
  height: number
}

function rectOf(el: HTMLElement): Rect {
  const r = el.getBoundingClientRect()
  return { top: r.top, left: r.left, width: r.width, height: r.height }
}

// nextPresent returns the index of the first step from `from` (moving by
// `dir`) whose anchor is on the page, or -1 if there is none.
function nextPresent(def: TourDef, from: number, dir: 1 | -1): number {
  for (let i = from; i >= 0 && i < def.steps.length; i += dir) {
    if (findAnchor(def.steps[i].anchor)) return i
  }
  return -1
}

export default function Tour({
  def,
  onEnd,
  onSkipAll,
}: {
  def: TourDef
  onEnd: () => void
  onSkipAll?: () => void
}): React.ReactElement | null {
  const [index, setIndex] = useState(() => nextPresent(def, 0, 1))
  const [rect, setRect] = useState<Rect | null>(null)
  const [popH, setPopH] = useState(140)

  const step = index >= 0 ? def.steps[index] : null
  const prevIndex = index > 0 ? nextPresent(def, index - 1, -1) : -1
  const nextIndex = index >= 0 ? nextPresent(def, index + 1, 1) : -1
  const last = nextIndex === -1

  const go = useCallback(
    (dir: 1 | -1) => {
      const to = dir === 1 ? nextIndex : prevIndex
      if (to === -1) {
        if (dir === 1) onEnd()
        return
      }
      setIndex(to)
    },
    [nextIndex, prevIndex, onEnd],
  )

  // Nothing to show at all: end straight away.
  useEffect(() => {
    if (index === -1) onEnd()
  }, [index, onEnd])

  // Scroll the anchor into view when the step changes, then track its
  // position every frame (cheap, and survives layout shifts and scrolling).
  useLayoutEffect(() => {
    if (!step) return
    const el = findAnchor(step.anchor)
    el?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    let raf = 0
    const tick = (): void => {
      const cur = findAnchor(step.anchor)
      if (cur) {
        const r = rectOf(cur)
        setRect((old) =>
          old &&
          old.top === r.top &&
          old.left === r.left &&
          old.width === r.width &&
          old.height === r.height
            ? old
            : r,
        )
      }
      raf = requestAnimationFrame(tick)
    }
    tick()
    return () => cancelAnimationFrame(raf)
  }, [step])

  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        onEnd()
      } else if (e.key === 'ArrowRight') go(1)
      else if (e.key === 'ArrowLeft') go(-1)
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [go, onEnd])

  // Re-measure the popover whenever the step (and so its text) changes.
  const popRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    if (popRef.current) setPopH(popRef.current.offsetHeight)
  }, [step, rect])

  if (!step || !rect) return null

  const vw = window.innerWidth
  const vh = window.innerHeight
  const hole = {
    top: rect.top - PAD,
    left: rect.left - PAD,
    width: rect.width + PAD * 2,
    height: rect.height + PAD * 2,
  }
  // Below the anchor if it fits, else above, else beside it on the right.
  let top = hole.top + hole.height + GAP
  let left = hole.left
  if (top + popH > vh - 8) {
    const above = hole.top - GAP - popH
    if (above >= 8) top = above
    else {
      top = Math.max(8, Math.min(vh - popH - 8, hole.top))
      left = hole.left + hole.width + GAP
      if (left + POPOVER_W > vw - 8) left = hole.left - GAP - POPOVER_W
    }
  }
  left = Math.max(8, Math.min(vw - POPOVER_W - 8, left))

  const presentSteps = def.steps.filter((s) => findAnchor(s.anchor))
  const position = presentSteps.indexOf(step) + 1

  return (
    <div className="fixed inset-0 z-[70]" role="dialog" aria-modal="true" aria-label={def.title}>
      {/* Click-blocking layer; the spotlight's shadow does the dimming. */}
      <div className="absolute inset-0" onClick={(e) => e.stopPropagation()} />
      <div
        className="pointer-events-none absolute rounded-md transition-all duration-200"
        style={{
          ...hole,
          boxShadow: '0 0 0 9999px rgba(0,0,0,0.6)',
          outline: '2px solid var(--color-primary)',
        }}
      />
      <div
        ref={popRef}
        className="absolute rounded-lg p-4 shadow-2xl"
        style={{
          top,
          left,
          width: POPOVER_W,
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
        }}
      >
        <div className="mb-1 flex items-start justify-between gap-2">
          <h3 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {step.title}
          </h3>
          <button
            onClick={onEnd}
            className="rounded p-0.5"
            style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
            title="Skip tour (Esc)"
          >
            <X size={14} />
          </button>
        </div>
        <p className="text-sm leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          {step.body}
        </p>
        <div className="mt-3 flex items-center justify-between">
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {position} of {presentSteps.length}
            {onSkipAll && (
              <>
                {' · '}
                <button
                  onClick={onSkipAll}
                  className="underline"
                  style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
                  title="Don't show any more tours. You can replay them from Help → Tours."
                >
                  Skip all tours
                </button>
              </>
            )}
          </span>
          <div className="flex gap-2">
            {prevIndex !== -1 && (
              <button
                onClick={() => go(-1)}
                className="rounded px-2.5 py-1 text-xs font-medium"
                style={{
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                  cursor: 'pointer',
                }}
              >
                Back
              </button>
            )}
            <button
              onClick={() => go(1)}
              className="rounded px-3 py-1 text-xs font-semibold"
              style={{ backgroundColor: 'var(--color-primary)', color: '#fff', cursor: 'pointer' }}
            >
              {last ? 'Done' : 'Next'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
