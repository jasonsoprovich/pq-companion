import React, { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, Crosshair, Percent, Search, X } from 'lucide-react'
import {
  getSpellsByClass,
  getInstrumentMods,
  getOverlayNPCTarget,
  getResistDebuffs,
  listCharacters,
  postResistCheck,
  searchNPCs,
  type Character,
  type InstrumentMods,
  type ResistCheckResponse,
  type ResistDebuff,
} from '../services/api'
import type { Spell } from '../types/spell'
import type { NPC } from '../types/npc'
import { npcDisplayName } from '../lib/npcHelpers'
import { applyLevelFormula } from '../lib/spellHelpers'

type ResistKey = 'mr' | 'cr' | 'fr' | 'dr' | 'pr'

// scaledDelta returns a debuff's total reduction to one resist at the given
// (debuffer) level, applying the EQ level-scaling formula. Negative = lowers.
function scaledDelta(d: ResistDebuff, resist: ResistKey, level: number): number {
  return d.mods
    .filter((m) => m.resist === resist)
    .reduce((acc, m) => acc + applyLevelFormula(m.formula, m.base, m.max, level), 0)
}

// moddedDelta applies a manual strength multiplier on top of the level-scaled
// value. mod=1 is a no-op (the default). For bards this is the instrument
// modifier (e.g. 3.2× with a matching instrument + Instrument Mastery); for
// any other class it's a free "what-if" adjustment. Rounded to a whole number
// since resists are integers in-game.
function moddedDelta(
  d: ResistDebuff,
  resist: ResistKey,
  level: number,
  mod: number,
): number {
  const base = scaledDelta(d, resist, level)
  return mod === 1 ? base : Math.round(base * mod)
}

// 0-based EQ class names (matches eqstat / spells_new ordering).
const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter',
  'Beastlord',
]

const ENCHANTER = 13

// The target is always a real NPC (picked in-game or from the database), so
// its level/resists are displayed read-only rather than hand-edited.
interface TargetNPC {
  name: string
  level: number // worst case: top of the NPC's level range
  mr: number
  cr: number
  fr: number
  dr: number
  pr: number
  specialAbilities: string
}

function pct(x: number): string {
  return `${(x * 100).toFixed(1)}%`
}

// Caster and target are independent enough to keep in plain state. The compute
// runs in an effect whenever any input changes (cheap: one backend call).
export default function ResistCalcPage(): React.ReactElement {
  const [casterLevel, setCasterLevel] = useState(60)
  const [casterClass, setCasterClass] = useState(ENCHANTER)
  const [casterCHA, setCasterCHA] = useState(75)

  const [spells, setSpells] = useState<Spell[]>([])
  const [spellFilter, setSpellFilter] = useState('')
  const [spellID, setSpellID] = useState<number | null>(null)

  const [target, setTarget] = useState<TargetNPC | null>(null)

  const [result, setResult] = useState<ResistCheckResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [targetNote, setTargetNote] = useState<string | null>(null)

  const [npcQuery, setNpcQuery] = useState('')
  const [npcResults, setNpcResults] = useState<NPC[]>([])
  const npcSearchRef = useRef<HTMLDivElement>(null)

  const [debuffList, setDebuffList] = useState<ResistDebuff[]>([])
  const [selectedDebuffs, setSelectedDebuffs] = useState<ResistDebuff[]>([])
  const [debuffQuery, setDebuffQuery] = useState('')
  const debuffSearchRef = useRef<HTMLDivElement>(null)

  // Per-debuff manual strength multiplier, keyed by debuff id (default 1).
  const [debuffMods, setDebuffMods] = useState<Record<number, number>>({})

  // Optional character binding. null = "manual" mode (the original behaviour:
  // pick any class/level by hand). Selecting a character seeds level/class/CHA.
  const [characters, setCharacters] = useState<Character[]>([])
  const [selectedCharId, setSelectedCharId] = useState<number | null>(null)
  // A selected bard's instrument modifier per song skill (null otherwise).
  const [instrumentMods, setInstrumentMods] = useState<InstrumentMods | null>(null)

  // Load the character list once and default to the active character (so the
  // page still auto-fills like before). Users can switch to "Manual" to keep
  // the class/level free for testing.
  useEffect(() => {
    listCharacters()
      .then((resp) => {
        setCharacters(resp.characters)
        const active = resp.characters.find((c) => c.name === resp.active)
        if (active) setSelectedCharId(active.id)
      })
      .catch(() => {})
  }, [])

  // Seed the caster from the selected character (level/class/base CHA). Runs
  // only when the selection changes, so manual edits afterwards stick. CHA is
  // editable since the formula wants total (buffed) charisma, not base.
  useEffect(() => {
    if (selectedCharId == null) return
    const c = characters.find((x) => x.id === selectedCharId)
    if (!c) return
    if (c.level > 0) setCasterLevel(c.level)
    if (c.class >= 0 && c.class <= 14) setCasterClass(c.class)
    if (c.base_cha > 0) setCasterCHA(c.base_cha)
  }, [selectedCharId, characters])

  // Load the selected character's bard instrument modifier (used to pre-fill
  // each bard-song debuff's multiplier). Manual edits to a debuff's × field
  // still override it. Switching/clearing the character resets the modifier and
  // any manual overrides so the new character's auto values apply cleanly.
  useEffect(() => {
    setDebuffMods({})
    if (selectedCharId == null) {
      setInstrumentMods(null)
      return
    }
    let cancelled = false
    getInstrumentMods(selectedCharId)
      .then((resp) => {
        if (!cancelled) setInstrumentMods(resp.is_bard ? resp.mods : null)
      })
      .catch(() => {
        if (!cancelled) setInstrumentMods(null)
      })
    return () => {
      cancelled = true
    }
  }, [selectedCharId])

  // Load the resist-debuff catalogue once.
  useEffect(() => {
    getResistDebuffs()
      .then(setDebuffList)
      .catch(() => {})
  }, [])

  // autoMod is the multiplier a bard song picks up from the selected
  // character's instrument modifier (1 for non-songs or no bard character).
  const autoMod = (d: ResistDebuff): number => {
    if (!instrumentMods || !d.bard_skill) return 1
    const m = instrumentMods
    const eff =
      d.bard_skill === 70 ? m.wind :
      d.bard_skill === 54 ? m.stringed :
      d.bard_skill === 41 ? m.brass :
      d.bard_skill === 12 ? m.percussion :
      d.bard_skill === 49 ? m.singing : 10
    return eff / 10
  }
  // effectiveMod prefers a manual override, else the auto (instrument) value.
  const effectiveMod = (d: ResistDebuff): number => debuffMods[d.id] ?? autoMod(d)

  // Effective resists = the NPC's base plus the sum of selected debuff deltas
  // (deltas are negative), floored at 1 like the in-game resist calc.
  const modified = useMemo(() => {
    if (!target) return null
    // Debuffs are assumed cast at the caster's level (the debuffer's level
    // isn't modelled separately in v1).
    const sum = (k: ResistKey): number =>
      selectedDebuffs.reduce(
        (acc, d) => acc + moddedDelta(d, k, casterLevel, effectiveMod(d)),
        0,
      )
    const clamp = (v: number): number => Math.max(1, v)
    return {
      mr: clamp(target.mr + sum('mr')),
      cr: clamp(target.cr + sum('cr')),
      fr: clamp(target.fr + sum('fr')),
      dr: clamp(target.dr + sum('dr')),
      pr: clamp(target.pr + sum('pr')),
    }
  }, [target, selectedDebuffs, casterLevel, debuffMods, instrumentMods])

  // Load the class spell list whenever the class changes; keep only detrimental
  // spells (the only ones a resist check is meaningful for) and sort by name.
  useEffect(() => {
    let cancelled = false
    getSpellsByClass(casterClass, 1000)
      .then((resp) => {
        if (cancelled) return
        const dets = resp.items
          .filter((s) => s.good_effect === 0)
          .sort((a, b) => a.name.localeCompare(b.name))
        setSpells(dets)
        setSpellID((prev) => (dets.some((s) => s.id === prev) ? prev : null))
      })
      .catch((err: Error) => setError(err.message))
    return () => {
      cancelled = true
    }
  }, [casterClass])

  const filteredSpells = useMemo(() => {
    const q = spellFilter.trim().toLowerCase()
    const list = q ? spells.filter((s) => s.name.toLowerCase().includes(q)) : spells
    return list.slice(0, 400)
  }, [spells, spellFilter])

  // Run the resist check whenever a spell is chosen, the caster changes, or a
  // new target is selected.
  useEffect(() => {
    if (spellID == null || !target || !modified) {
      setResult(null)
      return
    }
    let cancelled = false
    setError(null)
    postResistCheck({
      spell_id: spellID,
      caster_level: casterLevel,
      caster_class: casterClass,
      caster_cha: casterCHA,
      target_level: target.level,
      target_mr: modified.mr,
      target_cr: modified.cr,
      target_fr: modified.fr,
      target_dr: modified.dr,
      target_pr: modified.pr,
      target_special_abilities: target.specialAbilities,
    })
      .then((r) => {
        if (!cancelled) setResult(r)
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message)
          setResult(null)
        }
      })
    return () => {
      cancelled = true
    }
  }, [spellID, casterLevel, casterClass, casterCHA, target, modified])

  const addDebuff = (d: ResistDebuff): void => {
    setSelectedDebuffs((prev) => (prev.some((x) => x.id === d.id) ? prev : [...prev, d]))
    setDebuffQuery('')
  }
  const removeDebuff = (id: number): void => {
    setSelectedDebuffs((prev) => prev.filter((d) => d.id !== id))
    setDebuffMods((prev) => {
      if (!(id in prev)) return prev
      const next = { ...prev }
      delete next[id]
      return next
    })
  }
  const setDebuffMod = (id: number, mod: number): void => {
    setDebuffMods((prev) => ({ ...prev, [id]: mod }))
  }

  // Clear the target's debuffs when a new target is picked (resists differ).
  useEffect(() => {
    setSelectedDebuffs([])
    setDebuffMods({})
  }, [target?.name])

  const debuffMatches = useMemo(() => {
    const q = debuffQuery.trim().toLowerCase()
    if (!q) return []
    return debuffList
      .filter((d) => d.name.toLowerCase().includes(q) && !selectedDebuffs.some((s) => s.id === d.id))
      .slice(0, 10)
  }, [debuffQuery, debuffList, selectedDebuffs])

  // Dismiss the debuff results dropdown on click outside.
  useEffect(() => {
    if (debuffMatches.length === 0) return
    const handler = (e: MouseEvent) => {
      if (!debuffSearchRef.current?.contains(e.target as Node)) setDebuffQuery('')
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [debuffMatches.length])

  // applyNPCTarget loads the target from any NPC row — shared by the in-game
  // current-target button and the manual database search, so whichever is
  // invoked most recently wins.
  const applyNPCTarget = (npc: NPC): void => {
    setTarget({
      name: npcDisplayName(npc),
      // Worst case: use the top of the NPC's level range.
      level: Math.max(npc.level, npc.max_level || 0),
      mr: npc.mr,
      cr: npc.cr,
      fr: npc.fr,
      dr: npc.dr,
      pr: npc.pr,
      specialAbilities: npc.special_abilities ?? '',
    })
  }

  const useCurrentTarget = (): void => {
    setTargetNote(null)
    getOverlayNPCTarget()
      .then((t) => {
        // No in-game target: leave any previously selected target in place
        // rather than clearing it.
        if (!t.has_target || !t.npc_data) {
          setTargetNote('No NPC targeted in-game — keeping the current target.')
          return
        }
        applyNPCTarget(t.npc_data)
      })
      .catch(() => setTargetNote('Could not read the current target.'))
  }

  const selectSearchedNPC = (npc: NPC): void => {
    setTargetNote(null)
    applyNPCTarget(npc)
    setNpcQuery('')
    setNpcResults([])
  }

  // Search the NPC database as the user types (debounced). Include placeholder
  // NPCs (showPlaceholders=true) so results match the NPC database tab: named
  // bosses are stored with a leading '#' and are placeholder-filtered out
  // otherwise — yet they're exactly the targets you'd check resists against
  // (e.g. "rhag" → #Rhag`Zadune). A larger limit fits the wider result space.
  useEffect(() => {
    const q = npcQuery.trim()
    if (q.length < 2) {
      setNpcResults([])
      return
    }
    let cancelled = false
    const handle = setTimeout(() => {
      searchNPCs(q, 25, 0, true)
        .then((resp) => {
          // A no-match search returns items: null (Go nil slice), so default
          // to [] — otherwise npcResults.length throws and black-screens.
          if (!cancelled) setNpcResults(resp.items ?? [])
        })
        .catch(() => {
          if (!cancelled) setNpcResults([])
        })
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(handle)
    }
  }, [npcQuery])

  // Dismiss the NPC results dropdown on click outside the search box.
  useEffect(() => {
    if (npcResults.length === 0) return
    const handler = (e: MouseEvent) => {
      if (!npcSearchRef.current?.contains(e.target as Node)) setNpcResults([])
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [npcResults.length])

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-4 p-4">
      <div className="flex items-center gap-2">
        <Percent size={18} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-lg font-semibold">Resist Calculator</h1>
      </div>

      <CaveatBanner />

      {/* Caster */}
      <Section title="Caster">
        {characters.length > 0 && (
          <label className="mb-3 flex flex-col gap-1 text-xs">
            <span style={{ color: 'var(--color-muted)' }}>Character</span>
            <select
              value={selectedCharId ?? ''}
              onChange={(e) =>
                setSelectedCharId(e.target.value ? Number(e.target.value) : null)
              }
              className="rounded px-2 py-1.5 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              <option value="">Manual (no character)</option>
              {characters.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </label>
        )}
        <div className="grid grid-cols-3 gap-3">
          <NumField label="Level" value={casterLevel} onChange={setCasterLevel} min={1} max={65} />
          <label className="flex flex-col gap-1 text-xs">
            <span style={{ color: 'var(--color-muted)' }}>Class</span>
            <select
              value={casterClass}
              onChange={(e) => setCasterClass(Number(e.target.value))}
              className="rounded px-2 py-1.5 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              {CLASS_NAMES.map((n, i) => (
                <option key={i} value={i}>{n}</option>
              ))}
            </select>
          </label>
          <NumField
            label="Charisma"
            value={casterCHA}
            onChange={setCasterCHA}
            min={1}
            max={500}
            hint={casterClass === ENCHANTER ? 'Lowers charm/mez resist' : 'Only affects ENC charm/mez'}
          />
        </div>
      </Section>

      {/* Spell */}
      <Section title="Spell">
        <input
          type="text"
          value={spellFilter}
          onChange={(e) => setSpellFilter(e.target.value)}
          placeholder="Filter detrimental spells…"
          className="mb-2 w-full rounded px-2 py-1.5 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        />
        <select
          value={spellID ?? ''}
          onChange={(e) => setSpellID(e.target.value ? Number(e.target.value) : null)}
          size={8}
          className="w-full rounded px-2 py-1 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          {filteredSpells.length === 0 && <option value="" disabled>No spells</option>}
          {filteredSpells.map((s) => (
            <option key={s.id} value={s.id}>{s.name}</option>
          ))}
        </select>
        {spells.length > filteredSpells.length && (
          <p className="mt-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
            Showing {filteredSpells.length} of {spells.length} — narrow the filter to see more.
          </p>
        )}
      </Section>

      {/* Target */}
      <Section title="Target NPC">
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <button
            type="button"
            onClick={useCurrentTarget}
            className="flex shrink-0 items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium transition-colors"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
          >
            <Crosshair size={13} /> Use current target
          </button>
          <div ref={npcSearchRef} className="relative min-w-[12rem] flex-1">
            <Search
              size={13}
              className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2"
              style={{ color: 'var(--color-muted)' }}
            />
            <input
              type="text"
              value={npcQuery}
              onChange={(e) => setNpcQuery(e.target.value)}
              placeholder="Search NPC database…"
              className="w-full rounded py-1.5 pl-7 pr-2 text-xs"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            />
            {npcResults.length > 0 && (
              <ul
                className="absolute z-10 mt-1 max-h-60 w-full overflow-y-auto rounded shadow-lg"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {npcResults.map((npc) => (
                  <li key={npc.id}>
                    <button
                      type="button"
                      onClick={() => selectSearchedNPC(npc)}
                      className="flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-xs transition-colors hover:bg-(--color-surface-3)"
                    >
                      <span className="truncate">{npcDisplayName(npc)}</span>
                      <span className="shrink-0" style={{ color: 'var(--color-muted)' }}>
                        L{npc.level}
                        {npc.max_level > npc.level ? `–${npc.max_level}` : ''}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
          {target && (
            <span className="shrink-0 text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
              {target.name}
            </span>
          )}
        </div>
        {targetNote && (
          <p className="mb-2 text-xs" style={{ color: '#f59e0b' }}>{targetNote}</p>
        )}
        {target ? (
          <>
            <div className="grid grid-cols-3 gap-3">
              <ReadField label="Level (worst case)" value={target.level} />
              <ReadField label="Magic (MR)" value={target.mr} modified={modified?.mr} />
              <ReadField label="Fire (FR)" value={target.fr} modified={modified?.fr} />
              <ReadField label="Cold (CR)" value={target.cr} modified={modified?.cr} />
              <ReadField label="Disease (DR)" value={target.dr} modified={modified?.dr} />
              <ReadField label="Poison (PR)" value={target.pr} modified={modified?.pr} />
            </div>
            <DebuffPicker
              ref={debuffSearchRef}
              query={debuffQuery}
              setQuery={setDebuffQuery}
              matches={debuffMatches}
              selected={selectedDebuffs}
              level={casterLevel}
              modFor={effectiveMod}
              onAdd={addDebuff}
              onRemove={removeDebuff}
              onSetMod={setDebuffMod}
            />
          </>
        ) : (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            Pick a target with “Use current target” or the database search above.
            Resists and level come straight from the NPC.
          </p>
        )}
      </Section>

      {error && (
        <p className="text-sm" style={{ color: '#f87171' }}>{error}</p>
      )}

      {result && <Results result={result} />}
    </div>
  )
}

function Results({ result }: { result: ResistCheckResponse }): React.ReactElement {
  if (result.cannot_affect) {
    return (
      <Section title="Result — cannot affect target">
        <div className="flex items-start gap-2">
          <AlertTriangle size={16} className="mt-0.5 shrink-0" style={{ color: '#f87171' }} />
          <p className="text-sm">
            <strong style={{ color: 'var(--color-foreground)' }}>{result.spell_name}</strong>{' '}
            won&rsquo;t land on this target: {result.reason}
          </p>
        </div>
      </Section>
    )
  }

  if (result.unresistable) {
    return (
      <Section title="Result">
        <p className="text-sm">
          <strong style={{ color: 'var(--color-primary)' }}>{result.spell_name}</strong> is
          unresistable — it always lands fully.
        </p>
      </Section>
    )
  }

  const resistLine = (
    <p className="mb-3 text-xs" style={{ color: 'var(--color-muted)' }}>
      {result.resist_type_label} resist • target value {result.target_resist} • internal
      resist score {result.resist_chance}
    </p>
  )

  if (result.binary) {
    return (
      <Section title="Result — binary (land or resist)">
        {resistLine}
        <div className="grid grid-cols-2 gap-3">
          <Stat label="Land chance" value={pct(result.land_chance)} big primary />
          <Stat
            label="Avg casts to land"
            value={result.land_chance > 0 ? result.avg_casts_to_land.toFixed(1) : '—'}
            big
          />
        </div>
      </Section>
    )
  }

  return (
    <Section title="Result — partial-capable">
      {resistLine}
      <div className="grid grid-cols-3 gap-3">
        <Stat label="Full damage" value={pct(result.full_damage)} primary />
        <Stat label="Partial" value={pct(result.partial)} />
        <Stat label="Fully resisted" value={pct(result.full_resist)} />
      </div>
      <div className="mt-3 grid grid-cols-2 gap-3">
        <Stat label="Avg effectiveness" value={pct(result.expected_effectiveness)} big primary />
        <Stat
          label="Partial range"
          value={
            result.partial > 0
              ? `${pct(result.partial_min)} – ${pct(result.partial_max)}`
              : '—'
          }
          big
        />
      </div>
    </Section>
  )
}

// formatDeltas renders a debuff's resist reductions scaled at the given level
// (and manual multiplier), e.g. "MR -39" or "MR -60 · CR -60".
function formatDeltas(d: ResistDebuff, level: number, mod = 1): string {
  const order: ResistKey[] = ['mr', 'fr', 'cr', 'dr', 'pr']
  const parts: string[] = []
  for (const k of order) {
    const v = moddedDelta(d, k, level, mod)
    if (v !== 0) parts.push(`${k.toUpperCase()} ${v}`)
  }
  return parts.join(' · ')
}

interface DebuffPickerProps {
  query: string
  setQuery: (s: string) => void
  matches: ResistDebuff[]
  selected: ResistDebuff[]
  level: number
  modFor: (d: ResistDebuff) => number
  onAdd: (d: ResistDebuff) => void
  onRemove: (id: number) => void
  onSetMod: (id: number, mod: number) => void
}

const DebuffPicker = React.forwardRef<HTMLDivElement, DebuffPickerProps>(
  function DebuffPicker(
    { query, setQuery, matches, selected, level, modFor, onAdd, onRemove, onSetMod },
    ref,
  ) {
    return (
      <div className="mt-4">
        <span
          className="text-[10px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Resist debuffs
        </span>
        {selected.length > 0 && (
          <div className="mb-2 mt-1.5 flex flex-wrap gap-1.5">
            {selected.map((d) => {
              const mod = modFor(d)
              return (
              <span
                key={d.id}
                className="flex items-center gap-1.5 rounded px-2 py-1 text-[11px]"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <span className="font-medium">{d.name}</span>
                <span style={{ color: '#4ade80' }}>{formatDeltas(d, level, mod)}</span>
                <span
                  className="flex items-center gap-0.5"
                  title="Manual strength multiplier (e.g. bard instrument modifier)"
                >
                  <span style={{ color: 'var(--color-muted)' }}>×</span>
                  <input
                    type="number"
                    min={0}
                    step={0.05}
                    value={mod}
                    onChange={(e) =>
                      onSetMod(d.id, Math.max(0, Number(e.target.value) || 0))
                    }
                    className="w-12 rounded px-1 py-0.5 text-[11px] tabular-nums"
                    style={{
                      backgroundColor: 'var(--color-surface-3)',
                      border: `1px solid ${mod !== 1 ? 'var(--color-primary)' : 'var(--color-border)'}`,
                    }}
                  />
                </span>
                <button
                  type="button"
                  onClick={() => onRemove(d.id)}
                  className="opacity-60 hover:opacity-100"
                  title="Remove"
                >
                  <X size={11} />
                </button>
              </span>
              )
            })}
          </div>
        )}
        <div ref={ref} className="relative mt-1.5 max-w-sm">
          <Search
            size={13}
            className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2"
            style={{ color: 'var(--color-muted)' }}
          />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Add a resist debuff (Tashanian, Malo…)"
            className="w-full rounded py-1.5 pl-7 pr-2 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
            }}
          />
          {matches.length > 0 && (
            <ul
              className="absolute z-10 mt-1 max-h-60 w-full overflow-y-auto rounded shadow-lg"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              {matches.map((d) => (
                <li key={d.id}>
                  <button
                    type="button"
                    onClick={() => onAdd(d)}
                    className="flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-xs transition-colors hover:bg-(--color-surface-3)"
                  >
                    <span className="truncate">{d.name}</span>
                    <span className="shrink-0" style={{ color: 'var(--color-muted)' }}>
                      {formatDeltas(d, level, modFor(d))}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <p className="mt-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Assumes debuffs land. Same-line debuffs (e.g. Tashani + Tashanian)
          don&rsquo;t stack in-game — add only the strongest of a line. The
          &times; field scales a debuff&rsquo;s magnitude — auto-filled from a
          selected bard&rsquo;s instrument modifier (equipped instrument +
          Instrument/Singing Mastery), and editable for any class.
        </p>
      </div>
    )
  },
)

function CaveatBanner(): React.ReactElement {
  return (
    <div
      className="flex items-start gap-2 rounded-lg p-3 text-xs"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-muted-foreground)',
      }}
    >
      <AlertTriangle size={14} className="mt-0.5 shrink-0" style={{ color: '#f59e0b' }} />
      <span>
        Worst-case estimate. The resist math is a community-reverse-engineered
        approximation of Project Quarm&rsquo;s formula, the NPC level used is the
        top of its range, and results assume no resist debuffs are active. Treat
        these odds as a guide, not a guarantee.
      </span>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }): React.ReactElement {
  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-3 text-xs font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </h2>
      {children}
    </section>
  )
}

function NumField({
  label, value, onChange, min, max, hint,
}: {
  label: string
  value: number
  onChange: (n: number) => void
  min?: number
  max?: number
  hint?: string
}): React.ReactElement {
  return (
    <label className="flex flex-col gap-1 text-xs">
      <span style={{ color: 'var(--color-muted)' }}>{label}</span>
      <input
        type="number"
        value={value}
        min={min}
        max={max}
        onChange={(e) => onChange(Number(e.target.value))}
        className="rounded px-2 py-1.5 text-sm"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
        }}
      />
      {hint && <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>{hint}</span>}
    </label>
  )
}

// ReadField shows a derived target value (level / resist) read-only — the
// target's numbers come from the NPC and aren't hand-edited. When `modified`
// differs from the base value (resist debuffs applied) it's shown in brackets,
// green when lowered (easier to land), red when raised.
function ReadField({
  label, value, modified,
}: {
  label: string
  value: number
  modified?: number
}): React.ReactElement {
  const changed = modified != null && modified !== value
  const color = changed ? (modified < value ? '#4ade80' : '#f87171') : 'var(--color-foreground)'
  return (
    <div className="flex flex-col gap-1 text-xs">
      <span style={{ color: 'var(--color-muted)' }}>{label}</span>
      <div
        className="rounded px-2 py-1.5 text-sm tabular-nums"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-foreground)',
        }}
      >
        {value}
        {changed && (
          <span style={{ color }}> ({modified})</span>
        )}
      </div>
    </div>
  )
}

function Stat({
  label, value, big, primary,
}: {
  label: string
  value: string
  big?: boolean
  primary?: boolean
}): React.ReactElement {
  return (
    <div
      className="rounded p-3"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <div className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        {label}
      </div>
      <div
        className={big ? 'text-2xl font-bold' : 'text-lg font-semibold'}
        style={{ color: primary ? 'var(--color-primary)' : 'var(--color-foreground)' }}
      >
        {value}
      </div>
    </div>
  )
}
