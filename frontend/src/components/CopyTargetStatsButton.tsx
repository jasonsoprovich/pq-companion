import React, { useState } from 'react'
import { Clipboard, ClipboardCheck } from 'lucide-react'
import { buildTargetStatsLine } from '../lib/npcClipboard'
import type { TargetState } from '../types/overlay'

// CopyTargetStatsButton copies a one-line summary of the current target
// (name, HP, damage, resists, tags, signature spells) to the clipboard — a
// raid-leader convenience for calling the next target. Disabled when there's no
// NPC target to summarize (no target, or a player). Shared by the dashboard
// NPC panel and the popped-out overlay window; idleColor lets each pass its own
// resting tint (CSS var vs rgba) while keeping one implementation.
export default function CopyTargetStatsButton({
  state,
  idleColor = 'var(--color-muted)',
  size = 12,
}: {
  state: TargetState | null
  idleColor?: string
  size?: number
}): React.ReactElement {
  const [copied, setCopied] = useState(false)
  const line = buildTargetStatsLine(state)

  function handleCopy(): void {
    if (!line) return
    navigator.clipboard.writeText(line).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    }).catch(() => {})
  }

  return (
    <button
      onClick={handleCopy}
      disabled={!line}
      title={line ? 'Copy target stats to clipboard' : 'No target to copy'}
      style={{
        background: 'none',
        border: 'none',
        cursor: line ? 'pointer' : 'default',
        padding: '1px 3px',
        color: copied ? '#22c55e' : idleColor,
        display: 'flex',
        alignItems: 'center',
        opacity: line ? 1 : 0.4,
      }}
    >
      {copied ? <ClipboardCheck size={size} /> : <Clipboard size={size} />}
    </button>
  )
}
