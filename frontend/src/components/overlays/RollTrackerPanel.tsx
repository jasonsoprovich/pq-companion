import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Dice5,
  Trash2,
  Square,
  Trophy,
  ArrowDownAZ,
  ArrowUpAZ,
  Circle,
  X,
  Timer,
  Hand,
  ExternalLink,
  Copy,
  Check,
} from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import {
  getRolls,
  stopRollSession,
  removeRollSession,
  clearRolls,
  updateRollsSettings,
  setRollItemName,
} from '../../services/api'
import type { RollsState, RollSession, WinnerRule, RollMode, RollProfile } from '../../types/rolls'
import {
  winnersFor,
  sortRolls,
  countdownSeconds,
  buildRollSummary,
  groupContests,
  contestOutcome,
  buildContestSummary,
  type Contest,
} from '../../lib/rollHelpers'
import { ROLL_PROFILE_PRESETS, presetIdFor } from '../../lib/rollProfilePresets'
import OverlayWindow from '../OverlayWindow'

interface RollTrackerPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
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
      style={{
        borderBottom: '1px solid var(--color-border)',
        padding: '4px 8px 6px',
        flexShrink: 0,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, minWidth: 0, flex: 1 }}>
          <Dice5 size={12} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
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
              color: 'var(--color-foreground)',
              fontSize: 12,
              fontWeight: 600,
            }}
          />
          <span style={{ fontSize: 10, color: 'var(--color-muted)', flexShrink: 0 }}>
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
                backgroundColor: remaining <= 5 ? '#b45309' : 'var(--color-surface-3)',
                color: remaining <= 5 ? 'white' : 'var(--color-foreground)',
              }}
              title="Auto-stop countdown"
            >
              {remaining}s
            </span>
          )}
          {session.active ? (
            <span
              title="Live"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                width: 8,
                height: 8,
                borderRadius: '50%',
                backgroundColor: '#15803d',
              }}
            />
          ) : (
            <span style={{ fontSize: 9, color: 'var(--color-muted)', textTransform: 'uppercase' }}>
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
              color: copied ? '#22c55e' : 'var(--color-muted)',
              opacity: summary ? 1 : 0.3,
            }}
          >
            {copied ? <Check size={11} /> : <Copy size={11} />}
          </button>
          {session.active && (
            <button
              onClick={() => onStop(session.id)}
              title="Stop this session"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'var(--color-muted)' }}
            >
              <Square size={11} />
            </button>
          )}
          <button
            onClick={() => onRemove(session.id)}
            title="Remove this session"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'var(--color-muted)' }}
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
                  color: r.duplicate ? 'var(--color-muted)' : 'var(--color-foreground)',
                  paddingLeft: 4,
                }}
              >
                <span
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    textDecoration: r.duplicate ? 'line-through' : 'none',
                    fontWeight: isWinner ? 600 : 400,
                  }}
                >
                  {isWinner && <Trophy size={10} style={{ color: '#facc15' }} />}
                  {r.roller}
                </span>
                <span
                  style={{
                    fontFamily: 'monospace',
                    color: isWinner ? '#facc15' : 'inherit',
                    fontWeight: isWinner ? 600 : 400,
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

  return (
    <div style={{ borderBottom: '1px solid var(--color-border)', padding: '4px 8px 6px', flexShrink: 0 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 6 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, minWidth: 0, flex: 1 }}>
          <Trophy size={12} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
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
              color: 'var(--color-foreground)',
              fontSize: 12,
              fontWeight: 600,
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
              color: copied ? '#22c55e' : 'var(--color-muted)',
              opacity: summary ? 1 : 0.3,
            }}
          >
            {copied ? <Check size={11} /> : <Copy size={11} />}
          </button>
          {contest.active && (
            <button
              onClick={() => contest.sessions.forEach((s) => onStop(s.id))}
              title="Stop this contest"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'var(--color-muted)' }}
            >
              <Square size={11} />
            </button>
          )}
          <button
            onClick={() => contest.sessions.forEach((s) => onRemove(s.id))}
            title="Remove this contest"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 2px', color: 'var(--color-muted)' }}
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
                color: isWinningTier ? '#facc15' : 'var(--color-muted)',
                paddingLeft: 4,
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
                    color: r.duplicate ? 'var(--color-muted)' : 'var(--color-foreground)',
                    paddingLeft: 10,
                  }}
                >
                  <span
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 4,
                      textDecoration: r.duplicate ? 'line-through' : 'none',
                      fontWeight: isWinner ? 600 : 400,
                    }}
                  >
                    {isWinner && <Trophy size={10} style={{ color: '#facc15' }} />}
                    {r.roller}
                  </span>
                  <span
                    style={{
                      fontFamily: 'monospace',
                      color: isWinner ? '#facc15' : 'inherit',
                      fontWeight: isWinner ? 600 : 400,
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

export default function RollTrackerPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 300,
  defaultHeight = 380,
  snapGridSize,
  onLayoutChange,
}: RollTrackerPanelProps): React.ReactElement {
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
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])

  useEffect(() => {
    if (durationDraft === '') setDurationDraft(String(state.auto_stop_seconds))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.auto_stop_seconds])

  const wsState = useWebSocket(
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
  // Overlays switch between built-in presets; full tier editing lives on the
  // main Roll Tracker page. "Custom" is shown read-only when active.
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
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Dice5 size={13} style={{ color: 'var(--color-primary)' }} />
          Rolls
        </span>
      }
      headerRight={
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
          {/* Mode toggle — compact icon-only buttons. The active one is
              highlighted. Tooltips explain each choice in plain language. */}
          <div style={{ display: 'flex', border: '1px solid var(--color-border)', borderRadius: 3, overflow: 'hidden' }}>
            <button
              onClick={() => handleMode('manual')}
              title="Manual: sessions stay open until you stop them"
              style={{
                background: state.mode === 'manual' ? 'var(--color-primary)' : 'transparent',
                color: state.mode === 'manual' ? 'white' : 'var(--color-muted)',
                border: 'none', cursor: 'pointer', padding: '2px 5px', display: 'flex', alignItems: 'center',
              }}
            >
              <Hand size={11} />
            </button>
            <button
              onClick={() => handleMode('timer')}
              title="Timer: auto-stop each session after the configured window"
              style={{
                background: state.mode === 'timer' ? 'var(--color-primary)' : 'transparent',
                color: state.mode === 'timer' ? 'white' : 'var(--color-muted)',
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
              border: '1px solid var(--color-border)', borderRadius: 3,
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
                color: 'var(--color-foreground)', fontFamily: 'monospace', textAlign: 'right', fontSize: 11,
              }}
            />
            <span style={{ fontSize: 10, color: 'var(--color-muted)' }}>s</span>
          </label>

          <div style={{ display: 'flex', border: '1px solid var(--color-border)', borderRadius: 3, overflow: 'hidden' }}>
            <button
              onClick={() => handleRule('highest')}
              title="Highest roll wins"
              style={{
                background: state.winner_rule === 'highest' ? 'var(--color-primary)' : 'transparent',
                color: state.winner_rule === 'highest' ? 'white' : 'var(--color-muted)',
                border: 'none', cursor: 'pointer', padding: '2px 5px', display: 'flex', alignItems: 'center',
              }}
            >
              <ArrowUpAZ size={11} />
            </button>
            <button
              onClick={() => handleRule('lowest')}
              title="Lowest roll wins"
              style={{
                background: state.winner_rule === 'lowest' ? 'var(--color-primary)' : 'transparent',
                color: state.winner_rule === 'lowest' ? 'white' : 'var(--color-muted)',
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
              background: 'var(--color-surface)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
              borderRadius: 3,
              fontSize: 10,
              padding: '1px 2px',
              maxWidth: 110,
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
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
          >
            <Trash2 size={12} />
          </button>
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleRollTracker?.()}
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
      minWidth={260}
      minHeight={160}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {state.sessions.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <Dice5 size={28} style={{ opacity: 0.2, color: 'var(--color-primary)' }} />
            <p style={{ fontSize: 12, margin: 0, textAlign: 'center' }}>
              No rolls yet — waiting for /random results.
            </p>
            <span style={{ fontSize: 10, opacity: 0.6 }}>
              <Circle size={8} style={{ display: 'inline', marginRight: 3 }} />
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
    </OverlayWindow>
  )
}
