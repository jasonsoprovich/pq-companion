/**
 * OverlayStackContext — set around each tab of a stack window (several
 * overlays as tabs in one window; see StackWindowPage). An overlay page
 * rendered inside it gives up the window to the stack: its title bar is
 * hidden, and the window-level hooks (lock and click-through, opacity, fade,
 * collapse, drag) hand back the stack's values instead of managing the window
 * themselves.
 *
 * A mounted page is either always inside a stack or never, so those hooks
 * can pick their branch on the context without changing hook order.
 */
import { createContext, useContext } from 'react'
import type React from 'react'
import type { LockedMode } from '../lib/overlays'

export interface OverlayStackValue {
  // The stack's window key, `stack:<id>`.
  stackKey: string
  lock: {
    locked: boolean
    mode: LockedMode
    toggleLocked: () => void
    rowsInteractive: boolean
  }
  opacity: number
  // Chrome shown (not faded out).
  chrome: boolean
  // A tab says whether it has something to show (see useIdleCollapse), for
  // the stack's activity dots and its idle collapse.
  reportActive: (key: string, active: boolean) => void
  // A do-nothing collapse for the page: the stack collapses as a whole.
  collapse: {
    collapsed: false
    toggleCollapsed: () => void
    setCollapsed: (collapse: boolean) => void
    headerRef: React.RefObject<HTMLDivElement>
    ready: false
  }
}

export const OverlayStackContext = createContext<OverlayStackValue | null>(null)

export function useOverlayStack(): OverlayStackValue | null {
  return useContext(OverlayStackContext)
}
