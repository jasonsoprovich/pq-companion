import React, { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, PawPrint, Search } from 'lucide-react'
import {
  getCharmPets,
  getCharmSpells,
  listCharacters,
  searchZones,
  type CharmPet,
  type CharmPetsResponse,
  type CharmSpellOption,
} from '../services/api'
import type { Zone } from '../types/zone'

// The four charm-capable classes, in the spells_new / eqstat 0-based ordering.
const CHARM_CLASSES = [
  { idx: 13, name: 'Enchanter' },
  { idx: 10, name: 'Necromancer' },
  { idx: 5, name: 'Druid' },
  { idx: 7, name: 'Bard' },
]
const ENCHANTER = 13

type SortKey = 'dps' | 'level' | 'hp' | 'maxhit' | 'mr' | 'land'

interface SortState {
  key: SortKey
  dir: 'asc' | 'desc'
}

// pickDefaultSpell returns the strongest charm the class can cast at the given
// level (highest req level ≤ level), falling back to the lowest-tier charm so
// the dropdown always has a sensible selection.
function pickDefaultSpell(opts: CharmSpellOption[], level: number): number | null {
  if (opts.length === 0) return null
  const castable = opts.filter((o) => o.req_level <= level)
  if (castable.length > 0) {
    return castable.reduce((best, o) => (o.req_level > best.req_level ? o : best)).spell_id
  }
  // None castable yet — show the earliest charm in the line.
  return opts.reduce((low, o) => (o.req_level < low.req_level ? o : low)).spell_id
}

export default function CharmPetFinderPage(): React.ReactElement {
  const navigate = useNavigate()

  const [classIdx, setClassIdx] = useState(ENCHANTER)
  const [level, setLevel] = useState(60)
  const [casterCHA, setCasterCHA] = useState(75)

  const [zone, setZone] = useState<{ short: string; long: string } | null>(null)
  const [zoneQuery, setZoneQuery] = useState('')
  const [zoneResults, setZoneResults] = useState<Zone[]>([])
  const zoneBoxRef = useRef<HTMLDivElement>(null)

  const [spellOptions, setSpellOptions] = useState<CharmSpellOption[]>([])
  const [spellID, setSpellID] = useState<number | null>(null)

  const [data, setData] = useState<CharmPetsResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const [hideSummoners, setHideSummoners] = useState(false)
  const [levelSafeOnly, setLevelSafeOnly] = useState(false)
  const [sort, setSort] = useState<SortState>({ key: 'dps', dir: 'desc' })

  // Seed class/level/CHA from the active character when it's a charm class.
  useEffect(() => {
    listCharacters()
      .then((resp) => {
        const active = resp.characters.find((c) => c.name === resp.active)
        if (!active) return
        if (active.level > 0) setLevel(active.level)
        if (CHARM_CLASSES.some((c) => c.idx === active.class)) setClassIdx(active.class)
        if (active.base_cha > 0) setCasterCHA(active.base_cha)
      })
      .catch(() => {})
  }, [])

  // Load the class's charm spell line whenever the class changes, and default to
  // the strongest castable charm.
  useEffect(() => {
    let cancelled = false
    getCharmSpells(classIdx)
      .then((opts) => {
        if (cancelled) return
        setSpellOptions(opts)
        setSpellID((prev) =>
          opts.some((o) => o.spell_id === prev) ? prev : pickDefaultSpell(opts, level),
        )
      })
      .catch((err: Error) => setError(err.message))
    return () => {
      cancelled = true
    }
    // level intentionally excluded: changing level shouldn't reset a manual
    // spell choice; the default only applies on class change / first load.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [classIdx])

  // Zone search (debounced) — only when the typed query doesn't already match
  // the selected zone's name.
  useEffect(() => {
    const q = zoneQuery.trim()
    if (q.length < 2 || (zone && q === zone.long)) {
      setZoneResults([])
      return
    }
    let cancelled = false
    const t = setTimeout(() => {
      searchZones(q, {}, 12)
        .then((res) => {
          if (!cancelled) setZoneResults(res.items)
        })
        .catch(() => {})
    }, 180)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
  }, [zoneQuery, zone])

  // Close the zone dropdown on outside click.
  useEffect(() => {
    if (zoneResults.length === 0) return
    const handler = (e: MouseEvent): void => {
      if (zoneBoxRef.current && !zoneBoxRef.current.contains(e.target as Node)) {
        setZoneResults([])
      }
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [zoneResults.length])

  // Fetch the charmable NPCs whenever the inputs are complete.
  useEffect(() => {
    if (!zone || spellID == null || level < 1) {
      setData(null)
      return
    }
    let cancelled = false
    setLoading(true)
    setError(null)
    getCharmPets({ zone: zone.short, classIndex: classIdx, level, spellID, casterCHA })
      .then((res) => {
        if (!cancelled) setData(res)
      })
      .catch((err: Error) => {
        if (!cancelled) {
          setError(err.message)
          setData(null)
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
  }, [zone, classIdx, level, spellID, casterCHA])

  const selectedSpell = spellOptions.find((o) => o.spell_id === spellID) ?? null

  const visiblePets = useMemo(() => {
    if (!data) return []
    let pets = data.pets
    if (hideSummoners) pets = pets.filter((p) => !p.summon)
    if (levelSafeOnly) pets = pets.filter((p) => !p.level_warning)
    const dir = sort.dir === 'asc' ? 1 : -1
    const val = (p: CharmPet): number => {
      switch (sort.key) {
        case 'level':
          return p.level_max
        case 'hp':
          return p.hp_max
        case 'maxhit':
          return p.max_hit_max
        case 'mr':
          return p.mr
        case 'land':
          return p.land_chance
        default:
          return p.dps_max
      }
    }
    return [...pets].sort((a, b) => (val(a) - val(b)) * dir)
  }, [data, hideSummoners, levelSafeOnly, sort])

  const toggleSort = (key: SortKey): void => {
    setSort((prev) =>
      prev.key === key
        ? { key, dir: prev.dir === 'desc' ? 'asc' : 'desc' }
        : { key, dir: 'desc' },
    )
  }

  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-4 p-4">
      <div className="flex items-center gap-2">
        <PawPrint size={18} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-lg font-semibold">Charm Pet Finder</h1>
      </div>

      <p
        className="rounded-lg px-3 py-2 text-xs"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-muted-foreground)',
        }}
      >
        Lists a zone&rsquo;s charmable NPCs for your charm class and spell, ranked by
        melee DPS. HP/damage scaling across a spawn&rsquo;s level range and the charm
        land chance are best-effort estimates. A&nbsp;
        <AlertTriangle size={11} className="inline align-[-1px]" style={{ color: '#f59e0b' }} />
        &nbsp;means the name can spawn above the charm&rsquo;s level cap, so it
        won&rsquo;t hold on every spawn.
      </p>

      {/* Controls */}
      <Section title="Search">
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-4">
          {/* Zone */}
          <div ref={zoneBoxRef} className="relative flex flex-col gap-1">
            <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
              Zone
            </span>
            <div className="relative">
              <Search
                size={13}
                className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2"
                style={{ color: 'var(--color-muted)' }}
              />
              <input
                type="text"
                value={zoneQuery}
                onChange={(e) => setZoneQuery(e.target.value)}
                placeholder="Search zones…"
                className="w-full rounded py-1.5 pl-7 pr-2 text-sm"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              />
            </div>
            {zoneResults.length > 0 && (
              <ul
                className="absolute z-10 mt-[3.4rem] max-h-60 w-full overflow-y-auto rounded shadow-lg"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {zoneResults.map((z) => (
                  <li key={z.id}>
                    <button
                      type="button"
                      onClick={() => {
                        setZone({ short: z.short_name, long: z.long_name })
                        setZoneQuery(z.long_name)
                        setZoneResults([])
                      }}
                      className="flex w-full items-center justify-between gap-2 px-2 py-1.5 text-left text-xs transition-colors hover:bg-(--color-surface-3)"
                    >
                      <span className="truncate">{z.long_name}</span>
                      <span className="shrink-0" style={{ color: 'var(--color-muted)' }}>
                        {z.short_name}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {/* Class */}
          <label className="flex flex-col gap-1 text-xs">
            <span style={{ color: 'var(--color-muted)' }}>Class</span>
            <select
              value={classIdx}
              onChange={(e) => setClassIdx(Number(e.target.value))}
              className="rounded px-2 py-1.5 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              {CHARM_CLASSES.map((c) => (
                <option key={c.idx} value={c.idx}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>

          {/* Level */}
          <label className="flex flex-col gap-1 text-xs">
            <span style={{ color: 'var(--color-muted)' }}>Level</span>
            <input
              type="number"
              value={level}
              min={1}
              max={65}
              onChange={(e) => setLevel(Number(e.target.value))}
              className="rounded px-2 py-1.5 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            />
          </label>

          {/* Charm spell */}
          <label className="flex flex-col gap-1 text-xs">
            <span style={{ color: 'var(--color-muted)' }}>Charm spell</span>
            <select
              value={spellID ?? ''}
              onChange={(e) => setSpellID(e.target.value ? Number(e.target.value) : null)}
              className="rounded px-2 py-1.5 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              {spellOptions.length === 0 && (
                <option value="" disabled>
                  No charm spells
                </option>
              )}
              {spellOptions.map((o) => (
                <option key={o.spell_id} value={o.spell_id}>
                  {o.name} — caps L{o.max_charm_level} (req {o.req_level})
                  {o.req_level > level ? ' — not yet' : ''}
                </option>
              ))}
            </select>
          </label>
        </div>

        {classIdx === ENCHANTER && (
          <label className="mt-3 flex max-w-[12rem] flex-col gap-1 text-xs">
            <span style={{ color: 'var(--color-muted)' }}>
              Charisma <span style={{ color: 'var(--color-muted)' }}>(charm resist)</span>
            </span>
            <input
              type="number"
              value={casterCHA}
              min={1}
              max={500}
              onChange={(e) => setCasterCHA(Number(e.target.value))}
              className="rounded px-2 py-1.5 text-sm"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            />
          </label>
        )}

        <div className="mt-3 flex flex-wrap items-center gap-4 text-xs">
          <Check label="Hide summoners" checked={hideSummoners} onChange={setHideSummoners} />
          <Check label="Level-safe only" checked={levelSafeOnly} onChange={setLevelSafeOnly} />
        </div>
      </Section>

      {error && (
        <p className="text-sm" style={{ color: '#f87171' }}>
          {error}
        </p>
      )}

      {!zone && (
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Pick a zone to see its charmable NPCs.
        </p>
      )}

      {zone && data && (
        <Section
          title={
            `${data.count} charmable NPC${data.count === 1 ? '' : 's'} — ` +
            `${data.spell_name} (caps L${data.max_charm_level}` +
            `${data.restriction ? `, ${data.restriction} only` : ''})`
          }
        >
          {loading && (
            <p className="mb-2 text-xs" style={{ color: 'var(--color-muted)' }}>
              Loading…
            </p>
          )}
          {visiblePets.length === 0 ? (
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              No charmable NPCs match the current filters.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr style={{ color: 'var(--color-muted)' }}>
                    <Th label="Level" sortKey="level" sort={sort} onSort={toggleSort} />
                    <th className="px-2 py-1.5 font-semibold">Name</th>
                    <th className="px-2 py-1.5 font-semibold">Class</th>
                    <th className="px-2 py-1.5 font-semibold">Body</th>
                    <th className="px-2 py-1.5 font-semibold">Summon</th>
                    <Th label="HP" sortKey="hp" sort={sort} onSort={toggleSort} />
                    <Th label="Max Hit" sortKey="maxhit" sort={sort} onSort={toggleSort} />
                    <th className="px-2 py-1.5 font-semibold">Delay</th>
                    <Th label="DPS" sortKey="dps" sort={sort} onSort={toggleSort} />
                    <Th label="MR" sortKey="mr" sort={sort} onSort={toggleSort} />
                    <Th label="Land%" sortKey="land" sort={sort} onSort={toggleSort} />
                    <th className="px-2 py-1.5 font-semibold">Other</th>
                  </tr>
                </thead>
                <tbody>
                  {visiblePets.map((p) => (
                    <PetRow
                      key={p.npc_id}
                      pet={p}
                      onOpen={() => navigate(`/npcs?select=${p.npc_id}`)}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Section>
      )}

      {zone && !data && !loading && !error && (
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select a charm spell to search.
        </p>
      )}

      {selectedSpell && selectedSpell.req_level > level && (
        <p className="text-xs" style={{ color: '#f59e0b' }}>
          You can&rsquo;t cast {selectedSpell.name} until level {selectedSpell.req_level} — results
          show what it would charm.
        </p>
      )}
    </div>
  )
}

function PetRow({ pet, onOpen }: { pet: CharmPet; onOpen: () => void }): React.ReactElement {
  const levelText =
    pet.level_max > pet.level_min ? `${pet.level_min}–${pet.level_max}` : `${pet.level_min}`
  const hpText = range(pet.hp_min, pet.hp_max, (n) => n.toLocaleString())
  const hitText = range(pet.max_hit_min, pet.max_hit_max)
  const dpsText = range1(pet.dps_min, pet.dps_max)
  const landColor =
    pet.land_chance >= 0.9 ? '#4ade80' : pet.land_chance >= 0.6 ? '#facc15' : '#f87171'

  return (
    <tr style={{ borderTop: '1px solid var(--color-border)' }}>
      <td className="px-2 py-1.5">
        <span className="inline-flex items-center gap-1">
          {pet.level_warning && (
            <AlertTriangle size={12} style={{ color: '#f59e0b' }} aria-label="above charm cap" />
          )}
          {levelText}
        </span>
      </td>
      <td className="px-2 py-1.5">
        <button
          type="button"
          onClick={onOpen}
          className="text-left font-medium hover:underline"
          style={{ color: 'var(--color-primary)' }}
        >
          {pet.name.replace(/_/g, ' ')}
        </button>
      </td>
      <td className="px-2 py-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
        {pet.class_name || '—'}
      </td>
      <td className="px-2 py-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
        {pet.body_type_name || '—'}
      </td>
      <td className="px-2 py-1.5" style={{ color: pet.summon ? '#f87171' : 'var(--color-muted)' }}>
        {pet.summon ? 'Yes' : 'No'}
      </td>
      <td className="px-2 py-1.5">{hpText}</td>
      <td className="px-2 py-1.5">{hitText}</td>
      <td className="px-2 py-1.5">{(pet.attack_delay / 10).toFixed(1)}s</td>
      <td className="px-2 py-1.5 font-medium">{dpsText}</td>
      <td className="px-2 py-1.5">{pet.mr}</td>
      <td className="px-2 py-1.5 font-medium" style={{ color: landColor }}>
        {(pet.land_chance * 100).toFixed(0)}%
      </td>
      <td className="px-2 py-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
        {[pet.gate ? 'Gates' : '', ...(pet.abilities ?? [])].filter(Boolean).join(', ') || '—'}
      </td>
    </tr>
  )
}

function range(lo: number, hi: number, fmt: (n: number) => string = String): string {
  return hi > lo ? `${fmt(lo)}–${fmt(hi)}` : fmt(lo)
}

function range1(lo: number, hi: number): string {
  const f = (n: number): string => n.toFixed(1)
  return hi > lo ? `${f(lo)}–${f(hi)}` : f(lo)
}

function Th({
  label,
  sortKey,
  sort,
  onSort,
}: {
  label: string
  sortKey: SortKey
  sort: SortState
  onSort: (k: SortKey) => void
}): React.ReactElement {
  const active = sort.key === sortKey
  return (
    <th className="px-2 py-1.5 font-semibold">
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className="inline-flex items-center gap-1 hover:underline"
        style={{ color: active ? 'var(--color-foreground)' : 'inherit' }}
      >
        {label}
        {active && <span>{sort.dir === 'desc' ? '▼' : '▲'}</span>}
      </button>
    </th>
  )
}

function Check({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (b: boolean) => void
}): React.ReactElement {
  return (
    <label className="flex cursor-pointer items-center gap-1.5">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
    </label>
  )
}

function Section({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}): React.ReactElement {
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
