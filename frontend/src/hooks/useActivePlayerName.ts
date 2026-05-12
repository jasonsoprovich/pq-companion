import { useCallback, useEffect, useState } from 'react'
import { listCharacters } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

/**
 * Returns the active EQ character's display name, used by spell timer rows
 * to identify the "self" target so buffs cast on yourself don't show a
 * redundant "on Osui" suffix.
 *
 * Works in both the main app (where `ActiveCharacterContext` is also
 * populated by `CharacterSwitcher`) and standalone overlay windows that
 * aren't wrapped in that context — both paths converge on the same
 * `/api/characters` endpoint plus `config:character_detected` WS events,
 * which is why this hook is independent of the context provider.
 */
export function useActivePlayerName(): string {
  const [name, setName] = useState('')

  useEffect(() => {
    listCharacters()
      .then((d) => setName(d.active ?? ''))
      .catch(() => {})
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigCharacterDetected) return
    const payload = msg.data as { character?: string } | null
    if (payload?.character) setName(payload.character)
  }, [])
  useWebSocket(handle)

  return name
}

/**
 * Returns the on-screen "on <target>" suffix for a timer row, or an empty
 * string when no suffix should be shown. Self-cast buffs (target equals the
 * active player) and trigger-driven timers (no target at all) get no suffix.
 *
 * The literal "You" is also treated as self because the engine falls back to
 * it when the character context isn't yet known at startup.
 */
export function targetSuffix(targetName: string, activePlayer: string): string {
  if (!targetName) return ''
  if (targetName === activePlayer) return ''
  if (targetName === 'You') return ''
  return ` on ${targetName}`
}
