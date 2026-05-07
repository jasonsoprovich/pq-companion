import React, { useCallback, useEffect, useState } from 'react'
import { Skull, ExternalLink, Plus, Eraser, Circle, CheckCircle2, AlertTriangle, X } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useActivePlayerName } from '../../hooks/useActivePlayerName'
import { useDisplayThresholds, passesThreshold } from '../../hooks/useDisplayThresholds'
import { clearTimers, getLogStatus, getTimerState, removeTimer } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import CreateTriggerModal from '../CreateTriggerModal'
import SpellSearchPicker from '../SpellSearchPicker'
import { buildSpellTriggerPrefill } from '../../lib/spellHelpers'
import type { ActiveTimer, TimerCategory, TimerState } from '../../types/timer'
import type { LogTailerStatus } from '../../types/logEvent'
import type { Spell } from '../../types/spell'
import { SpellIcon } from '../Icon'

interface DetrimTimerPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

const DETRIM_CATEGORIES = new Set<TimerCategory>(['debuff', 'dot', 'mez', 'stun'])

const CATEGORY_COLORS: Record<TimerCategory, string> = {
  buff: '#22c55e',
  debuff: '#f97316',
  dot: '#ef4444',
  mez: '#a855f7',
  stun: '#eab308',
}

function detrimTarget(targetName: string, activePlayer: string): string {
  if (!targetName) return ''
  if (targetName === activePlayer) return ''
  if (targetName === 'You') return ''
  return targetName
}

function fmtRemaining(secs: number): string {
  if (secs <= 0) return '0s'
  if (secs < 60) return `${Math.ceil(secs)}s`
  return `${Math.ceil(secs / 60)}m`
}

function barColor(remaining: number, total: number, category: TimerCategory): string {
  if (total <= 0) return CATEGORY_COLORS[category]
  return remaining / total > 0.2 ? CATEGORY_COLORS[category] : '#ef4444'
}

function DetrimRow({ timer, activePlayer }: { timer: ActiveTimer; activePlayer: string }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColor(timer.remaining_seconds, timer.duration_seconds, timer.category)
  const urgent = pct < 0.2
  const catColor = CATEGORY_COLORS[timer.category]
  const target = detrimTarget(timer.target_name, activePlayer)

  return (
    <div style={{ position: 'relative', padding: '3px 10px', borderBottom: '1px solid var(--color-border)', overflow: 'hidden', flexShrink: 0 }}>
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${pct * 100}%`, backgroundColor: color, opacity: 0.15,
          pointerEvents: 'none', transition: 'width 1s linear',
        }}
      />
      <div style={{ position: 'absolute', left: 0, top: 0, bottom: 0, width: 2, backgroundColor: catColor, opacity: 0.6 }} />
      <div style={{ position: 'relative', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 6, paddingLeft: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0 }}>
          <SpellIcon id={timer.icon} name={timer.spell_name} size={16} loading="eager" />
          <span style={{ fontSize: 12, color: urgent ? '#f87171' : 'var(--color-foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: urgent ? 600 : 400 }}>
            {timer.spell_name}
            {target && (
              <span style={{ color: 'var(--color-muted)', fontWeight: 400 }}>{` — ${target}`}</span>
            )}
          </span>
        </div>
        <span style={{ fontSize: 11, color: urgent ? '#f87171' : color, fontVariantNumeric: 'tabular-nums', flexShrink: 0, fontWeight: urgent ? 700 : 400 }}>
          {fmtRemaining(timer.remaining_seconds)}
        </span>
        <button
          onClick={() => removeTimer(timer.id).catch(() => {})}
          title="Remove this timer"
          style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 0, color: 'var(--color-muted)', display: 'flex', alignItems: 'center', flexShrink: 0, lineHeight: 0 }}
        >
          <X size={11} />
        </button>
      </div>
    </div>
  )
}

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  const style: React.CSSProperties = {
    display: 'flex', alignItems: 'center', gap: 6,
    padding: '6px 10px', fontSize: 11,
    borderBottom: '1px solid var(--color-border)', flexShrink: 0,
    backgroundColor: 'var(--color-surface-2)',
  }
  if (!status) return <div style={{ ...style, color: 'var(--color-muted)' }}><Circle size={10} /> Loading…</div>
  if (!status.enabled) return <div style={{ ...style, color: '#f97316' }}><AlertTriangle size={11} /> Log parsing disabled — enable in Settings</div>
  if (!status.file_exists) return <div style={{ ...style, color: '#f97316' }}><AlertTriangle size={11} /> Log file not found</div>
  return <div style={{ ...style, color: '#22c55e' }}><CheckCircle2 size={11} /> Tailing log</div>
}

function ConnPill({ state }: { state: string }): React.ReactElement {
  const color = state === 'open' ? '#22c55e' : state === 'connecting' ? '#f97316' : '#6b7280'
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color }}>
      <span style={{ width: 7, height: 7, borderRadius: '50%', backgroundColor: color, display: 'inline-block' }} />
      {state === 'open' ? 'Live' : state === 'connecting' ? 'Connecting' : 'Off'}
    </span>
  )
}

export default function DetrimTimerPanel({
  defaultX = 340,
  defaultY = 24,
  defaultWidth = 300,
  defaultHeight = 380,
  snapGridSize,
  onLayoutChange,
}: DetrimTimerPanelProps): React.ReactElement {
  const [timerState, setTimerState] = useState<TimerState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [pickedSpell, setPickedSpell] = useState<Spell | null>(null)
  const activePlayer = useActivePlayerName()
  const thresholds = useDisplayThresholds()

  useEffect(() => {
    getTimerState().then(setTimerState).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:timers') setTimerState(msg.data as TimerState)
  }, [])

  const wsState = useWebSocket(handleMessage)

  const detrims = (timerState?.timers ?? [])
    .filter((t) => DETRIM_CATEGORIES.has(t.category))
    .filter((t) => passesThreshold(t, thresholds))

  return (
    <>
      <OverlayWindow
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <Skull size={13} style={{ color: '#ef4444' }} />
            Detrimental
          </span>
        }
        headerRight={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <button
              onClick={() => clearTimers('detrimental').catch(() => {})}
              title="Clear all active detrimental timers"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
            >
              <Eraser size={12} />
            </button>
            <button
              onClick={() => setPickerOpen(true)}
              title="Add a detrimental timer from a spell"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
            >
              <Plus size={12} />
            </button>
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleDetrimTimer()}
                title="Pop out as floating overlay"
                style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
              >
                <ExternalLink size={12} />
              </button>
            )}
            <ConnPill state={wsState} />
          </div>
        }
        defaultWidth={defaultWidth}
        defaultHeight={defaultHeight}
        defaultX={defaultX}
        defaultY={defaultY}
        minWidth={220}
        minHeight={160}
        snapGridSize={snapGridSize}
        onLayoutChange={onLayoutChange}
      >
        <StatusBar status={status} />
        <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
          {timerState === null ? (
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
              <Skull size={28} style={{ opacity: 0.2, color: '#ef4444' }} />
              <p style={{ fontSize: 12, margin: 0 }}>Loading…</p>
            </div>
          ) : detrims.length === 0 ? (
            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
              <Skull size={28} style={{ opacity: 0.2, color: '#ef4444' }} />
              <p style={{ fontSize: 12, margin: 0 }}>No active detrimentals</p>
            </div>
          ) : (
            detrims.map((t) => <DetrimRow key={t.id} timer={t} activePlayer={activePlayer} />)
          )}
        </div>
      </OverlayWindow>

      {pickerOpen && (
        <SpellSearchPicker
          onPick={(spell) => { setPickedSpell(spell); setPickerOpen(false) }}
          onClose={() => setPickerOpen(false)}
        />
      )}

      {pickedSpell && (
        <CreateTriggerModal
          prefill={{ ...buildSpellTriggerPrefill(pickedSpell), timerType: 'detrimental' }}
          onClose={() => setPickedSpell(null)}
        />
      )}
    </>
  )
}
