import React, { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, Crosshair, Percent, Search } from 'lucide-react'
import {
  getSpellsByClass,
  getOverlayNPCTarget,
  listCharacters,
  postResistCheck,
  searchNPCs,
  type ResistCheckResponse,
} from '../services/api'
import type { Spell } from '../types/spell'
import type { NPC } from '../types/npc'

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

  // Seed the caster from the active character (level/class/base CHA). CHA is
  // editable since the formula wants total (buffed) charisma, not base.
  useEffect(() => {
    listCharacters()
      .then((resp) => {
        const active = resp.characters.find((c) => c.name === resp.active)
        if (!active) return
        if (active.level > 0) setCasterLevel(active.level)
        if (active.class >= 0 && active.class <= 14) setCasterClass(active.class)
        if (active.base_cha > 0) setCasterCHA(active.base_cha)
      })
      .catch(() => {})
  }, [])

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
    if (spellID == null || !target) {
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
      target_mr: target.mr,
      target_cr: target.cr,
      target_fr: target.fr,
      target_dr: target.dr,
      target_pr: target.pr,
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
  }, [spellID, casterLevel, casterClass, casterCHA, target])

  // applyNPCTarget loads the target from any NPC row — shared by the in-game
  // current-target button and the manual database search, so whichever is
  // invoked most recently wins.
  const applyNPCTarget = (npc: NPC): void => {
    setTarget({
      name: npc.name,
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

  // Search the NPC database as the user types (debounced).
  useEffect(() => {
    const q = npcQuery.trim()
    if (q.length < 2) {
      setNpcResults([])
      return
    }
    let cancelled = false
    const handle = setTimeout(() => {
      searchNPCs(q, 10)
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
                      <span className="truncate">{npc.name}</span>
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
          <div className="grid grid-cols-3 gap-3">
            <ReadField label="Level (worst case)" value={target.level} />
            <ReadField label="Magic (MR)" value={target.mr} />
            <ReadField label="Fire (FR)" value={target.fr} />
            <ReadField label="Cold (CR)" value={target.cr} />
            <ReadField label="Disease (DR)" value={target.dr} />
            <ReadField label="Poison (PR)" value={target.pr} />
          </div>
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
// target's numbers come from the NPC and aren't hand-edited.
function ReadField({ label, value }: { label: string; value: number }): React.ReactElement {
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
