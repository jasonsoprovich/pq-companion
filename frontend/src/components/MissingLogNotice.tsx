import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, Settings } from 'lucide-react'
import { getEqDiagnostics, type EqDiagnostics } from '../services/api'

// reason returns a short, cause-specific explanation for why no logs are
// present, mirroring the backend's validate-eq-path diagnosis.
function reason(d: EqDiagnostics): string {
  if (d.log_found && !d.log_enabled) {
    return 'EverQuest logging is turned off, so no log file is being written.'
  }
  if (!d.log_found) {
    return "EverQuest logging doesn't appear to be enabled in eqclient.ini."
  }
  if (d.zeal_installed && d.export_on_camp_found && !d.export_on_camp) {
    return 'Zeal’s "output on camp" is off, so character data isn’t exported.'
  }
  return 'No EverQuest log file has been detected yet.'
}

/**
 * MissingLogNotice renders a friendly, cause-specific banner when this page's
 * features need the EQ log but it isn't available. Renders nothing once logs
 * are detected, so it's safe to drop at the top of any log-dependent page.
 */
export default function MissingLogNotice(): React.ReactElement | null {
  const [diag, setDiag] = useState<EqDiagnostics | null>(null)
  const navigate = useNavigate()

  useEffect(() => {
    getEqDiagnostics().then(setDiag).catch(() => {})
  }, [])

  if (!diag || diag.has_logs) return null

  return (
    <div
      className="mb-3 flex items-start gap-3 rounded-lg p-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid #f97316' }}
    >
      <AlertTriangle size={16} style={{ color: '#f97316', marginTop: 1 }} />
      <div className="flex-1">
        <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          This page needs your EverQuest log
        </p>
        <p className="mt-0.5 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          {reason(diag)} Database pages (Items, Spells, NPCs, Zones) still work without logs.
        </p>
      </div>
      <button
        onClick={() => navigate('/settings')}
        className="flex shrink-0 items-center gap-1.5 rounded px-2.5 py-1.5 text-xs font-medium"
        style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: '1px solid transparent' }}
      >
        <Settings size={12} /> Fix in Settings
      </button>
    </div>
  )
}
