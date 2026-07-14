/**
 * RollTrackerWindowPage — transparent always-on-top overlay showing live
 * /random sessions. Renders in a dedicated frameless Electron window;
 * no sidebar or title bar. Header has the manual/timer mode toggle,
 * the auto-stop window length input, and the winner-rule toggle so an
 * officer can drive a roll-call without leaving the overlay.
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Dice5,
  Trash2,
  Square,
  Trophy,
  ArrowDownAZ,
  ArrowUpAZ,
  X,
  Timer,
  Hand,
  Copy,
  Check,
  ListOrdered,
} from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import {
  getRolls,
  stopRollSession,
  removeRollSession,
  clearRolls,
  updateRollsSettings,
  setRollItemName,
} from '../services/api'
import type { RollsState, RollSession, WinnerRule, RollMode } from '../types/rolls'
import {
  winnersFor,
  sortRolls,
  countdownSeconds,
  buildRollSummary,
  buildPickOrderSummary,
  groupContests,
  contestOutcome,
  buildContestSummary,
  buildContestPickOrderSummary,
  type Contest,
} from '../lib/rollHelpers'
import { ROLL_PROFILE_PRESETS, presetIdFor } from '../lib/rollProfilePresets'
import { WSEvent } from '../lib/wsEvents'

function SessionRow({
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
  const ordered = useMemo(() => sortRolls(session.rolls, rule), [session.rolls, rule])
  const remaining = session.active ? countdownSeconds(session, now) : null
  const summary = useMemo(() => buildRollSummary(session, rule), [session, rule])
  const pickOrderSummary = useMemo(() => buildPickOrderSummary(session, rule), [session, rule])

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

  const [pickOrderCopied, setPickOrderCopied] = useState(false)
  const handleCopyPickOrder = (): void => {
    if (!pickOrderSummary) return
    navigator.clipboard
      ?.writeText(pickOrderSummary)
      .then(() => {
        setPickOrderCopied(true)
        setTimeout(() => setPickOrderCopied(false), 1500)
      })
      .catch(() => {})
  }

  return (
    <div
      style={{
        padding: '4px 8px 6px',
        borderBottom: '1px solid rgba(255,255,255,0.1)',
        flexShrink: 0,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, minWidth: 0, flex: 1 }}>
          <Dice5 size={12} style={{ color: '#a5b4fc', flexShrink: 0 }} />
          <input
            value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            onBlur={commitName}
            onKeyDown={(e) => {
              if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
            }}
            placeholder="Item name…"
            title="Label this roll with the item it's for"
            style={{
              flex: 1,
              minWidth: 0,
              background: 'transparent',
              border: 'none',
              outline: 'none',
              color: 'rgba(255,255,255,0.95)',
              fontSize: 12,
              fontWeight: 700,
              textShadow: '0 1px 2px rgba(0,0,0,0.9)',
            }}
          />
          <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.45)', flexShrink: 0 }}>
            {session.min}–{session.max} ({session.rolls.length})
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          {remaining !== null && (
            <span
              style={{
                fontFamily: 'monospace',
                fontSize: 10,
                padding: '1px 5px',
                borderRadius: 3,
                backgroundColor: remaining <= 5 ? '#b45309' : 'rgba(255,255,255,0.12)',
                color: remaining <= 5 ? 'white' : 'rgba(255,255,255,0.85)',
                textShadow: '0 1px 2px rgba(0,0,0,0.9)',
              }}
            >
              {remaining}s
            </span>
          )}
          {session.active ? (
            <span
              style={{ display: 'inline-block', width: 8, height: 8, borderRadius: '50%', backgroundColor: '#22c55e' }}
              title="Live"
            />
          ) : (
            <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase' }}>
              Stopped
            </span>
          )}
          <button
            onClick={handleCopy}
            disabled={!summary}
            title="Copy result to paste in game"
            style={{
              background: 'none',
              border: 'none',
              cursor: summary ? 'pointer' : 'default',
              padding: '1px 2px',
              color: copied ? '#22c55e' : 'rgba(255,255,255,0.6)',
              opacity: summary ? 1 : 0.3,
            }}
          >
            {copied ? <Check size={11} /> : <Copy size={11} />}
          </button>
          <button
            onClick={handleCopyPickOrder}
            disabled={!pickOrderSummary}
            title="Copy full pick order to paste in game"
            style={{
              background: 'none',
              border: 'none',
              cursor: pickOrderSummary ? 'pointer' : 'default',
              padding: '1px 2px',
              color: pickOrderCopied ? '#22c55e' : 'rgba(255,255,255,0.6)',
              opacity: pickOrderSummary ? 1 : 0.3,
            }}
          >
            {pickOrderCopied ? <Check size={11} /> : <ListOrdered size={11} />}
          </button>
          {session.active && (
            <button
              onClick={() => onStop(session.id)}
              title="Stop this session"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'rgba(255,255,255,0.6)' }}
            >
              <Square size={11} />
            </button>
          )}
          <button
            onClick={() => onRemove(session.id)}
            title="Remove this session"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'rgba(255,255,255,0.6)' }}
          >
            <X size={11} />
          </button>
        </div>
      </div>
      {ordered.length > 0 && (
        <div style={{ marginTop: 3, display: 'flex', flexDirection: 'column', gap: 1 }}>
          {ordered.map((r, idx) => {
            const isWinner = winners.has(r.roller) && !r.duplicate
            return (
              <div
                key={`${r.roller}-${r.timestamp}-${idx}`}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  fontSize: 11,
                  color: r.duplicate ? 'rgba(255,255,255,0.4)' : 'rgba(255,255,255,0.9)',
                  paddingLeft: 4,
                  textShadow: '0 1px 2px rgba(0,0,0,0.85)',
                }}
              >
                <span
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    textDecoration: r.duplicate ? 'line-through' : 'none',
                    fontWeight: isWinner ? 700 : 500,
                  }}
                >
                  {isWinner && <Trophy size={10} style={{ color: '#facc15' }} />}
                  {r.roller}
                </span>
                <span
                  style={{
                    fontFamily: 'monospace',
                    color: isWinner ? '#facc15' : 'inherit',
                    fontWeight: isWinner ? 700 : 500,
                  }}
                >
                  {r.value}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function ContestRow({
  contest,
  rule,
  onStop,
  onRemove,
  onSetItemName,
}: {
  contest: Contest
  rule: WinnerRule
  onStop: (id: number) => void
  onRemove: (id: number) => void
  onSetItemName: (id: number, name: string) => void
}): React.ReactElement {
  const outcome = useMemo(() => contestOutcome(contest, rule), [contest, rule])
  const summary = useMemo(() => buildContestSummary(contest, rule), [contest, rule])
  const pickOrderSummary = useMemo(() => buildContestPickOrderSummary(contest, rule), [contest, rule])

  const [nameDraft, setNameDraft] = useState(contest.itemName)
  useEffect(() => setNameDraft(contest.itemName), [contest.itemName])
  const commitName = (): void => {
    const next = nameDraft.trim()
    if (next === contest.itemName) return
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

  const [pickOrderCopied, setPickOrderCopied] = useState(false)
  const handleCopyPickOrder = (): void => {
    if (!pickOrderSummary) return
    navigator.clipboard
      ?.writeText(pickOrderSummary)
      .then(() => {
        setPickOrderCopied(true)
        setTimeout(() => setPickOrderCopied(false), 1500)
      })
      .catch(() => {})
  }

  return (
    <div style={{ padding: '4px 8px 6px', borderBottom: '1px solid rgba(255,255,255,0.1)', flexShrink: 0 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, minWidth: 0, flex: 1 }}>
          <Trophy size={12} style={{ color: '#a5b4fc', flexShrink: 0 }} />
          <input
            value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            onBlur={commitName}
            onKeyDown={(e) => {
              if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
            }}
            placeholder="Item name…"
            style={{
              flex: 1,
              minWidth: 0,
              background: 'transparent',
              border: 'none',
              outline: 'none',
              color: 'rgba(255,255,255,0.95)',
              fontSize: 12,
              fontWeight: 700,
              textShadow: '0 1px 2px rgba(0,0,0,0.9)',
            }}
          />
          {outcome && (
            <span style={{ fontSize: 9, color: '#facc15', textTransform: 'uppercase', flexShrink: 0 }}>
              {outcome.tierLabel}
            </span>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          <button
            onClick={handleCopy}
            disabled={!summary}
            title="Copy result to paste in game"
            style={{
              background: 'none',
              border: 'none',
              cursor: summary ? 'pointer' : 'default',
              padding: '1px 2px',
              color: copied ? '#22c55e' : 'rgba(255,255,255,0.6)',
              opacity: summary ? 1 : 0.3,
            }}
          >
            {copied ? <Check size={11} /> : <Copy size={11} />}
          </button>
          <button
            onClick={handleCopyPickOrder}
            disabled={!pickOrderSummary}
            title="Copy full pick order to paste in game"
            style={{
              background: 'none',
              border: 'none',
              cursor: pickOrderSummary ? 'pointer' : 'default',
              padding: '1px 2px',
              color: pickOrderCopied ? '#22c55e' : 'rgba(255,255,255,0.6)',
              opacity: pickOrderSummary ? 1 : 0.3,
            }}
          >
            {pickOrderCopied ? <Check size={11} /> : <ListOrdered size={11} />}
          </button>
          {contest.active && (
            <button
              onClick={() => contest.sessions.forEach((s) => onStop(s.id))}
              title="Stop this contest"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'rgba(255,255,255,0.6)' }}
            >
              <Square size={11} />
            </button>
          )}
          <button
            onClick={() => contest.sessions.forEach((s) => onRemove(s.id))}
            title="Remove this contest"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'rgba(255,255,255,0.6)' }}
          >
            <X size={11} />
          </button>
        </div>
      </div>
      {contest.tiers.map((tier) => {
        const isWinningTier = outcome?.tierLabel === tier.label
        const ordered = sortRolls(tier.rolls, rule)
        return (
          <div key={tier.label} style={{ marginTop: 2 }}>
            <div
              style={{
                fontSize: 9,
                textTransform: 'uppercase',
                letterSpacing: 0.4,
                color: isWinningTier ? '#facc15' : 'rgba(255,255,255,0.45)',
                paddingLeft: 4,
                textShadow: '0 1px 2px rgba(0,0,0,0.85)',
              }}
            >
              {tier.label} · {tier.max}
            </div>
            {ordered.map((r, idx) => {
              const isWinner = isWinningTier && !r.duplicate && outcome?.winners.includes(r.roller)
              return (
                <div
                  key={`${r.roller}-${r.timestamp}-${idx}`}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    fontSize: 11,
                    color: r.duplicate ? 'rgba(255,255,255,0.4)' : 'rgba(255,255,255,0.9)',
                    paddingLeft: 10,
                    textShadow: '0 1px 2px rgba(0,0,0,0.85)',
                  }}
                >
                  <span
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                      textDecoration: r.duplicate ? 'line-through' : 'none',
                      fontWeight: isWinner ? 700 : 500,
                    }}
                  >
                    {isWinner && <Trophy size={10} style={{ color: '#facc15' }} />}
                    {r.roller}
                  </span>
                  <span
                    style={{
                      fontFamily: 'monospace',
                      color: isWinner ? '#facc15' : 'inherit',
                      fontWeight: isWinner ? 700 : 500,
                    }}
                  >
                    {r.value}
                  </span>
                </div>
              )
            })}
          </div>
        )
      })}
    </div>
  )
}

export default function RollTrackerWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('rollTracker')
  const onDragMouseDown = useWindowDrag()
  const [state, setState] = useState<RollsState>({
    sessions: [],
    winner_rule: 'highest',
    mode: 'manual',
    auto_stop_seconds: 45,
    profile: { mode: 'simple' },
  })
  const [now, setNow] = useState(() => Date.now())
  const [durationDraft, setDurationDraft] = useState<string>('')

  useEffect(() => {
    getRolls().then(setState).catch(() => {})
  }, [])

  // Tick for live countdowns only while a session is active — no point
  // re-rendering once a second with nothing counting down.
  const hasActiveSession = state.sessions?.some((s) => s.active) ?? false
  useEffect(() => {
    if (!hasActiveSession) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [hasActiveSession])

  useEffect(() => {
    if (durationDraft === '') setDurationDraft(String(state.auto_stop_seconds))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.auto_stop_seconds])

  useWebSocket(
    useCallback((msg: { type: string; data: unknown }) => {
      if (msg.type === WSEvent.OverlayRolls) setState(msg.data as RollsState)
    }, []),
  )

  const handleStop = (id: number): void => { stopRollSession(id).then(setState).catch(() => {}) }
  const handleRemove = (id: number): void => { removeRollSession(id).catch(() => {}) }
  const handleSetItemName = (id: number, name: string): void => {
    setRollItemName(id, name).then(setState).catch(() => {})
  }
  const handleRule = (rule: WinnerRule): void => {
    if (rule === state.winner_rule) return
    updateRollsSettings({ winner_rule: rule }).then(setState).catch(() => {})
  }
  const handleMode = (mode: RollMode): void => {
    if (mode === state.mode) return
    updateRollsSettings({ mode }).then(setState).catch(() => {})
  }
  const handlePreset = (id: string): void => {
    const preset = ROLL_PROFILE_PRESETS.find((p) => p.id === id)
    if (preset) updateRollsSettings({ profile: preset.profile }).then(setState).catch(() => {})
  }
  const { contests, ungrouped } = useMemo(
    () => groupContests(state.sessions, state.profile),
    [state.sessions, state.profile],
  )
  const presetId = presetIdFor(state.profile)
  const commitDuration = (): void => {
    const parsed = parseInt(durationDraft, 10)
    if (Number.isNaN(parsed) || parsed < 5 || parsed > 600) {
      setDurationDraft(String(state.auto_stop_seconds))
      return
    }
    if (parsed === state.auto_stop_seconds) return
    updateRollsSettings({ auto_stop_seconds: parsed }).then(setState).catch(() => {})
  }

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        border: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
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
          gap: 8,
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          flexWrap: 'wrap',
          // Fade-when-inactive: hide the title bar with the rest of the
          // chrome; pointerEvents off so invisible buttons can't be clicked.
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Dice5 size={11} style={{ color: '#a5b4fc' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Rolls
          </span>
          {state.sessions.length > 0 && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {state.sessions.length}
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', border: '1px solid rgba(255,255,255,0.15)', borderRadius: 3, overflow: 'hidden' }}>
            <button
              onClick={() => handleMode('manual')}
              title="Manual: sessions stay open until you stop them"
              style={{
                background: state.mode === 'manual' ? 'rgba(99,102,241,0.65)' : 'transparent',
                color: state.mode === 'manual' ? 'white' : 'rgba(255,255,255,0.55)',
                border: 'none', cursor: 'pointer', padding: '2px 5px', display: 'flex', alignItems: 'center',
              }}
            >
              <Hand size={11} />
            </button>
            <button
              onClick={() => handleMode('timer')}
              title="Timer: auto-stop each session after the configured window"
              style={{
                background: state.mode === 'timer' ? 'rgba(99,102,241,0.65)' : 'transparent',
                color: state.mode === 'timer' ? 'white' : 'rgba(255,255,255,0.55)',
                border: 'none', cursor: 'pointer', padding: '2px 5px', display: 'flex', alignItems: 'center',
              }}
            >
              <Timer size={11} />
            </button>
          </div>
          <label
            title="Auto-stop window in seconds (5 – 600)"
            style={{
              display: 'flex', alignItems: 'center', gap: 2,
              border: '1px solid rgba(255,255,255,0.15)', borderRadius: 3,
              padding: '0 4px', height: 18,
              opacity: state.mode === 'timer' ? 1 : 0.55,
            }}
          >
            <input
              type="number"
              min={5}
              max={600}
              value={durationDraft}
              onChange={(e) => setDurationDraft(e.target.value)}
              onBlur={commitDuration}
              onKeyDown={(e) => {
                if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
              }}
              style={{
                width: 28, background: 'transparent', border: 'none', outline: 'none',
                color: 'rgba(255,255,255,0.95)', fontFamily: 'monospace', textAlign: 'right', fontSize: 11,
              }}
            />
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.5)' }}>s</span>
          </label>
          <div style={{ display: 'flex', border: '1px solid rgba(255,255,255,0.15)', borderRadius: 3, overflow: 'hidden' }}>
            <button
              onClick={() => handleRule('highest')}
              title="Highest roll wins"
              style={{
                background: state.winner_rule === 'highest' ? 'rgba(99,102,241,0.65)' : 'transparent',
                color: state.winner_rule === 'highest' ? 'white' : 'rgba(255,255,255,0.55)',
                border: 'none', cursor: 'pointer', padding: '2px 5px', display: 'flex', alignItems: 'center',
              }}
            >
              <ArrowUpAZ size={11} />
            </button>
            <button
              onClick={() => handleRule('lowest')}
              title="Lowest roll wins"
              style={{
                background: state.winner_rule === 'lowest' ? 'rgba(99,102,241,0.65)' : 'transparent',
                color: state.winner_rule === 'lowest' ? 'white' : 'rgba(255,255,255,0.55)',
                border: 'none', cursor: 'pointer', padding: '2px 5px', display: 'flex', alignItems: 'center',
              }}
            >
              <ArrowDownAZ size={11} />
            </button>
          </div>
          <select
            value={presetId}
            onChange={(e) => handlePreset(e.target.value)}
            disabled={presetId === 'custom'}
            title="Grouping — edit Custom on the Roll Tracker page"
            style={{
              background: 'rgba(20,20,24,0.9)',
              color: 'rgba(255,255,255,0.85)',
              border: '1px solid rgba(255,255,255,0.15)',
              borderRadius: 3,
              fontSize: 10,
              padding: '1px 2px',
              maxWidth: 104,
            }}
          >
            {ROLL_PROFILE_PRESETS.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
            {presetId === 'custom' && <option value="custom">Custom</option>}
          </select>
          <button
            onClick={() => { clearRolls().catch(() => {}) }}
            title="Clear all sessions"
            style={{
              display: 'flex', alignItems: 'center', padding: '1px 5px', borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)', backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)', cursor: 'pointer', lineHeight: 1,
            }}
          >
            <Trash2 size={11} />
          </button>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeRollTracker?.()}
            style={{
              fontSize: 11, lineHeight: 1, padding: '1px 5px', borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)', backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)', cursor: 'pointer',
            }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {/* display:flex + column so the empty-state child's flex:1 can stretch
          to fill — without this the icon/label clings to the top edge instead
          of centering. */}
      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {state.sessions.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 6, padding: 16, opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease' }}>
            <Dice5 size={22} style={{ opacity: 0.15, color: '#a5b4fc' }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>
              No rolls yet
            </p>
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.25)' }}>
              {state.mode === 'timer' ? `${state.auto_stop_seconds}s auto-stop` : 'Manual stop'}
            </span>
          </div>
        ) : (
          <>
            {contests.map((c) => (
              <ContestRow
                key={c.key}
                contest={c}
                rule={state.winner_rule}
                onStop={handleStop}
                onRemove={handleRemove}
                onSetItemName={handleSetItemName}
              />
            ))}
            {ungrouped.map((s) => (
              <SessionRow
                key={s.id}
                session={s}
                rule={state.winner_rule}
                now={now}
                onStop={handleStop}
                onRemove={handleRemove}
                onSetItemName={handleSetItemName}
              />
            ))}
          </>
        )}
      </div>
    </div>
  )
}
