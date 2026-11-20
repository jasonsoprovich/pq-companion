import React, { useEffect, useRef, useState } from 'react'
import { Download, RefreshCw, X } from 'lucide-react'

type UpdateState =
  | { phase: 'idle' }
  | { phase: 'available'; version: string }
  | { phase: 'downloading'; version: string; percent: number | null }
  | { phase: 'downloaded'; version: string; countdown: number }
  | { phase: 'installing'; version: string }
  | { phase: 'error'; message: string }
  // Portable/unmanaged copy: can't self-update, so link the zip instead.
  | { phase: 'manual'; version: string; downloadUrl: string; releaseUrl: string }

const RESTART_COUNTDOWN = 5

export default function UpdateNotification(): React.ReactElement | null {
  const [state, setState] = useState<UpdateState>({ phase: 'idle' })
  const [dismissed, setDismissed] = useState(false)
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (!window.electron?.updater) return

    const offAvailable = window.electron.updater.onAvailable(({ version }) => {
      setState({ phase: 'available', version })
      setDismissed(false)
    })

    const offProgress = window.electron.updater.onProgress(({ percent }) => {
      setState((prev) => {
        const version = prev.phase === 'available' || prev.phase === 'downloading'
          ? prev.version
          : '?'
        return { phase: 'downloading', version, percent }
      })
    })

    const offDownloaded = window.electron.updater.onDownloaded(({ version }) => {
      setDismissed(false)
      setState({ phase: 'downloaded', version, countdown: RESTART_COUNTDOWN })
    })

    const offManual = window.electron.updater.onManual((info) => {
      setState({ phase: 'manual', ...info })
      setDismissed(false)
    })

    const offError = window.electron.updater.onError((message) => {
      setState({ phase: 'error', message })
    })

    return () => {
      offAvailable()
      offProgress()
      offDownloaded()
      offManual()
      offError()
    }
  }, [])

  // Countdown timer: tick down, then auto-install
  useEffect(() => {
    if (state.phase !== 'downloaded') {
      if (countdownRef.current) {
        clearInterval(countdownRef.current)
        countdownRef.current = null
      }
      return
    }

    countdownRef.current = setInterval(() => {
      setState((prev) => {
        if (prev.phase !== 'downloaded') return prev
        if (prev.countdown <= 1) {
          clearInterval(countdownRef.current!)
          countdownRef.current = null
          window.electron.updater.quitAndInstall()
          return { phase: 'installing', version: prev.version }
        }
        return { ...prev, countdown: prev.countdown - 1 }
      })
    }, 1000)

    return () => {
      if (countdownRef.current) clearInterval(countdownRef.current)
    }
  }, [state.phase])

  if (state.phase === 'idle' || dismissed) return null

  if (state.phase === 'error') {
    return (
      <div className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)', color: 'var(--color-muted)' }}>
        <span>Update failed — {state.message}</span>
        <button
          onClick={() => { setState({ phase: 'idle' }); window.electron.updater.check() }}
          className="ml-2 px-2 py-0.5 rounded text-xs font-medium"
          style={{ backgroundColor: 'var(--color-surface-hover)', color: 'var(--color-text)' }}
        >
          Retry
        </button>
        <button onClick={() => setDismissed(true)} className="ml-auto" aria-label="Dismiss"
          style={{ color: 'var(--color-muted)' }}>
          <X size={12} />
        </button>
      </div>
    )
  }

  if (state.phase === 'manual') {
    // target=_blank links are routed to the system browser by the main
    // process's window-open handler (hardenWebContents).
    return (
      <div className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}>
        <Download size={13} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
        <span style={{ color: 'var(--color-muted)' }}>
          Update <span style={{ color: 'var(--color-primary)' }}>v{state.version}</span> available
          {' '}— this portable copy updates manually: download the zip and
          extract it over this folder.
        </span>
        <a
          href={state.downloadUrl}
          target="_blank"
          rel="noreferrer noopener"
          className="ml-2 shrink-0 px-3 py-0.5 rounded text-xs font-medium"
          style={{ backgroundColor: 'var(--color-primary)', color: '#0a0a0a' }}
        >
          Download zip
        </a>
        <a
          href={state.releaseUrl}
          target="_blank"
          rel="noreferrer noopener"
          className="shrink-0 hover:underline"
          style={{ color: 'var(--color-primary)' }}
        >
          Release notes
        </a>
        <button onClick={() => setDismissed(true)} className="ml-auto" aria-label="Dismiss"
          style={{ color: 'var(--color-muted)' }}>
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
          Update <span style={{ color: 'var(--color-primary)' }}>v{state.version}</span> available
        </span>
        <button
          onClick={() => {
            // Switch to an indeterminate downloading state immediately so the
            // user gets instant feedback — download-progress events can be
            // sparse or delayed (esp. fast/differential downloads), and
            // without this the banner appears frozen after the click.
            setState({ phase: 'downloading', version: state.version, percent: null })
            window.electron.updater.download()
          }}
          className="ml-2 px-3 py-0.5 rounded text-xs font-medium"
          style={{ backgroundColor: 'var(--color-primary)', color: '#0a0a0a' }}
        >
          Update
        </button>
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
          {state.percent === null
            ? `Starting download of v${state.version}…`
            : `Downloading v${state.version}…`}
        </span>
        <div className="flex-1 mx-2 rounded-full overflow-hidden" style={{ height: 4, backgroundColor: 'var(--color-border)' }}>
          <div
            className={`h-full rounded-full transition-all${state.percent === null ? ' animate-pulse' : ''}`}
            style={{
              width: state.percent === null ? '100%' : `${state.percent}%`,
              backgroundColor: 'var(--color-primary)',
            }}
          />
        </div>
        <span style={{ color: 'var(--color-muted)', minWidth: '2.5rem', textAlign: 'right' }}>
          {state.percent === null ? '…' : `${state.percent}%`}
        </span>
      </div>
    )
  }

  if (state.phase === 'installing') {
    return (
      <div className="flex items-center gap-2 px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface)', borderTop: '1px solid var(--color-border)' }}>
        <RefreshCw size={13} style={{ color: 'var(--color-primary)', flexShrink: 0 }} className="animate-spin" />
        <span style={{ color: 'var(--color-muted)' }}>
          Installing v{state.version} — restarting…
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
        v{state.version} downloaded — restarting in {state.countdown}s
      </span>
      <button
        onClick={() => {
          if (countdownRef.current) clearInterval(countdownRef.current)
          setState({ phase: 'installing', version: state.version })
          window.electron.updater.quitAndInstall()
        }}
        className="ml-2 px-3 py-0.5 rounded text-xs font-medium"
        style={{ backgroundColor: 'var(--color-primary)', color: '#0a0a0a' }}
      >
        Restart now
      </button>
    </div>
  )
}
