import React, { useCallback, useEffect, useState } from 'react'
import { TrendingUp, RefreshCw, AlertCircle } from 'lucide-react'
import { getZealQuarmy, getCharacterAAs, listCharacters } from '../services/api'
import type { QuarmyData, CharacterAA, Character } from '../services/api'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'

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
  const [tab, setTab] = useState<Tab>('stats')
  const [quarmy, setQuarmy] = useState<QuarmyData | null>(null)
  const [aas, setAAs] = useState<CharacterAA[]>([])
  const [activeChar, setActiveChar] = useState<Character | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

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
        setAAs(aaResp.aas)
      } else {
        setAAs([])
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
              Alternate Advancement {aas.length > 0 ? `(${aas.length})` : ''}
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
                <GearPanel gear={equippedGear} hasQuarmy={!!quarmy} />
              )}
              {tab === 'aas' && (
                <AAPanel aas={aas} />
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
}

function GearPanel({ gear, hasQuarmy }: GearPanelProps): React.ReactElement {
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
      className="rounded-lg overflow-hidden"
      style={{ border: '1px solid var(--color-border)' }}
    >
      <table className="w-full text-sm">
        <thead>
          <tr style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)', width: '120px' }}>Slot</th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>Item</th>
          </tr>
        </thead>
        <tbody>
          {gear.map((item, i) => (
            <tr
              key={`${item.location}-${i}`}
              style={{
                backgroundColor: i % 2 === 0 ? 'var(--color-surface)' : 'var(--color-surface-2)',
                borderTop: '1px solid var(--color-border)',
              }}
            >
              <td
                className="px-4 py-2 text-xs font-medium"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                {item.location}
              </td>
              <td className="px-4 py-2" style={{ color: 'var(--color-foreground)' }}>
                {item.name}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── AA Panel ──────────────────────────────────────────────────────────────────

interface AAPanelProps {
  aas: CharacterAA[]
}

function AAPanel({ aas }: AAPanelProps): React.ReactElement {
  if (aas.length === 0) {
    return (
      <EmptyState
        message="No AA data available"
        hint="Alternate Advancement abilities are imported automatically when your character logs out in EverQuest (requires Zeal plugin). Make sure your EQ path is configured in Settings."
      />
    )
  }

  return (
    <div
      className="rounded-lg overflow-hidden"
      style={{ border: '1px solid var(--color-border)' }}
    >
      <table className="w-full text-sm">
        <thead>
          <tr style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)', width: '80px' }}>AA ID</th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)', width: '80px' }}>Rank</th>
          </tr>
        </thead>
        <tbody>
          {aas.map((aa, i) => (
            <tr
              key={aa.aa_id}
              style={{
                backgroundColor: i % 2 === 0 ? 'var(--color-surface)' : 'var(--color-surface-2)',
                borderTop: '1px solid var(--color-border)',
              }}
            >
              <td className="px-4 py-2 font-mono text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {aa.aa_id}
              </td>
              <td className="px-4 py-2">
                <span
                  className="inline-flex items-center justify-center rounded px-2 py-0.5 text-xs font-semibold"
                  style={{
                    backgroundColor: 'color-mix(in srgb, var(--color-primary) 15%, transparent)',
                    color: 'var(--color-primary)',
                  }}
                >
                  Rank {aa.rank}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div
        className="px-4 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', borderTop: '1px solid var(--color-border)' }}
      >
        {aas.length} {aas.length === 1 ? 'ability' : 'abilities'} purchased
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
