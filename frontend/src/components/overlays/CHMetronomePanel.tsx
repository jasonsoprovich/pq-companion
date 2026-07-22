/**
 * CHMetronomePanel — in-dashboard version of the personal Complete-Heal timing
 * helper. Mirrors CHMetronomeOverlayWindowPage but renders inside the
 * draggable/resizable OverlayWindow used by the Overlays dashboard. Watches the
 * ch_chain timer feed for the position immediately before yours and counts down
 * to your cast moment, flashing CAST NOW when it's time to press your heal key.
 *
 * Config (your position, the chain size, your desired delay) persists to the
 * same localStorage keys as the popout window so the two stay in sync on
 * reload.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Gauge, ExternalLink } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { useCHChainConfig } from '../../hooks/useCHChainConfig'
import { getTimerState } from '../../services/api'
import {
  acceptNewAnchor,
  CH_CAST,
  type AnchorResult,
  type ChainView,
  computeAnchorMs,
  loadSeen,
  mergeSeen,
  saveSeen,
  seenStorageKey,
  watchPosition,
} from '../../lib/chMetronome'
import OverlayWindow from '../OverlayWindow'
import type { ActiveTimer, TimerState } from '../../types/timer'

interface CHMetronomePanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// How long to hold the CAST NOW flash after the cast moment passes.
const PULSE_SECS = 1.5
// Keep using the last anchor for a little past the 10s cast so the flash and
// post-cast state still render even after the source timer expires.
const ANCHOR_GRACE_SECS = 3

type Cfg = { position: number; chainSize: number; delay: number }

const DEFAULT_CFG: Cfg = { position: 2, chainSize: 3, delay: 4 }

// ChainView ('main' | 'ramp') is shared via lib/chMetronome. The selector is
// only shown when the secondary chain is enabled in settings. The choice shares
// the same localStorage key as the popout window so the two stay in sync.
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
  const read = (k: string, d: number): number => {
    const raw = localStorage.getItem(`chMetronome:${k}`)
    const n = raw == null ? NaN : parseInt(raw, 10)
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
function Stepper(props: {
  label: string
  value: number
  min: number
  max: number
  onChange: (v: number) => void
  suffix?: string
}): React.ReactElement {
  const { label, value, min, max, onChange, suffix } = props
  const clamp = (v: number): number => Math.max(min, Math.min(max, v))
  const btn: React.CSSProperties = {
    width: 18, height: 18, lineHeight: '16px', textAlign: 'center', borderRadius: 3,
    border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)', cursor: 'pointer', fontSize: 12, padding: 0,
  }
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
      <span style={{ fontSize: 9, color: 'var(--color-muted)', textTransform: 'uppercase', letterSpacing: 0.4 }}>
        {label}
      </span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
        <button style={btn} onClick={() => onChange(clamp(value - 1))} title={`Decrease ${label}`}>−</button>
        <span style={{ minWidth: 22, textAlign: 'center', fontSize: 12, fontWeight: 700, color: 'var(--color-foreground)', fontVariantNumeric: 'tabular-nums' }}>
          {value}
          {suffix ?? ''}
        </span>
        <button style={btn} onClick={() => onChange(clamp(value + 1))} title={`Increase ${label}`}>+</button>
      </div>
    </div>
  )
}

// ChainSwitch is a compact Main/Secondary segmented control matching the
// Stepper layout (label above control), shown only when the secondary chain
// is enabled. Styled with theme tokens to match the dashboard panel.
function ChainSwitch({ chain, onChange }: { chain: ChainView; onChange: (v: ChainView) => void }): React.ReactElement {
  const btn = (active: boolean): React.CSSProperties => ({
    background: active ? 'var(--color-surface-2)' : 'transparent',
    color: active ? 'var(--color-foreground)' : 'var(--color-muted)',
    border: 'none', cursor: 'pointer', fontSize: 10, fontWeight: 600,
    padding: '2px 6px', borderRadius: 3, lineHeight: 1.4,
  })
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
      <span style={{ fontSize: 9, color: 'var(--color-muted)', textTransform: 'uppercase', letterSpacing: 0.4 }}>
        Chain
      </span>
      <div style={{ display: 'inline-flex', gap: 2, backgroundColor: 'rgba(0,0,0,0.25)', borderRadius: 4, padding: 1 }}>
        <button style={btn(chain === 'main')} onClick={() => onChange('main')}>Main</button>
        <button style={btn(chain === 'ramp')} onClick={() => onChange('ramp')}>Secondary</button>
      </div>
    </div>
  )
}

export default function CHMetronomePanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 240,
  defaultHeight = 272,
  snapGridSize,
  onLayoutChange,
}: CHMetronomePanelProps): React.ReactElement {
  const [cfg, setCfg] = useState<Cfg>(loadCfg)
  const [chain, setChain] = useState<ChainView>(loadChain)
  const chConfig = useCHChainConfig()
  const secondaryEnabled = chConfig?.secondary_enabled ?? false
  // With the secondary chain off in settings, always follow the main chain —
  // a stale 'ramp' selection would otherwise watch a feed that never fires.
  const activeChain: ChainView = secondaryEnabled ? chain : 'main'
  // anchorRef holds the local-clock ms at which the watched cleric's cast
  // started (their heal lands anchor + 10s), plus whether that time is a
  // confirmed callout or a projection from another slot's callout (see
  // lib/chMetronome.computeAnchorMs). Updated from the timer feed; read by the
  // 100ms render ticker so the countdown stays smooth between the 1s
  // WebSocket pulses.
  const anchorRef = useRef<AnchorResult | null>(null)
  const cfgRef = useRef(cfg)
  const chainRef = useRef(activeChain)
  const timersRef = useRef<ActiveTimer[]>([])
  // seenRef accumulates the distinct chain-call numbers observed for the active
  // chain so coded sequences (111/222/333) can be ranked into slots even though
  // the live feed never holds them all at once. Hydrated from localStorage (not
  // a fresh Map) and persisted on every update so this panel and the popped-out
  // overlay window — two views of one metronome — share the same learning
  // progress instead of each relearning from scratch on its own mount/reload.
  const seenRef = useRef<Map<number, number>>(loadSeen(activeChain))
  // Guards the chain-switch effect below from clearing the just-hydrated seen
  // map on initial mount; only an actual chain change should reset it.
  const mountedRef = useRef(false)
  const [, setTick] = useState(0)

  // recomputeAnchor re-derives the local anchor from the watched cleric's most
  // recent callout (see lib/chMetronome.computeAnchorMs). It leaves the existing
  // anchor untouched when no match is found yet, so a missed callout just coasts
  // on the last cycle rather than dropping the countdown.
  const activeRef = useRef(false)
  const recomputeAnchor = useCallback((timers: ActiveTimer[]) => {
    const anchor = computeAnchorMs(timers, cfgRef.current, chainRef.current, seenRef.current, Date.now())
    saveSeen(chainRef.current, seenRef.current)
    if (anchor != null && acceptNewAnchor(anchorRef.current, anchor, cfgRef.current.delay)) {
      anchorRef.current = anchor
      // Force a render so a new anchor re-activates the (possibly idle) tick.
      setTick((t) => (t + 1) % 1_000_000)
    }
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
    if (mountedRef.current) {
      // Switching chains drops the old chain's anchor and learned slots — a
      // countdown keyed to the other chain's cadence/numbering would flash
      // CAST NOW at the wrong moment. On mount seenRef is already hydrated
      // for activeChain above, so this only fires on a real switch.
      anchorRef.current = null
      seenRef.current = loadSeen(activeChain)
    }
    mountedRef.current = true
    recomputeAnchor(timersRef.current)
  }, [chain, activeChain, recomputeAnchor])

  // Pick up learning progress from the sibling renderer (popout overlay vs
  // in-app panel) as it happens, not just at mount — localStorage 'storage'
  // events fire on other windows only. Merges (keeps the newer timestamp per
  // number) so a slightly stale write from one window can't erase progress
  // the other already made.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key !== seenStorageKey(chainRef.current)) return
      mergeSeen(seenRef.current, loadSeen(chainRef.current))
      recomputeAnchor(timersRef.current)
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [recomputeAnchor])

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
  // WebSocket pulses and the CAST NOW flash lands on time. Only re-renders
  // while an anchor is active — when idle, setTick returns the same value so
  // React bails, avoiding needless renders in the dashboard card. A new anchor
  // re-activates via the setTick bump in recomputeAnchor.
  useEffect(() => {
    const id = setInterval(() => {
      if (activeRef.current) setTick((t) => (t + 1) % 1_000_000)
    }, 100)
    return () => clearInterval(id)
  }, [])

  const watch = watchPosition(cfg)
  const now = Date.now()
  const anchorResult = anchorRef.current
  const anchor = anchorResult?.anchorMs ?? null
  const predicted = anchorResult?.predicted ?? false
  const active = anchor != null && now - anchor <= (CH_CAST + ANCHOR_GRACE_SECS) * 1000
  activeRef.current = active

  // Derived timing for the current cycle.
  const elapsed = active ? (now - (anchor as number)) / 1000 : 0 // since watched callout
  const castIn = active ? cfg.delay - elapsed : 0 // seconds until I should cast
  const flashing = active && castIn <= 0.15 && elapsed <= cfg.delay + PULSE_SECS

  let bigText = '—'
  let bigColor = 'var(--color-muted)'
  let subText = `Waiting for ${posLabel(watch, activeChain)}…`
  if (active) {
    // predicted = the watched slot missed its call this cycle and this
    // countdown is projected from another slot's real callout, not a
    // confirmed one (lib/chMetronome.computeAnchorMs).
    const predictedNote = predicted ? ` (${posLabel(watch, activeChain)} predicted — may have missed)` : ''
    if (flashing) {
      bigText = 'CAST NOW'
      bigColor = '#22c55e'
      subText = `heal lands ~${cfg.delay}s into the chain${predictedNote}`
    } else if (castIn > 0.15) {
      bigText = `${castIn.toFixed(1)}s`
      bigColor = castIn <= 1 ? '#fbbf24' : '#60a5fa'
      subText = `until your cast${predictedNote}`
    } else {
      bigText = 'CAST SENT'
      bigColor = 'var(--color-muted)'
      subText = `next: ${posLabel(watch, activeChain)} calls again`
    }
  }

  // Track: 0s (left) → 10s land (right). Playhead = elapsed; markers at the
  // cast point (delay) and the land point (10s).
  const elapsedPct = Math.max(0, Math.min(1, elapsed / CH_CAST)) * 100
  const castPct = Math.max(0, Math.min(1, cfg.delay / CH_CAST)) * 100

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Gauge size={13} style={{ color: '#22c55e' }} />
          CH Metronome
          <span style={{ fontSize: 10, color: 'var(--color-muted)', fontWeight: 400 }}>
            {posLabel(cfg.position, activeChain)}/{cfg.chainSize}
            {activeChain === 'ramp' ? ' · secondary' : ''}
          </span>
        </span>
      }
      headerRight={
        window.electron?.overlay ? (
          <button
            onClick={() => window.electron.overlay.toggleCHMetronome()}
            title="Pop out as floating overlay"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
          >
            <ExternalLink size={12} />
          </button>
        ) : undefined
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
      minWidth={200}
      minHeight={232}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <div
        style={{
          flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column',
          alignItems: 'center', justifyContent: 'center', gap: 8, padding: '8px 10px',
          boxShadow: flashing ? 'inset 0 0 0 2px rgba(34,197,94,0.6)' : 'none',
          transition: 'box-shadow 120ms linear',
        }}
      >
        <div
          style={{
            fontSize: flashing ? 28 : 32, fontWeight: 800, letterSpacing: flashing ? 1 : 0,
            color: bigColor, fontVariantNumeric: 'tabular-nums',
            textShadow: flashing ? '0 0 14px rgba(34,197,94,0.6)' : 'none',
            lineHeight: 1, transition: 'color 120ms linear',
          }}
        >
          {bigText}
        </div>
        <div style={{ fontSize: 10, color: 'var(--color-muted)' }}>{subText}</div>

        {/* 10s cast track: playhead sweeps left→right; amber marker is your
            cast point, green marker is when the watched heal lands. */}
        <div
          style={{
            position: 'relative', width: '100%', height: 8, borderRadius: 4,
            backgroundColor: 'var(--color-surface-2)', overflow: 'hidden',
            opacity: active ? 1 : 0.4,
          }}
        >
          <div style={{ position: 'absolute', left: 0, top: 0, bottom: 0, width: `${elapsedPct}%`, backgroundColor: 'rgba(96,165,250,0.45)' }} />
          {/* cast-point marker */}
          <div style={{ position: 'absolute', left: `${castPct}%`, top: -2, bottom: -2, width: 2, backgroundColor: '#fbbf24' }} />
          {/* land marker (right edge) */}
          <div style={{ position: 'absolute', right: 0, top: -2, bottom: -2, width: 2, backgroundColor: '#22c55e' }} />
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }}>
          <span style={{ fontSize: 8, color: '#fbbf24' }}>▲ cast ({cfg.delay}s)</span>
          <span style={{ fontSize: 8, color: '#22c55e' }}>they land (10s) ▲</span>
        </div>
      </div>

      <div
        style={{
          display: 'flex', justifyContent: 'space-around', alignItems: 'center',
          padding: '5px 6px', borderTop: '1px solid var(--color-border)',
          backgroundColor: 'var(--color-surface-2)', flexShrink: 0,
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
          min={1}
          max={CH_CAST - 1}
          suffix="s"
          onChange={(v) => setCfg((c) => ({ ...c, delay: v }))}
        />
        {secondaryEnabled && <ChainSwitch chain={activeChain} onChange={setChain} />}
      </div>
    </OverlayWindow>
  )
}
