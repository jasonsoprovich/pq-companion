/**
 * useMeasuredRegen — a character's HP/mana regen per tick as measured in
 * game this session (needs Zeal). Seeds from REST, then follows regen:state
 * broadcasts for that character.
 */
import { useEffect, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getMeasuredRegen } from '../services/api'
import type { MeasuredRegen } from '../types/regen'

export function useMeasuredRegen(character: string): MeasuredRegen | null {
  const [regen, setRegen] = useState<MeasuredRegen | null>(null)

  const ready = useWebSocket((msg) => {
    if (msg.type !== WSEvent.Regen) return
    const r = msg.data as MeasuredRegen
    if (r.character.toLowerCase() === character.toLowerCase()) setRegen(r)
  })

  useEffect(() => {
    setRegen(null)
    if (!character || ready !== 'open') return
    getMeasuredRegen(character).then(setRegen).catch(() => {})
  }, [character, ready])

  return regen
}
