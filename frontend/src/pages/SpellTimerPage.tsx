/**
 * SpellTimerPage — in-app view showing two floating, draggable overlay panels:
 *   • Buffs panel  — timers with category === 'buff'
 *   • Detrimental panel — timers with category in debuff | dot | mez | stun
 *
 * Each panel has a "pop out" button to open it as a standalone transparent
 * always-on-top window via Electron IPC.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Shield, Skull, ExternalLink, Circle, CheckCircle2, AlertTriangle, Bell, Plus, Power } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getTimerState, getLogStatus } from '../services/api'
import OverlayWindow from '../components/OverlayWindow'
import TimerAlertsPanel from '../components/TimerAlertsPanel'
import CreateTriggerModal from '../components/CreateTriggerModal'
import SpellSearchPicker from '../components/SpellSearchPicker'
import type { ActiveTimer, TimerCategory, TimerState } from '../types/timer'
import type { LogTailerStatus } from '../types/logEvent'
import type { Spell } from '../types/spell'
import type { TimerType } from '../types/trigger'
import { buildSpellTriggerPrefill } from '../lib/spellHelpers'

// ── Constants ──────────────────────────────────────────────────────────────────

const DETRIM_CATEGORIES = new Set<TimerCategory>(['debuff', 'dot', 'mez', 'stun'])

const CATEGORY_COLORS: Record<TimerCategory, string> = {
  buff: '#22c55e',
  debuff: '#f97316',
  dot: '#ef4444',
  mez: '#a855f7',
  stun: '#eab308',
}

const CATEGORY_LABELS: Record<TimerCategory, string> = {
  buff: 'Buff',
  debuff: 'Debuff',
  dot: 'DoT',
  mez: 'Mez',
  stun: 'Stun',
}

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmtRemaining(secs: number): string {
  if (secs <= 0) return '0s'
  if (secs < 60) return `${Math.ceil(secs)}s`
  const m = Math.floor(secs / 60)
  const s = Math.ceil(secs % 60)
  return s > 0 ? `${m}m ${s}s` : `${m}m`
}

function barColorBuff(remaining: number, total: number): string {
  if (total <= 0) return '#22c55e'
  const pct = remaining / total
  if (pct > 0.5) return '#22c55e'
  if (pct > 0.2) return '#f97316'
  return '#ef4444'
}

function barColorDetrim(remaining: number, total: number, category: TimerCategory): string {
  if (total <= 0) return CATEGORY_COLORS[category]
  return remaining / total > 0.2 ? CATEGORY_COLORS[category] : '#ef4444'
}

// ── Status bar ─────────────────────────────────────────────────────────────────

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  const style: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    padding: '6px 10px',
    fontSize: 11,
    borderBottom: '1px solid var(--color-border)',
    flexShrink: 0,
    backgroundColor: 'var(--color-surface-2)',
  }

  if (!status) {
    return (
      <div style={{ ...style, color: 'var(--color-muted)' }}>
        <Circle size={10} /> Loading…
      </div>
    )
  }
  if (!status.enabled) {
    return (
      <div style={{ ...style, color: '#f97316' }}>
        <AlertTriangle size={11} /> Log parsing disabled — enable in Settings
      </div>
    )
  }
  if (!status.file_exists) {
    return (
      <div style={{ ...style, color: '#f97316' }}>
        <AlertTriangle size={11} /> Log file not found
      </div>
    )
  }
  return (
    <div style={{ ...style, color: '#22c55e' }}>
      <CheckCircle2 size={11} /> Tailing log
    </div>
  )
}

// ── Buff timer row ─────────────────────────────────────────────────────────────

function BuffRow({ timer }: { timer: ActiveTimer }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColorBuff(timer.remaining_seconds, timer.duration_seconds)
  const urgent = pct < 0.2

  return (
    <div
      style={{
        position: 'relative',
        padding: '5px 10px',
        borderBottom: '1px solid var(--color-border)',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: color,
          opacity: 0.15,
          pointerEvents: 'none',
          transition: 'width 1s linear',
        }}
      />
      <div
        style={{
          position: 'relative',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <span
          style={{
            fontSize: 12,
            color: urgent ? '#f87171' : 'var(--color-foreground)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontWeight: urgent ? 600 : 400,
          }}
        >
          {timer.spell_name}
        </span>
        <span
          style={{
            fontSize: 11,
            color: urgent ? '#f87171' : color,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: urgent ? 700 : 400,
          }}
        >
          {fmtRemaining(timer.remaining_seconds)}
        </span>
      </div>
    </div>
  )
}

// ── Detrimental timer row ──────────────────────────────────────────────────────

function DetrimRow({ timer }: { timer: ActiveTimer }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColorDetrim(timer.remaining_seconds, timer.duration_seconds, timer.category)
  const urgent = pct < 0.2
  const catColor = CATEGORY_COLORS[timer.category]

  return (
    <div
      style={{
        position: 'relative',
        padding: '5px 10px',
        borderBottom: '1px solid var(--color-border)',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: color,
          opacity: 0.15,
          pointerEvents: 'none',
          transition: 'width 1s linear',
        }}
      />
      {/* left accent */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 2,
          backgroundColor: catColor,
          opacity: 0.6,
        }}
      />
      <div
        style={{
          position: 'relative',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 6,
          paddingLeft: 6,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0 }}>
          <span
            style={{
              fontSize: 9,
              fontWeight: 700,
              textTransform: 'uppercase',
              letterSpacing: '0.05em',
              color: catColor,
              flexShrink: 0,
              opacity: 0.85,
            }}
          >
            {CATEGORY_LABELS[timer.category]}
          </span>
          <span
            style={{
              fontSize: 12,
              color: urgent ? '#f87171' : 'var(--color-foreground)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontWeight: urgent ? 600 : 400,
            }}
          >
            {timer.spell_name}
          </span>
        </div>
        <span
          style={{
            fontSize: 11,
            color: urgent ? '#f87171' : color,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: urgent ? 700 : 400,
          }}
        >
          {fmtRemaining(timer.remaining_seconds)}
        </span>
      </div>
    </div>
  )
}

// ── Empty state ────────────────────────────────────────────────────────────────

function EmptyState({
  icon,
  message,
}: {
  icon: React.ReactNode
  message: string
}): React.ReactElement {
  return (
    <div
      style={{
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 8,
        color: 'var(--color-muted)',
        padding: 16,
      }}
    >
      {icon}
      <p style={{ fontSize: 12, margin: 0 }}>{message}</p>
    </div>
  )
}

// ── Conn pill ──────────────────────────────────────────────────────────────────

function ConnPill({ state }: { state: string }): React.ReactElement {
  const color =
    state === 'open' ? '#22c55e' : state === 'connecting' ? '#f97316' : '#6b7280'
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color }}>
      <span
        style={{
          width: 7,
          height: 7,
          borderRadius: '50%',
          backgroundColor: color,
          display: 'inline-block',
        }}
      />
      {state === 'open' ? 'Live' : state === 'connecting' ? 'Connecting' : 'Off'}
    </span>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function SpellTimerPage(): React.ReactElement {
  const [timerState, setTimerState] = useState<TimerState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [alertsPanelOpen, setAlertsPanelOpen] = useState(false)
  // When set, a spell picker is open and the caller wants a timer of this type.
  const [pickerTimerType, setPickerTimerType] = useState<TimerType | null>(null)
  // When set, a trigger-create modal is open for the picked spell.
  const [pickedSpell, setPickedSpell] = useState<{ spell: Spell; type: TimerType } | null>(null)
  // Global on/off — when off, all timer entries are hidden. Persists across sessions.
  const [globalEnabled, setGlobalEnabled] = useState<boolean>(() => {
    return localStorage.getItem('spell-timers-enabled') !== '0'
  })
  useEffect(() => {
    localStorage.setItem('spell-timers-enabled', globalEnabled ? '1' : '0')
  }, [globalEnabled])

  const handleSpellPicked = (spell: Spell) => {
    if (!pickerTimerType) return
    setPickedSpell({ spell, type: pickerTimerType })
    setPickerTimerType(null)
  }

  useEffect(() => {
    getTimerState().then(setTimerState).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:timers') {
      const data = msg.data as TimerState
      // eslint-disable-next-line no-console
      console.log('[timer-debug] SpellTimerPage state update:', {
        total: data?.timers?.length ?? 0,
        buffs: data?.timers?.filter((t) => t.category === 'buff').length ?? 0,
        detrim: data?.timers?.filter((t) => t.category !== 'buff').length ?? 0,
      })
      setTimerState(data)
    }
  }, [])

  const wsState = useWebSocket(handleMessage)

  const buffs = globalEnabled
    ? (timerState?.timers ?? []).filter((t) => t.category === 'buff')
    : []
  const detrims = globalEnabled
    ? (timerState?.timers ?? []).filter((t) => DETRIM_CATEGORIES.has(t.category))
    : []

  return (
    <div
      style={{
        position: 'relative',
        height: '100%',
        overflow: 'hidden',
        backgroundColor: 'var(--color-background)',
      }}
    >
      {/* Background hint */}
      <div
        style={{
          position: 'absolute',
          inset: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          pointerEvents: 'none',
          userSelect: 'none',
        }}
      >
        <p style={{ fontSize: 12, color: 'var(--color-muted)', opacity: 0.4 }}>
          Drag title bars to move · Drag edges/corners to resize
        </p>
      </div>

      {/* ── Buff timer panel ───────────────────────────────────────────── */}
      <OverlayWindow
        title={
          <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <Shield size={13} style={{ color: '#22c55e' }} />
            Buffs
          </span>
        }
        headerRight={
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <button
              onClick={() => setGlobalEnabled((v) => !v)}
              title={globalEnabled ? 'Disable all spell timers' : 'Enable all spell timers'}
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: '1px 3px',
                color: globalEnabled ? '#22c55e' : '#6b7280',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              <Power size={12} />
            </button>
            <button
              onClick={() => setPickerTimerType('buff')}
              title="Add a buff timer from a spell"
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: '1px 3px',
                color: 'var(--color-muted)',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              <Plus size={12} />
            </button>
            <button
              onClick={() => setAlertsPanelOpen((v) => !v)}
              title="Configure timer audio alerts"
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: '1px 3px',
                color: alertsPanelOpen ? 'var(--color-primary)' : 'var(--color-muted)',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              <Bell size={12} />
            </button>
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleBuffTimer()}
                title="Pop out as floating overlay"
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  padding: '1px 3px',
                  color: 'var(--color-muted)',
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <ExternalLink size={12} />
              </button>
            )}
            <ConnPill state={wsState} />
          </div>
        }
        defaultWidth={300}
        defaultHeight={380}
        defaultX={24}
        defaultY={24}
        minWidth={220}
        minHeight={160}
      >
        <StatusBar status={status} />
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
          {timerState === null ? (
            <EmptyState
              icon={<Shield size={28} style={{ opacity: 0.2, color: '#22c55e' }} />}
              message="Loading…"
            />
          ) : buffs.length === 0 ? (
            <EmptyState
              icon={<Shield size={28} style={{ opacity: 0.2, color: '#22c55e' }} />}
              message="No active buffs"
            />
          ) : (
            buffs.map((t) => <BuffRow key={t.id} timer={t} />)
          )}
        </div>
      </OverlayWindow>

      {/* ── Timer alerts configuration panel ──────────────────────────── */}
      {alertsPanelOpen && (
        <TimerAlertsPanel onClose={() => setAlertsPanelOpen(false)} />
      )}

      {/* ── Detrimental timer panel ────────────────────────────────────── */}
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
              onClick={() => setPickerTimerType('detrimental')}
              title="Add a detrimental timer from a spell"
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: '1px 3px',
                color: 'var(--color-muted)',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              <Plus size={12} />
            </button>
            {window.electron?.overlay && (
              <button
                onClick={() => window.electron.overlay.toggleDetrimTimer()}
                title="Pop out as floating overlay"
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  padding: '1px 3px',
                  color: 'var(--color-muted)',
                  display: 'flex',
                  alignItems: 'center',
                }}
              >
                <ExternalLink size={12} />
              </button>
            )}
            <ConnPill state={wsState} />
          </div>
        }
        defaultWidth={300}
        defaultHeight={380}
        defaultX={340}
        defaultY={24}
        minWidth={220}
        minHeight={160}
      >
        <StatusBar status={status} />
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
          {timerState === null ? (
            <EmptyState
              icon={<Skull size={28} style={{ opacity: 0.2, color: '#ef4444' }} />}
              message="Loading…"
            />
          ) : detrims.length === 0 ? (
            <EmptyState
              icon={<Skull size={28} style={{ opacity: 0.2, color: '#ef4444' }} />}
              message="No active detrimentals"
            />
          ) : (
            detrims.map((t) => <DetrimRow key={t.id} timer={t} />)
          )}
        </div>
      </OverlayWindow>

      {/* Spell search → pick a spell to create a timer trigger from */}
      {pickerTimerType && (
        <SpellSearchPicker
          onPick={handleSpellPicked}
          onClose={() => setPickerTimerType(null)}
        />
      )}

      {/* Create-trigger modal, pre-filled from the picked spell */}
      {pickedSpell && (
        <CreateTriggerModal
          prefill={{
            ...buildSpellTriggerPrefill(pickedSpell.spell),
            timerType: pickedSpell.type,
          }}
          onClose={() => setPickedSpell(null)}
        />
      )}
    </div>
  )
}
