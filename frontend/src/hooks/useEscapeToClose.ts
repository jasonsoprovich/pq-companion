import { useEffect, useRef } from 'react'

// useEscapeToClose binds the Escape key as a universal "cancel/close" for a
// modal or transient mode. Several hand-rolled modals in the app only close via
// a button, which can trap the user if that button fails to render (see the
// multi-monitor overlay bug). Wiring Escape into every one gives a guaranteed
// escape hatch.
//
// Layers are tracked in a shared stack so nesting works correctly: when a modal
// opens another modal (or a positioning mode inside a trigger editor), Escape is
// handled ONLY by the topmost layer and is stopped from propagating to the
// layers beneath. Without this, a single Escape would close every stacked
// modal at once — e.g. cancelling overlay positioning would also close the whole
// trigger editor.
//
// `active` gates participation for callers whose modal stays mounted while
// closed (an `open`/`positioning` prop). Modals that mount only while visible
// can omit it.

type Layer = { handle: () => void }

const stack: Layer[] = []
let installed = false

function onGlobalKeyDown(e: KeyboardEvent): void {
  if (e.key !== 'Escape') return
  const top = stack[stack.length - 1]
  if (!top) return
  // Capture-phase + stopPropagation keeps the keypress from reaching any other
  // window-level Escape listener (including modals that roll their own), so
  // only the topmost layer reacts.
  e.stopPropagation()
  top.handle()
}

export function useEscapeToClose(onClose: () => void, active = true): void {
  // Keep the latest callback without re-ordering the stack: the layer's
  // position must stay fixed for the lifetime of its `active` window, even as
  // the parent re-renders with a fresh inline onClose each time.
  const cb = useRef(onClose)
  useEffect(() => {
    cb.current = onClose
  })

  useEffect(() => {
    if (!active) return
    const layer: Layer = { handle: () => cb.current() }
    stack.push(layer)
    if (!installed) {
      window.addEventListener('keydown', onGlobalKeyDown, true)
      installed = true
    }
    return () => {
      const i = stack.indexOf(layer)
      if (i >= 0) stack.splice(i, 1)
    }
  }, [active])
}
