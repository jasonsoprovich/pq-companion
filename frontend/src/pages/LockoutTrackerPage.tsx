/**
 * LockoutTrackerPage — per-character loot & legacy-item lockout tracker.
 *
 * Data comes from parsing the in-game `/sll` command out of the log file
 * (backend lockout consumer → user.db). Each row stores the absolute instant
 * the lockout expires, so the countdown shown here is computed live from
 * `expires_at − now` and keeps ticking correctly even if the game and app were
 * closed for days between captures. When a timer elapses (or the target was
 * already "Available" at capture), the row turns green and stays listed until
 * the next `/sll` refresh.
 */
import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Hourglass, RefreshCw, AlertCircle } from 'lucide-react'
import { getLockoutForCharacter } from '../services/api'
import type { LockoutEntry, LockoutSection } from '../types/lockouts'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket } from '../hooks/useWebSocket'
import CharacterSubTabs from '../components/CharacterSubTabs'

// ── Time formatting ─────────────────────────────────────────────────────────

/** Formats a positive remaining duration (ms) as "5d 13h 25m 07s", dropping
 *  leading zero units but always keeping seconds for a live-ticking feel. */
function formatRemaining(ms: number): string {
  let s = Math.floor(ms / 1000)
  const d = Math.floor(s / 86400)
  s -= d * 86400
  const h = Math.floor(s / 3600)
  s -= h * 3600
  const m = Math.floor(s / 60)
  s -= m * 60
  const parts: string[] = []
  if (d > 0) parts.push(`${d}d`)
  if (d > 0 || h > 0) parts.push(`${h}h`)
  if (d > 0 || h > 0 || m > 0) parts.push(`${m}m`)
  parts.push(`${String(s).padStart(2, '0')}s`)
  return parts.join(' ')
}

/** Human "x ago" string for a unix-seconds timestamp relative to nowMs. */
function formatAgo(unixSec: number, nowMs: number): string {
  const sec = Math.max(0, Math.floor(nowMs / 1000) - unixSec)
  if (sec < 60) return 'just now'
  const m = Math.floor(sec / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

const SECTION_LABEL: Record<LockoutSection, string> = {
  loot: 'Loot Lockouts',
  legacy: 'Legacy Item Lockouts',
}
// Render order — loot first, then legacy (matches /sll output order).
const SECTION_ORDER: LockoutSection[] = ['loot', 'legacy']

// ── Row ─────────────────────────────────────────────────────────────────────

interface RowProps {
  entry: LockoutEntry
  nowMs: number
}

function LockoutRow({ entry, nowMs }: RowProps): React.ReactElement {
  const remainingMs = entry.expires_at > 0 ? entry.expires_at * 1000 - nowMs : 0
  const available = entry.expires_at === 0 || remainingMs <= 0
  // Highlight lockouts about to lift (under an hour) so they stand out.
  const soon = !available && remainingMs < 60 * 60 * 1000

  const color = available
    ? 'var(--color-success)'
    : soon
      ? 'var(--color-warning, #ffaa00)'
      : 'var(--color-foreground)'
  const bg = available
    ? 'color-mix(in srgb, var(--color-success) 12%, transparent)'
    : 'transparent'

  return (
    <div
      className="flex items-center gap-2 rounded px-2.5 py-1.5 text-xs"
      style={{ backgroundColor: bg }}
    >
      <span
        className="flex-1 font-medium"
        style={{ color: 'var(--color-foreground)' }}
      >
        {entry.target_name}
      </span>
      <span className="tabular-nums shrink-0" style={{ color }}>
        {available ? 'Available' : formatRemaining(remainingMs)}
      </span>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function LockoutTrackerPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [character, setCharacter] = useState('')
  const [entries, setEntries] = useState<LockoutEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [nowMs, setNowMs] = useState(() => Date.now())

  // Default the viewed character to the active one once it's known.
  useEffect(() => {
    if (!character && active) setCharacter(active)
  }, [character, active])

  const load = useCallback(async (name: string): Promise<void> => {
    if (!name) {
      setEntries([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const res = await getLockoutForCharacter(name)
      setEntries(res.entries ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load lockouts')
      setEntries([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load(character)
  }, [character, load])

  // Tick once a second to drive the live countdowns.
  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])

  // Live refresh: when the backend commits an /sll snapshot for the character
  // being viewed, refetch in place.
  useWebSocket((msg) => {
    if (msg.type !== 'lockouts.snapshot') return
    const data = msg.data as { character?: string } | null
    const snapChar = data?.character ?? ''
    if (
      snapChar &&
      character &&
      snapChar.toLowerCase() === character.toLowerCase()
    ) {
      void load(character)
    }
  })

  // Group rows by section, keeping snapshot (position) order within each.
  const grouped = useMemo<Record<LockoutSection, LockoutEntry[]>>(() => {
    const g: Record<LockoutSection, LockoutEntry[]> = { loot: [], legacy: [] }
    for (const e of entries) {
      if (e.section === 'loot' || e.section === 'legacy') g[e.section].push(e)
    }
    return g
  }, [entries])

  const observedAt = useMemo(
    () => entries.reduce((max, e) => Math.max(max, e.observed_at), 0),
    [entries],
  )
  const activeCount = useMemo(
    () =>
      entries.filter((e) => e.expires_at > 0 && e.expires_at * 1000 - nowMs > 0)
        .length,
    [entries, nowMs],
  )

  return (
    <div className="flex h-full flex-col">
      <CharacterSubTabs
        value={character}
        onChange={setCharacter}
        rightSlot={
          <button
            type="button"
            onClick={() => void load(character)}
            disabled={!character}
            className="inline-flex items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-muted-foreground)',
            }}
            title="Refresh lockouts for this character"
          >
            <RefreshCw size={11} />
            Refresh
          </button>
        }
      />

      {/* Status row: snapshot age + active-lockout count + how to update. */}
      <div
        className="flex items-center gap-3 border-b px-4 py-2 text-xs shrink-0"
        style={{
          borderColor: 'var(--color-border)',
          backgroundColor: 'var(--color-surface)',
        }}
      >
        <Hourglass size={13} style={{ color: 'var(--color-primary)' }} />
        {observedAt > 0 ? (
          <span style={{ color: 'var(--color-muted-foreground)' }}>
            <span style={{ color: 'var(--color-warning, #ffaa00)' }}>
              {activeCount}
            </span>{' '}
            active · captured {formatAgo(observedAt, nowMs)}
          </span>
        ) : (
          <span style={{ color: 'var(--color-muted-foreground)' }}>
            No lockout data yet
          </span>
        )}
        <span
          className="ml-auto text-[10px]"
          style={{ color: 'var(--color-muted)' }}
          title="Type /sll in-game to refresh lockouts for the active character. Timers keep counting down here even while the game is closed."
        >
          /sll to update
        </span>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-4 py-3">
        {loading ? (
          <div className="flex items-center justify-center py-6">
            <RefreshCw
              size={16}
              className="animate-spin"
              style={{ color: 'var(--color-muted)' }}
            />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center gap-2 py-6">
            <AlertCircle size={20} style={{ color: 'var(--color-danger)' }} />
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
              {error}
            </p>
          </div>
        ) : !character ? (
          <p
            className="text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            Pick a character above to see their lockouts.
          </p>
        ) : entries.length === 0 ? (
          <p
            className="text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            No lockouts recorded for this character. Type{' '}
            <span className="font-mono">/sll</span> in-game to capture them.
          </p>
        ) : (
          <div className="space-y-4">
            {SECTION_ORDER.map((section) => {
              const rows = grouped[section]
              if (rows.length === 0) return null
              return (
                <div key={section}>
                  <p
                    className="text-[11px] uppercase tracking-wider mb-1.5"
                    style={{ color: 'var(--color-muted-foreground)' }}
                  >
                    {SECTION_LABEL[section]}
                  </p>
                  <div className="space-y-0.5">
                    {rows.map((e) => (
                      <LockoutRow
                        key={`${section}-${e.position}`}
                        entry={e}
                        nowMs={nowMs}
                      />
                    ))}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
