/**
 * useCharmFinder — data for the Pet overlay's compact charm finder: the active
 * character's class/level/CHA, their class's charm line, the chosen spell, and
 * the current zone's charmable NPCs from /api/charm/pets.
 *
 * The spell defaults to the strongest castable charm in the character's Zeal
 * spellbook export (falling back to the level-based pick when there is no
 * export); a manual pick from the dropdown is remembered per class.
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { usePersistentState } from './usePersistentState'
import { WSEvent } from '../lib/wsEvents'
import { isCharmClass, pickDefaultSpell, pickScribedSpell } from '../lib/charmFinder'
import {
  getCharacterDerivedStats,
  getCharmPets,
  getCharmSpells,
  getZealSpellbook,
  listCharacters,
  type CharmPetsResponse,
  type CharmSpellOption,
} from '../services/api'
import type { Spellbook } from '../types/zeal'

interface ActiveChar {
  name: string
  classIdx: number
  level: number
  cha: number
}

export interface CharmFinderHook {
  character: ActiveChar | null
  charmClass: boolean
  options: CharmSpellOption[]
  // The spell actually used: the manual pick when valid, else autoSpellID.
  spellID: number | null
  autoSpellID: number | null
  // '' = automatic; otherwise a spell id from options, as a string.
  spellPick: string
  setSpellPick: (id: string) => void
  data: CharmPetsResponse | null
  loading: boolean
  error: string
}

export function useCharmFinder(zone: string | undefined, enabled: boolean): CharmFinderHook {
  const [character, setCharacter] = useState<ActiveChar | null>(null)
  const [book, setBook] = useState<Spellbook | null>(null)
  const [options, setOptions] = useState<CharmSpellOption[]>([])
  const [data, setData] = useState<CharmPetsResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [charGen, setCharGen] = useState(0)

  // Re-read the active character (and its spellbook) when it changes.
  useWebSocket((msg) => {
    if (msg.type === WSEvent.ConfigCharacterDetected) setCharGen((g) => g + 1)
  })

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    listCharacters()
      .then((resp) => {
        const active = resp.characters.find((c) => c.name === resp.active)
        if (cancelled || !active) return
        const base: ActiveChar = {
          name: active.name,
          classIdx: active.class,
          level: active.level > 0 ? active.level : 60,
          cha: active.base_cha,
        }
        setCharacter(base)
        // Prefer the full Equipped-layer CHA (base + worn items + AAs), as the
        // Finder page does; keep base_cha if it can't be derived.
        getCharacterDerivedStats(active.id, [], [])
          .then((d) => {
            if (!cancelled && d.equipped.cha > 0) setCharacter({ ...base, cha: d.equipped.cha })
          })
          .catch(() => {})
      })
      .catch(() => {})
    getZealSpellbook()
      .then((r) => { if (!cancelled) setBook(r.spellbook) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [enabled, charGen])

  const classIdx = character?.classIdx ?? -1
  const charmClass = isCharmClass(classIdx)

  useEffect(() => {
    if (!enabled || !charmClass) {
      setOptions([])
      return
    }
    let cancelled = false
    getCharmSpells(classIdx)
      .then((opts) => { if (!cancelled) setOptions(opts) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [enabled, charmClass, classIdx])

  // Stored as "<class>:<spell id>" under one key: usePersistentState reads
  // storage once on mount, before the active class is known, so a per-class
  // key would never load. A pick saved for another class reads as automatic.
  const [storedPick, setStoredPick] = usePersistentState<string>('pq-pet-finder-spell', '', null)
  const [pickClass, pickID] = storedPick.split(':')
  const spellPick = pickClass === String(classIdx) && pickID ? pickID : ''
  const setSpellPick = useCallback(
    (id: string) => setStoredPick(id === '' ? '' : `${classIdx}:${id}`),
    [classIdx, setStoredPick],
  )

  const level = character?.level ?? 0
  const autoSpellID = useMemo(
    () => pickScribedSpell(options, level, book) ?? pickDefaultSpell(options, level),
    [options, level, book],
  )
  const picked = Number(spellPick)
  const spellID =
    spellPick !== '' && options.some((o) => o.spell_id === picked) ? picked : autoSpellID

  const cha = character?.cha ?? 0
  useEffect(() => {
    if (!enabled || !zone || !charmClass || spellID == null || level < 1) {
      setData(null)
      return
    }
    let cancelled = false
    setLoading(true)
    getCharmPets({ zone, classIndex: classIdx, level, spellID, casterCHA: cha })
      .then((r) => { if (!cancelled) { setData(r); setError('') } })
      .catch((e: Error) => { if (!cancelled) setError(e.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [enabled, zone, charmClass, classIdx, level, spellID, cha])

  return {
    character,
    charmClass,
    options,
    spellID,
    autoSpellID,
    spellPick,
    setSpellPick,
    data,
    loading,
    error,
  }
}
