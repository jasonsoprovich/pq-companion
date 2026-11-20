import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useCachedState } from '../hooks/useCachedState'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  ArrowDown, ArrowUp, CheckCircle2, Circle, Flag, Search, X,
} from 'lucide-react'
import {
  getNPCsByZone,
  getPopFlagDataset,
  getPopFlags,
  getZone,
  getZoneByShortName,
  getZoneConnections,
  getZoneDrops,
  getZoneExpansions,
  getZoneForage,
  getZoneGroundSpawns,
  getZoneLockouts,
  getZoneNPCSpawns,
  getZoneRaw,
  searchZones,
} from '../services/api'
import type { ZoneDropsOptions, ZoneDropsSort } from '../services/api'
import RawDataModal from '../components/RawDataModal'
import { ZoneMapPanel } from '../components/maps/ZoneMapPanel'
import { useHasMap } from '../hooks/useMapZones'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import type { NPC } from '../types/npc'
import type {
  Zone,
  ZoneConnection,
  ZoneDropItem,
  ZoneForageItem,
  ZoneGroundSpawn,
  ZoneNPCSpawn,
} from '../types/zone'
import type { ZoneLockoutBoss } from '../types/lockouts'
import type { PoPFlag } from '../types/popflag'
import { className, npcDisplayName, npcLevelLabel } from '../lib/npcHelpers'
import { zoneExpansionName } from '../lib/enumsCache'
import { matchesSearch, WILDCARD_HINT } from '../lib/textSearch'

function expansionName(id: number): string {
  return zoneExpansionName(id)
}

function bindLabel(canbind: number): string {
  if (canbind === 0) return 'No'
  if (canbind === 1) return 'Druid / Wizard only'
  return 'Yes'
}

function expModLabel(mod: number): string {
  if (mod == null || !isFinite(mod)) return '—'
  return `${Math.round(mod * 100)}%`
}

function formatRespawn(ms: number): string {
  if (ms <= 0) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return rem > 0 ? `${m}m ${rem}s` : `${m}m`
}

function npcNameDisplay(raw: string): string {
  return raw.replace(/_/g, ' ')
}

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (zone: Zone) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useCachedState('zones.query', '')
  const [expansion, setExpansion] = useCachedState<number | null>('zones.expansion', null)
  const [expansionOptions, setExpansionOptions] = useState<number[]>([])
  const [zones, setZones] = useState<Zone[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Monotonic token so a slow earlier request can't clobber a newer one.
  const seqRef = useRef(0)

  const runSearch = useCallback((q: string, exp: number | null) => {
    const seq = ++seqRef.current
    setLoading(true)
    setError(null)
    searchZones(q, exp !== null ? { expansion: exp } : {}, 1000, 0)
      .then((res) => {
        if (seq !== seqRef.current) return
        setZones(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => { if (seq === seqRef.current) setError(err.message) })
      .finally(() => { if (seq === seqRef.current) setLoading(false) })
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, expansion), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, expansion, runSearch])

  // Fire the search immediately on mount so the list populates without waiting
  // for the 300ms debounce. Uses the (possibly cached) query/expansion so a
  // restored search shows its results right away (issue #9).
  useEffect(() => {
    runSearch(query, expansion)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runSearch])

  useEffect(() => {
    getZoneExpansions()
      .then((opts) => setExpansionOptions(opts ?? []))
      .catch(() => setExpansionOptions([]))
  }, [])

  return (
    <div
      className="flex w-72 shrink-0 flex-col border-r"
      style={{ borderColor: 'var(--color-border)' }}
    >
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
        <input
          type="text"
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
          style={{ color: 'var(--color-foreground)' }}
          placeholder="Search zones…"
          title={WILDCARD_HINT}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          spellCheck={false}
        />
        {query && (
          <button onClick={() => setQuery('')} className="shrink-0">
            <X size={12} style={{ color: 'var(--color-muted)' }} />
          </button>
        )}
      </div>

      <div
        className="flex items-center gap-2 border-b px-3 py-1.5"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <label
          className="text-[10px] font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Expansion
        </label>
        <select
          className="flex-1 rounded border px-1.5 py-0.5 text-xs outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            borderColor: 'var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          value={expansion === null ? '' : String(expansion)}
          onChange={(e) =>
            setExpansion(e.target.value === '' ? null : Number(e.target.value))
          }
        >
          <option value="">All</option>
          {expansionOptions.map((id) => (
            <option key={id} value={id}>
              {expansionName(id)}
            </option>
          ))}
        </select>
      </div>

      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} zones`}
      </div>

      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>
            {error}
          </p>
        )}
        {!error &&
          zones.map((zone) => (
            <button
              key={zone.id}
              onClick={() => onSelect(zone)}
              className="w-full px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor:
                  selectedId === zone.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  selectedId === zone.id
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              <div
                className="truncate text-sm font-medium"
                style={{
                  color:
                    selectedId === zone.id
                      ? 'var(--color-primary)'
                      : 'var(--color-foreground)',
                }}
              >
                {zone.long_name || zone.short_name}
              </div>
              <div className="mt-0.5 flex items-center gap-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                <span className="truncate">
                  {zone.short_name}
                  {zone.npc_level_max > 0 && (
                    zone.npc_level_min === zone.npc_level_max
                      ? ` · Lv ${zone.npc_level_min}`
                      : ` · Lv ${zone.npc_level_min}–${zone.npc_level_max}`
                  )}
                </span>
                {isFinite(zone.exp_mod) && zone.exp_mod !== 1.0 && (
                  <span
                    className="shrink-0 rounded px-1 py-0.5 text-[10px] font-semibold"
                    style={{
                      backgroundColor:
                        zone.exp_mod > 1.0
                          ? 'rgba(34,197,94,0.15)'
                          : 'rgba(234,179,8,0.15)',
                      color: zone.exp_mod > 1.0 ? 'rgb(134,239,172)' : 'rgb(253,224,71)',
                    }}
                  >
                    ZEM {Math.round(zone.exp_mod * 100)}%
                  </span>
                )}
              </div>
            </button>
          ))}
      </div>
    </div>
  )
}

// ── Shared helpers ─────────────────────────────────────────────────────────────

interface StatRowProps {
  label: string
  value: string | number
}

function StatRow({ label, value }: StatRowProps): React.ReactElement {
  return (
    <div className="flex justify-between py-0.5 text-sm">
      <span style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

interface SectionProps {
  title: string
  children: React.ReactNode
}

function Section({ title, children }: SectionProps): React.ReactElement {
  return (
    <div>
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      <div
        className="rounded border px-3 py-1"
        style={{
          backgroundColor: 'var(--color-surface)',
          borderColor: 'var(--color-border)',
        }}
      >
        {children}
      </div>
    </div>
  )
}

function EmptyState({ message }: { message: string }): React.ReactElement {
  return (
    <p className="py-4 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
      {message}
    </p>
  )
}

function LoadingState(): React.ReactElement {
  return (
    <p className="py-4 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
      Loading…
    </p>
  )
}

function ErrorState({ message }: { message: string }): React.ReactElement {
  return (
    <p className="py-4 text-center text-sm" style={{ color: 'var(--color-destructive)' }}>
      {message}
    </p>
  )
}

// ── List toolbar (sort + filter controls shared by the list tabs) ─────────────

const controlStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  borderColor: 'var(--color-border)',
  color: 'var(--color-foreground)',
}

function Toolbar({ children }: { children: React.ReactNode }): React.ReactElement {
  return <div className="mb-2 flex flex-wrap items-center gap-2">{children}</div>
}

function FilterInput({
  value,
  onChange,
  placeholder,
}: {
  value: string
  onChange: (v: string) => void
  placeholder: string
}): React.ReactElement {
  return (
    <div
      className="flex min-w-40 flex-1 items-center gap-1.5 rounded border px-2 py-0.5"
      style={controlStyle}
    >
      <Search size={12} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
      <input
        type="text"
        className="flex-1 bg-transparent text-xs outline-none placeholder:text-(--color-muted)"
        style={{ color: 'var(--color-foreground)' }}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
      />
      {value && (
        <button onClick={() => onChange('')} className="shrink-0">
          <X size={11} style={{ color: 'var(--color-muted)' }} />
        </button>
      )}
    </div>
  )
}

interface SortOption<K extends string> {
  key: K
  label: string
  // Direction applied when this key is picked; numeric columns read best
  // largest-first, names alphabetically.
  defaultDesc: boolean
}

interface SortState<K extends string> {
  key: K
  desc: boolean
}

function SortControl<K extends string>({
  options,
  value,
  onChange,
}: {
  options: SortOption<K>[]
  value: SortState<K>
  onChange: (v: SortState<K>) => void
}): React.ReactElement {
  return (
    <div className="flex items-center gap-1">
      <label
        className="text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        Sort
      </label>
      <select
        className="rounded border px-1.5 py-0.5 text-xs outline-none"
        style={controlStyle}
        value={value.key}
        onChange={(e) => {
          const opt = options.find((o) => o.key === e.target.value)
          if (opt) onChange({ key: opt.key, desc: opt.defaultDesc })
        }}
      >
        {options.map((o) => (
          <option key={o.key} value={o.key}>
            {o.label}
          </option>
        ))}
      </select>
      <button
        onClick={() => onChange({ ...value, desc: !value.desc })}
        className="rounded border p-1"
        style={controlStyle}
        title={value.desc ? 'Descending' : 'Ascending'}
      >
        {value.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} />}
      </button>
    </div>
  )
}

// compareBy builds a comparator for one sort key, falling back to name so
// equal values keep a stable, readable order.
function compareBy<T>(
  get: (row: T) => number | string,
  name: (row: T) => string,
  desc: boolean,
): (a: T, b: T) => number {
  return (a, b) => {
    const av = get(a)
    const bv = get(b)
    let c =
      typeof av === 'number' && typeof bv === 'number'
        ? av - bv
        : String(av).localeCompare(String(bv))
    if (desc) c = -c
    return c !== 0 ? c : name(a).localeCompare(name(b))
  }
}

function parseLevel(raw: string): number | null {
  const n = parseInt(raw, 10)
  return Number.isFinite(n) ? n : null
}

// ── Flag requirement ──────────────────────────────────────────────────────────

interface AccessStep {
  flag: PoPFlag
  done: boolean | null // null = no active character / status unavailable
}

// FlagRequiredSection lists the PoP Flags tracker steps that unlock a
// flag-gated zone, with the active character's progress when one is set.
// zone.flag_needed only says THAT a flag is needed; the steps come from the
// backend's curated popflag.ZoneAccess table.
function FlagRequiredSection({ zone }: { zone: Zone }): React.ReactElement {
  const navigate = useNavigate()
  const { active } = useActiveCharacter()
  const [steps, setSteps] = useState<AccessStep[] | null>(null)

  useEffect(() => {
    let cancelled = false
    setSteps(null)
    Promise.all([
      getPopFlagDataset(),
      active ? getPopFlags(active).catch(() => null) : Promise.resolve(null),
    ])
      .then(([ds, resolved]) => {
        if (cancelled) return
        const ids = ds.zone_access?.[zone.short_name] ?? []
        const byID = new Map(ds.flags.map((f) => [f.id, f]))
        const doneByID = new Map(
          (resolved?.flags ?? []).map((f) => [f.id, f.done]),
        )
        setSteps(
          ids.flatMap((id) => {
            const flag = byID.get(id)
            if (!flag) return []
            return [{ flag, done: resolved ? (doneByID.get(id) ?? false) : null }]
          }),
        )
      })
      .catch(() => {
        if (!cancelled) setSteps([])
      })
    return () => {
      cancelled = true
    }
  }, [zone.short_name, active])

  const allDone =
    steps != null && steps.length > 0 && steps.every((s) => s.done === true)

  return (
    <Section title="Flag Required">
      <div
        className="flex items-center gap-2 py-1 text-sm"
        style={{ color: 'var(--color-foreground)' }}
      >
        <Flag
          size={14}
          style={{
            color: allDone ? 'var(--color-success)' : 'var(--color-primary)',
          }}
        />
        <span>
          {allDone
            ? `${active} has the flag for this zone.`
            : 'A zone flag is required to enter.'}
        </span>
      </div>
      {steps != null && steps.length > 0 && (
        <div className="flex flex-col gap-0.5 pb-1">
          {steps.map(({ flag, done }) => (
            <div key={flag.id} className="flex items-start gap-2 text-sm">
              <span className="mt-0.5 shrink-0">
                {done ? (
                  <CheckCircle2
                    size={13}
                    style={{ color: 'var(--color-success)' }}
                  />
                ) : (
                  <Circle size={13} style={{ color: 'var(--color-muted)' }} />
                )}
              </span>
              <span style={{ color: 'var(--color-foreground)' }}>
                {flag.label}
                <span style={{ color: 'var(--color-muted-foreground)' }}>
                  {' '}
                  — {flag.zone}
                </span>
              </span>
            </div>
          ))}
        </div>
      )}
      {steps != null && (
        <div className="pb-1">
          <button
            onClick={() => navigate('/pop-flags')}
            className="cursor-pointer text-xs hover:underline"
            style={{ color: 'var(--color-primary)' }}
          >
            {steps.length > 0
              ? 'Open PoP Flags tracker →'
              : 'See the PoP Flags tracker →'}
          </button>
        </div>
      )}
    </Section>
  )
}

// ── Tab: Overview ──────────────────────────────────────────────────────────────

function OverviewTab({ zone }: { zone: Zone }): React.ReactElement {
  const navigate = useNavigate()
  const coordStr = `Y: ${zone.safe_y.toFixed(1)}, X: ${zone.safe_x.toFixed(1)}, Z: ${zone.safe_z.toFixed(1)}`
  const gy = zone.graveyard
  const gyCoordStr = gy
    ? `Y: ${gy.y.toFixed(1)}, X: ${gy.x.toFixed(1)}, Z: ${gy.z.toFixed(1)}`
    : ''
  const gyTimerStr = gy
    ? gy.timer_minutes === 1
      ? '1 minute'
      : `${gy.timer_minutes} minutes`
    : ''
  return (
    <div className="flex flex-col gap-3">
      <Section title="Quick Facts">
        <StatRow label="Expansion" value={expansionName(zone.expansion)} />
        <StatRow label="XP Modifier" value={expModLabel(zone.exp_mod)} />
        <StatRow label="Outdoor" value={zone.outdoor ? 'Yes' : 'No'} />
        <StatRow label="Hotzone" value={zone.hotzone ? 'Yes' : 'No'} />
        <StatRow label="Levitation" value={zone.can_levitate ? 'Allowed' : 'Restricted'} />
        <StatRow label="Binding" value={bindLabel(zone.can_bind)} />
      </Section>
      {zone.flag_needed && <FlagRequiredSection zone={zone} />}
      <Section title="Zone Info">
        <StatRow label="Zone ID" value={zone.zone_id_number} />
        <StatRow
          label="Level Range"
          value={
            zone.npc_level_max > 0
              ? zone.npc_level_min === zone.npc_level_max
                ? `${zone.npc_level_min}`
                : `${zone.npc_level_min}–${zone.npc_level_max}`
              : 'Unknown'
          }
        />
        <StatRow label="Succor Point" value={coordStr} />
        {zone.min_level > 0 && (
          <StatRow label="Min Level to Enter" value={zone.min_level} />
        )}
        {zone.note && <StatRow label="Note" value={zone.note} />}
      </Section>
      <Section title="Bard Swarm Limits">
        <StatRow
          label="PBAE spell hit cap"
          value="12 targets"
        />
        <StatRow
          label="Damaged-mob warp limit"
          value="15 pulled"
        />
        <StatRow
          label="Undamaged warp limit"
          value={`${zone.pull_limit} pulled`}
        />
        <div
          className="pt-1 text-[10px] leading-snug"
          style={{ color: 'var(--color-muted)' }}
        >
          Pull more than the undamaged limit and untouched mobs begin warping;
          damage more than 15 and the engaged mobs warp. PBAE hit cap and
          damaged-warp limit are server-wide; the undamaged limit is per-zone.
        </div>
      </Section>
      {gy && (
        <Section title="Graveyard">
          <div className="flex justify-between py-0.5 text-sm">
            <span style={{ color: 'var(--color-muted-foreground)' }}>Pops out to</span>
            <button
              onClick={() => navigate(`/zones?select=${gy.zone_id}`)}
              className="cursor-pointer text-right hover:underline"
              style={{ color: 'var(--color-primary)' }}
            >
              {gy.long_name || gy.short_name}
            </button>
          </div>
          <StatRow label="Location" value={gyCoordStr} />
          <StatRow label="Corpse Timer" value={gyTimerStr} />
        </Section>
      )}
    </div>
  )
}

// ── Tab: NPCs ──────────────────────────────────────────────────────────────────

type NPCSortKey = 'name' | 'level' | 'hp' | 'class' | 'spawn'

const NPC_SORT_OPTIONS: SortOption<NPCSortKey>[] = [
  { key: 'name', label: 'Name', defaultDesc: false },
  { key: 'level', label: 'Level', defaultDesc: true },
  { key: 'hp', label: 'HP', defaultDesc: true },
  { key: 'class', label: 'Class', defaultDesc: false },
  { key: 'spawn', label: 'Spawn %', defaultDesc: true },
]

function fmtPct(p: number): string {
  if (p >= 10 || Number.isInteger(p)) return `${Math.round(p)}%`
  return `${p.toFixed(1)}%`
}

// spawnLabel summarizes an NPC's spawn-group share, e.g. "20% spawn · 4 pts"
// or "5–50% spawn" when its groups disagree.
function spawnLabel(s: ZoneNPCSpawn): string {
  if (s.inactive === 'other_era') return 'Other era'
  if (s.inactive === 'zero_weight') return 'Never rolls'
  const lo = fmtPct(s.min_pct)
  const hi = fmtPct(s.max_pct)
  const pct = lo === hi ? hi : `${lo.replace('%', '')}–${hi}`
  return `${pct} spawn · ${s.points} pt${s.points === 1 ? '' : 's'}`
}

function npcSortValue(
  npc: NPC,
  key: NPCSortKey,
  spawns: Map<number, ZoneNPCSpawn>,
): number | string {
  switch (key) {
    case 'spawn':
      // Script-spawned and inactive NPCs have no share (inactive rows carry
      // 0); both sort as rarer than anything a spawn point can roll.
      return spawns.get(npc.id)?.max_pct || -1
    case 'level':
      // Ranged spawns sort by their top level, with the floor as a
      // tie-breaker baked into the fraction.
      return Math.max(npc.level, npc.max_level) + npc.level / 1000
    case 'hp':
      return npc.hp
    case 'class':
      return className(npc.class)
    default:
      return npcDisplayName(npc).toLowerCase()
  }
}

function NPCsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [npcs, setNpcs] = useState<NPC[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // Sort/filter persist across zone switches so comparing zones keeps the
  // same view.
  const [sort, setSort] = useCachedState<SortState<NPCSortKey>>('zones.npcs.sort', {
    key: 'name',
    desc: false,
  })
  const [filter, setFilter] = useCachedState('zones.npcs.filter', '')
  const [minLevel, setMinLevel] = useCachedState('zones.npcs.minLevel', '')
  const [maxLevel, setMaxLevel] = useCachedState('zones.npcs.maxLevel', '')
  const [hideTrivial, setHideTrivial] = useCachedState('zones.npcs.hideTrivial', false)
  const [showInactive, setShowInactive] = useCachedState('zones.npcs.showInactive', false)
  const [spawns, setSpawns] = useState<Map<number, ZoneNPCSpawn>>(new Map())

  useEffect(() => {
    setLoading(true)
    setError(null)
    getNPCsByZone(shortName)
      .then((res) => {
        setNpcs(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
    // Spawn shares are supplementary — if they fail the list still works.
    let stale = false
    setSpawns(new Map())
    getZoneNPCSpawns(shortName)
      .then((rows) => {
        if (!stale) setSpawns(new Map(rows.map((r) => [r.npc_id, r])))
      })
      .catch(() => {})
    return () => {
      stale = true
    }
  }, [shortName])

  const visible = useMemo(() => {
    const q = filter.trim().toLowerCase()
    const lo = parseLevel(minLevel)
    const hi = parseLevel(maxLevel)
    const rows = npcs.filter((npc) => {
      if (q && !matchesSearch(npcDisplayName(npc), q)) return false
      // A ranged spawn passes if any part of its range overlaps the filter.
      const top = Math.max(npc.level, npc.max_level)
      if (lo !== null && top < lo) return false
      if (hi !== null && npc.level > hi) return false
      if (hideTrivial && (npc.hp <= 0 || npc.level <= 0 || npc.class <= 0)) return false
      if (!showInactive && spawns.get(npc.id)?.inactive) return false
      return true
    })
    return rows.sort(
      compareBy(
        (n) => npcSortValue(n, sort.key, spawns),
        (n) => npcDisplayName(n),
        sort.desc,
      ),
    )
  }, [npcs, filter, minLevel, maxLevel, hideTrivial, showInactive, sort, spawns])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (npcs.length === 0) return <EmptyState message="No spawn data found." />

  const filtered = visible.length !== npcs.length

  return (
    <div>
      <Toolbar>
        <FilterInput value={filter} onChange={setFilter} placeholder="Filter NPCs…" />
        <div className="flex items-center gap-1">
          <label
            className="text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Lv
          </label>
          {[
            { v: minLevel, set: setMinLevel, ph: 'min' },
            { v: maxLevel, set: setMaxLevel, ph: 'max' },
          ].map(({ v, set, ph }) => (
            <input
              key={ph}
              type="text"
              inputMode="numeric"
              className="w-10 rounded border px-1 py-0.5 text-center text-xs outline-none placeholder:text-(--color-muted)"
              style={controlStyle}
              placeholder={ph}
              value={v}
              onChange={(e) => set(e.target.value.replace(/\D/g, ''))}
            />
          ))}
        </div>
        <SortControl options={NPC_SORT_OPTIONS} value={sort} onChange={setSort} />
        <label
          className="flex cursor-pointer items-center gap-1 text-xs"
          style={{ color: 'var(--color-muted-foreground)' }}
          title="Hide entries with no HP, level, or class (script helpers, placeholders)"
        >
          <input
            type="checkbox"
            checked={hideTrivial}
            onChange={(e) => setHideTrivial(e.target.checked)}
          />
          Hide trivial
        </label>
        <label
          className="flex cursor-pointer items-center gap-1 text-xs"
          style={{ color: 'var(--color-muted-foreground)' }}
          title={
            'Show NPCs listed in this zone that cannot spawn right now — rows' +
            ' locked to another era (mostly old-model variants), or with a' +
            ' spawn weight of 0'
          }
        >
          <input
            type="checkbox"
            checked={showInactive}
            onChange={(e) => setShowInactive(e.target.checked)}
          />
          Show inactive
        </label>
      </Toolbar>
      {(filtered || total > npcs.length) && (
        <p className="mb-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Showing {visible.length.toLocaleString()} of {total.toLocaleString()}
        </p>
      )}
      {visible.length === 0 ? (
        <EmptyState message="No NPCs match these filters." />
      ) : (
        <div
          className="rounded border"
          style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        >
          {visible.map((npc, i) => {
            const spawn = spawns.get(npc.id)
            return (
              <button
                key={npc.id}
                onClick={() => navigate(`/npcs?select=${npc.id}`)}
                className="flex w-full cursor-pointer items-baseline justify-between px-3 py-1.5 text-left transition-colors"
                style={{
                  borderTop: i > 0 ? '1px solid var(--color-border)' : undefined,
                  borderLeft: '2px solid transparent',
                  opacity: spawn?.inactive ? 0.5 : undefined,
                }}
                onMouseEnter={(e) => {
                  ;(e.currentTarget as HTMLElement).style.backgroundColor = 'var(--color-surface-2)'
                  ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'var(--color-primary)'
                }}
                onMouseLeave={(e) => {
                  ;(e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'
                  ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'transparent'
                }}
              >
                <div className="flex flex-col">
                  <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                    {npcDisplayName(npc)}
                  </span>
                  <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                    {className(npc.class)}
                    {spawn && (
                      <span
                        title={
                          spawn.inactive === 'other_era'
                            ? 'Only spawns in another era — every spawn entry' +
                              ' for this row is expansion-locked out of the' +
                              ' current one'
                            : spawn.inactive === 'zero_weight'
                              ? 'In a spawn group with weight 0, so the' +
                                ' server never picks it'
                              : 'Chance a spawn point rolls this NPC (its' +
                                ' share of the spawn group), and how many' +
                                ' spawn points can roll it. Not a loot chance.'
                        }
                      >
                        {' · '}
                        {spawnLabel(spawn)}
                      </span>
                    )}
                  </span>
                </div>
                <div className="ml-4 shrink-0 text-right">
                  <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                    Lv {npcLevelLabel(npc)}
                  </span>
                  {npc.hp > 0 && (
                    <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                      {npc.hp.toLocaleString()} HP
                    </div>
                  )}
                </div>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

// ── Tab: Connected Zones ───────────────────────────────────────────────────────

function ConnectionsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [connections, setConnections] = useState<ZoneConnection[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneConnections(shortName)
      .then(setConnections)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (connections.length === 0) return <EmptyState message="No connected zones found." />

  return (
    <div
      className="rounded border"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      {connections.map((c, i) => (
        <button
          key={c.zone_id}
          onClick={() => navigate(`/zones?select=${c.zone_id}`)}
          className="flex w-full cursor-pointer items-center justify-between px-3 py-2 text-left transition-colors"
          style={{
            borderTop: i > 0 ? '1px solid var(--color-border)' : undefined,
            borderLeft: '2px solid transparent',
          }}
          onMouseEnter={(e) => {
            ;(e.currentTarget as HTMLElement).style.backgroundColor = 'var(--color-surface-2)'
            ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'var(--color-primary)'
          }}
          onMouseLeave={(e) => {
            ;(e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'
            ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'transparent'
          }}
        >
          <div className="flex flex-col">
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              {c.long_name || c.short_name}
            </span>
            <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
              {c.short_name}
            </span>
          </div>
          <span
            className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold"
            style={{
              backgroundColor: 'rgba(100,116,139,0.15)',
              color: 'var(--color-muted-foreground)',
            }}
          >
            {expansionName(c.expansion)}
          </span>
        </button>
      ))}
    </div>
  )
}

// ── Tab: Drops ─────────────────────────────────────────────────────────────────

const ZONE_DROPS_PAGE_SIZE = 100

const DROP_SORT_OPTIONS: SortOption<ZoneDropsSort>[] = [
  { key: 'item', label: 'Item', defaultDesc: false },
  { key: 'npc', label: 'NPC', defaultDesc: false },
  { key: 'chance', label: 'Drop %', defaultDesc: true },
]

const DROP_MIN_CHANCE_OPTIONS = [0, 1, 5, 10, 25, 50, 100]

function DropsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [drops, setDrops] = useState<ZoneDropItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [sort, setSort] = useCachedState<SortState<ZoneDropsSort>>('zones.drops.sort', {
    key: 'item',
    desc: false,
  })
  const [filter, setFilter] = useCachedState('zones.drops.filter', '')
  const [minChance, setMinChance] = useCachedState('zones.drops.minChance', 0)
  // Big zones run to ~19k rows, so sorting and filtering happen server-side;
  // the text filter is debounced to avoid a request per keystroke.
  const [debouncedFilter, setDebouncedFilter] = useState(filter)
  // Monotonic token so a slow earlier request can't clobber a newer one.
  const seqRef = useRef(0)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedFilter(filter), 300)
    return () => clearTimeout(t)
  }, [filter])

  const opts = useMemo<ZoneDropsOptions>(
    () => ({ sort: sort.key, desc: sort.desc, q: debouncedFilter, minChance }),
    [sort, debouncedFilter, minChance],
  )

  useEffect(() => {
    const seq = ++seqRef.current
    setLoading(true)
    setError(null)
    getZoneDrops(shortName, opts, ZONE_DROPS_PAGE_SIZE, 0)
      .then((res) => {
        if (seq !== seqRef.current) return
        setDrops(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => { if (seq === seqRef.current) setError(err.message) })
      .finally(() => { if (seq === seqRef.current) setLoading(false) })
  }, [shortName, opts])

  function loadMore() {
    const seq = seqRef.current
    setLoadingMore(true)
    getZoneDrops(shortName, opts, ZONE_DROPS_PAGE_SIZE, drops.length)
      .then((res) => {
        if (seq !== seqRef.current) return
        setDrops((prev) => [...prev, ...(res.items ?? [])])
        setTotal(res.total)
      })
      .catch((err: Error) => { if (seq === seqRef.current) setError(err.message) })
      .finally(() => { if (seq === seqRef.current) setLoadingMore(false) })
  }

  const filtering = debouncedFilter.trim() !== '' || minChance > 0
  // Without any filter an empty result means the zone has no loot at all, so
  // the controls would be useless — show the plain empty state instead.
  if (!loading && !error && drops.length === 0 && !filtering) {
    return <EmptyState message="No drops found." />
  }

  const hasMore = !loading && drops.length < total

  return (
    <div>
      <Toolbar>
        <FilterInput value={filter} onChange={setFilter} placeholder="Filter by item or NPC…" />
        <div className="flex items-center gap-1">
          <label
            className="text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Min
          </label>
          <select
            className="rounded border px-1.5 py-0.5 text-xs outline-none"
            style={controlStyle}
            value={String(minChance)}
            onChange={(e) => setMinChance(Number(e.target.value))}
          >
            {DROP_MIN_CHANCE_OPTIONS.map((c) => (
              <option key={c} value={c}>
                {c === 0 ? 'Any %' : `${c}%`}
              </option>
            ))}
          </select>
        </div>
        <SortControl options={DROP_SORT_OPTIONS} value={sort} onChange={setSort} />
      </Toolbar>
      {!loading && !error && (
        <p className="mb-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {total.toLocaleString()} drop{total === 1 ? '' : 's'}
          {filtering ? ' match' : ''}
        </p>
      )}
      {loading ? (
        <LoadingState />
      ) : error ? (
        <ErrorState message={error} />
      ) : drops.length === 0 ? (
        <EmptyState message="No drops match these filters." />
      ) : (
        <div
          className="rounded border"
          style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
        >
          {drops.map((d, i) => (
            <button
              key={`${d.item_id}-${d.npc_id}`}
              onClick={() => navigate(`/items?select=${d.item_id}`)}
              className="flex w-full cursor-pointer items-baseline justify-between px-3 py-1.5 text-left transition-colors"
              style={{
                borderTop: i > 0 ? '1px solid var(--color-border)' : undefined,
                borderLeft: '2px solid transparent',
              }}
              onMouseEnter={(e) => {
                ;(e.currentTarget as HTMLElement).style.backgroundColor = 'var(--color-surface-2)'
                ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'var(--color-primary)'
              }}
              onMouseLeave={(e) => {
                ;(e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'
                ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'transparent'
              }}
            >
              <div className="flex flex-col">
                <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  {d.item_name}
                </span>
                <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {npcNameDisplay(d.npc_name)}
                </span>
              </div>
              {d.chance > 0 && (
                <span className="ml-4 shrink-0 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                  {d.chance.toFixed(2)}%
                </span>
              )}
            </button>
          ))}
          {hasMore && (
            <div className="px-3 py-2">
              <button
                onClick={loadMore}
                disabled={loadingMore}
                className="w-full rounded border py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  borderColor: 'var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                }}
              >
                {loadingMore ? 'Loading…' : `Show more (${(total - drops.length).toLocaleString()} remaining)`}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Tab: Ground Spawns ─────────────────────────────────────────────────────────

type GroundSortKey = 'name' | 'respawn'

const GROUND_SORT_OPTIONS: SortOption<GroundSortKey>[] = [
  { key: 'name', label: 'Name', defaultDesc: false },
  { key: 'respawn', label: 'Respawn', defaultDesc: false },
]

function GroundSpawnsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [spawns, setSpawns] = useState<ZoneGroundSpawn[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [sort, setSort] = useCachedState<SortState<GroundSortKey>>(
    'zones.ground.sort',
    { key: 'name', desc: false },
  )

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneGroundSpawns(shortName)
      .then(setSpawns)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  const sorted = useMemo(
    () =>
      [...spawns].sort(
        compareBy(
          (g) =>
            sort.key === 'respawn'
              ? g.respawn_timer
              : (g.item_name || g.name).toLowerCase(),
          (g) => g.item_name || g.name,
          sort.desc,
        ),
      ),
    [spawns, sort],
  )

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (spawns.length === 0) return <EmptyState message="No ground spawns found." />

  return (
    <div>
      <Toolbar>
        <SortControl options={GROUND_SORT_OPTIONS} value={sort} onChange={setSort} />
      </Toolbar>
      <div
        className="rounded border"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
      >
        {sorted.map((g, i) => (
          <button
            key={g.id}
            onClick={() => g.item_id > 0 && navigate(`/items?select=${g.item_id}`)}
            className="flex w-full cursor-pointer items-baseline justify-between px-3 py-1.5 text-left transition-colors"
            style={{
              borderTop: i > 0 ? '1px solid var(--color-border)' : undefined,
              borderLeft: '2px solid transparent',
            }}
            onMouseEnter={(e) => {
              ;(e.currentTarget as HTMLElement).style.backgroundColor = 'var(--color-surface-2)'
              ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'var(--color-primary)'
            }}
            onMouseLeave={(e) => {
              ;(e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'
              ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'transparent'
            }}
          >
            <div className="flex flex-col">
              <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                {g.item_name || g.name}
              </span>
              {g.item_name && g.name && g.name !== g.item_name && (
                <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {g.name}
                </span>
              )}
            </div>
            <div className="ml-4 shrink-0 text-right">
              <div className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                {formatRespawn(g.respawn_timer)}
              </div>
              {g.max_allowed > 1 && (
                <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  ×{g.max_allowed}
                </div>
              )}
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}

// ── Tab: Forage ────────────────────────────────────────────────────────────────

type ForageSortKey = 'name' | 'chance'

const FORAGE_SORT_OPTIONS: SortOption<ForageSortKey>[] = [
  { key: 'name', label: 'Name', defaultDesc: false },
  { key: 'chance', label: 'Chance', defaultDesc: true },
]

function ForageTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [items, setItems] = useState<ZoneForageItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [sort, setSort] = useCachedState<SortState<ForageSortKey>>(
    'zones.forage.sort',
    { key: 'chance', desc: true },
  )

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneForage(shortName)
      .then(setItems)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  const sorted = useMemo(
    () =>
      [...items].sort(
        compareBy(
          (f) => (sort.key === 'chance' ? f.chance : f.item_name.toLowerCase()),
          (f) => f.item_name,
          sort.desc,
        ),
      ),
    [items, sort],
  )

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (items.length === 0) return <EmptyState message="No forageable items found." />

  return (
    <div>
      <Toolbar>
        <SortControl options={FORAGE_SORT_OPTIONS} value={sort} onChange={setSort} />
      </Toolbar>
      <div
        className="rounded border"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
      >
        {sorted.map((f, i) => (
          <button
            key={f.id}
            onClick={() => navigate(`/items?select=${f.item_id}`)}
            className="flex w-full cursor-pointer items-baseline justify-between px-3 py-1.5 text-left transition-colors"
            style={{
              borderTop: i > 0 ? '1px solid var(--color-border)' : undefined,
              borderLeft: '2px solid transparent',
            }}
            onMouseEnter={(e) => {
              ;(e.currentTarget as HTMLElement).style.backgroundColor = 'var(--color-surface-2)'
              ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'var(--color-primary)'
            }}
            onMouseLeave={(e) => {
              ;(e.currentTarget as HTMLElement).style.backgroundColor = 'transparent'
              ;(e.currentTarget as HTMLElement).style.borderLeftColor = 'transparent'
            }}
          >
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              {f.item_name}
            </span>
            <span className="ml-4 shrink-0 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              {f.chance}%
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}

// ── Tab: Lockouts ──────────────────────────────────────────────────────────────

/** Formats a positive remaining duration (ms) as "5d 13h 25m", dropping
 *  leading zero units. Matches LockoutTrackerPage's format, minus seconds —
 *  this view ticks less urgently since it's a zone-wide overview, not a
 *  single character's live countdown. */
function formatRemainingShort(ms: number): string {
  let s = Math.floor(ms / 1000)
  const d = Math.floor(s / 86400)
  s -= d * 86400
  const h = Math.floor(s / 3600)
  s -= h * 3600
  const m = Math.floor(s / 60)
  const parts: string[] = []
  if (d > 0) parts.push(`${d}d`)
  if (d > 0 || h > 0) parts.push(`${h}h`)
  parts.push(`${m}m`)
  return parts.join(' ')
}

function LockoutsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const { active } = useActiveCharacter()
  const [bosses, setBosses] = useState<ZoneLockoutBoss[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [nowMs, setNowMs] = useState(() => Date.now())

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneLockouts(shortName)
      .then((res) => setBosses(res.bosses ?? []))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  // Ticks the countdowns; a minute is plenty granular for a zone overview.
  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), 60_000)
    return () => clearInterval(id)
  }, [])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (bosses.length === 0) {
    return <EmptyState message="No raid targets tracked in this zone." />
  }

  return (
    <div
      className="rounded border"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      {bosses.map((boss, i) => {
        // The active character's chip leads, then everyone else who has data,
        // alphabetically. Characters with no captured entry for this boss are
        // left out entirely rather than shown as an unknown state.
        const chars = [...(boss.characters ?? [])].sort((a, b) => {
          if (a.character === active) return -1
          if (b.character === active) return 1
          return a.character.localeCompare(b.character)
        })
        return (
          <div
            key={boss.target_name}
            className="flex flex-wrap items-baseline gap-x-4 gap-y-1 px-3 py-2"
            style={{ borderTop: i > 0 ? '1px solid var(--color-border)' : undefined }}
          >
            {boss.npc_id ? (
              <button
                onClick={() => navigate(`/npcs?select=${boss.npc_id}`)}
                className="shrink-0 text-left text-sm font-medium underline decoration-dotted"
                style={{ color: 'var(--color-foreground)' }}
                title="Open in the NPC database"
              >
                {boss.target_name}
              </button>
            ) : (
              <span
                className="shrink-0 text-sm font-medium"
                style={{ color: 'var(--color-foreground)' }}
              >
                {boss.target_name}
              </span>
            )}
            {chars.length === 0 ? (
              <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                No lockout data
              </span>
            ) : (
              <div className="flex flex-wrap items-center gap-1.5">
                {chars.map((c) => {
                  const remainingMs = c.expires_at > 0 ? c.expires_at * 1000 - nowMs : 0
                  const available = c.expires_at === 0 || remainingMs <= 0
                  return (
                    <span
                      key={c.character}
                      className="rounded px-1.5 py-0.5 text-[11px] tabular-nums"
                      style={{
                        backgroundColor: available
                          ? 'color-mix(in srgb, var(--color-success) 15%, transparent)'
                          : 'var(--color-surface-2)',
                        color: available ? 'var(--color-success)' : 'var(--color-foreground)',
                        fontWeight: c.character === active ? 700 : 400,
                      }}
                    >
                      {c.character}: {available ? 'Available' : formatRemainingShort(remainingMs)}
                    </span>
                  )
                })}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

type TabKey =
  | 'overview'
  | 'map'
  | 'npcs'
  | 'connections'
  | 'drops'
  | 'ground-spawns'
  | 'forage'
  | 'lockouts'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'map', label: 'Map' },
  { key: 'npcs', label: 'NPCs' },
  { key: 'connections', label: 'Connected Zones' },
  { key: 'drops', label: 'Drops' },
  { key: 'ground-spawns', label: 'Ground Spawns' },
  { key: 'forage', label: 'Forage' },
  { key: 'lockouts', label: 'Lockouts' },
]

interface DetailPanelProps {
  zone: Zone | null
}

function DetailPanel({ zone }: DetailPanelProps): React.ReactElement {
  const [activeTab, setActiveTab] = useState<TabKey>('overview')
  // Hide the Map tab for zones with no geometry (instanced copies, and any
  // build shipped without maps.db) rather than offering a dead tab.
  const hasMap = useHasMap(zone?.short_name)
  const [rawOpen, setRawOpen] = useState(false)
  const rawFetcher = useCallback(() => getZoneRaw(zone!.id), [zone?.id])

  useEffect(() => {
    setActiveTab('overview')
  }, [zone?.id])

  if (!zone) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select a zone to view details
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Header */}
      <div
        className="shrink-0 border-b px-5 pt-4 pb-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-start justify-between gap-3">
          <h2
            className="text-xl font-bold leading-tight"
            style={{ color: 'var(--color-primary)' }}
          >
            {zone.long_name || zone.short_name}
          </h2>
          <button
            onClick={() => setRawOpen(true)}
            className="shrink-0 rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-1)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-muted)',
            }}
            title="View raw database row"
          >
            Raw Data
          </button>
        </div>
        <div
          className="mt-1 mb-3 flex flex-wrap items-center gap-2 text-sm"
          style={{ color: 'var(--color-muted-foreground)' }}
        >
          <span>
            {zone.short_name}
            {zone.file_name && zone.file_name !== zone.short_name && (
              <span className="ml-2" style={{ color: 'var(--color-muted)' }}>
                ({zone.file_name})
              </span>
            )}
          </span>
          <span
            className="rounded px-1.5 py-0.5 text-[11px] font-semibold"
            style={{
              backgroundColor:
                zone.exp_mod > 1.0
                  ? 'rgba(34,197,94,0.15)'
                  : zone.exp_mod < 1.0
                    ? 'rgba(234,179,8,0.15)'
                    : 'rgba(100,116,139,0.15)',
              color:
                zone.exp_mod > 1.0
                  ? 'rgb(134,239,172)'
                  : zone.exp_mod < 1.0
                    ? 'rgb(253,224,71)'
                    : 'var(--color-muted)',
            }}
          >
            ZEM {expModLabel(zone.exp_mod)}
          </span>
        </div>

        {/* Tabs */}
        <div className="flex gap-0 overflow-x-auto">
          {TABS.filter((t) => t.key !== 'map' || hasMap).map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className="shrink-0 px-3 py-1.5 text-xs font-medium transition-colors"
              style={{
                color: activeTab === tab.key ? 'var(--color-primary)' : 'var(--color-muted)',
                borderBottom:
                  activeTab === tab.key
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab content. The map tab sizes itself to the pane instead of
          scrolling — a fixed-height canvas left dead space below it. */}
      <div
        className={`flex-1 px-5 py-4 ${
          activeTab === 'map' ? 'min-h-0 overflow-hidden' : 'overflow-y-auto'
        }`}
      >
        {activeTab === 'overview' && <OverviewTab zone={zone} />}
        {activeTab === 'map' && <ZoneMapPanel zoneShortName={zone.short_name} />}
        {activeTab === 'npcs' && <NPCsTab shortName={zone.short_name} />}
        {activeTab === 'connections' && <ConnectionsTab shortName={zone.short_name} />}
        {activeTab === 'drops' && <DropsTab shortName={zone.short_name} />}
        {activeTab === 'ground-spawns' && <GroundSpawnsTab shortName={zone.short_name} />}
        {activeTab === 'forage' && <ForageTab shortName={zone.short_name} />}
        {activeTab === 'lockouts' && <LockoutsTab shortName={zone.short_name} />}
      </div>

      <RawDataModal
        open={rawOpen}
        title={zone.long_name || zone.short_name}
        fetcher={rawFetcher}
        onClose={() => setRawOpen(false)}
      />
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function ZonesPage(): React.ReactElement {
  const [selected, setSelected] = useState<Zone | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  // Selection is driven by the URL (?select=id) so each pick is its own history
  // entry — Back steps from one selected zone to the previously selected one
  // rather than jumping straight to the prior page (issue #9).
  useEffect(() => {
    const raw = searchParams.get('select')
    if (!raw) {
      setSelected(null)
      return
    }
    // Already showing it? Internal picks push the numeric id; source links from
    // items/spells pass the zone short_name — accept either.
    if (selected && (String(selected.id) === raw || selected.short_name === raw)) return
    const numId = Number(raw)
    const fetchZone = Number.isFinite(numId) && numId > 0 ? getZone(numId) : getZoneByShortName(raw)
    let cancelled = false
    fetchZone.then((z) => { if (!cancelled) setSelected(z) }).catch(() => {/* ignore */})
    return () => { cancelled = true }
  }, [searchParams, selected])

  // Push a new ?select entry on each pick so it lands in history; clearing
  // selection drops the param.
  const handleSelect = useCallback(
    (zone: Zone | null) => {
      setSelected(zone)
      setSearchParams(zone ? { select: String(zone.id) } : {})
    },
    [setSearchParams],
  )

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={handleSelect} />
      {/* Key by id so the panel (and its per-tab fetches) remount on each pick
          instead of lingering on the previously viewed zone's data. */}
      <DetailPanel key={selected?.id ?? 'none'} zone={selected} />
    </div>
  )
}
