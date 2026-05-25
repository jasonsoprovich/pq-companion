import React, { useEffect, useState } from 'react'
import { AlertTriangle, X } from 'lucide-react'
import { detectZeal } from '../services/api'
import type { ZealInstallStatus } from '../types/zeal'

// ZealVersionWarning renders bottom-of-app banners for Zeal install issues
// serious enough that the user should see them no matter which page they're
// on. Two distinct conditions are surfaced here:
//
//  - Red banner: installed Zeal is below MinSupportedVersion. Pq-companion's
//    UI will have visibly broken pieces until they update.
//  - Amber banner: ExportOnCamp is disabled in zeal.ini. The app still runs
//    but inventory / quarmy / spellbook / spellsets data goes stale because
//    Zeal never writes the export files. Comes with inline fix instructions.
//
// A "newer Zeal available but min is met" notice is NOT shown here — that
// soft hint lives only on the Settings page so it doesn't nag a fully
// functional install.
//
// Detection runs once on mount via /api/zeal/detect. No WebSocket — neither
// installed version nor zeal.ini change while the app is running. Dismissal
// is session-only and keyed on the relevant value (version for the red
// banner, "ec" for the amber one).

const ZEAL_RELEASE_URL = 'https://github.com/CoastalRedwood/Zeal/releases/latest'

export default function ZealVersionWarning(): React.ReactElement | null {
  const [status, setStatus] = useState<ZealInstallStatus | null>(null)
  const [dismissed, setDismissed] = useState<Set<string>>(new Set())

  useEffect(() => {
    let cancelled = false
    detectZeal()
      .then((s) => {
        if (cancelled) return
        if (!s.installed) return
        setStatus(s)
      })
      .catch(() => {
        // Backend not reachable yet or detect failed — stay silent rather
        // than show a misleading warning.
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (!status) return null

  const showVersion =
    !status.version_ok && !!status.version && !dismissed.has(`v:${status.version}`)
  const showExport =
    status.export_on_camp_found && !status.export_on_camp && !dismissed.has('ec')

  if (!showVersion && !showExport) return null

  function dismiss(key: string): void {
    setDismissed((prev) => {
      const next = new Set(prev)
      next.add(key)
      return next
    })
  }

  return (
    <>
      {showVersion && status.version && (
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
            Zeal <span style={{ color: '#fca5a5' }}>v{status.version}</span> is outdated —
            pq-companion needs{' '}
            <span style={{ color: '#fca5a5' }}>v{status.min_version}+</span> for full
            functionality. Some features may be missing or incomplete until you update.
          </span>
          <a
            href={ZEAL_RELEASE_URL}
            target="_blank"
            rel="noreferrer"
            className="ml-2 px-3 py-0.5 rounded text-xs font-medium"
            style={{ backgroundColor: '#dc2626', color: '#fff' }}
          >
            Update Zeal
          </a>
          <button
            onClick={() => dismiss(`v:${status.version}`)}
            className="ml-auto"
            aria-label="Dismiss"
            style={{ color: 'var(--color-muted)' }}
          >
            <X size={12} />
          </button>
        </div>
      )}

      {showExport && (
        <div
          className="flex items-start gap-2 px-4 py-2 text-xs"
          style={{
            backgroundColor: 'rgba(245, 158, 11, 0.12)',
            borderTop: '1px solid rgba(245, 158, 11, 0.45)',
            color: 'var(--color-text)',
          }}
        >
          <AlertTriangle size={13} style={{ color: '#f59e0b', flexShrink: 0, marginTop: 2 }} />
          <span className="flex-1">
            Zeal&apos;s <code>ExportOnCamp</code> setting is{' '}
            <span style={{ color: '#fcd34d' }}>disabled</span> — character inventory,
            spellbook, and stats won&apos;t refresh when you camp, so much of
            pq-companion will show stale data.{' '}
            <span style={{ color: 'var(--color-muted-foreground)' }}>
              Fix: open <code>zeal.ini</code> in your EverQuest folder, set{' '}
              <code>ExportOnCamp=TRUE</code> under <code>[Zeal]</code>, and relaunch EQ.
            </span>
          </span>
          <button
            onClick={() => dismiss('ec')}
            className="ml-2 self-start"
            aria-label="Dismiss"
            style={{ color: 'var(--color-muted)' }}
          >
            <X size={12} />
          </button>
        </div>
      )}
    </>
  )
}
