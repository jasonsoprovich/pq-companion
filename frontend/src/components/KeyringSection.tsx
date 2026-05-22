/**
 * KeyringSection — per-character keyring snapshot panel, rendered as the
 * content of the "Keyring" tab on the Key Tracker page.
 *
 * Two data sources, three resulting states per key:
 *   - "keyring": the key has been used in-game and is permanently on the
 *     character's keyring (parsed from /keys log output → keyring_entries).
 *   - "inventory": the key item is in the character's inventory or shared
 *     bank, but hasn't been used yet (so it isn't on the keyring). This is
 *     a fallback for characters that hold key items but haven't ever /keys'd
 *     in a way the app could parse.
 *   - "missing": neither source reports the key.
 *
 * Keyring always wins over inventory — once a key is added to the keyring
 * the in-game item can be destroyed and the keyring entry persists, so a
 * char that's on the keyring but no longer holding the item must still
 * read as fully owned (green), not yellow.
 */
import React, { useEffect, useMemo, useState } from 'react'
import { Search, RefreshCw } from 'lucide-react'
import { getKeyringMaster, getKeyringForCharacter, getAllInventories } from '../services/api'
import type { KeyringMasterEntry } from '../types/keyring'
import type { AllInventoriesResponse } from '../types/zeal'
import CharacterSubTabs from './CharacterSubTabs'

type OwnedFilter = 'all' | 'owned' | 'missing'

// Ownership states for a single key relative to the selected character.
// "keyring" is permanent (in /keys output); "inventory" is the fallback
// when the key item is on hand but hasn't been used.
type Ownership = 'keyring' | 'inventory' | 'missing'

interface KeyringSectionProps {
  /** Active character set from the parent page so the panel opens on the same view. */
  initialCharacter?: string
}

function stageBadge(stage: number): string | null {
  return stage > 0 ? `${stage}` : null
}

export default function KeyringSection({ initialCharacter }: KeyringSectionProps): React.ReactElement {
  const [master, setMaster] = useState<KeyringMasterEntry[]>([])
  const [character, setCharacter] = useState(initialCharacter ?? '')
  const [owned, setOwned] = useState<Set<number>>(new Set())
  const [inv, setInv] = useState<AllInventoriesResponse | null>(null)
  const [filter, setFilter] = useState<OwnedFilter>('all')
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Master list never changes at runtime (sourced from quarm.db). Load once.
  // Pair it with the all-character inventory snapshot so the inventory
  // fallback ("key item in bag but not on /keys'd yet") can be computed
  // without an extra network round-trip per character switch.
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([getKeyringMaster(), getAllInventories().catch(() => null)])
      .then(([masterRes, invRes]) => {
        if (cancelled) return
        setMaster(masterRes.keys ?? [])
        setInv(invRes)
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

  // Selected character's inventory item IDs ∪ shared-bank IDs. Shared bank
  // counts for every character on the account — if Osui doesn't have the
  // key in their bags but the shared bank does, Osui can still grab it, so
  // we count that as "in inventory" for fallback purposes.
  const inventoryIDs = useMemo<Set<number>>(() => {
    const ids = new Set<number>()
    if (!inv || !character) return ids
    const charInv = inv.characters.find(
      (c) => c.character.toLowerCase() === character.toLowerCase(),
    )
    if (charInv) {
      for (const e of charInv.entries) ids.add(e.id)
    }
    for (const e of inv.shared_bank) ids.add(e.id)
    return ids
  }, [inv, character])

  function ownershipOf(keyItem: number): Ownership {
    if (owned.has(keyItem)) return 'keyring'
    if (inventoryIDs.has(keyItem)) return 'inventory'
    return 'missing'
  }

  // Group keys by zone, applying search + owned filter inside each group.
  // Filter semantics: "owned" = keyring OR inventory (anywhere we can prove
  // ownership); "missing" = neither. The color in each row still
  // distinguishes keyring (green) from inventory (yellow).
  const grouped = useMemo(() => {
    const q = search.trim().toLowerCase()
    const groups = new Map<string, KeyringMasterEntry[]>()
    for (const k of master) {
      const state = ownershipOf(k.key_item)
      if (filter === 'owned' && state === 'missing') continue
      if (filter === 'missing' && state !== 'missing') continue
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [master, owned, inventoryIDs, filter, search])

  const totalCount = master.length
  const counts = useMemo(() => {
    let onKeyring = 0
    let inInv = 0
    for (const k of master) {
      const s = ownershipOf(k.key_item)
      if (s === 'keyring') onKeyring++
      else if (s === 'inventory') inInv++
    }
    return { onKeyring, inInv, total: onKeyring + inInv }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [master, owned, inventoryIDs])

  return (
    <div className="flex flex-col flex-1 min-h-0">
      <CharacterSubTabs value={character} onChange={setCharacter} />

      {/* Controls row: search + owned filter + count */}
      <div
        className="flex items-center gap-2 px-4 py-2 border-b shrink-0"
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
        <div className="ml-auto flex items-center gap-3 text-xs">
          {character ? (
            <span style={{ color: 'var(--color-muted-foreground)' }}>
              <span style={{ color: 'var(--color-success)' }}>{counts.onKeyring}</span>
              {counts.inInv > 0 && (
                <>
                  {' + '}
                  <span style={{ color: 'var(--color-warning, #ffaa00)' }}>{counts.inInv}</span>
                </>
              )}
              {' / '}
              {totalCount}
            </span>
          ) : (
            <span style={{ color: 'var(--color-muted-foreground)' }}>{totalCount} total</span>
          )}
          <span
            className="text-[10px]"
            style={{ color: 'var(--color-muted)' }}
            title="Type /keys in-game to refresh this list for the active character. Keys in inventory or shared bank are also detected from Zeal exports."
          >
            /keys to update
          </span>
        </div>
      </div>

      {/* Color legend — only shows when there's a character selected and the
          inventory fallback can actually produce yellow rows. */}
      {character && inv?.configured && (
        <div
          className="flex items-center gap-4 px-4 py-1.5 border-b shrink-0 text-[10px]"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <LegendDot color="var(--color-success)" label="On keyring (used)" />
          <LegendDot color="var(--color-warning, #ffaa00)" label="Item carried (not yet used)" />
          <LegendDot color="var(--color-muted)" label="Missing" muted />
        </div>
      )}

      {/* Body — zone-grouped key list */}
      <div className="flex-1 overflow-y-auto px-4 py-3">
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
              <KeyringZoneGroup key={zone} zone={zone} keys={keys} ownership={ownershipOf} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ── Sub-components ────────────────────────────────────────────────────────────

interface LegendDotProps {
  color: string
  label: string
  muted?: boolean
}

function LegendDot({ color, label, muted }: LegendDotProps): React.ReactElement {
  return (
    <span className="inline-flex items-center gap-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
      <span
        className="inline-block rounded-full"
        style={{ width: 8, height: 8, backgroundColor: color, opacity: muted ? 0.55 : 1 }}
      />
      {label}
    </span>
  )
}

interface KeyringZoneGroupProps {
  zone: string
  keys: KeyringMasterEntry[]
  ownership: (keyItem: number) => Ownership
}

function KeyringZoneGroup({ zone, keys, ownership }: KeyringZoneGroupProps): React.ReactElement {
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
          <KeyringRow key={k.key_item} entry={k} state={ownership(k.key_item)} />
        ))}
      </div>
    </div>
  )
}

interface KeyringRowProps {
  entry: KeyringMasterEntry
  state: Ownership
}

// Colors per ownership state. Warning uses a fallback hex because some
// themes don't define --color-warning yet — keeps yellow consistent across
// themes.
const stateStyle: Record<Ownership, { color: string; bg: string; opacity: number; title: string }> = {
  keyring: {
    color: 'var(--color-success)',
    bg: 'color-mix(in srgb, var(--color-success) 12%, transparent)',
    opacity: 1,
    title: 'On keyring — used in-game and permanently recorded',
  },
  inventory: {
    color: 'var(--color-warning, #ffaa00)',
    bg: 'color-mix(in srgb, var(--color-warning, #ffaa00) 12%, transparent)',
    opacity: 1,
    title: 'Key item is in inventory or shared bank but has not been added to the keyring yet',
  },
  missing: {
    color: 'var(--color-muted-foreground)',
    bg: 'transparent',
    opacity: 0.55,
    title: 'Not on keyring and not in inventory',
  },
}

function KeyringRow({ entry, state }: KeyringRowProps): React.ReactElement {
  const stage = stageBadge(entry.stage)
  const s = stateStyle[state]
  return (
    <div
      className="flex items-center gap-2 rounded px-2 py-1 text-xs"
      style={{ backgroundColor: s.bg, color: s.color, opacity: s.opacity }}
      title={s.title}
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
