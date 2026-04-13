import React, { useEffect, useState } from 'react'
import { Minus, Square, X } from 'lucide-react'

const isMac = navigator.userAgent.includes('Macintosh')

export default function TitleBar(): React.ReactElement {
  const [maximized, setMaximized] = useState(false)

  useEffect(() => {
    window.electron?.window.isMaximized().then(setMaximized)
  }, [])

  async function handleMinimize(): Promise<void> {
    await window.electron?.window.minimize()
  }

  async function handleMaximize(): Promise<void> {
    await window.electron?.window.maximize()
    const isMax = await window.electron?.window.isMaximized()
    setMaximized(isMax ?? false)
  }

  async function handleClose(): Promise<void> {
    await window.electron?.window.close()
  }

  return (
    <header
      className="drag-region flex h-9 shrink-0 items-center border-b"
      style={{
        backgroundColor: 'var(--color-surface)',
        borderColor: 'var(--color-border)',
      }}
    >
      {/* Left padding — space for macOS traffic lights */}
      {isMac && <div className="w-[72px] shrink-0" />}

      {/* App name — centered */}
      <div className="flex flex-1 items-center justify-center">
        <span
          className="text-xs font-semibold tracking-widest uppercase"
          style={{ color: 'var(--color-primary)' }}
        >
          PQ Companion
        </span>
      </div>

      {/* Window controls — Windows/Linux only */}
      {!isMac && (
        <div className="no-drag flex h-full shrink-0">
          <button
            onClick={handleMinimize}
            className="flex h-full w-11 items-center justify-center transition-colors hover:bg-white/10"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Minimize"
          >
            <Minus size={12} />
          </button>
          <button
            onClick={handleMaximize}
            className="flex h-full w-11 items-center justify-center transition-colors hover:bg-white/10"
            style={{ color: 'var(--color-muted-foreground)' }}
            title={maximized ? 'Restore' : 'Maximize'}
          >
            <Square size={11} />
          </button>
          <button
            onClick={handleClose}
            className="flex h-full w-11 items-center justify-center transition-colors hover:bg-red-600"
            style={{ color: 'var(--color-muted-foreground)' }}
            title="Close"
          >
            <X size={12} />
          </button>
        </div>
      )}
    </header>
  )
}
