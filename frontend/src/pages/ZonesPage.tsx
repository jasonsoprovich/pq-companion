import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Search, X } from 'lucide-react'
import { getNPCsByZone, getZone, searchZones } from '../services/api'
import type { NPC } from '../types/npc'
import type { Zone } from '../types/zone'
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
  return `${Math.round(mod * 100)}%`
}

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (zone: Zone) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [zones, setZones] = useState<Zone[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const runSearch = useCallback((q: string) => {
    setLoading(true)
    setError(null)
    searchZones(q, 50, 0)
      .then((res) => {
        setZones(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, runSearch])

  useEffect(() => {
    runSearch('')
  }, [runSearch])

  return (
    <div
      className="flex w-72 shrink-0 flex-col border-r"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {/* Search input */}
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

      {/* Result count */}
      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} zones`}
      </div>

      {/* Results list */}
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
                  {zone.min_level > 0 && ` · Lv ${zone.min_level}+`}
                </span>
                {zone.exp_mod !== 1.0 && (
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

// ── Detail panel helpers ───────────────────────────────────────────────────────

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

// ── NPC list within detail panel ───────────────────────────────────────────────

interface NPCListProps {
  shortName: string
}

function NPCList({ shortName }: NPCListProps): React.ReactElement {
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

  if (loading) {
    return (
      <p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        Loading…
      </p>
    )
  }

  if (error) {
    return (
      <p className="py-2 text-xs" style={{ color: 'var(--color-destructive)' }}>
        {error}
      </p>
    )
  }

  if (npcs.length === 0) {
    return (
      <p className="py-2 text-xs" style={{ color: 'var(--color-muted)' }}>
        No spawn data found.
      </p>
    )
  }

  return (
    <div>
      {total > npcs.length && (
        <p className="mb-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Showing {npcs.length} of {total.toLocaleString()}
        </p>
      )}
      <div
        className="rounded border"
        style={{
          backgroundColor: 'var(--color-surface)',
          borderColor: 'var(--color-border)',
        }}
      >
        {npcs.map((npc, i) => (
          <div
            key={npc.id}
            className="flex items-baseline justify-between px-3 py-1.5"
            style={{
              borderTop: i > 0 ? '1px solid var(--color-border)' : undefined,
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
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

interface DetailPanelProps {
  zone: Zone | null
}

function DetailPanel({ zone }: DetailPanelProps): React.ReactElement {
  if (!zone) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select a zone to view details
        </p>
      </div>
    )
  }

  const coordStr = `${zone.safe_x.toFixed(1)}, ${zone.safe_y.toFixed(1)}, ${zone.safe_z.toFixed(1)}`

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4">
      {/* Header */}
      <div className="mb-4">
        <h2
          className="text-xl font-bold leading-tight"
          style={{ color: 'var(--color-primary)' }}
        >
          {zone.long_name || zone.short_name}
        </h2>
        <div className="mt-1 flex flex-wrap items-center gap-2 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
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
            ZEM {Math.round(zone.exp_mod * 100)}%
          </span>
        </div>
      </div>

      <div className="flex flex-col gap-3">
        {/* Quick Facts */}
        <Section title="Quick Facts">
          <StatRow label="Expansion" value={expansionName(zone.expansion)} />
          <StatRow label="XP Modifier" value={expModLabel(zone.exp_mod)} />
          <StatRow label="Outdoor" value={zone.outdoor ? 'Yes' : 'No'} />
          <StatRow label="Hotzone" value={zone.hotzone ? 'Yes' : 'No'} />
          <StatRow label="Levitation" value={zone.can_levitate ? 'Allowed' : 'Restricted'} />
          <StatRow label="Binding" value={bindLabel(zone.can_bind)} />
        </Section>

        {/* Zone Info */}
        <Section title="Zone Info">
          <StatRow label="Zone ID" value={zone.zone_id_number} />
          <StatRow label="Min Level" value={zone.min_level > 0 ? zone.min_level : 'None'} />
          <StatRow label="Safe Point" value={coordStr} />
          {zone.note && <StatRow label="Note" value={zone.note} />}
        </Section>

        {/* Residents */}
        <div>
          <div
            className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Residents
          </div>
          <NPCList shortName={zone.short_name} />
        </div>
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
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel zone={selected} />
    </div>
  )
}
