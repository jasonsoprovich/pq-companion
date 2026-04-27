import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { TrendingUp, RefreshCw, AlertCircle, Check, Search } from 'lucide-react'
import { getZealQuarmy, getCharacterAAs, listCharacters, getItem } from '../services/api'
import type { QuarmyData, CharacterAA, AAInfo, Character } from '../services/api'
import type { Item } from '../types/item'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import ItemDetailModal from '../components/ItemDetailModal'

// ── Equipment slot ordering ────────────────────────────────────────────────────

const EQUIPMENT_SLOTS = [
  'Charm', 'Ear', 'Head', 'Face', 'Neck', 'Shoulders', 'Arms', 'Back',
  'Wrist', 'Hands', 'Primary', 'Secondary', 'Range', 'Ammo',
  'Fingers', 'Chest', 'Waist', 'Legs', 'Feet',
]

function isEquipmentSlot(location: string): boolean {
  if (location.includes(':') || location.startsWith('General') ||
      location.startsWith('Bank') || location.startsWith('SharedBank') ||
      location === 'Cursor' || location.endsWith('-Coin') || location === 'Currency') {
    return false
  }
  return true
}

// ── Stat display ──────────────────────────────────────────────────────────────

interface StatBarProps {
  label: string
  value: number
  max?: number
}

function StatBar({ label, value, max = 255 }: StatBarProps): React.ReactElement {
  const pct = Math.min(100, Math.round((value / max) * 100))
  return (
    <div className="flex items-center gap-3">
      <span
        className="w-8 text-right text-xs font-mono font-semibold"
        style={{ color: 'var(--color-primary)', minWidth: '2.5rem' }}
      >
        {label}
      </span>
      <div
        className="flex-1 h-2 rounded-full overflow-hidden"
        style={{ backgroundColor: 'var(--color-surface-3)' }}
      >
        <div
          className="h-full rounded-full transition-all"
          style={{ width: `${pct}%`, backgroundColor: 'var(--color-primary)', opacity: 0.8 }}
        />
      </div>
      <span className="text-xs font-mono w-8 text-right" style={{ color: 'var(--color-foreground)' }}>
        {value}
      </span>
    </div>
  )
}

// ── Tabs ──────────────────────────────────────────────────────────────────────

type Tab = 'stats' | 'gear' | 'aas'

interface TabButtonProps {
  active: boolean
  onClick: () => void
  children: React.ReactNode
}

function TabButton({ active, onClick, children }: TabButtonProps): React.ReactElement {
  return (
    <button
      onClick={onClick}
      className="px-4 py-2 text-sm font-medium rounded-t transition-colors"
      style={{
        backgroundColor: active ? 'var(--color-surface)' : 'transparent',
        borderBottom: active ? '2px solid var(--color-primary)' : '2px solid transparent',
        color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
        cursor: 'pointer',
        border: 'none',
      }}
    >
      {children}
    </button>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function CharacterProgressPage(): React.ReactElement {
  const { active: activeCharacter } = useActiveCharacter()
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('stats')
  const [quarmy, setQuarmy] = useState<QuarmyData | null>(null)
  const [trainedAAs, setTrainedAAs] = useState<CharacterAA[]>([])
  const [availableAAs, setAvailableAAs] = useState<AAInfo[]>([])
  const [activeChar, setActiveChar] = useState<Character | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [modalItem, setModalItem] = useState<Item | null>(null)
  const [modalOpen, setModalOpen] = useState(false)

  const handleLookup = useCallback(
    (id: number) => {
      if (!id) return
      getItem(id)
        .then((item) => {
          setModalItem(item)
          setModalOpen(true)
        })
        .catch(() => {
          navigate(`/items?select=${id}`)
        })
    },
    [navigate],
  )

  const load = useCallback(async () => {
    setError(null)
    try {
      const [quarmyResp, charsResp] = await Promise.all([
        getZealQuarmy(),
        listCharacters(),
      ])
      setQuarmy(quarmyResp.quarmy)
      const found = charsResp.characters.find(
        (c) => c.name.toLowerCase() === activeCharacter.toLowerCase()
      ) ?? null
      setActiveChar(found)

      if (found) {
        const aaResp = await getCharacterAAs(found.id)
        setTrainedAAs(aaResp.trained ?? [])
        setAvailableAAs(aaResp.available ?? [])
      } else {
        setTrainedAAs([])
        setAvailableAAs([])
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load data')
    }
  }, [activeCharacter])

  useEffect(() => {
    setLoading(true)
    load().finally(() => setLoading(false))
  }, [load])

  const statsSource = quarmy?.stats ?? (activeChar ? {
    base_str: activeChar.base_str,
    base_sta: activeChar.base_sta,
    base_cha: activeChar.base_cha,
    base_dex: activeChar.base_dex,
    base_int: activeChar.base_int,
    base_agi: activeChar.base_agi,
    base_wis: activeChar.base_wis,
  } : null)

  const hasStats = statsSource && Object.values(statsSource).some((v) => v > 0)

  const equippedGear = (quarmy?.inventory ?? []).filter((e) => isEquipmentSlot(e.location))
  equippedGear.sort((a, b) => {
    const ai = EQUIPMENT_SLOTS.indexOf(a.location)
    const bi = EQUIPMENT_SLOTS.indexOf(b.location)
    if (ai === -1 && bi === -1) return a.location.localeCompare(b.location)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  return (
    <div className="flex h-full flex-col overflow-auto p-6">
      <ItemDetailModal
        item={modalItem}
        open={modalOpen}
        onClose={() => setModalOpen(false)}
      />

      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <TrendingUp size={20} style={{ color: 'var(--color-primary)' }} />
          <div>
            <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
              Character Progress
            </h1>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {activeCharacter
                ? `Showing data for ${activeCharacter} — imported from Quarmy.txt on logout`
                : 'Select a character to view progression data'}
            </p>
          </div>
        </div>
        <button
          onClick={() => { setLoading(true); load().finally(() => setLoading(false)) }}
          disabled={loading}
          className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            cursor: loading ? 'not-allowed' : 'pointer',
            opacity: loading ? 0.6 : 1,
          }}
        >
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {error && (
        <div
          className="mb-4 flex items-center gap-2 rounded px-4 py-3 text-sm"
          style={{
            backgroundColor: 'color-mix(in srgb, #f87171 12%, transparent)',
            border: '1px solid color-mix(in srgb, #f87171 30%, transparent)',
            color: '#f87171',
          }}
        >
          <AlertCircle size={14} />
          {error}
        </div>
      )}

      {!activeCharacter ? (
        <div
          className="flex flex-col items-center justify-center rounded-lg py-12 text-center"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <TrendingUp size={32} style={{ color: 'var(--color-muted)', marginBottom: '12px' }} />
          <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>No active character</p>
          <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Select a character in the Characters page to see progression data.
          </p>
        </div>
      ) : (
        <>
          {/* Tabs */}
          <div
            className="mb-4 flex gap-1 border-b"
            style={{ borderColor: 'var(--color-border)' }}
          >
            <TabButton active={tab === 'stats'} onClick={() => setTab('stats')}>Stats</TabButton>
            <TabButton active={tab === 'gear'} onClick={() => setTab('gear')}>Gear</TabButton>
            <TabButton active={tab === 'aas'} onClick={() => setTab('aas')}>
              Alternate Advancement {trainedAAs.filter(t => t.rank > 0).length > 0 ? `(${trainedAAs.filter(t => t.rank > 0).length})` : ''}
            </TabButton>
          </div>

          {loading ? (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
          ) : (
            <>
              {tab === 'stats' && (
                <StatsPanel stats={statsSource} hasStats={!!hasStats} />
              )}
              {tab === 'gear' && (
                <GearPanel gear={equippedGear} hasQuarmy={!!quarmy} onLookup={handleLookup} />
              )}
              {tab === 'aas' && (
                <AAPanel trained={trainedAAs} available={availableAAs} />
              )}
            </>
          )}
        </>
      )}
    </div>
  )
}

// ── Stats Panel ───────────────────────────────────────────────────────────────

interface StatsPanelProps {
  stats: { base_str: number; base_sta: number; base_cha: number; base_dex: number; base_int: number; base_agi: number; base_wis: number } | null
  hasStats: boolean
}

function StatsPanel({ stats, hasStats }: StatsPanelProps): React.ReactElement {
  if (!hasStats || !stats) {
    return (
      <EmptyState
        message="No stat data available"
        hint="Stats are imported automatically when your character logs out in EverQuest (requires Zeal plugin). Make sure your EQ path is configured in Settings."
      />
    )
  }

  return (
    <div
      className="rounded-lg p-5"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', maxWidth: '420px' }}
    >
      <p className="mb-4 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        Base Stats
      </p>
      <div className="space-y-3">
        <StatBar label="STR" value={stats.base_str} />
        <StatBar label="STA" value={stats.base_sta} />
        <StatBar label="AGI" value={stats.base_agi} />
        <StatBar label="DEX" value={stats.base_dex} />
        <StatBar label="WIS" value={stats.base_wis} />
        <StatBar label="INT" value={stats.base_int} />
        <StatBar label="CHA" value={stats.base_cha} />
      </div>
    </div>
  )
}

// ── Gear Panel ────────────────────────────────────────────────────────────────

interface GearPanelProps {
  gear: Array<{ location: string; name: string; id: number; count: number }>
  hasQuarmy: boolean
  onLookup: (id: number) => void
}

function GearPanel({ gear, hasQuarmy, onLookup }: GearPanelProps): React.ReactElement {
  if (!hasQuarmy) {
    return (
      <EmptyState
        message="No gear data available"
        hint="Gear is imported automatically when your character logs out in EverQuest (requires Zeal plugin). Make sure your EQ path is configured in Settings."
      />
    )
  }

  if (gear.length === 0) {
    return <EmptyState message="No equipped items found" hint="Equipment slots appear empty in the quarmy export." />
  }

  return (
    <div
      className="rounded-lg overflow-hidden overflow-y-auto"
      style={{ border: '1px solid var(--color-border)' }}
    >
      <table className="w-full text-sm">
        <thead className="sticky top-0">
          <tr style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)', width: '120px' }}>Slot</th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>Item</th>
          </tr>
        </thead>
        <tbody>
          {gear.map((item, i) => {
            const clickable = item.id > 0
            return (
              <tr
                key={`${item.location}-${i}`}
                onClick={clickable ? () => onLookup(item.id) : undefined}
                style={{
                  backgroundColor: i % 2 === 0 ? 'var(--color-surface)' : 'var(--color-surface-2)',
                  borderTop: '1px solid var(--color-border)',
                  cursor: clickable ? 'pointer' : 'default',
                }}
              >
                <td
                  className="px-4 py-2 text-xs font-medium"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  {item.location}
                </td>
                <td
                  className="px-4 py-2"
                  style={{ color: clickable ? 'var(--color-primary)' : 'var(--color-foreground)' }}
                >
                  {item.name}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

// ── AA Panel ──────────────────────────────────────────────────────────────────

// AA categories map to altadv_vars.type. type=7 (e.g. Fletching Mastery) is
// folded into Class for the rare cases it shows up.
const AA_CATEGORIES: Array<{ key: AACategoryKey; label: string; types: number[] }> = [
  { key: 'general',    label: 'General',     types: [1] },
  { key: 'archetype',  label: 'Archetype',   types: [2] },
  { key: 'class',      label: 'Class',       types: [3, 7] },
  { key: 'pop_advance',label: 'PoP Advance', types: [4] },
  { key: 'pop_ability',label: 'PoP Ability', types: [5] },
]

type AACategoryKey = 'general' | 'archetype' | 'class' | 'pop_advance' | 'pop_ability'

interface AARow extends AAInfo {
  rank: number
  isMaxed: boolean
  pointsSpent: number
  nextRankCost: number
}

// Cumulative cost for the first `rank` ranks of an AA. EQEmu's formula is:
//   rank_cost(k) = cost + (k - 1) * cost_inc, for k = 1..rank
function cumulativeCost(cost: number, costInc: number, rank: number): number {
  if (rank <= 0) return 0
  let total = 0
  for (let k = 1; k <= rank; k++) total += cost + (k - 1) * costInc
  return total
}

interface AAPanelProps {
  trained: CharacterAA[]
  available: AAInfo[]
}

function AAPanel({ trained, available }: AAPanelProps): React.ReactElement {
  const [category, setCategory] = useState<AACategoryKey>('general')
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<AARow | null>(null)

  // Index trained ranks by aa_id (eqmacid) for quick lookup.
  const trainedByID = useMemo(() => {
    const m = new Map<number, number>()
    for (const t of trained) m.set(t.aa_id, t.rank)
    return m
  }, [trained])

  // Build rows merging the catalog with the character's trained ranks.
  const allRows = useMemo<AARow[]>(() => {
    return available.map((info) => {
      const rank = trainedByID.get(info.aa_id) ?? 0
      const max = Math.max(info.max_level, rank)
      const isMaxed = rank > 0 && rank >= max
      return {
        ...info,
        rank,
        isMaxed,
        pointsSpent: cumulativeCost(info.cost, info.cost_inc, rank),
        nextRankCost: info.cost + rank * info.cost_inc,
      }
    })
  }, [available, trainedByID])

  // Total points spent across all categories — based on the catalog so we don't
  // double-count duplicated old/new rows that the dedupe collapsed.
  const totalPointsSpent = useMemo(
    () => allRows.reduce((sum, r) => sum + r.pointsSpent, 0),
    [allRows],
  )

  const cat = AA_CATEGORIES.find((c) => c.key === category) ?? AA_CATEGORIES[0]
  const term = search.trim().toLowerCase()

  const visibleRows = useMemo(() => {
    return allRows
      .filter((r) => cat.types.includes(r.type))
      .filter((r) => term === '' || r.name.toLowerCase().includes(term))
      .sort((a, b) => a.aa_id - b.aa_id)
  }, [allRows, cat, term])

  // Empty-catalog case: backend returned no class-eligible AAs (e.g. new
  // character with class = -1, or quarm.db unavailable).
  if (available.length === 0 && trained.length === 0) {
    return (
      <EmptyState
        message="No AA data available"
        hint="Alternate Advancement abilities are imported automatically when your character logs out in EverQuest (requires Zeal plugin). Make sure your EQ path is configured in Settings."
      />
    )
  }

  return (
    <div className="flex flex-col gap-3" style={{ minHeight: 0 }}>
      {/* Header: points spent + search */}
      <div className="flex items-center justify-between gap-4">
        <div className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          AA Points Spent:{' '}
          <span className="font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {totalPointsSpent}
          </span>
        </div>
        <div
          className="flex items-center gap-2 rounded px-2 py-1"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
            minWidth: '220px',
          }}
        >
          <Search size={14} style={{ color: 'var(--color-muted)' }} />
          <input
            type="text"
            placeholder="Search AAs…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="flex-1 bg-transparent text-sm outline-none"
            style={{ color: 'var(--color-foreground)' }}
          />
        </div>
      </div>

      {/* Category sub-tabs */}
      <div className="flex gap-1 border-b" style={{ borderColor: 'var(--color-border)' }}>
        {AA_CATEGORIES.map((c) => (
          <TabButton key={c.key} active={category === c.key} onClick={() => setCategory(c.key)}>
            {c.label}
          </TabButton>
        ))}
      </div>

      {/* Ability list */}
      <div
        className="rounded-lg overflow-hidden flex-1"
        style={{ border: '1px solid var(--color-border)', minHeight: 0 }}
      >
        <div className="overflow-y-auto h-full">
          <table className="w-full text-sm">
            <thead className="sticky top-0 z-10">
              <tr style={{ backgroundColor: 'var(--color-surface-2)' }}>
                <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                  Title
                </th>
                <th className="px-4 py-2 text-right text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)', width: '100px' }}>
                  Cur/Max
                </th>
                <th className="px-4 py-2 text-right text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)', width: '70px' }}>
                  Cost
                </th>
              </tr>
            </thead>
            <tbody>
              {visibleRows.length === 0 ? (
                <tr>
                  <td colSpan={3} className="px-4 py-6 text-center text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    {term ? 'No matching abilities.' : 'No abilities in this category.'}
                  </td>
                </tr>
              ) : (
                visibleRows.map((r, i) => {
                  const dim = r.rank === 0
                  const isSel = selected?.aa_id === r.aa_id
                  const max = Math.max(r.max_level, r.rank)
                  return (
                    <tr
                      key={r.aa_id}
                      onClick={() => setSelected(r)}
                      style={{
                        backgroundColor: isSel
                          ? 'color-mix(in srgb, var(--color-primary) 12%, transparent)'
                          : i % 2 === 0
                          ? 'var(--color-surface)'
                          : 'var(--color-surface-2)',
                        borderTop: '1px solid var(--color-border)',
                        cursor: 'pointer',
                        opacity: dim ? 0.45 : 1,
                      }}
                    >
                      <td className="px-4 py-2" style={{ color: 'var(--color-foreground)' }}>
                        {r.name}
                      </td>
                      <td
                        className="px-4 py-2 text-right font-mono text-xs"
                        style={{ color: r.isMaxed ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}
                      >
                        {r.rank}/{max}
                      </td>
                      <td className="px-4 py-2 text-right font-mono text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                        {r.isMaxed ? (
                          <Check size={14} style={{ color: 'var(--color-primary)', display: 'inline' }} />
                        ) : (
                          r.nextRankCost
                        )}
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Description placeholder */}
      <div
        className="rounded-lg p-3"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <p className="mb-1 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Description
        </p>
        {selected ? (
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            <span className="font-semibold">{selected.name}</span>
            <span className="ml-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              max rank {Math.max(selected.max_level, selected.rank)} · base cost {selected.cost}
              {selected.cost_inc > 0 ? ` (+${selected.cost_inc}/rank)` : ''}
            </span>
          </p>
        ) : (
          <p className="text-sm italic" style={{ color: 'var(--color-muted-foreground)' }}>
            Select an AA to see its description.
          </p>
        )}
      </div>
    </div>
  )
}

// ── Empty State ───────────────────────────────────────────────────────────────

function EmptyState({ message, hint }: { message: string; hint: string }): React.ReactElement {
  return (
    <div
      className="flex flex-col items-center justify-center rounded-lg py-12 text-center"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <AlertCircle size={28} style={{ color: 'var(--color-muted)', marginBottom: '10px' }} />
      <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>{message}</p>
      <p className="mt-1 max-w-sm text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{hint}</p>
    </div>
  )
}
