import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Search, X } from 'lucide-react'
import {
  getNPCsByZone,
  getZone,
  getZoneConnections,
  getZoneDrops,
  getZoneExpansions,
  getZoneForage,
  getZoneGroundSpawns,
  searchZones,
} from '../services/api'
import type { NPC } from '../types/npc'
import type { Zone, ZoneConnection, ZoneDropItem, ZoneForageItem, ZoneGroundSpawn } from '../types/zone'
import { className, npcDisplayName } from '../lib/npcHelpers'

const EQ_EXPANSIONS: Record<number, string> = {
  0: 'Classic',
  1: 'Ruins of Kunark',
  2: 'Scars of Velious',
  3: 'Shadows of Luclin',
  4: 'Planes of Power',
  5: 'Legacy of Ykesha',
  6: 'Lost Dungeons of Norrath',
  7: 'Gates of Discord',
  8: 'Omens of War',
  9: 'Dragons of Norrath',
  10: 'Depths of Darkhollow',
  11: 'Prophecy of Ro',
  12: "The Serpent's Spine",
  13: 'The Buried Sea',
  14: 'Secrets of Faydwer',
}

function expansionName(id: number): string {
  return EQ_EXPANSIONS[id] ?? `Expansion ${id}`
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
  const [query, setQuery] = useState('')
  const [expansion, setExpansion] = useState<number | null>(null)
  const [expansionOptions, setExpansionOptions] = useState<number[]>([])
  const [zones, setZones] = useState<Zone[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string, exp: number | null) => {
    setLoading(true)
    setError(null)
    searchZones(q, exp !== null ? { expansion: exp } : {}, 1000, 0)
      .then((res) => {
        setZones(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, expansion), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, expansion, runSearch])

  // Fire the initial empty-query search immediately so the list populates
  // on mount without waiting for the 300ms debounce. Matches the pattern
  // used by ItemsPage / SpellsPage / NpcsPage.
  useEffect(() => {
    runSearch('', null)
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
          className="flex-1 rounded border bg-transparent px-1.5 py-0.5 text-xs outline-none"
          style={{
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

// ── Tab: Overview ──────────────────────────────────────────────────────────────

function OverviewTab({ zone }: { zone: Zone }): React.ReactElement {
  const coordStr = `Y: ${zone.safe_y.toFixed(1)}, X: ${zone.safe_x.toFixed(1)}, Z: ${zone.safe_z.toFixed(1)}`
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
        {zone.note && <StatRow label="Note" value={zone.note} />}
      </Section>
    </div>
  )
}

// ── Tab: NPCs ──────────────────────────────────────────────────────────────────

function NPCsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [npcs, setNpcs] = useState<NPC[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getNPCsByZone(shortName, 200, 0)
      .then((res) => {
        setNpcs(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (npcs.length === 0) return <EmptyState message="No spawn data found." />

  return (
    <div>
      {total > npcs.length && (
        <p className="mb-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Showing {npcs.length} of {total.toLocaleString()}
        </p>
      )}
      <div
        className="rounded border"
        style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
      >
        {npcs.map((npc, i) => (
          <button
            key={npc.id}
            onClick={() => navigate(`/npcs?select=${npc.id}`)}
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
                {npcDisplayName(npc)}
              </span>
              <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {className(npc.class)}
              </span>
            </div>
            <div className="ml-4 shrink-0 text-right">
              <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                Lv {npc.level}
              </span>
              {npc.hp > 0 && (
                <div className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  {npc.hp.toLocaleString()} HP
                </div>
              )}
            </div>
          </button>
        ))}
      </div>
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

function DropsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [drops, setDrops] = useState<ZoneDropItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneDrops(shortName)
      .then(setDrops)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (drops.length === 0) return <EmptyState message="No drops found." />

  return (
    <div>
      {drops.length >= 500 && (
        <p className="mb-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Showing first 500 results
        </p>
      )}
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
      </div>
    </div>
  )
}

// ── Tab: Ground Spawns ─────────────────────────────────────────────────────────

function GroundSpawnsTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [spawns, setSpawns] = useState<ZoneGroundSpawn[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneGroundSpawns(shortName)
      .then(setSpawns)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (spawns.length === 0) return <EmptyState message="No ground spawns found." />

  return (
    <div
      className="rounded border"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      {spawns.map((g, i) => (
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
  )
}

// ── Tab: Forage ────────────────────────────────────────────────────────────────

function ForageTab({ shortName }: { shortName: string }): React.ReactElement {
  const navigate = useNavigate()
  const [items, setItems] = useState<ZoneForageItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    setError(null)
    getZoneForage(shortName)
      .then(setItems)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [shortName])

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} />
  if (items.length === 0) return <EmptyState message="No forageable items found." />

  return (
    <div
      className="rounded border"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      {items.map((f, i) => (
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
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

type TabKey = 'overview' | 'npcs' | 'connections' | 'drops' | 'ground-spawns' | 'forage'

const TABS: { key: TabKey; label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'npcs', label: 'NPCs' },
  { key: 'connections', label: 'Connected Zones' },
  { key: 'drops', label: 'Drops' },
  { key: 'ground-spawns', label: 'Ground Spawns' },
  { key: 'forage', label: 'Forage' },
]

interface DetailPanelProps {
  zone: Zone | null
}

function DetailPanel({ zone }: DetailPanelProps): React.ReactElement {
  const [activeTab, setActiveTab] = useState<TabKey>('overview')

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
        <h2
          className="text-xl font-bold leading-tight"
          style={{ color: 'var(--color-primary)' }}
        >
          {zone.long_name || zone.short_name}
        </h2>
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
          {TABS.map((tab) => (
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

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto px-5 py-4">
        {activeTab === 'overview' && <OverviewTab zone={zone} />}
        {activeTab === 'npcs' && <NPCsTab shortName={zone.short_name} />}
        {activeTab === 'connections' && <ConnectionsTab shortName={zone.short_name} />}
        {activeTab === 'drops' && <DropsTab shortName={zone.short_name} />}
        {activeTab === 'ground-spawns' && <GroundSpawnsTab shortName={zone.short_name} />}
        {activeTab === 'forage' && <ForageTab shortName={zone.short_name} />}
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function ZonesPage(): React.ReactElement {
  const [selected, setSelected] = useState<Zone | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) return
    getZone(id)
      .then(setSelected)
      .catch(() => {/* ignore */})
      .finally(() => setSearchParams({}, { replace: true }))
  }, [searchParams, setSearchParams])

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel zone={selected} />
    </div>
  )
}
