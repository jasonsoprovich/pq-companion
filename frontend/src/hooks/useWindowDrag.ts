import { useCallback } from 'react'

/**
 * useWindowDrag — drives a cross-monitor window drag from a title-bar
 * `onMouseDown`.
 *
 * CSS `-webkit-app-region: drag` can't move a frameless window across monitor
 * boundaries on Windows (Chromium clamps the drag to the originating display).
 * Instead the main process moves the window to follow the global cursor while
 * a drag is active. This hook starts that loop on mousedown and ends it on the
 * next mouseup. Because the window follows the cursor, the cursor stays over
 * the window and the document `mouseup` reliably fires.
 *
 * Mousedowns that originate on an interactive control (anything inside a
 * `.no-drag` element, or a button) are ignored so header buttons keep working.
 */
export function useWindowDrag(): (e: React.MouseEvent) => void {
  return useCallback((e: React.MouseEvent) => {
    if (e.button !== 0) return // left button only
    const target = e.target as HTMLElement | null
    if (target?.closest('.no-drag, button, a, input, select, textarea')) return

    e.preventDefault()
    void window.electron?.window.dragStart()

    const onUp = (): void => {
      document.removeEventListener('mouseup', onUp)
      void window.electron?.window.dragEnd()
    }
    document.addEventListener('mouseup', onUp)
  }, [])
}
