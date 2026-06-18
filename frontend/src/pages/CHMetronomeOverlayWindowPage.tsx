/**
 * CHMetronomeOverlayWindowPage — a personal Complete-Heal-chain timing helper
 * for a single cleric. Renders in a dedicated frameless Electron window.
 *
 * How it works: a CH cast takes 10s and a callout fires at cast-start, so when
 * the cleric *ahead of you* in the rotation calls out, you have a 10s window
 * before their heal lands. To make your heal land `delay` seconds after theirs
 * you start casting `delay` seconds after their callout (your 10s cast then
 * lands `delay` after their 10s cast). This window watches the ch_chain timer
 * feed for the position immediately before yours and counts down to your cast
 * moment, flashing CAST NOW when it's time to press your heal key.
 *
 * Config (your position, the chain size, and your desired delay) lives inline
 * and persists to localStorage so it survives reopening the window. When the
 * secondary (ramp/split) chain is enabled in settings, a Main/Ramp switch
 * picks which chain's timer feed (ch_chain vs ch_chain_2) the metronome
 * follows.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Gauge, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { useCHChainConfig } from '../hooks/useCHChainConfig'
import OverlayLockButton from '../components/OverlayLockButton'
import { getTimerState } from '../services/api'
import {
  CH_CAST,
  type ChainView,
  computeAnchorMs,
  watchPosition,
} from '../lib/chMetronome'
import type { ActiveTimer, TimerState } from '../types/timer'

// How long to hold the CAST NOW flash after the cast moment passes.
const PULSE_SECS = 1.5
// Keep using the last anchor for a little past the 10s cast so the flash and
// post-cast state still render even after the source timer expires.
const ANCHOR_GRACE_SECS = 3

type Cfg = { position: number; chainSize: number; delay: number }

const DEFAULT_CFG: Cfg = { position: 2, chainSize: 3, delay: 4 }

// ChainView ('main' | 'ramp') is shared via lib/chMetronome. The selector is
// only shown when the secondary chain is enabled in settings.
const CHAIN_STORAGE_KEY = 'chMetronome:chain'

function loadChain(): ChainView {
  return localStorage.getItem(CHAIN_STORAGE_KEY) === 'ramp' ? 'ramp' : 'main'
}

// posLabel renders a chain position for display: ramp chains call letters
// (AAA = 1 → "A"), the main chain calls numbers ("#1").
function posLabel(position: number, chain: ChainView): string {
  if (chain === 'ramp' && position >= 1 && position <= 26) {
    return String.fromCharCode(64 + position)
  }
  return `#${position}`
}

function loadCfg(): Cfg {
  // parseFloat (not parseInt): delay supports half-second values like 4.5.
  const read = (k: string, d: number): number => {
    const raw = localStorage.getItem(`chMetronome:${k}`)
    const n = raw == null ? NaN : parseFloat(raw)
    return Number.isFinite(n) ? n : d
  }
  return {
    position: read('position', DEFAULT_CFG.position),
    chainSize: read('chainSize', DEFAULT_CFG.chainSize),
    delay: read('delay', DEFAULT_CFG.delay),
  }
}

function saveCfg(c: Cfg): void {
  localStorage.setItem('chMetronome:position', String(c.position))
  localStorage.setItem('chMetronome:chainSize', String(c.chainSize))
  localStorage.setItem('chMetronome:delay', String(c.delay))
}

// Stepper is a small "− value +" control used for the three config fields.
// step defaults to 1; the delay stepper passes 0.5 for finer chain spacing.
function Stepper(props: {
  label: string
  value: number
  min: number
  max: number
  onChange: (v: number) => void
  suffix?: string
  step?: number
}): React.ReactElement {
  const { label, value, min, max, onChange, suffix, step = 1 } = props
  const clamp = (v: number): number => Math.max(min, Math.min(max, v))
  const btn: React.CSSProperties = {
    width: 18,
    height: 18,
    lineHeight: '16px',
    textAlign: 'center',
    borderRadius: 3,
    border: '1px solid rgba(255,255,255,0.15)',
    backgroundColor: 'rgba(255,255,255,0.06)',
    color: 'rgba(255,255,255,0.8)',
    cursor: 'pointer',
    fontSize: 12,
    padding: 0,
  }
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
      <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: 0.4 }}>
        {label}
      </span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
        <button style={btn} onClick={() => onChange(clamp(value - step))} title={`Decrease ${label}`}>
          −
        </button>
        <span
          style={{
            minWidth: 22,
            textAlign: 'center',
            fontSize: 12,
            fontWeight: 700,
            color: 'rgba(255,255,255,0.9)',
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {value}
          {suffix ?? ''}
        </span>
        <button style={btn} onClick={() => onChange(clamp(value + step))} title={`Increase ${label}`}>
          +
        </button>
      </div>
    </div>
  )
}

// ChainSwitch is a compact Main/Secondary segmented control matching the Stepper
// layout (label above control), shown only when the secondary chain exists.
function ChainSwitch({ chain, onChange }: { chain: ChainView; onChange: (v: ChainView) => void }): React.ReactElement {
  const btn = (active: boolean): React.CSSProperties => ({
    background: active ? 'rgba(255,255,255,0.12)' : 'transparent',
    color: active ? 'rgba(255,255,255,0.9)' : 'rgba(255,255,255,0.4)',
    border: 'none',
    cursor: 'pointer',
    fontSize: 10,
    fontWeight: 600,
    padding: '2px 6px',
    borderRadius: 3,
    lineHeight: 1.4,
  })
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
      <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: 0.4 }}>
        Chain
      </span>
      <div style={{ display: 'inline-flex', gap: 2, backgroundColor: 'rgba(0,0,0,0.25)', borderRadius: 4, padding: 1 }}>
        <button style={btn(chain === 'main')} onClick={() => onChange('main')}>Main</button>
        <button style={btn(chain === 'ramp')} onClick={() => onChange('ramp')}>Secondary</button>
      </div>
    </div>
  )
}

export default function CHMetronomeOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('chMetronome')
  const onDragMouseDown = useWindowDrag()

  const [cfg, setCfg] = useState<Cfg>(loadCfg)
  const [chain, setChain] = useState<ChainView>(loadChain)
  const chConfig = useCHChainConfig()
  const secondaryEnabled = chConfig?.secondary_enabled ?? false
  // With the secondary chain off in settings, always follow the main chain —
  // a stale 'ramp' selection would otherwise watch a feed that never fires.
  const activeChain: ChainView = secondaryEnabled ? chain : 'main'
  // anchorRef holds the local-clock ms at which the watched cleric's cast
  // started (their heal lands anchor + 10s). Updated from the timer feed;
  // read by the 100ms render ticker so the countdown stays smooth between the
  // 1s WebSocket pulses.
  const anchorRef = useRef<number | null>(null)
  const cfgRef = useRef(cfg)
  const chainRef = useRef(activeChain)
  const timersRef = useRef<ActiveTimer[]>([])
  // seenRef accumulates the distinct chain-call numbers observed for the active
  // chain so coded sequences (111/222/333) can be ranked into slots even though
  // the live feed never holds them all at once. Cleared on chain switch.
  const seenRef = useRef<Map<number, number>>(new Map())
  const [, setTick] = useState(0)

  // recomputeAnchor re-derives the local anchor from the watched cleric's most
  // recent callout (see lib/chMetronome.computeAnchorMs). It leaves the existing
  // anchor untouched when no match is found yet, so a missed callout just coasts
  // on the last cycle rather than dropping the countdown.
  const recomputeAnchor = useCallback((timers: ActiveTimer[]) => {
    const anchor = computeAnchorMs(timers, cfgRef.current, chainRef.current, seenRef.current, Date.now())
    if (anchor != null) anchorRef.current = anchor
  }, [])

  useEffect(() => {
    cfgRef.current = cfg
    saveCfg(cfg)
    // Re-evaluate the anchor against the latest feed when config changes so
    // switching position takes effect without waiting for the next callout.
    recomputeAnchor(timersRef.current)
  }, [cfg, recomputeAnchor])

  useEffect(() => {
    chainRef.current = activeChain
    localStorage.setItem(CHAIN_STORAGE_KEY, chain)
    // Switching chains drops the old chain's anchor and learned slots — a
    // countdown keyed to the other chain's cadence/numbering would flash CAST
    // NOW at the wrong moment.
    anchorRef.current = null
    seenRef.current.clear()
    recomputeAnchor(timersRef.current)
  }, [chain, activeChain, recomputeAnchor])

  useEffect(() => {
    getTimerState()
      .then((s) => {
        timersRef.current = s.timers
        recomputeAnchor(s.timers)
      })
      .catch(() => {})
  }, [recomputeAnchor])

  const handleMessage = useCallback(
    (msg: { type: string; data: unknown }) => {
      if (msg.type === WSEvent.OverlayTimers) {
        const s = msg.data as TimerState
        timersRef.current = s.timers
        recomputeAnchor(s.timers)
      }
    },
    [recomputeAnchor],
  )
  useWebSocket(handleMessage)

  // Drive a smooth 100ms render tick so the big countdown ticks down between
  // WebSocket pulses and the CAST NOW flash lands on time.
  useEffect(() => {
    const id = setInterval(() => setTick((t) => (t + 1) % 1_000_000), 100)
    return () => clearInterval(id)
  }, [])

  const watch = watchPosition(cfg)
  const now = Date.now()
  const anchor = anchorRef.current
  const active = anchor != null && now - anchor <= (CH_CAST + ANCHOR_GRACE_SECS) * 1000

  // Derived timing for the current cycle.
  const elapsed = active ? (now - (anchor as number)) / 1000 : 0 // since watched callout
  const castIn = active ? cfg.delay - elapsed : 0 // seconds until I should cast
  const flashing = active && castIn <= 0.15 && elapsed <= cfg.delay + PULSE_SECS

  let bigText = '—'
  let bigColor = 'rgba(255,255,255,0.25)'
  let subText = `Waiting for ${posLabel(watch, activeChain)}…`
  if (active) {
    if (flashing) {
      bigText = 'CAST NOW'
      bigColor = '#22c55e'
      subText = `heal lands ~${cfg.delay}s into the chain`
    } else if (castIn > 0.15) {
      bigText = `${castIn.toFixed(1)}s`
      bigColor = castIn <= 1 ? '#fbbf24' : '#93c5fd'
      subText = 'until your cast'
    } else {
      bigText = 'cast sent'
      bigColor = 'rgba(255,255,255,0.4)'
      subText = `next: ${posLabel(watch, activeChain)} calls again`
    }
  }

  // Track: 0s (left) → 10s land (right). Playhead = elapsed; markers at the
  // cast point (delay) and the land point (10s).
  const elapsedPct = Math.max(0, Math.min(1, elapsed / CH_CAST)) * 100
  const castPct = Math.max(0, Math.min(1, cfg.delay / CH_CAST)) * 100

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        // The CAST NOW flash border is a functional cue, so it stays visible
        // even while the fade-when-inactive chrome is hidden.
        border: `1px solid ${flashing ? 'rgba(34,197,94,0.7)' : `rgba(255,255,255,${chrome ? 0.12 : 0})`}`,
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
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          // Fade-when-inactive: hide the title bar with the rest of the
          // chrome; pointerEvents off so invisible buttons can't be clicked.
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Gauge size={11} style={{ color: '#22c55e' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            CH Metronome
          </span>
          <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
            {posLabel(cfg.position, activeChain)}/{cfg.chainSize}
            {activeChain === 'ramp' ? ' · secondary' : ''}
          </span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeCHMetronome()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close overlay"
          >
            <X size={11} />
          </button>
        </div>
      </div>

      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 8,
          padding: '8px 10px',
        }}
      >
        <div
          style={{
            fontSize: flashing ? 30 : 34,
            fontWeight: 800,
            letterSpacing: flashing ? 1 : 0,
            color: bigColor,
            fontVariantNumeric: 'tabular-nums',
            textShadow: flashing ? '0 0 14px rgba(34,197,94,0.7)' : '0 1px 3px rgba(0,0,0,0.9)',
            lineHeight: 1,
            transition: 'color 120ms linear',
          }}
        >
          {bigText}
        </div>
        <div style={{ fontSize: 10, color: 'rgba(255,255,255,0.45)' }}>{subText}</div>

        {/* 10s cast track: playhead sweeps left→right; amber marker is your
            cast point, green marker is when the watched heal lands. */}
        <div
          style={{
            position: 'relative',
            width: '100%',
            height: 8,
            borderRadius: 4,
            backgroundColor: 'rgba(255,255,255,0.08)',
            overflow: 'hidden',
            opacity: active ? 1 : 0.3,
          }}
        >
          <div
            style={{
              position: 'absolute',
              left: 0,
              top: 0,
              bottom: 0,
              width: `${elapsedPct}%`,
              backgroundColor: 'rgba(59,130,246,0.35)',
            }}
          />
          {/* cast-point marker */}
          <div
            style={{
              position: 'absolute',
              left: `${castPct}%`,
              top: -2,
              bottom: -2,
              width: 2,
              backgroundColor: '#fbbf24',
            }}
          />
          {/* land marker (right edge) */}
          <div
            style={{
              position: 'absolute',
              right: 0,
              top: -2,
              bottom: -2,
              width: 2,
              backgroundColor: '#22c55e',
            }}
          />
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }}>
          <span style={{ fontSize: 8, color: '#fbbf24' }}>▲ cast ({cfg.delay}s)</span>
          <span style={{ fontSize: 8, color: '#22c55e' }}>they land (10s) ▲</span>
        </div>
      </div>

      <div
        className="no-drag"
        style={{
          display: 'flex',
          justifyContent: 'space-around',
          alignItems: 'center',
          padding: '5px 6px',
          borderTop: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.03)',
          flexShrink: 0,
        }}
      >
        <Stepper
          label="My #"
          value={cfg.position}
          min={1}
          max={12}
          onChange={(v) => setCfg((c) => ({ ...c, position: Math.min(v, c.chainSize) }))}
        />
        <Stepper
          label="Clerics"
          value={cfg.chainSize}
          min={1}
          max={12}
          onChange={(v) => setCfg((c) => ({ ...c, chainSize: v, position: Math.min(c.position, v) }))}
        />
        <Stepper
          label="Delay"
          value={cfg.delay}
          min={0.5}
          max={CH_CAST - 1}
          step={0.5}
          suffix="s"
          onChange={(v) => setCfg((c) => ({ ...c, delay: v }))}
        />
        {secondaryEnabled && <ChainSwitch chain={activeChain} onChange={setChain} />}
      </div>
    </div>
  )
}
