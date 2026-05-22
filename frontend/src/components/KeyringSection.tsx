/**
 * KeyringSection — collapsible per-character keyring tracker that lives at
 * the top of the Key Tracker page.
 *
 * Distinct from the multi-component KeyCard tracker below it: this view is
 * driven by the EQ /keys command. The backend parses /keys log output into
 * keyring_entries rows; this component renders the union of the
 * keyring_data master list and the active character's owned set.
 */
import React, { useEffect, useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, Search, RefreshCw, KeyRound } from 'lucide-react'
import { getKeyringMaster, getKeyringForCharacter } from '../services/api'
import type { KeyringMasterEntry } from '../types/keyring'
import CharacterSubTabs from './CharacterSubTabs'

type OwnedFilter = 'all' | 'owned' | 'missing'

interface KeyringSectionProps {
  /** Active character set from the parent page so the section starts on the same view. */
  initialCharacter?: string
}

// Stage labels: "1", "2", … rendered beside the key name when the master row
// belongs to a multi-stage progression (e.g. Tower of Frozen Shadow's seven
// keys). Single-key zones leave stage as 0.
function stageBadge(stage: number): string | null {
  return stage > 0 ? `${stage}` : null
}

export default function KeyringSection({ initialCharacter }: KeyringSectionProps): React.ReactElement {
  const [expanded, setExpanded] = useState(true)
  const [master, setMaster] = useState<KeyringMasterEntry[]>([])
  const [character, setCharacter] = useState(initialCharacter ?? '')
  const [owned, setOwned] = useState<Set<number>>(new Set())
  const [filter, setFilter] = useState<OwnedFilter>('all')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Master list never changes at runtime (sourced from quarm.db). Load once.
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getKeyringMaster()
      .then((res) => {
        if (cancelled) return
        setMaster(res.keys ?? [])
        setError(null)
      })
      .catch((err) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Failed to load keyring master list')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [])

  // Owned set refreshes whenever the character changes.
  useEffect(() => {
    if (!character) {
      setOwned(new Set())
      return
    }
    let cancelled = false
    getKeyringForCharacter(character)
      .then((res) => {
        if (cancelled) return
        setOwned(new Set((res.entries ?? []).map((e) => e.key_item)))
      })
      .catch(() => {
        if (cancelled) return
        setOwned(new Set())
      })
    return () => { cancelled = true }
  }, [character])

  // Group keys by zone, applying search + owned filter inside each group.
  // Empty zones drop out so the rendered list is tight when filtering.
  const grouped = useMemo(() => {
    const q = search.trim().toLowerCase()
    const groups = new Map<string, KeyringMasterEntry[]>()
    for (const k of master) {
      const isOwned = owned.has(k.key_item)
      if (filter === 'owned' && !isOwned) continue
      if (filter === 'missing' && isOwned) continue
      if (q) {
        const hay = `${k.key_name} ${k.item_name} ${k.zone_name}`.toLowerCase()
        if (!hay.includes(q)) continue
      }
      const zone = k.zone_name || 'Unknown zone'
      const bucket = groups.get(zone)
      if (bucket) bucket.push(k)
      else groups.set(zone, [k])
    }
    return Array.from(groups.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [master, owned, filter, search])

  const totalCount = master.length
  const ownedCount = useMemo(() => {
    let n = 0
    for (const k of master) if (owned.has(k.key_item)) n++
    return n
  }, [master, owned])

  return (
    <div
      className="border-b shrink-0"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      {/* Section header — always visible; click to collapse / expand. */}
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="w-full flex items-center gap-2 px-4 py-2 text-left transition-colors"
        style={{ color: 'var(--color-foreground)' }}
      >
        {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <KeyRound size={14} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold">Keyring</span>
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {character ? `${ownedCount} / ${totalCount} owned` : `${totalCount} total`}
        </span>
        <span
          className="ml-auto text-[10px]"
          style={{ color: 'var(--color-muted)' }}
          title="Type /keys in-game to refresh this list for the active character."
        >
          /keys to update
        </span>
      </button>

      {expanded && (
        <div className="flex flex-col">
          <CharacterSubTabs value={character} onChange={setCharacter} />

          {/* Controls row: search + owned filter */}
          <div
            className="flex items-center gap-2 px-4 py-2 border-b"
            style={{ borderColor: 'var(--color-border)' }}
          >
            <div className="relative flex-1 max-w-sm">
              <Search
                size={12}
                className="absolute left-2.5 top-1/2 -translate-y-1/2"
                style={{ color: 'var(--color-muted)' }}
              />
              <input
                type="text"
                placeholder="Search keys, zones…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="w-full rounded pl-7 pr-3 py-1.5 text-xs outline-none"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
            </div>
            <div className="flex items-center rounded overflow-hidden" style={{ border: '1px solid var(--color-border)' }}>
              {(['all', 'owned', 'missing'] as OwnedFilter[]).map((f) => {
                const active = filter === f
                const label = f === 'all' ? 'All' : f === 'owned' ? 'Owned' : 'Not owned'
                return (
                  <button
                    key={f}
                    type="button"
                    onClick={() => setFilter(f)}
                    className="px-2.5 py-1 text-xs font-medium transition-colors"
                    style={{
                      backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                      color: active ? 'var(--color-primary-foreground)' : 'var(--color-muted-foreground)',
                    }}
                  >
                    {label}
                  </button>
                )
              })}
            </div>
          </div>

          {/* Body — zone-grouped key list */}
          <div className="max-h-[440px] overflow-y-auto px-4 py-3">
            {loading ? (
              <div className="flex items-center justify-center py-6">
                <RefreshCw size={16} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
              </div>
            ) : error ? (
              <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>
            ) : !character ? (
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Pick a character above to see their keyring.
              </p>
            ) : grouped.length === 0 ? (
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {filter === 'owned'
                  ? 'No keys recorded for this character. Type /keys in-game to record.'
                  : filter === 'missing'
                    ? 'All keys for the current filter are owned.'
                    : 'No keys match your search.'}
              </p>
            ) : (
              <div className="space-y-3">
                {grouped.map(([zone, keys]) => (
                  <KeyringZoneGroup key={zone} zone={zone} keys={keys} owned={owned} />
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// ── Sub-components ────────────────────────────────────────────────────────────

interface KeyringZoneGroupProps {
  zone: string
  keys: KeyringMasterEntry[]
  owned: Set<number>
}

function KeyringZoneGroup({ zone, keys, owned }: KeyringZoneGroupProps): React.ReactElement {
  // Stable sort: multi-stage zones (Tower of Frozen Shadow, Vex Thal door keys)
  // sort by stage; single-stage zones fall back to alpha.
  const sorted = useMemo(() => {
    return [...keys].sort((a, b) => {
      if (a.stage !== b.stage) return a.stage - b.stage
      return a.key_name.localeCompare(b.key_name)
    })
  }, [keys])

  return (
    <div>
      <p
        className="text-[11px] uppercase tracking-wider mb-1.5"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        {zone}
      </p>
      <div className="space-y-1">
        {sorted.map((k) => (
          <KeyringRow key={k.key_item} entry={k} owned={owned.has(k.key_item)} />
        ))}
      </div>
    </div>
  )
}

interface KeyringRowProps {
  entry: KeyringMasterEntry
  owned: boolean
}

function KeyringRow({ entry, owned }: KeyringRowProps): React.ReactElement {
  const stage = stageBadge(entry.stage)
  // Color: owned rows pop in the success/green color; missing rows fade so
  // the eye lands on owned ones first. text uses CSS opacity rather than a
  // separate color so the theme's muted palette stays consistent.
  return (
    <div
      className="flex items-center gap-2 rounded px-2 py-1 text-xs"
      style={{
        backgroundColor: owned ? 'color-mix(in srgb, var(--color-success) 12%, transparent)' : 'transparent',
        color: owned ? 'var(--color-success)' : 'var(--color-muted-foreground)',
        opacity: owned ? 1 : 0.55,
      }}
    >
      {stage !== null && (
        <span
          className="inline-flex items-center justify-center rounded text-[10px] font-semibold px-1.5"
          style={{
            minWidth: 18,
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
          }}
          title={`Stage ${entry.stage}`}
        >
          {stage}
        </span>
      )}
      <span className="font-medium">{entry.key_name}</span>
      {entry.item_name && entry.item_name !== entry.key_name && (
        <span
          className="text-[10px]"
          style={{ color: 'var(--color-muted)' }}
          title="Underlying item name"
        >
          ({entry.item_name})
        </span>
      )}
    </div>
  )
}
