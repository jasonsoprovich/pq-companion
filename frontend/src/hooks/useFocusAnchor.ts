// useFocusAnchor scrolls to and briefly highlights the element tagged
// data-setting="<focus>" when the URL carries ?focus=<focus> (Ctrl+K and
// Settings search results). The request is captured into a ref on first
// sight and kept until the element is found or ~2 s pass, so a page that
// rewrites its own URL (e.g. the Overlays hub consuming ?panel=) or mounts
// the target late can't cancel it.
import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'

const TIMEOUT_MS = 2000
const PULSE_MS = 1800

export function useFocusAnchor(): void {
  const location = useLocation()
  const pending = useRef<{ anchor: string; deadline: number } | null>(null)
  const raf = useRef(0)

  useEffect(() => {
    const focus = new URLSearchParams(location.search).get('focus')
    if (!focus) return
    pending.current = { anchor: focus, deadline: Date.now() + TIMEOUT_MS }
    cancelAnimationFrame(raf.current)
    const tick = (): void => {
      const req = pending.current
      if (!req) return
      const el = document.querySelector<HTMLElement>(`[data-setting="${CSS.escape(req.anchor)}"]`)
      if (el) {
        pending.current = null
        el.scrollIntoView({ block: 'center', behavior: 'smooth' })
        el.classList.remove('pq-focus-pulse')
        // Restart the animation if the same row is picked twice.
        void el.offsetWidth
        el.classList.add('pq-focus-pulse')
        window.setTimeout(() => el.classList.remove('pq-focus-pulse'), PULSE_MS)
        return
      }
      if (Date.now() > req.deadline) {
        pending.current = null
        return
      }
      raf.current = requestAnimationFrame(tick)
    }
    raf.current = requestAnimationFrame(tick)
    // location.key: re-run when the same result is picked again.
  }, [location.key, location.search])

  useEffect(() => () => cancelAnimationFrame(raf.current), [])
}
