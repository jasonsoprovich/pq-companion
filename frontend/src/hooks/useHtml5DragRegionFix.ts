import { useEffect } from 'react'

/**
 * Windows + Electron: native HTML5 drag-and-drop silently breaks while any
 * element in the window carries `-webkit-app-region: drag`. The main window's
 * title bar and sidebar are window-drag regions, and on Windows the OS-level
 * window-move hit-testing swallows the drag so `dragover`/`drop` never fire on
 * the target. That's the exact symptom seen on the trigger list / category
 * moves and the wishlist reordering. macOS ignores app-region during an HTML5
 * drag, which is why it only surfaced in the Windows smoke test.
 *
 * Fix: while a native drag is in flight, neutralise every drag region (a body
 * class flips `.drag-region` to `no-drag`), then restore them when the drag
 * ends. The window isn't meant to move during an item drag anyway, so dropping
 * the drag regions for the duration is invisible to the user. Listeners are on
 * `document` with capture so they fire wherever the drag starts or ends.
 *
 * See electron/electron#1354 and #27149.
 */
export function useHtml5DragRegionFix(): void {
  useEffect(() => {
    const enable = (): void => document.body.classList.add('dnd-active')
    const disable = (): void => document.body.classList.remove('dnd-active')
    document.addEventListener('dragstart', enable, true)
    // `dragend` always fires on the source after a drop or cancel; `drop` is a
    // belt-and-braces clear for drops that land outside any registered target.
    document.addEventListener('dragend', disable, true)
    document.addEventListener('drop', disable, true)
    return () => {
      document.removeEventListener('dragstart', enable, true)
      document.removeEventListener('dragend', disable, true)
      document.removeEventListener('drop', disable, true)
    }
  }, [])
}
