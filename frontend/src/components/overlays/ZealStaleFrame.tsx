import React, { createContext, useContext } from 'react'
import { useLiveValue } from '../../hooks/useLiveValue'
import { LIVE_TTL, STALE_OPACITY, fmtAge } from '../../lib/live'

// ZealStaleContext is true inside a frame that is already dimmed, so live
// values within it (the HP bar) don't dim a second time.
const ZealStaleContext = createContext(false)

export function useInZealStaleFrame(): boolean {
  return useContext(ZealStaleContext)
}

// ZealStaleFrame wraps the NPC target content. When Zeal stops confirming
// the target (target_at ages past LIVE_TTL.target), the content dims and a
// "Zeal stale · 12s" badge shows in the top-right corner. The target itself
// stays until Zeal or the log changes it. A target from the log, or a
// pinned one, has no stamp and never dims.
//
// Inline styles only: the popout overlay window can't rely on the app's
// theme stylesheet.
export default function ZealStaleFrame({
  at,
  children,
}: {
  at: string | undefined
  children: React.ReactNode
}): React.ReactElement {
  const live = useLiveValue(at, LIVE_TTL.target)
  const stale = Boolean(at) && live.freshness !== 'fresh'
  return (
    <div style={{ position: 'relative', flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      <div
        style={{
          flex: 1,
          minHeight: 0,
          display: 'flex',
          flexDirection: 'column',
          opacity: stale ? STALE_OPACITY : undefined,
        }}
      >
        <ZealStaleContext.Provider value={stale}>{children}</ZealStaleContext.Provider>
      </div>
      {stale && (
        <span
          title="Zeal hasn't confirmed the target since then. It stays until Zeal or the log picks a new one."
          style={{
            position: 'absolute',
            top: 6,
            right: 10,
            borderRadius: 3,
            padding: '1px 6px',
            fontSize: 10,
            fontWeight: 600,
            fontVariantNumeric: 'tabular-nums',
            color: '#facc15',
            backgroundColor: 'rgba(20,20,24,0.92)',
            border: '1px solid rgba(250,204,21,0.4)',
          }}
        >
          Zeal stale · {fmtAge(live.ageMs)}
        </span>
      )}
    </div>
  )
}
