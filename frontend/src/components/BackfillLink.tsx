import React from 'react'
import { useNavigate } from 'react-router-dom'
import { DatabaseBackup } from 'lucide-react'

/**
 * BackfillLink is a small header button that deep-links to Settings → Logs,
 * where the Log Backfill controls live. Dropped into tracker pages (Chat
 * History, Loot Tracker, Player Tracker) so users can easily find how to
 * backfill history from their logs.
 */
export default function BackfillLink(): React.ReactElement {
  const navigate = useNavigate()
  return (
    <button
      onClick={() => navigate('/settings?tab=logs')}
      className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
      style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
      title="Backfill history from your logs (Settings → Logs)"
    >
      <DatabaseBackup size={11} />
      Backfill
    </button>
  )
}
