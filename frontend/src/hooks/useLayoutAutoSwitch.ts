/**
 * useLayoutAutoSwitch — switches to a character's linked overlay layout when
 * the app detects you've logged in on that character: the backend's
 * config:character_detected event, plus one check at launch for a character
 * already in game. Picking a character by hand in the sidebar doesn't count,
 * and a layout chosen by hand stays until the next detection. Main does the
 * lookup (layouts:switch-for-character); unlinked characters change nothing.
 * Mounted once, in the main window.
 */
import { useCallback, useEffect, useRef } from 'react'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { listCharacters } from '../services/api'

export function useLayoutAutoSwitch(): void {
  const supported = typeof window.electron?.overlay?.layoutsSwitchForCharacter === 'function'
  // The last character acted on, so the launch check and the first event
  // for the same log-in don't both switch.
  const last = useRef('')

  const detected = useCallback(
    (name: string) => {
      const n = name.trim()
      if (!supported || !n || n.toLowerCase() === last.current) return
      last.current = n.toLowerCase()
      window.electron.overlay.layoutsSwitchForCharacter(n).catch(() => {})
    },
    [supported],
  )

  useEffect(() => {
    if (!supported) return
    listCharacters()
      .then((d) => detected(d.detected ?? ''))
      .catch(() => {})
  }, [supported, detected])

  useWebSocket(
    useCallback(
      (msg: WsMessage) => {
        if (msg.type !== WSEvent.ConfigCharacterDetected) return
        detected((msg.data as { character?: string } | null)?.character ?? '')
      },
      [detected],
    ),
  )
}
