import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Play, Square, RotateCcw, RefreshCw, Zap, Clock } from 'lucide-react'
import {
  runTriggerTest,
  stopTriggerTest,
  getTriggerTestStatus,
  type Character,
} from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { TestLineResult, TestLineStatus, TestMatch } from '../types/trigger'

const TEXT_STORAGE_KEY = 'triggers.testerText'
const PLACEHOLDER =
  '[Fri Feb 03 20:27:50 2023] Paste lines from your log here and click Run.\n' +
  '[Fri Feb 03 20:27:52 2023] A gnoll goes on a RAMPAGE!'

function loadSavedText(): string {
  try {
    return localStorage.getItem(TEXT_STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

function saveText(text: string): void {
  try {
    localStorage.setItem(TEXT_STORAGE_KEY, text)
  } catch {
    // Private window / blocked storage — the paste box just won't persist.
  }
}

const STATUS_LABEL: Record<TestLineStatus, string> = {
  matched: 'Matched',
  excluded: 'Excluded',
  cooldown: 'Cooldown',
  wrong_character: 'Wrong character',
}

function statusColor(status: TestLineStatus): string {
  switch (status) {
    case 'matched':
      return 'var(--color-success)'
    case 'excluded':
      return 'var(--color-muted)'
    case 'cooldown':
      return 'var(--color-warning)'
    case 'wrong_character':
      return 'var(--color-muted)'
  }
}

interface TestSummary {
  lines: number
  fired: number
  matched: number
  excluded: number
  cooldowns: number
}

function summarize(lines: TestLineResult[]): TestSummary {
  const s: TestSummary = { lines: lines.length, fired: 0, matched: 0, excluded: 0, cooldowns: 0 }
  for (const line of lines) {
    for (const m of line.matches) {
      if (m.fired) s.fired++
      switch (m.status) {
        case 'matched':
          s.matched++
          break
        case 'excluded':
          s.excluded++
          break
        case 'cooldown':
          s.cooldowns++
          break
      }
    }
  }
  return s
}

export function MatchRow({
  match,
  onOpenTrigger,
}: {
  match: TestMatch
  onOpenTrigger: (name: string) => void
}): React.ReactElement {
  const captureEntries = match.captures
    ? Object.entries(match.captures).filter(([k]) => k !== '0')
    : []
  return (
    <div
      className="rounded px-2.5 py-2 space-y-1"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center gap-2 flex-wrap">
        <button
          type="button"
          onClick={() => onOpenTrigger(match.trigger_name)}
          className="text-xs font-semibold hover:underline"
          style={{ color: 'var(--color-foreground)', cursor: 'pointer' }}
        >
          {match.trigger_name}
        </button>
        <span
          className="text-[10px] px-1.5 py-0.5 rounded font-medium"
          style={{ color: statusColor(match.status), border: `1px solid ${statusColor(match.status)}` }}
        >
          {STATUS_LABEL[match.status]}
        </span>
        {match.worn_off && (
          <span className="text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
            worn-off match
          </span>
        )}
        {match.pattern_label && match.pattern_label !== 'primary' && (
          <span className="text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
            {match.pattern_label}
          </span>
        )}
        {match.fired && (
          <span className="text-[10px] flex items-center gap-0.5" style={{ color: 'var(--color-primary)' }}>
            <Zap size={9} /> fired
          </span>
        )}
      </div>

      {match.status === 'excluded' && match.exclude_pattern && (
        <p className="text-[11px] font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
          excluded by: {match.exclude_pattern}
        </p>
      )}

      {captureEntries.length > 0 && (
        <p className="text-[11px] font-mono truncate" style={{ color: 'var(--color-muted-foreground)' }}>
          {captureEntries.map(([k, v]) => `${k}=${v}`).join('  ')}
        </p>
      )}

      {match.actions && match.actions.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {match.actions.map((a, ai) => (
            <span
              key={ai}
              className="text-[11px] px-1.5 py-0.5 rounded"
              style={{
                backgroundColor: 'var(--color-surface-3)',
                color: a.color || 'var(--color-foreground)',
              }}
            >
              {a.text || `(${a.type})`}
            </span>
          ))}
        </div>
      )}

      {match.timer && (
        <p className="text-[11px] flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}>
          <Clock size={10} />
          {match.worn_off
            ? `stops timer "${match.timer.key}"`
            : `${match.timer.category ?? 'timer'} "${match.timer.key}"${
                match.timer.duration_secs ? ` — ${match.timer.duration_secs}s` : ''
              }${match.timer.target ? ` on ${match.timer.target}` : ''}`}
        </p>
      )}

      {match.webhooks && match.webhooks.length > 0 && (
        <div className="space-y-0.5">
          {match.webhooks.map((wh, wi) => (
            <p key={wi} className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
              {wh.resolved ? 'would post to Discord: ' : 'webhook not found (would skip): '}
              <span style={{ color: 'var(--color-foreground)' }}>{wh.text}</span>
            </p>
          ))}
        </div>
      )}
    </div>
  )
}

function LineRow({
  result,
  onOpenTrigger,
}: {
  result: TestLineResult
  onOpenTrigger: (name: string) => void
}): React.ReactElement {
  const hasMatches = result.matches.length > 0
  return (
    <div className="space-y-1">
      <p
        className="text-[11px] font-mono truncate"
        style={{ color: hasMatches ? 'var(--color-foreground)' : 'var(--color-muted)' }}
        title={result.line}
      >
        {result.line || <span style={{ color: 'var(--color-muted)' }}>(blank line)</span>}
      </p>
      {hasMatches && (
        <div className="pl-3 space-y-1.5">
          {result.matches.map((m, mi) => (
            <MatchRow key={mi} match={m} onOpenTrigger={onOpenTrigger} />
          ))}
        </div>
      )}
    </div>
  )
}

interface TriggerTesterTabProps {
  chars: Character[]
  activeCharacter: string
  onOpenTrigger: (name: string) => void
}

export default function TriggerTesterTab({
  chars,
  activeCharacter,
  onOpenTrigger,
}: TriggerTesterTabProps): React.ReactElement {
  const [text, setText] = useState<string>(() => loadSavedText())
  const [character, setCharacter] = useState<string>('')
  const [fireEffects, setFireEffects] = useState(false)
  const [realtime, setRealtime] = useState(false)
  const [running, setRunning] = useState(false)
  const [lines, setLines] = useState<TestLineResult[]>([])
  const [errors, setErrors] = useState<string[]>([])
  const runSeq = useRef(0)

  // Pick up an already-running real-time session (e.g. the tab was
  // remounted, or another window started one) rather than assuming idle.
  useEffect(() => {
    getTriggerTestStatus()
      .then((s) => setRunning(s.state === 'playing'))
      .catch(() => {})
  }, [])

  useEffect(() => saveText(text), [text])

  useWebSocket((msg) => {
    if (msg.type === WSEvent.TriggerTestLine) {
      setLines((prev) => [...prev, msg.data as TestLineResult])
    } else if (msg.type === WSEvent.TriggerTestStatus) {
      const status = msg.data as { state: 'idle' | 'playing'; errors?: string[] }
      setRunning(status.state === 'playing')
      if (status.state === 'idle' && status.errors && status.errors.length > 0) {
        setErrors(status.errors)
      }
    }
  })

  const run = useCallback(() => {
    if (!text.trim() || running) return
    const mySeq = ++runSeq.current
    setErrors([])
    setLines([])
    setRunning(true)
    runTriggerTest({
      lines: text,
      character: character || undefined,
      fire_effects: fireEffects,
      realtime,
    })
      .then((report) => {
        if (mySeq !== runSeq.current) return // superseded by a newer run
        if (realtime) return // real-time results stream in via WS instead
        setLines(report.lines)
        setErrors(report.errors ?? [])
        setRunning(false)
      })
      .catch((e: Error) => {
        if (mySeq !== runSeq.current) return
        setErrors([e.message])
        setRunning(false)
      })
  }, [text, character, fireEffects, realtime, running])

  const stop = useCallback(() => {
    stopTriggerTest().catch(() => {})
  }, [])

  const summary = summarize(lines)

  return (
    <div className="flex h-full flex-col">
      <div
        className="flex flex-wrap items-center gap-2 border-b px-4 py-2 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <select
          value={character}
          onChange={(e) => setCharacter(e.target.value)}
          className="rounded px-2 py-1.5 text-xs outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          title="Which character's per-character triggers and {c}/{char}/{self} substitution to test against"
        >
          <option value="">
            {activeCharacter ? `Active (${activeCharacter})` : 'Active character'}
          </option>
          {chars
            .filter((c) => c.name !== activeCharacter)
            .map((c) => (
              <option key={c.name} value={c.name}>
                {c.name}
              </option>
            ))}
        </select>

        <button
          type="button"
          onClick={running && realtime ? stop : run}
          disabled={!text.trim() || (running && !realtime)}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: running && realtime ? 'var(--color-destructive)' : 'var(--color-primary)',
            color: 'var(--color-background)',
            opacity: !text.trim() || (running && !realtime) ? 0.6 : 1,
          }}
        >
          {running ? (
            realtime ? (
              <>
                <Square size={11} /> Stop
              </>
            ) : (
              <>
                <RefreshCw size={11} className="animate-spin" /> Running…
              </>
            )
          ) : (
            <>
              <Play size={11} /> Run Test
            </>
          )}
        </button>

        <button
          type="button"
          onClick={() => {
            setText('')
            setLines([])
            setErrors([])
          }}
          disabled={running}
          className="flex items-center gap-1.5 text-xs px-2 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          <RotateCcw size={11} />
          Reset Text
        </button>

        <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <input
            type="checkbox"
            checked={realtime}
            disabled={running}
            onChange={(e) => setRealtime(e.target.checked)}
          />
          Real-time
        </label>
        <label
          className="flex items-center gap-1.5 text-xs"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Also start the real timer and preview the overlay/audio for a match — never posts a Discord webhook or writes trigger history"
        >
          <input
            type="checkbox"
            checked={fireEffects}
            onChange={(e) => setFireEffects(e.target.checked)}
          />
          Fire alerts
        </label>

        {lines.length > 0 && (
          <span className="ml-auto text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            {summary.lines} line{summary.lines === 1 ? '' : 's'} · {summary.matched} matched
            {summary.fired > 0 && ` · ${summary.fired} fired`}
            {summary.excluded > 0 && ` · ${summary.excluded} excluded`}
            {summary.cooldowns > 0 && ` · ${summary.cooldowns} on cooldown`}
          </span>
        )}
      </div>

      {errors.length > 0 && (
        <div className="px-4 py-2 shrink-0 space-y-0.5">
          {errors.map((e, i) => (
            <p key={i} className="text-xs" style={{ color: 'var(--color-danger)' }}>
              {e}
            </p>
          ))}
        </div>
      )}

      <div className="flex-1 min-h-0 flex flex-col p-4 gap-3 overflow-hidden">
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={PLACEHOLDER}
          spellCheck={false}
          className="w-full rounded px-3 py-2 text-xs font-mono outline-none resize-y"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            minHeight: 120,
            maxHeight: '40%',
          }}
        />

        <div className="flex-1 overflow-y-auto space-y-2.5 pr-1">
          {lines.length === 0 ? (
            <p className="text-xs italic" style={{ color: 'var(--color-muted)' }}>
              Paste one or more log lines above (with or without the "[Day Mon D H:M:S Year]"
              timestamp) and click Run Test to see which triggers would fire.
            </p>
          ) : (
            lines.map((l, i) => <LineRow key={i} result={l} onOpenTrigger={onOpenTrigger} />)
          )}
        </div>
      </div>
    </div>
  )
}
