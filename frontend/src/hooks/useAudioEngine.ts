import { useCallback } from 'react'
import { useWebSocket } from './useWebSocket'
import { playSound, speakText } from '../services/audio'
import type { TriggerFired } from '../types/trigger'

const TRIGGER_FIRED_EVENT = 'trigger:fired'

/**
 * useAudioEngine subscribes to WebSocket trigger:fired events and dispatches
 * any play_sound or text_to_speech actions to the audio service.
 *
 * Mount this hook once at the App level so audio fires regardless of which
 * page the user is on.
 */
export function useAudioEngine(): void {
  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== TRIGGER_FIRED_EVENT) return

    const fired = msg.data as TriggerFired
    if (!fired?.actions) return

    for (const action of fired.actions) {
      const vol = action.volume > 0 ? action.volume : 1.0

      if (action.type === 'play_sound') {
        playSound(action.sound_path, vol)
      } else if (action.type === 'text_to_speech') {
        speakText(action.text, action.voice, vol)
      }
    }
  }, [])

  useWebSocket(handleMessage)
}
