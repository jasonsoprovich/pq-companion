import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Dice5, Trash2, Square, Trophy, ArrowDownAZ, ArrowUpAZ, Circle, X, Timer, Hand } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import {
  getRolls,
  stopRollSession,
  removeRollSession,
  clearRolls,
  updateRollsSettings,
} from '../services/api'
import type { RollsState, RollSession, WinnerRule } from '../types/rolls'
import { winnersFor, sortRolls, fmtRollTime, countdownSeconds } from '../lib/rollHelpers'

const WS_ROLLS = 'overlay:rolls'

function SessionCard({
  session,
  rule,
  now,
  onStop,
  onRemove,
}: {
  session: RollSession
  rule: WinnerRule
  now: number
  onStop: (id: number) => void
  onRemove: (id: number) => void
}): React.ReactElement {
  const winners = useMemo(() => winnersFor(session, rule), [session, rule])
  const orderedRolls = useMemo(() => sortRolls(session.rolls, rule), [session.rolls, rule])
  const remaining = session.active ? countdownSeconds(session, now) : null

  return (
    <div
      className="rounded-lg border"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      <div
        className="flex items-center justify-between gap-3 border-b px-4 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-3">
          <Dice5 size={16} style={{ color: 'var(--color-primary)' }} />
          <div>
            <div className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
              0 – {session.max}
            </div>
            <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {session.rolls.length} roll{session.rolls.length === 1 ? '' : 's'} ·
              {' '}started {fmtRollTime(session.started_at)}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {remaining !== null && (
            <span
              className="flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-mono tabular-nums"
              style={{
                backgroundColor: remaining <= 5 ? '#b45309' : 'var(--color-surface-3)',
                color: remaining <= 5 ? 'white' : 'var(--color-foreground)',
              }}
              title="Time remaining until this session auto-stops"
            >
              <Timer size={11} /> {remaining}s
            </span>
          )}
          {session.active ? (
            <span
              className="flex items-center gap-1 rounded px-2 py-0.5 text-[10px] font-semibold uppercase"
              style={{ backgroundColor: '#15803d', color: 'white' }}
            >
              <Circle size={8} fill="white" /> Live
            </span>
          ) : (
            <span
              className="rounded px-2 py-0.5 text-[10px] font-semibold uppercase"
              style={{ backgroundColor: 'var(--color-surface-3)', color: 'var(--color-muted)' }}
            >
              Stopped
            </span>
          )}
          {session.active && (
            <button
              onClick={() => onStop(session.id)}
              className="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-(--color-surface-3)"
              style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
              title="Stop accepting new rolls for this range"
            >
              <Square size={11} /> Stop
            </button>
          )}
          <button
            onClick={() => onRemove(session.id)}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-(--color-surface-3)"
            style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            title="Remove this roll session"
          >
            <X size={11} /> Remove
          </button>
        </div>
      </div>

      {orderedRolls.length === 0 ? (
        <div className="px-4 py-3 text-xs" style={{ color: 'var(--color-muted)' }}>
          Waiting for rolls…
        </div>
      ) : (
        <ul className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
          {orderedRolls.map((r, idx) => {
            const isWinner = winners.has(r.roller) && !r.duplicate
            return (
              <li
                key={`${r.roller}-${r.timestamp}-${idx}`}
                className="flex items-center justify-between px-4 py-1.5 text-sm"
                style={{
                  borderColor: 'var(--color-border)',
                  color: r.duplicate ? 'var(--color-muted)' : 'var(--color-foreground)',
                }}
              >
                <div className="flex items-center gap-2">
                  {isWinner && <Trophy size={13} style={{ color: '#facc15' }} />}
                  <span
                    style={{
                      textDecoration: r.duplicate ? 'line-through' : 'none',
                      fontWeight: isWinner ? 600 : 400,
                    }}
                  >
                    {r.roller}
                  </span>
                  {r.duplicate && (
                    <span
                      className="rounded px-1.5 py-0.5 text-[9px] uppercase tracking-wider"
                      style={{ backgroundColor: 'var(--color-surface-3)', color: 'var(--color-muted)' }}
                    >
                      reroll
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    {fmtRollTime(r.timestamp)}
                  </span>
                  <span
                    className="tabular-nums font-mono"
                    style={{ color: isWinner ? '#facc15' : 'var(--color-foreground)' }}
                  >
                    {r.value}
                  </span>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

export default function RollTrackerPage(): React.ReactElement {
  const [state, setState] = useState<RollsState>({
    sessions: [],
    winner_rule: 'highest',
    mode: 'manual',
    auto_stop_seconds: 45,
  })
  const [error, setError] = useState<string | null>(null)
  // 1s tick so countdown badges, "Live" indicators, and the "started Xs
  // ago" copy stay current between WS broadcasts. Cheap — only one
  // setState per second.
  const [now, setNow] = useState(() => Date.now())
  const [durationDraft, setDurationDraft] = useState<string>('')
  const stateRef = useRef(state)
  stateRef.current = state

  useEffect(() => {
    getRolls()
      .then(setState)
      .catch((e) => setError(String(e)))
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])

  // Keep the duration input in sync with backend state when the user
  // isn't actively editing it (empty draft).
  useEffect(() => {
    if (durationDraft === '') setDurationDraft(String(state.auto_stop_seconds))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.auto_stop_seconds])

  useWebSocket((msg) => {
    if (msg.type === WS_ROLLS) setState(msg.data as RollsState)
  })

  const handleStop = (id: number): void => {
    stopRollSession(id).then(setState).catch((e) => setError(String(e)))
  }

  const handleRemove = (id: number): void => {
    removeRollSession(id).catch((e) => setError(String(e)))
  }

  const handleClear = (): void => {
    if (!window.confirm('Clear every roll session?')) return
    clearRolls().catch((e) => setError(String(e)))
  }

  const handleRule = (rule: WinnerRule): void => {
    if (rule === state.winner_rule) return
    updateRollsSettings({ winner_rule: rule }).then(setState).catch((e) => setError(String(e)))
  }

  const handleMode = (mode: 'manual' | 'timer'): void => {
    if (mode === state.mode) return
    updateRollsSettings({ mode }).then(setState).catch((e) => setError(String(e)))
  }

  const commitDuration = (): void => {
    const parsed = parseInt(durationDraft, 10)
    if (Number.isNaN(parsed) || parsed < 5 || parsed > 600) {
      setDurationDraft(String(state.auto_stop_seconds))
      return
    }
    if (parsed === state.auto_stop_seconds) return
    updateRollsSettings({ auto_stop_seconds: parsed })
      .then(setState)
      .catch((e) => setError(String(e)))
  }

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-2">
          <Dice5 size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Roll Tracker
          </h1>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {/* Mode toggle */}
          <div
            className="flex items-center overflow-hidden rounded"
            style={{ border: '1px solid var(--color-border)' }}
          >
            <button
              onClick={() => handleMode('manual')}
              className="flex items-center gap-1 px-2.5 py-1 text-xs transition-colors"
              style={{
                backgroundColor: state.mode === 'manual' ? 'var(--color-primary)' : 'transparent',
                color: state.mode === 'manual' ? 'white' : 'var(--color-foreground)',
              }}
              title="Sessions stay open until you stop them manually"
            >
              <Hand size={11} /> Manual
            </button>
            <button
              onClick={() => handleMode('timer')}
              className="flex items-center gap-1 px-2.5 py-1 text-xs transition-colors"
              style={{
                backgroundColor: state.mode === 'timer' ? 'var(--color-primary)' : 'transparent',
                color: state.mode === 'timer' ? 'white' : 'var(--color-foreground)',
              }}
              title="Auto-stop each session after the configured number of seconds"
            >
              <Timer size={11} /> Timer
            </button>
          </div>

          {/* Duration input — only meaningful when mode=timer, but always
              shown so the user sees the value they'll get when they toggle. */}
          <label
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{
              border: '1px solid var(--color-border)',
              opacity: state.mode === 'timer' ? 1 : 0.6,
            }}
            title="Auto-stop window in seconds (5 – 600)"
          >
            <span style={{ color: 'var(--color-muted)' }}>Window</span>
            <input
              type="number"
              min={5}
              max={600}
              value={durationDraft}
              onChange={(e) => setDurationDraft(e.target.value)}
              onBlur={commitDuration}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  ;(e.target as HTMLInputElement).blur()
                }
              }}
              className="w-12 bg-transparent text-right tabular-nums font-mono outline-none"
              style={{ color: 'var(--color-foreground)' }}
            />
            <span style={{ color: 'var(--color-muted)' }}>s</span>
          </label>

          {/* Winner rule */}
          <div
            className="flex items-center overflow-hidden rounded"
            style={{ border: '1px solid var(--color-border)' }}
          >
            <button
              onClick={() => handleRule('highest')}
              className="flex items-center gap-1 px-2.5 py-1 text-xs transition-colors"
              style={{
                backgroundColor: state.winner_rule === 'highest' ? 'var(--color-primary)' : 'transparent',
                color: state.winner_rule === 'highest' ? 'white' : 'var(--color-foreground)',
              }}
              title="Highest roll wins"
            >
              <ArrowUpAZ size={11} /> Highest
            </button>
            <button
              onClick={() => handleRule('lowest')}
              className="flex items-center gap-1 px-2.5 py-1 text-xs transition-colors"
              style={{
                backgroundColor: state.winner_rule === 'lowest' ? 'var(--color-primary)' : 'transparent',
                color: state.winner_rule === 'lowest' ? 'white' : 'var(--color-foreground)',
              }}
              title="Lowest roll wins"
            >
              <ArrowDownAZ size={11} /> Lowest
            </button>
          </div>

          <button
            onClick={handleClear}
            disabled={state.sessions.length === 0}
            className="flex items-center gap-1 rounded px-2.5 py-1 text-xs transition-colors hover:bg-(--color-surface-3) disabled:opacity-30"
            style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            title="Clear all sessions"
          >
            <Trash2 size={11} /> Clear
          </button>
        </div>
      </div>

      {error && (
        <div
          className="rounded px-3 py-2 text-xs"
          style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316' }}
        >
          {error}
        </div>
      )}

      {state.sessions.length === 0 ? (
        <div
          className="flex flex-1 items-center justify-center rounded border text-sm"
          style={{
            borderColor: 'var(--color-border)',
            color: 'var(--color-muted)',
            backgroundColor: 'var(--color-surface)',
          }}
        >
          No rolls yet — waiting for `/random` results to appear in the log.
        </div>
      ) : (
        <div className="flex flex-col gap-3 overflow-y-auto">
          {state.sessions.map((s) => (
            <SessionCard
              key={s.id}
              session={s}
              rule={state.winner_rule}
              now={now}
              onStop={handleStop}
              onRemove={handleRemove}
            />
          ))}
        </div>
      )}
    </div>
  )
}
