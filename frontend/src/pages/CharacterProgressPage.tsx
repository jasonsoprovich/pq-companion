import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { TrendingUp, RefreshCw, AlertCircle, Check, Search } from 'lucide-react'
import {
  getZealQuarmy, getCharacterAAs, listCharacters, getItem,
  getCharacterSpellModifiers, searchSpells, getCharacterEquippedStats,
} from '../services/api'
import type {
  QuarmyData, CharacterAA, AAInfo, Character,
  SpellModifier, SpellModifierResolution, EquippedStats, StatBlock,
} from '../services/api'
import type { Spell } from '../types/spell'
import type { Item } from '../types/item'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import ItemDetailModal from '../components/ItemDetailModal'
import CharacterSubTabs from '../components/CharacterSubTabs'

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

type Tab = 'stats' | 'gear' | 'aas' | 'modifiers'

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
  const [viewedCharacter, setViewedCharacter] = useState('')
  // Default the viewed character to the active one when it first becomes known.
  useEffect(() => {
    if (!viewedCharacter && activeCharacter) setViewedCharacter(activeCharacter)
  }, [activeCharacter, viewedCharacter])
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('stats')
  const [quarmy, setQuarmy] = useState<QuarmyData | null>(null)
  const [trainedAAs, setTrainedAAs] = useState<CharacterAA[]>([])
  const [availableAAs, setAvailableAAs] = useState<AAInfo[]>([])
  const [modifiers, setModifiers] = useState<SpellModifier[] | null>(null)
  const [equippedStats, setEquippedStats] = useState<EquippedStats | null>(null)
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
    if (!viewedCharacter) return
    try {
      const charsResp = await listCharacters()
      const found = charsResp.characters.find(
        (c) => c.name.toLowerCase() === viewedCharacter.toLowerCase()
      ) ?? null
      setActiveChar(found)

      // For the active character we use the cached watcher data; for any other
      // character we ask the backend to parse that character's quarmy file.
      const isActive = activeCharacter && viewedCharacter.toLowerCase() === activeCharacter.toLowerCase()
      const quarmyResp = await getZealQuarmy(isActive ? undefined : viewedCharacter)
      setQuarmy(quarmyResp.quarmy)

      if (found) {
        const aaResp = await getCharacterAAs(found.id)
        setTrainedAAs(aaResp.trained ?? [])
        setAvailableAAs(aaResp.available ?? [])
        try {
          const modResp = await getCharacterSpellModifiers(found.id)
          setModifiers(modResp.contributors ?? [])
        } catch {
          // Quarmy export not available — modifiers panel will show its own empty state
          setModifiers(null)
        }
        try {
          const eq = await getCharacterEquippedStats(found.id)
          setEquippedStats(eq)
        } catch {
          setEquippedStats(null)
        }
      } else {
        setTrainedAAs([])
        setAvailableAAs([])
        setModifiers(null)
        setEquippedStats(null)
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load data')
    }
  }, [viewedCharacter, activeCharacter])

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
    <div className="flex h-full flex-col">
      <ItemDetailModal
        item={modalItem}
        open={modalOpen}
        onClose={() => setModalOpen(false)}
      />

      <CharacterSubTabs
        value={viewedCharacter}
        onChange={setViewedCharacter}
      />

      <div className="flex-1 overflow-auto p-6">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <TrendingUp size={20} style={{ color: 'var(--color-primary)' }} />
          <div>
            <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
              Character Info
            </h1>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {viewedCharacter
                ? `Showing data for ${viewedCharacter}`
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

      {!viewedCharacter ? (
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
            <TabButton active={tab === 'modifiers'} onClick={() => setTab('modifiers')}>
              Spell Modifiers {modifiers && modifiers.length > 0 ? `(${modifiers.length})` : ''}
            </TabButton>
          </div>

          {loading ? (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
          ) : (
            <>
              {tab === 'stats' && (
                <StatsPanel stats={statsSource} hasStats={!!hasStats} gear={equippedStats} />
              )}
              {tab === 'gear' && (
                <GearPanel gear={equippedGear} hasQuarmy={!!quarmy} onLookup={handleLookup} />
              )}
              {tab === 'aas' && (
                <AAPanel trained={trainedAAs} available={availableAAs} />
              )}
              {tab === 'modifiers' && (
                <SpellModifiersPanel
                  characterID={activeChar?.id ?? null}
                  contributors={modifiers}
                />
              )}
            </>
          )}
        </>
      )}
      </div>
    </div>
  )
}

// ── Stats Panel ───────────────────────────────────────────────────────────────

type StatMode = 'base' | 'equipped' | 'buffed'
type AegolismChoice = 'aegolism' | 'glades'

// EQ caps individual attribute stats at 255. Anything above the cap is wasted.
const STAT_CAP = 255

// StatDelta covers every dimension a buff or gear set might touch. Optional
// fields keep buff definitions readable.
interface StatDelta {
  hp?: number; mana?: number; ac?: number
  str?: number; sta?: number; agi?: number; dex?: number
  wis?: number; int?: number; cha?: number
  pr?: number; mr?: number; dr?: number; fr?: number; cr?: number
  attack?: number; haste?: number; regen?: number
  mana_regen?: number; ft?: number; dmg_shield?: number
}

interface BuffDef {
  name: string
  delta: StatDelta
}

// Raid-buff preset matching quarmy.com's defaults. Aegolism / Protection of
// the Glades are mutually exclusive (different druid/cleric tiers); the rest
// stack additively.
const AEGOLISM_BUFF: BuffDef = {
  name: 'Ancient: Gift of Aegolism',
  delta: { hp: 1300, ac: 89 },
}
const GLADES_BUFF: BuffDef = {
  name: 'Protection of the Glades',
  delta: { hp: 250, ac: 39 },
}

const RAID_BUFFS: BuffDef[] = [
  { name: "Khura's Focusing",            delta: { hp: 430, str: 67, dex: 60 } },
  { name: 'Talisman of the Brute',       delta: { sta: 50 } },
  { name: 'Talisman of the Cat',         delta: { agi: 52 } },
  { name: "Brell's Mountainous Barrier", delta: { hp: 225 } },
  { name: 'Visions of Grandeur',         delta: { agi: 40, dex: 25, haste: 58, attack: 26 } },
  { name: "Koadic's Endless Intellect",  delta: { mana: 250, wis: 25, int: 25, ft: 14 } },
  { name: 'Circle of the Seasons',       delta: { pr: 15, mr: 15, dr: 15, fr: 15, cr: 15 } },
  { name: 'Group Resist Magic',          delta: { mr: 10, fr: 10, cr: 10 } },
]

function emptyDelta(): Required<StatDelta> {
  return {
    hp: 0, mana: 0, ac: 0,
    str: 0, sta: 0, agi: 0, dex: 0, wis: 0, int: 0, cha: 0,
    pr: 0, mr: 0, dr: 0, fr: 0, cr: 0,
    attack: 0, haste: 0, regen: 0, mana_regen: 0, ft: 0, dmg_shield: 0,
  }
}

function addDelta(into: Required<StatDelta>, d: StatDelta): void {
  into.hp += d.hp ?? 0
  into.mana += d.mana ?? 0
  into.ac += d.ac ?? 0
  into.str += d.str ?? 0
  into.sta += d.sta ?? 0
  into.agi += d.agi ?? 0
  into.dex += d.dex ?? 0
  into.wis += d.wis ?? 0
  into.int += d.int ?? 0
  into.cha += d.cha ?? 0
  into.pr += d.pr ?? 0
  into.mr += d.mr ?? 0
  into.dr += d.dr ?? 0
  into.fr += d.fr ?? 0
  into.cr += d.cr ?? 0
  into.attack += d.attack ?? 0
  into.haste += d.haste ?? 0
  into.regen += d.regen ?? 0
  into.mana_regen += d.mana_regen ?? 0
  into.ft += d.ft ?? 0
  into.dmg_shield += d.dmg_shield ?? 0
}

function statBlockToDelta(b: StatBlock): StatDelta {
  return { ...b }
}

interface StatsPanelProps {
  stats: { base_str: number; base_sta: number; base_cha: number; base_dex: number; base_int: number; base_agi: number; base_wis: number } | null
  hasStats: boolean
  gear: EquippedStats | null
}

function StatsPanel({ stats, hasStats, gear }: StatsPanelProps): React.ReactElement {
  const [mode, setMode] = useState<StatMode>('equipped')
  const [aegolism, setAegolism] = useState<AegolismChoice>('aegolism')

  if (!hasStats || !stats) {
    return (
      <EmptyState
        message="No stat data available"
        hint="Stats are imported automatically when your character logs out in EverQuest (requires Zeal plugin). Make sure your EQ path is configured in Settings."
      />
    )
  }

  const includesGear = mode !== 'base'
  const includesBuffs = mode === 'buffed'

  const buffs: BuffDef[] = includesBuffs
    ? [aegolism === 'aegolism' ? AEGOLISM_BUFF : GLADES_BUFF, ...RAID_BUFFS]
    : []

  // Sum every active source into one delta. Base attribs come from quarmy on
  // the character row; the backend's `base` block fills in HP/Mana from
  // base_data and the +25 starting resists.
  const total = emptyDelta()
  if (gear?.base) {
    addDelta(total, statBlockToDelta(gear.base))
  } else {
    // Fall back to quarmy attribs only if the equipped-stats endpoint hasn't
    // resolved yet — at least the seven attributes still show.
    addDelta(total, {
      str: stats.base_str, sta: stats.base_sta, agi: stats.base_agi,
      dex: stats.base_dex, wis: stats.base_wis, int: stats.base_int, cha: stats.base_cha,
    })
  }
  if (includesGear && gear?.equipment) addDelta(total, statBlockToDelta(gear.equipment))
  for (const b of buffs) addDelta(total, b.delta)

  const capped = (v: number) => Math.min(STAT_CAP, v)
  const gearMissing = includesGear && !gear

  return (
    <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
      {/* Stats column */}
      <div
        className="rounded-lg p-5"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', flex: '0 0 auto', minWidth: '420px', maxWidth: '480px' }}
      >
        <StatModeToggle mode={mode} onChange={setMode} />

        {gearMissing && (
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Equipment stats unavailable — make sure Zeal is installed and the character has logged out at least once.
          </p>
        )}

        <div className="mb-4 grid grid-cols-3 gap-3">
          <VitalRow label="HP" value={total.hp} />
          <VitalRow label="Mana" value={total.mana} />
          <VitalRow label="AC" value={total.ac} />
          <VitalRow label="ATK" value={total.attack} />
        </div>

        <div className="space-y-3">
          <StatBar label="STR" value={capped(total.str)} max={STAT_CAP} />
          <StatBar label="STA" value={capped(total.sta)} max={STAT_CAP} />
          <StatBar label="AGI" value={capped(total.agi)} max={STAT_CAP} />
          <StatBar label="DEX" value={capped(total.dex)} max={STAT_CAP} />
          <StatBar label="WIS" value={capped(total.wis)} max={STAT_CAP} />
          <StatBar label="INT" value={capped(total.int)} max={STAT_CAP} />
          <StatBar label="CHA" value={capped(total.cha)} max={STAT_CAP} />
        </div>

        <div className="mt-4 grid grid-cols-5 gap-2">
          <ResistRow label="POISON"  value={total.pr} />
          <ResistRow label="MAGIC"   value={total.mr} />
          <ResistRow label="DISEASE" value={total.dr} />
          <ResistRow label="FIRE"    value={total.fr} />
          <ResistRow label="COLD"    value={total.cr} />
        </div>

        {includesGear && (
          <div
            className="mt-4 border-t pt-3 text-xs"
            style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
          >
            <div className="grid grid-cols-2 gap-y-1">
              <span>Haste</span>      <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>{total.haste}%</span>
              <span>Regen</span>      <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>+{total.regen}/tick</span>
              <span>Mana Regen</span> <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>+{total.mana_regen}/tick</span>
              <span>Flowing Thought</span> <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>{total.ft}</span>
              <span>Damage Shield</span> <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>{total.dmg_shield}</span>
            </div>
            <p className="mt-2 italic" style={{ color: 'var(--color-muted)' }}>
              Worn-effect contributions from items (haste, FT, regen, ATK, DS) aren&rsquo;t parsed yet — values shown here come from raid buffs only.
            </p>
          </div>
        )}
      </div>

      {/* Raid Buffs side panel */}
      {includesBuffs && (
        <RaidBuffsPanel
          aegolism={aegolism}
          onSwap={() => setAegolism(aegolism === 'aegolism' ? 'glades' : 'aegolism')}
          buffs={buffs}
        />
      )}
    </div>
  )
}

interface StatModeToggleProps {
  mode: StatMode
  onChange: (m: StatMode) => void
}

function StatModeToggle({ mode, onChange }: StatModeToggleProps): React.ReactElement {
  const options: Array<{ value: StatMode; label: string }> = [
    { value: 'base',     label: 'Base' },
    { value: 'equipped', label: '+Equipment' },
    { value: 'buffed',   label: '+Raid Buffs' },
  ]
  return (
    <div
      className="mb-4 inline-flex rounded p-0.5"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      {options.map((o) => {
        const active = mode === o.value
        return (
          <button
            key={o.value}
            onClick={() => onChange(o.value)}
            className="rounded px-3 py-1 text-xs font-medium transition-colors"
            style={{
              backgroundColor: active ? 'var(--color-primary)' : 'transparent',
              color: active ? '#fff' : 'var(--color-muted-foreground)',
              border: 'none',
              cursor: 'pointer',
            }}
          >
            {o.label}
          </button>
        )
      })}
    </div>
  )
}

function VitalRow({ label, value }: { label: string; value: number }): React.ReactElement {
  return (
    <div
      className="rounded px-3 py-2"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <p className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>{label}</p>
      <p className="font-mono text-sm" style={{ color: 'var(--color-primary)' }}>{value}</p>
    </div>
  )
}

function ResistRow({ label, value }: { label: string; value: number }): React.ReactElement {
  return (
    <div className="text-center">
      <p className="text-[10px] font-semibold tracking-wide" style={{ color: 'var(--color-muted)' }}>{label}</p>
      <p className="font-mono text-sm" style={{ color: 'var(--color-primary)' }}>{value}</p>
    </div>
  )
}

// ── Raid Buffs side panel ─────────────────────────────────────────────────────

interface RaidBuffsPanelProps {
  aegolism: AegolismChoice
  onSwap: () => void
  buffs: BuffDef[]
}

function RaidBuffsPanel({ aegolism, onSwap, buffs }: RaidBuffsPanelProps): React.ReactElement {
  const total = emptyDelta()
  for (const b of buffs) addDelta(total, b.delta)

  return (
    <div
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', flex: '1 1 auto', minWidth: '320px' }}
    >
      <div className="mb-3 flex items-center justify-between">
        <span
          className="rounded-full px-3 py-1 text-xs font-semibold"
          style={{
            border: '1px solid color-mix(in srgb, var(--color-primary) 60%, transparent)',
            color: 'var(--color-primary)',
          }}
        >
          Raid Buffs
        </span>
        <button
          onClick={onSwap}
          className="rounded px-2 py-1 text-xs font-medium"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            cursor: 'pointer',
          }}
          title={`Swap to ${aegolism === 'aegolism' ? 'Protection of the Glades' : 'Ancient: Gift of Aegolism'}`}
        >
          ⇄ Swap Aegolism / PotG
        </button>
      </div>

      <p className="mb-3 text-xs italic" style={{ color: 'var(--color-muted-foreground)' }}>
        Preset baseline — values match quarmy.com defaults. Aegolism and Protection of the Glades don&rsquo;t stack;
        the rest are additive.
      </p>

      <div className="divide-y" style={{ borderColor: 'var(--color-border)' }}>
        {buffs.map((b) => (
          <div key={b.name} className="py-2">
            <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>{b.name}</p>
            <p className="font-mono text-xs" style={{ color: 'var(--color-primary)' }}>
              {formatBuffEffects(b.delta) || <span style={{ color: 'var(--color-muted)' }}>—</span>}
            </p>
          </div>
        ))}
      </div>

      <div
        className="mt-3 border-t pt-3 text-xs"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <p className="mb-1 text-[10px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Total
        </p>
        <p className="font-mono leading-relaxed" style={{ color: 'var(--color-primary)' }}>
          {formatBuffEffects(total)}
        </p>
      </div>
    </div>
  )
}

// formatBuffEffects renders one row of "+stat" tokens for a buff delta. Skips
// zero-valued fields, and uses the same labels (Haste%, FT, ATK, etc) as the
// stats panel so the two read consistently.
function formatBuffEffects(d: StatDelta | Required<StatDelta>): string {
  const parts: string[] = []
  const push = (label: string, v: number | undefined, suffix = '') => {
    if (v && v !== 0) parts.push(`${label}+${v}${suffix}`)
  }
  push('AC', d.ac)
  push('HP', d.hp)
  push('Mana', d.mana)
  push('STR', d.str)
  push('STA', d.sta)
  push('AGI', d.agi)
  push('DEX', d.dex)
  push('WIS', d.wis)
  push('INT', d.int)
  push('CHA', d.cha)
  push('PR', d.pr)
  push('MR', d.mr)
  push('DR', d.dr)
  push('FR', d.fr)
  push('CR', d.cr)
  push('Haste', d.haste, '%')
  push('ATK', d.attack)
  push('Regen', d.regen)
  push('Mana Regen', d.mana_regen)
  push('FT', d.ft)
  push('DS', d.dmg_shield)
  return parts.join(' ')
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

      {/* Description */}
      <div
        className="rounded-lg p-3"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <p className="mb-1 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Description
        </p>
        {selected ? (
          <div>
            <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              <span className="font-semibold">{selected.name}</span>
              <span className="ml-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                max rank {Math.max(selected.max_level, selected.rank)} · base cost {selected.cost}
                {selected.cost_inc > 0 ? ` (+${selected.cost_inc}/rank)` : ''}
              </span>
            </p>
            {selected.description && (
              <p className="mt-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
                {selected.description}
              </p>
            )}
          </div>
        ) : (
          <p className="text-sm italic" style={{ color: 'var(--color-muted-foreground)' }}>
            Select an AA to see its description.
          </p>
        )}
      </div>
    </div>
  )
}

// ── Spell Modifiers Panel ─────────────────────────────────────────────────────

function spaLabel(spa: number): string {
  if (spa === 128) return 'Duration'
  if (spa === 127) return 'Cast Time'
  return `SPA ${spa}`
}

function spellTypeLabel(type: number): string {
  if (type === 1) return 'beneficial'
  if (type === 0) return 'detrimental'
  if (type === 2) return 'any'
  return 'any'
}

interface SpellModifiersPanelProps {
  characterID: number | null
  contributors: SpellModifier[] | null
}

function SpellModifiersPanel({ characterID, contributors }: SpellModifiersPanelProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Spell[]>([])
  const [resolution, setResolution] = useState<SpellModifierResolution | null>(null)
  const [resolveLoading, setResolveLoading] = useState(false)

  // Debounced spell search.
  useEffect(() => {
    if (!query.trim()) { setResults([]); return }
    const t = setTimeout(() => {
      searchSpells(query, 8)
        .then((r) => setResults(r.items ?? []))
        .catch(() => setResults([]))
    }, 200)
    return () => clearTimeout(t)
  }, [query])

  const resolve = useCallback(async (spellID: number) => {
    if (!characterID) return
    setResolveLoading(true)
    try {
      const resp = await getCharacterSpellModifiers(characterID, spellID)
      setResolution(resp.resolution ?? null)
    } finally {
      setResolveLoading(false)
    }
  }, [characterID])

  if (contributors === null) {
    return (
      <EmptyState
        message="No quarmy export available"
        hint="Spell modifiers are computed from your most recent <CharName>-Quarmy.txt file. Make sure Zeal is installed and that you've logged out at least once with this character."
      />
    )
  }
  if (contributors.length === 0) {
    return (
      <EmptyState
        message="No focus modifiers detected"
        hint="No equipped items have focus effects, and no duration-extending AAs are trained."
      />
    )
  }

  const items = contributors.filter((c) => c.source === 'item')
  const aas = contributors.filter((c) => c.source === 'aa')

  return (
    <div className="space-y-5">
      {/* Contributors */}
      <div
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <p className="mb-3 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Contributors ({contributors.length})
        </p>
        {items.length > 0 && (
          <div className="mb-3">
            <p className="mb-2 text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
              From equipped items
            </p>
            <div className="space-y-1.5">
              {items.map((m, i) => (
                <ModifierRow key={`item-${i}`} m={m} />
              ))}
            </div>
          </div>
        )}
        {aas.length > 0 && (
          <div>
            <p className="mb-2 text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
              From trained AAs
            </p>
            <div className="space-y-1.5">
              {aas.map((m, i) => (
                <ModifierRow key={`aa-${i}`} m={m} />
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Test Resolution */}
      <div
        className="rounded-lg p-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <p className="mb-1 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          Test Resolution
        </p>
        <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Pick a spell to see exactly how this character's modifiers apply, after EQEmu's filter and stacking rules
          (best item-focus + AA percentages summed).
        </p>
        <div className="relative mb-3">
          <Search size={14} style={{
            position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)',
            color: 'var(--color-muted-foreground)',
          }} />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search spells (e.g. Aegolism, Snare, Clarity)…"
            className="w-full rounded px-3 py-1.5 pl-8 text-sm"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
          />
        </div>
        {results.length > 0 && (
          <div
            className="mb-3 max-h-48 overflow-y-auto rounded"
            style={{ border: '1px solid var(--color-border)' }}
          >
            {results.map((s) => (
              <button
                key={s.id}
                onClick={() => { setQuery(s.name); setResults([]); resolve(s.id) }}
                className="block w-full px-3 py-1.5 text-left text-sm"
                style={{
                  backgroundColor: 'transparent',
                  color: 'var(--color-foreground)',
                  borderBottom: '1px solid var(--color-border)',
                  cursor: 'pointer',
                  border: 'none',
                }}
                onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = 'var(--color-surface-2)')}
                onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
              >
                {s.name}
              </button>
            ))}
          </div>
        )}
        {resolveLoading && (
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Resolving…</p>
        )}
        {resolution && !resolveLoading && (
          <ResolutionDisplay r={resolution} />
        )}
      </div>
    </div>
  )
}

function ModifierRow({ m }: { m: SpellModifier }): React.ReactElement {
  const sourceLabel = m.source === 'item'
    ? `${m.source_item_name}${m.source_item_slot ? ` (${m.source_item_slot})` : ''}`
    : `${m.source_aa_name}${m.source_aa_rank ? ` rank ${m.source_aa_rank}` : ''}`
  const focusLabel = m.focus_spell_name ? ` · ${m.focus_spell_name}` : ''
  const sign = m.spa === 127 ? '−' : '+'
  return (
    <div
      className="rounded px-3 py-2 text-xs"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center justify-between gap-2">
        <span style={{ color: 'var(--color-foreground)' }}>
          <span className="font-medium">{sourceLabel}</span>
          <span style={{ color: 'var(--color-muted-foreground)' }}>{focusLabel}</span>
        </span>
        <span className="font-mono font-semibold" style={{ color: 'var(--color-primary)' }}>
          {sign}{m.percent}% {spaLabel(m.spa)}
        </span>
      </div>
      <div className="mt-1 flex flex-wrap gap-1">
        <FilterTag label={spellTypeLabel(m.limits.spell_type)} />
        {m.limits.max_level ? <FilterTag label={`≤ L${m.limits.max_level}`} /> : null}
        {m.limits.min_level ? <FilterTag label={`≥ L${m.limits.min_level}`} /> : null}
        {m.limits.min_duration_sec ? <FilterTag label={`≥ ${m.limits.min_duration_sec}s`} /> : null}
        {m.limits.exclude_effects && m.limits.exclude_effects.length > 0 ? (
          <FilterTag label={`excl. ${m.limits.exclude_effects.length} effect${m.limits.exclude_effects.length > 1 ? 's' : ''}`} />
        ) : null}
      </div>
    </div>
  )
}

function FilterTag({ label }: { label: string }): React.ReactElement {
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px]"
      style={{
        backgroundColor: 'var(--color-surface-3)',
        color: 'var(--color-muted-foreground)',
      }}
    >
      {label}
    </span>
  )
}

function formatDuration(sec: number): string {
  if (sec <= 0) return '0s'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  const parts: string[] = []
  if (h) parts.push(`${h}h`)
  if (m) parts.push(`${m}m`)
  if (s || parts.length === 0) parts.push(`${s}s`)
  return parts.join(' ')
}

function ResolutionDisplay({ r }: { r: SpellModifierResolution }): React.ReactElement {
  const ctSign = r.cast_time_percent > 0 ? '−' : ''
  return (
    <div
      className="rounded p-3"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {r.spell_name}
        </span>
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          spell L{r.spell_level} · cast at L{r.caster_level} · {spellTypeLabel(r.spell_type)}
        </span>
      </div>

      {/* Duration block — AAs apply first, then item focus on top of that. */}
      <div className="mb-3 grid grid-cols-4 gap-3 text-xs">
        <div>
          <p className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Base
          </p>
          <p className="mt-0.5 font-mono" style={{ color: 'var(--color-foreground)' }}>
            {r.base_duration_sec}s
          </p>
          <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            {formatDuration(r.base_duration_sec)}
          </p>
        </div>
        <div>
          <p className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            AA Modifier
          </p>
          <p className="mt-0.5 font-mono font-semibold" style={{ color: r.duration_aa_percent > 0 ? 'var(--color-primary)' : 'var(--color-muted)' }}>
            +{r.duration_aa_percent}%
          </p>
          <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            applied first
          </p>
        </div>
        <div>
          <p className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Item Modifier
          </p>
          <p className="mt-0.5 font-mono font-semibold" style={{ color: r.duration_item_percent > 0 ? 'var(--color-primary)' : 'var(--color-muted)' }}>
            +{r.duration_item_percent}%
          </p>
          <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            stacked on top
          </p>
        </div>
        <div>
          <p className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Effective
          </p>
          <p className="mt-0.5 font-mono font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {r.extended_duration_sec}s
          </p>
          <p className="text-[11px]" style={{ color: 'var(--color-primary)' }}>
            {formatDuration(r.extended_duration_sec)}
          </p>
        </div>
      </div>
      <p className="mb-3 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
        Total {r.base_duration_sec}s × {(1 + r.duration_aa_percent / 100).toFixed(2)} × {(1 + r.duration_item_percent / 100).toFixed(2)} = {r.extended_duration_sec}s · cast time {ctSign}{r.cast_time_percent}%
      </p>

      {r.applied.length > 0 && (
        <div>
          <p className="mb-1 text-[10px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
            Applied ({r.applied.length})
          </p>
          <div className="space-y-1">
            {r.applied.map((m, i) => (
              <div key={i} className="text-xs" style={{ color: 'var(--color-foreground)' }}>
                {m.source === 'item' ? m.source_item_name : `${m.source_aa_name} rank ${m.source_aa_rank}`}
                <span className="ml-2 font-mono" style={{ color: 'var(--color-primary)' }}>
                  {m.spa === 127 ? '−' : '+'}{m.percent}% {spaLabel(m.spa)}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
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
