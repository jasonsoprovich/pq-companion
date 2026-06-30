import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Dice5, Trash2, Square, Trophy, ArrowDownAZ, ArrowUpAZ, Circle, X, Timer, Hand, Copy, Check } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import {
  getRolls,
  stopRollSession,
  removeRollSession,
  clearRolls,
  updateRollsSettings,
  setRollItemName,
} from '../services/api'
import type { RollsState, RollSession, WinnerRule, RollProfile } from '../types/rolls'
import {
  winnersFor,
  sortRolls,
  fmtRollTime,
  countdownSeconds,
  buildRollSummary,
  groupContests,
  contestOutcome,
  buildContestSummary,
  type Contest,
} from '../lib/rollHelpers'
import RollProfileControl from '../components/RollProfileControl'
import { WSEvent } from '../lib/wsEvents'

function SessionCard({
  session,
  rule,
  now,
  onStop,
  onRemove,
  onSetItemName,
}: {
  session: RollSession
  rule: WinnerRule
  now: number
  onStop: (id: number) => void
  onRemove: (id: number) => void
  onSetItemName: (id: number, name: string) => void
}): React.ReactElement {
  const winners = useMemo(() => winnersFor(session, rule), [session, rule])
  const orderedRolls = useMemo(() => sortRolls(session.rolls, rule), [session.rolls, rule])
  const remaining = session.active ? countdownSeconds(session, now) : null
  const summary = useMemo(() => buildRollSummary(session, rule), [session, rule])

  // Local draft for the item-name field so typing stays smooth between WS
  // broadcasts. Re-sync whenever the backend value changes (the only other
  // writer is our own commit, which echoes back identically).
  const [nameDraft, setNameDraft] = useState(session.item_name)
  useEffect(() => setNameDraft(session.item_name), [session.item_name])
  const commitName = (): void => {
    const next = nameDraft.trim()
    if (next === session.item_name) return
    onSetItemName(session.id, next)
  }

  const [copied, setCopied] = useState(false)
  const handleCopy = (): void => {
    if (!summary) return
    navigator.clipboard
      ?.writeText(summary)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      })
      .catch(() => {})
  }

  return (
    <div
      className="rounded-lg border"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      <div
        className="flex items-center justify-between gap-3 border-b px-4 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <Dice5 size={16} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
          <div className="min-w-0 flex-1">
            <input
              value={nameDraft}
              onChange={(e) => setNameDraft(e.target.value)}
              onBlur={commitName}
              onKeyDown={(e) => {
                if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
              }}
              placeholder="Add item name…"
              className="w-full bg-transparent text-sm font-semibold outline-none placeholder:font-normal placeholder:text-(--color-muted)"
              style={{ color: 'var(--color-foreground)' }}
              title="Label this roll with the item it's for"
            />
            <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {session.min} – {session.max} · {session.rolls.length} roll{session.rolls.length === 1 ? '' : 's'} ·
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
          <button
            onClick={handleCopy}
            disabled={!summary}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-(--color-surface-3) disabled:opacity-30"
            style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            title="Copy the result to paste in game"
          >
            {copied ? <Check size={11} /> : <Copy size={11} />} {copied ? 'Copied' : 'Copy'}
          </button>
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

function ContestCard({
  contest,
  rule,
  now,
  onStop,
  onRemove,
  onSetItemName,
}: {
  contest: Contest
  rule: WinnerRule
  now: number
  onStop: (id: number) => void
  onRemove: (id: number) => void
  onSetItemName: (id: number, name: string) => void
}): React.ReactElement {
  const outcome = useMemo(() => contestOutcome(contest, rule), [contest, rule])
  const summary = useMemo(() => buildContestSummary(contest, rule), [contest, rule])
  const totalRolls = contest.tiers.reduce((n, t) => n + t.rolls.length, 0)
  // Countdown shown from the soonest-stopping live session in the contest.
  const remaining = contest.active
    ? contest.sessions
        .map((s) => (s.active ? countdownSeconds(s, now) : null))
        .filter((v): v is number => v !== null)
        .sort((a, b) => a - b)[0] ?? null
    : null

  const [nameDraft, setNameDraft] = useState(contest.itemName)
  useEffect(() => setNameDraft(contest.itemName), [contest.itemName])
  const commitName = (): void => {
    const next = nameDraft.trim()
    if (next === contest.itemName) return
    // Apply to every backing session so the label sticks regardless of which
    // tier represents the contest.
    for (const s of contest.sessions) onSetItemName(s.id, next)
  }

  const [copied, setCopied] = useState(false)
  const handleCopy = (): void => {
    if (!summary) return
    navigator.clipboard
      ?.writeText(summary)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      })
      .catch(() => {})
  }

  return (
    <div
      className="rounded-lg border"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      <div
        className="flex items-center justify-between gap-3 border-b px-4 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <Trophy size={16} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
          <div className="min-w-0 flex-1">
            <input
              value={nameDraft}
              onChange={(e) => setNameDraft(e.target.value)}
              onBlur={commitName}
              onKeyDown={(e) => {
                if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
              }}
              placeholder="Add item name…"
              className="w-full bg-transparent text-sm font-semibold outline-none placeholder:font-normal placeholder:text-(--color-muted)"
              style={{ color: 'var(--color-foreground)' }}
            />
            <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {contest.tiers.map((t) => `${t.label} ${t.max}`).join(' · ')} · {totalRolls} roll
              {totalRolls === 1 ? '' : 's'}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {outcome && (
            <span
              className="hidden items-center gap-1 rounded px-2 py-0.5 text-[11px] font-semibold sm:flex"
              style={{ backgroundColor: 'var(--color-surface-3)', color: '#facc15' }}
              title="Winning tier"
            >
              {outcome.tierLabel}
            </span>
          )}
          {remaining !== null && (
            <span
              className="flex items-center gap-1 rounded px-2 py-0.5 text-[11px] font-mono tabular-nums"
              style={{
                backgroundColor: remaining <= 5 ? '#b45309' : 'var(--color-surface-3)',
                color: remaining <= 5 ? 'white' : 'var(--color-foreground)',
              }}
            >
              <Timer size={11} /> {remaining}s
            </span>
          )}
          {contest.active ? (
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
          <button
            onClick={handleCopy}
            disabled={!summary}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-(--color-surface-3) disabled:opacity-30"
            style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            title="Copy the result to paste in game"
          >
            {copied ? <Check size={11} /> : <Copy size={11} />} {copied ? 'Copied' : 'Copy'}
          </button>
          {contest.active && (
            <button
              onClick={() => contest.sessions.forEach((s) => onStop(s.id))}
              className="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-(--color-surface-3)"
              style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
              title="Stop accepting new rolls for this contest"
            >
              <Square size={11} /> Stop
            </button>
          )}
          <button
            onClick={() => contest.sessions.forEach((s) => onRemove(s.id))}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors hover:bg-(--color-surface-3)"
            style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            title="Remove this contest"
          >
            <X size={11} /> Remove
          </button>
        </div>
      </div>

      <div className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
        {contest.tiers.map((tier) => {
          const isWinningTier = outcome?.tierLabel === tier.label
          const ordered = sortRolls(tier.rolls, rule)
          return (
            <div key={tier.label}>
              <div
                className="flex items-center justify-between px-4 py-1 text-[11px] font-semibold uppercase tracking-wider"
                style={{
                  color: isWinningTier ? '#facc15' : 'var(--color-muted)',
                  backgroundColor: 'var(--color-surface-2)',
                }}
              >
                <span>
                  {tier.label} · {tier.max}
                </span>
                <span>
                  {tier.rolls.length} roll{tier.rolls.length === 1 ? '' : 's'}
                </span>
              </div>
              {ordered.length === 0 ? (
                <div className="px-4 py-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>
                  Waiting for rolls…
                </div>
              ) : (
                <ul>
                  {ordered.map((r, idx) => {
                    const isWinner =
                      isWinningTier && !r.duplicate && outcome?.winners.includes(r.roller)
                    return (
                      <li
                        key={`${r.roller}-${r.timestamp}-${idx}`}
                        className="flex items-center justify-between px-4 py-1.5 text-sm"
                        style={{ color: r.duplicate ? 'var(--color-muted)' : 'var(--color-foreground)' }}
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
                        <span
                          className="tabular-nums font-mono"
                          style={{ color: isWinner ? '#facc15' : 'var(--color-foreground)' }}
                        >
                          {r.value}
                        </span>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

export default function RollTrackerPage(): React.ReactElement {
  const [state, setState] = useState<RollsState>({
    sessions: [],
    winner_rule: 'highest',
    mode: 'manual',
    auto_stop_seconds: 45,
    profile: { mode: 'simple' },
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
    if (msg.type === WSEvent.OverlayRolls) setState(msg.data as RollsState)
  })

  const handleStop = (id: number): void => {
    stopRollSession(id).then(setState).catch((e) => setError(String(e)))
  }

  const handleRemove = (id: number): void => {
    removeRollSession(id).catch((e) => setError(String(e)))
  }

  const handleSetItemName = (id: number, name: string): void => {
    setRollItemName(id, name).then(setState).catch((e) => setError(String(e)))
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

  const handleProfile = (profile: RollProfile): void => {
    updateRollsSettings({ profile }).then(setState).catch((e) => setError(String(e)))
  }

  const { contests, ungrouped } = useMemo(
    () => groupContests(state.sessions, state.profile),
    [state.sessions, state.profile],
  )

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

      <RollProfileControl profile={state.profile} onChange={handleProfile} />

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
          {contests.map((c) => (
            <ContestCard
              key={c.key}
              contest={c}
              rule={state.winner_rule}
              now={now}
              onStop={handleStop}
              onRemove={handleRemove}
              onSetItemName={handleSetItemName}
            />
          ))}
          {ungrouped.map((s) => (
            <SessionCard
              key={s.id}
              session={s}
              rule={state.winner_rule}
              now={now}
              onStop={handleStop}
              onRemove={handleRemove}
              onSetItemName={handleSetItemName}
            />
          ))}
        </div>
      )}
    </div>
  )
}
