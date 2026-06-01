import { useEffect } from 'react'

// useEscapeToClose binds the Escape key as a universal "cancel/close" for a
// modal. Several hand-rolled modals in the app only close via a button, which
// can trap the user if that button fails to render (see the multi-monitor
// overlay bug). Wiring Escape into every modal gives a guaranteed escape hatch.
//
// `active` lets callers gate the listener when a modal uses an `open` prop and
// stays mounted while closed. Modals that mount only while visible can omit it.
export function useEscapeToClose(onClose: () => void, active = true): void {
  useEffect(() => {
    if (!active) return
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [active, onClose])
}
