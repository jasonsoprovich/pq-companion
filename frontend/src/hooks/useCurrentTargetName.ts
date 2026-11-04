import { useCallback, useEffect, useState } from 'react'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { getOverlayNPCTarget } from '../services/api'
import { WSEvent } from '../lib/wsEvents'
import type { TargetState } from '../types/overlay'

export interface CurrentTarget {
  name: string
  // Zeal spawn id; undefined without Zeal or with no target.
  id?: number
}

const NO_TARGET: CurrentTarget = { name: '' }

function fromState(s: TargetState | null | undefined): CurrentTarget {
  if (!s?.has_target) return NO_TARGET
  return { name: s.target_name ?? '', id: s.target_id }
}

/**
 * The current target as the NPC overlay sees it (Zeal pipe target first,
 * log-inferred otherwise), with its spawn id when Zeal reports one.
 */
export function useCurrentTarget(): CurrentTarget {
  const [target, setTarget] = useState<CurrentTarget>(NO_TARGET)

  useEffect(() => {
    getOverlayNPCTarget()
      .then((s) => setTarget(fromState(s)))
      .catch(() => {})
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.OverlayNPCTarget) return
    const next = fromState(msg.data as TargetState | null)
    // The NPC feed broadcasts for HP and level changes too; keep the same
    // object so consumers don't recompute on those.
    setTarget((prev) => (prev.name === next.name && prev.id === next.id ? prev : next))
  }, [])
  useWebSocket(handle)

  return target
}

/** Name of the current target; '' when there's no target. */
export function useCurrentTargetName(): string {
  return useCurrentTarget().name
}
