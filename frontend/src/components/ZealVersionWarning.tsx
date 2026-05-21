import React, { useEffect, useState } from 'react'
import { AlertTriangle, X } from 'lucide-react'
import { detectZeal } from '../services/api'

// ZealVersionWarning renders a red banner at the bottom of the app when the
// installed Zeal version is below the minimum pq-companion needs. Mounted
// above UpdateNotification in Layout so both can coexist — the Zeal warning
// stacks on top of the update banner.
//
// Detection runs once on mount via /api/zeal/detect. No WebSocket — the
// installed version doesn't change while the app is running.
//
// Dismissal is session-only and keyed on the detected version: dismissing
// the warning for 1.3.2 won't auto-dismiss a later detection of 1.3.5,
// since the user should be reminded if their install state changes.
export default function ZealVersionWarning(): React.ReactElement | null {
  const [version, setVersion] = useState<string>('')
  const [minVersion, setMinVersion] = useState<string>('')
  const [needsUpdate, setNeedsUpdate] = useState(false)
  const [dismissedVersion, setDismissedVersion] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    detectZeal()
      .then((status) => {
        if (cancelled) return
        if (!status.installed) return
        if (status.version_ok) return
        if (!status.version) return
        setVersion(status.version)
        setMinVersion(status.min_version ?? '')
        setNeedsUpdate(true)
      })
      .catch(() => {
        // Backend not reachable yet or detect failed — stay silent rather
        // than show a misleading warning.
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (!needsUpdate || dismissedVersion === version) return null

  return (
    <div
      className="flex items-center gap-2 px-4 py-2 text-xs"
      style={{
        backgroundColor: 'rgba(220, 38, 38, 0.12)',
        borderTop: '1px solid rgba(220, 38, 38, 0.5)',
        color: 'var(--color-text)',
      }}
    >
      <AlertTriangle size={13} style={{ color: '#dc2626', flexShrink: 0 }} />
      <span>
        Zeal <span style={{ color: '#fca5a5' }}>v{version}</span> is outdated —
        pq-companion needs <span style={{ color: '#fca5a5' }}>v{minVersion}+</span> for
        full functionality. Some features may be missing or incomplete until you update.
      </span>
      <a
        href="https://github.com/CoastalRedwood/Zeal/releases/latest"
        target="_blank"
        rel="noreferrer"
        className="ml-2 px-3 py-0.5 rounded text-xs font-medium"
        style={{ backgroundColor: '#dc2626', color: '#fff' }}
      >
        Update Zeal
      </a>
      <button
        onClick={() => setDismissedVersion(version)}
        className="ml-auto"
        aria-label="Dismiss"
        style={{ color: 'var(--color-muted)' }}
      >
        <X size={12} />
      </button>
    </div>
  )
}
