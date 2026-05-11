import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Dice5, Trash2, Square, Trophy, ArrowDownAZ, ArrowUpAZ, Circle, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getRolls, stopRollSession, removeRollSession, clearRolls, setRollWinnerRule } from '../services/api'
import type { RollsState, RollSession, WinnerRule, Roll } from '../types/rolls'

const WS_ROLLS = 'overlay:rolls'

function fmtTime(iso: string): string {
  if (!iso) return ''
  try {
    return new Date(iso).toLocaleTimeString()
  } catch {
    return iso
  }
}

function winnersFor(session: RollSession, rule: WinnerRule): Set<string> {
  // Pick the winning value among each player's first roll (duplicates are
  // ignored). Multiple players tied on the winning value all return as
  // co-winners so the user can resolve the tie however they like.
  const firstByPlayer = new Map<string, Roll>()
  for (const r of session.rolls) {
    if (!firstByPlayer.has(r.roller)) firstByPlayer.set(r.roller, r)
  }
  if (firstByPlayer.size === 0) return new Set()
  const values = [...firstByPlayer.values()].map((r) => r.value)
  const target = rule === 'highest' ? Math.max(...values) : Math.min(...values)
  const winners = new Set<string>()
  for (const r of firstByPlayer.values()) {
    if (r.value === target) winners.add(r.roller)
  }
  return winners
}

function SessionCard({
  session,
  rule,
  onStop,
  onRemove,
}: {
  session: RollSession
  rule: WinnerRule
  onStop: (id: number) => void
  onRemove: (id: number) => void
}): React.ReactElement {
  const winners = useMemo(() => winnersFor(session, rule), [session, rule])

  // Display in winner-first order while the session is being watched —
  // makes the leader pop without forcing the user to scan a long list.
  const orderedRolls = useMemo(() => {
    const rolls = [...session.rolls]
    rolls.sort((a, b) => (rule === 'highest' ? b.value - a.value : a.value - b.value))
    return rolls
  }, [session.rolls, rule])

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
              {' '}started {fmtTime(session.started_at)}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
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
                    {fmtTime(r.timestamp)}
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
  const [state, setState] = useState<RollsState>({ sessions: [], winner_rule: 'highest' })
  const [error, setError] = useState<string | null>(null)
  // Re-render every 30s so the "started Xs ago" text and Live/Stopped
  // ordering by recency stays accurate while no new events arrive.
  const [, setTick] = useState(0)
  const stateRef = useRef(state)
  stateRef.current = state

  useEffect(() => {
    getRolls()
      .then(setState)
      .catch((e) => setError(String(e)))
    const id = setInterval(() => setTick((t) => t + 1), 30000)
    return () => clearInterval(id)
  }, [])

  useWebSocket((msg) => {
    if (msg.type === WS_ROLLS) {
      setState(msg.data as RollsState)
    }
  })

  const handleStop = (id: number): void => {
    stopRollSession(id).then(setState).catch((e) => setError(String(e)))
  }

  const handleRemove = (id: number): void => {
    // No confirm prompt — the WebSocket broadcast will refresh state, and
    // removing a single mis-tracked roll set should feel snappy. Users can
    // always re-create a session by rolling again on the same range.
    removeRollSession(id).catch((e) => setError(String(e)))
  }

  const handleClear = (): void => {
    if (!window.confirm('Clear every roll session?')) return
    clearRolls()
      .then(() => setState({ sessions: [], winner_rule: stateRef.current.winner_rule }))
      .catch((e) => setError(String(e)))
  }

  const handleRule = (rule: WinnerRule): void => {
    if (rule === state.winner_rule) return
    setRollWinnerRule(rule).then(setState).catch((e) => setError(String(e)))
  }

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Dice5 size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Roll Tracker
          </h1>
        </div>
        <div className="flex items-center gap-2">
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
              onStop={handleStop}
              onRemove={handleRemove}
            />
          ))}
        </div>
      )}
    </div>
  )
}
