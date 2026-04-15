import React, { useEffect, useState } from 'react'
import { Download, RefreshCw, X } from 'lucide-react'

type UpdateState =
  | { phase: 'idle' }
  | { phase: 'available'; version: string }
  | { phase: 'downloading'; version: string; percent: number }
  | { phase: 'downloaded'; version: string }
  | { phase: 'error'; message: string }

export default function UpdateNotification(): React.ReactElement | null {
  const [state, setState] = useState<UpdateState>({ phase: 'idle' })
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    if (!window.electron?.updater) return

    const offAvailable = window.electron.updater.onAvailable(({ version }) => {
      setState({ phase: 'available', version })
      setDismissed(false)
    })

    const offProgress = window.electron.updater.onProgress(({ percent, transferred, total }) => {
      setState((prev) => {
        const version = prev.phase === 'available' || prev.phase === 'downloading'
          ? prev.version
          : '?'
        return { phase: 'downloading', version, percent }
      })
      // suppress unused-var lint — values are available for future detail display
      void transferred
      void total
    })

    const offDownloaded = window.electron.updater.onDownloaded(({ version }) => {
      setState({ phase: 'downloaded', version })
      setDismissed(false)
    })

    const offError = window.electron.updater.onError((message) => {
      setState({ phase: 'error', message })
    })

    return () => {
      offAvailable()
      offProgress()
      offDownloaded()
      offError()
    }
  }, [])

  if (state.phase === 'idle' || dismissed) return null

  if (state.phase === 'error') {
    return (
      <div className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)', color: 'var(--color-muted)' }}>
        <span>Update check failed — will retry next launch.</span>
        <button onClick={() => setDismissed(true)} className="ml-auto" aria-label="Dismiss">
          <X size={12} />
        </button>
      </div>
    )
  }

  if (state.phase === 'available') {
    return (
      <div className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}>
        <Download size={13} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
        <span style={{ color: 'var(--color-muted)' }}>
          Update <span style={{ color: 'var(--color-primary)' }}>v{state.version}</span> available — downloading in the background…
        </span>
        <button onClick={() => setDismissed(true)} className="ml-auto" aria-label="Dismiss"
          style={{ color: 'var(--color-muted)' }}>
          <X size={12} />
        </button>
      </div>
    )
  }

  if (state.phase === 'downloading') {
    return (
      <div className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}>
        <Download size={13} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
        <span style={{ color: 'var(--color-muted)' }}>
          Downloading v{state.version}…
        </span>
        {/* Progress bar */}
        <div className="flex-1 mx-2 rounded-full overflow-hidden" style={{ height: 4, backgroundColor: 'var(--color-border)' }}>
          <div
            className="h-full rounded-full transition-all"
            style={{ width: `${state.percent}%`, backgroundColor: 'var(--color-primary)' }}
          />
        </div>
        <span style={{ color: 'var(--color-muted)', minWidth: '2.5rem', textAlign: 'right' }}>
          {state.percent}%
        </span>
      </div>
    )
  }

  // phase === 'downloaded'
  return (
    <div className="flex items-center gap-2 px-4 py-2 text-xs"
      style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}>
      <RefreshCw size={13} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
      <span style={{ color: 'var(--color-muted)' }}>
        v{state.version} ready — restart to install.
      </span>
      <button
        onClick={() => window.electron.updater.quitAndInstall()}
        className="ml-2 px-3 py-0.5 rounded text-xs font-medium"
        style={{ backgroundColor: 'var(--color-primary)', color: '#0a0a0a' }}
      >
        Restart
      </button>
      <button onClick={() => setDismissed(true)} className="ml-auto" aria-label="Dismiss later"
        style={{ color: 'var(--color-muted)' }}>
        <X size={12} />
      </button>
    </div>
  )
}
