import React, { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import type { ZealCapRow, ZealCapState, ZealSettings } from '../types/zeal'

// Zeal settings checklist: each Zeal feature PQC benefits from that the
// user's Zeal supports, with a status dot and the command to turn it on.
// Used by Settings › Zeal and the setup wizard.

const DOT: Record<ZealCapState, { color: string; dashed?: boolean; label: string }> = {
  working: { color: '#22c55e', label: 'Working' },
  on: { color: '#f59e0b', label: 'On' },
  off: { color: 'var(--color-muted)', label: 'Off' },
  unknown: { color: 'var(--color-muted)', dashed: true, label: "Can't tell" },
  not_seen: { color: 'var(--color-muted)', dashed: true, label: 'Not seen yet' },
}

// A command is a slash command when it starts with '/'; otherwise it's an
// instruction (e.g. the Zeal options window) and gets no copy button.
function isSlash(cmd?: string): cmd is string {
  return !!cmd && cmd.startsWith('/')
}

function CopyCommand({ cmd }: { cmd: string }): React.ReactElement {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      title="Copy command"
      onClick={() => {
        navigator.clipboard
          .writeText(cmd)
          .then(() => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1200)
          })
          .catch(() => null)
      }}
      className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-foreground)',
        fontSize: '11px',
        cursor: 'pointer',
      }}
    >
      {cmd}
      {copied ? (
        <Check size={11} style={{ color: '#22c55e' }} />
      ) : (
        <Copy size={11} style={{ color: 'var(--color-muted-foreground)' }} />
      )}
    </button>
  )
}

function Row({
  row,
  first,
}: {
  row: ZealCapRow
  first: boolean
}): React.ReactElement {
  const dot = DOT[row.state] ?? DOT.unknown
  const showCommand = row.state !== 'working' && row.state !== 'on'
  const stateText =
    row.value && (row.state === 'on' || row.state === 'working')
      ? `${dot.label} (${row.value})`
      : dot.label
  return (
    <li
      className="flex items-start gap-2 py-1.5"
      style={first ? undefined : { borderTop: '1px solid var(--color-border)' }}
    >
      <span
        title={stateText}
        className="mt-1 inline-block h-2.5 w-2.5 shrink-0 rounded-full"
        style={
          dot.dashed
            ? { border: `1.5px dashed ${dot.color}` }
            : { backgroundColor: dot.color }
        }
      />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span
            className="text-sm"
            style={{ color: 'var(--color-foreground)' }}
          >
            {row.label}
          </span>
          <span
            className="text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            {stateText}
          </span>
          {row.pending && (
            <span
              title="From a Zeal test build; not in a Zeal release yet"
              className="rounded px-1 text-[10px] uppercase tracking-wide"
              style={{
                border: '1px solid var(--color-border)',
                color: 'var(--color-muted-foreground)',
              }}
            >
              test build
            </span>
          )}
        </div>
        <p
          className="text-xs"
          style={{ color: 'var(--color-muted-foreground)' }}
        >
          {row.benefit}
        </p>
        {(row.hint || (showCommand && row.command)) && (
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
            {showCommand && row.command && isSlash(row.command) && (
              <CopyCommand cmd={row.command} />
            )}
            {showCommand && row.command && !isSlash(row.command) && (
              <span style={{ color: 'var(--color-foreground)' }}>
                {row.command}
              </span>
            )}
            {row.hint && (
              <span style={{ color: 'var(--color-muted-foreground)' }}>
                {row.hint}
              </span>
            )}
          </div>
        )}
      </div>
    </li>
  )
}

interface ZealChecklistProps {
  settings: ZealSettings | null
  /** Only list recommended entries (the setup wizard). */
  recommendedOnly?: boolean
}

export default function ZealChecklist({
  settings,
  recommendedOnly,
}: ZealChecklistProps): React.ReactElement | null {
  if (!settings?.installed) return null
  const rows = settings.capabilities.filter(
    (c) => !recommendedOnly || c.recommended,
  )
  if (rows.length === 0) return null
  return (
    <div>
      <ul>
        {rows.map((r, i) => (
          <Row key={r.id} row={r} first={i === 0} />
        ))}
      </ul>
      <p
        className="mt-1 text-xs"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        Type commands in the EverQuest chat box; Zeal saves them in{' '}
        <code>zeal.ini</code>, and this list updates on its own.
        {settings.pipe_connected && !settings.pipe_matches && (
          <>
            {' '}
            The connected game isn&apos;t running from this EverQuest folder,
            so live checks don&apos;t apply here.
          </>
        )}
      </p>
    </div>
  )
}
