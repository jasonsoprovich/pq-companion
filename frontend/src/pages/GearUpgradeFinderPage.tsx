import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { Wand2, ChevronDown, ChevronRight, Star, Loader2, AlertTriangle, Sliders, RotateCcw, Save } from 'lucide-react'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ItemIcon } from '../components/Icon'
import { SourceNPCLink } from '../components/SourceNPCLink'
import ItemDetailModal from '../components/ItemDetailModal'
import {
  listCharacters,
  getItem,
  getItemSources,
  getCharacterUpgrades,
  getCharacterUpgradeWeights,
  setCharacterUpgradeWeights,
  resetCharacterUpgradeWeights,
  type Character,
  type UpgradeWeights,
  type UpgradeCandidate,
  type UpgradesResponse,
} from '../services/api'
import type { Item, ItemSources } from '../types/item'

// Logical worn slots, keyed to the backend slot keys.
const SLOTS: { key: string; label: string }[] = [
  { key: 'ear', label: 'Ear' },
  { key: 'head', label: 'Head' },
  { key: 'face', label: 'Face' },
  { key: 'neck', label: 'Neck' },
  { key: 'shoulders', label: 'Shoulders' },
  { key: 'arms', label: 'Arms' },
  { key: 'back', label: 'Back' },
  { key: 'wrist', label: 'Wrist' },
  { key: 'hands', label: 'Hands' },
  { key: 'fingers', label: 'Fingers' },
  { key: 'chest', label: 'Chest' },
  { key: 'legs', label: 'Legs' },
  { key: 'feet', label: 'Feet' },
  { key: 'waist', label: 'Waist' },
  { key: 'primary', label: 'Primary' },
  { key: 'secondary', label: 'Secondary' },
  { key: 'range', label: 'Range' },
  { key: 'charm', label: 'Charm' },
  { key: 'ammo', label: 'Ammo' },
]

const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight', 'Druid', 'Monk',
  'Bard', 'Rogue', 'Shaman', 'Necromancer', 'Wizard', 'Magician', 'Enchanter',
  'Beastlord',
]

// Order + labels for the stat-delta chips and the weights editor.
const STAT_KEYS: { key: keyof UpgradeWeights; label: string }[] = [
  { key: 'hp', label: 'HP' }, { key: 'mana', label: 'Mana' }, { key: 'ac', label: 'AC' },
  { key: 'str', label: 'STR' }, { key: 'sta', label: 'STA' }, { key: 'agi', label: 'AGI' },
  { key: 'dex', label: 'DEX' }, { key: 'wis', label: 'WIS' }, { key: 'int', label: 'INT' },
  { key: 'cha', label: 'CHA' }, { key: 'mr', label: 'MR' }, { key: 'fr', label: 'FR' },
  { key: 'cr', label: 'CR' }, { key: 'dr', label: 'DR' }, { key: 'pr', label: 'PR' },
]

const STAT_LABEL: Record<string, string> = Object.fromEntries(
  STAT_KEYS.map((s) => [s.key, s.label]),
)

function inputStyle(): React.CSSProperties {
  return {
    background: 'var(--color-background)',
    border: '1px solid var(--color-border)',
    borderRadius: 4,
    color: 'var(--color-foreground)',
    fontSize: 12,
    padding: '3px 6px',
    outline: 'none',
  }
}

export default function GearUpgradeFinderPage(): React.ReactElement {
  const [characters, setCharacters] = useState<Character[]>([])
  const [viewed, setViewed] = useState('')
  const [slot, setSlot] = useState('head')

  const [data, setData] = useState<UpgradesResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [showAll, setShowAll] = useState(false)
  const [focusOnly, setFocusOnly] = useState(false)
  const [hideNoDrop, setHideNoDrop] = useState(false)

  const [weights, setWeights] = useState<UpgradeWeights | null>(null)
  const [weightsCustom, setWeightsCustom] = useState(false)
  const [showWeights, setShowWeights] = useState(false)

  const [modalItem, setModalItem] = useState<Item | null>(null)

  useEffect(() => {
    listCharacters().then((r) => setCharacters(r.characters)).catch(() => {})
  }, [])

  const selected = useMemo(
    () => characters.find((c) => c.name === viewed) ?? null,
    [characters, viewed],
  )

  // Load saved weights when the character changes.
  useEffect(() => {
    if (!selected) return
    let cancelled = false
    getCharacterUpgradeWeights(selected.id)
      .then((r) => {
        if (cancelled) return
        setWeights(r.weights)
        setWeightsCustom(r.is_custom)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [selected])

  // Fetch (and re-rank) whenever character, slot, weights, or show-all change.
  // Weights are passed inline so slider edits re-rank live without saving;
  // a short debounce keeps dragging from spamming the backend.
  useEffect(() => {
    if (!selected || !weights) return
    let cancelled = false
    const id = setTimeout(() => {
      setLoading(true)
      setError(null)
      getCharacterUpgrades(selected.id, { slot, showAll, weights, limit: 100 })
        .then((r) => {
          if (!cancelled) setData(r)
        })
        .catch((e: Error) => {
          if (!cancelled) setError(e.message)
        })
        .finally(() => {
          if (!cancelled) setLoading(false)
        })
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(id)
    }
  }, [selected, slot, showAll, weights])

  const openItem = useCallback((id: number) => {
    getItem(id).then(setModalItem).catch(() => {})
  }, [])

  const updateWeight = (key: keyof UpgradeWeights, value: number): void => {
    setWeights((w) => (w ? { ...w, [key]: value } : w))
    setWeightsCustom(true)
  }

  const saveWeights = (): void => {
    if (!selected || !weights) return
    setCharacterUpgradeWeights(selected.id, weights)
      .then(() => setWeightsCustom(true))
      .catch(() => {})
  }

  const resetWeights = (): void => {
    if (!selected) return
    resetCharacterUpgradeWeights(selected.id)
      .then((w) => {
        setWeights(w)
        setWeightsCustom(false)
      })
      .catch(() => {})
  }

  const visible = useMemo(() => {
    let list = data?.candidates ?? []
    if (focusOnly) list = list.filter((c) => c.focus_effect > 0)
    if (hideNoDrop) list = list.filter((c) => c.nodrop === 0)
    return list
  }, [data, focusOnly, hideNoDrop])

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 border-b px-6 pt-3" style={{ borderColor: 'var(--color-border)' }}>
        <div className="mb-2 flex items-center gap-2">
          <Wand2 size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Gear Upgrade Finder
          </h1>
          {selected && (
            <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
              {selected.level} {CLASS_NAMES[selected.class] ?? '—'}
            </span>
          )}
        </div>
        <CharacterSubTabs value={viewed} onChange={setViewed} />
      </div>

      {!selected ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 text-sm"
          style={{ color: 'var(--color-muted)' }}>
          <Wand2 size={28} />
          Pick a character to find gear upgrades.
        </div>
      ) : (
        <div className="flex flex-1 min-h-0 flex-col">
          {/* Slot selector */}
          <div className="shrink-0 flex flex-wrap gap-1 border-b px-6 py-2"
            style={{ borderColor: 'var(--color-border)' }}>
            {SLOTS.map((s) => (
              <button
                key={s.key}
                onClick={() => setSlot(s.key)}
                className="rounded px-2 py-1 text-xs transition-colors"
                style={{
                  backgroundColor: slot === s.key ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: slot === s.key ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                }}
              >
                {s.label}
              </button>
            ))}
          </div>

          {/* Controls + current item */}
          <div className="shrink-0 flex flex-wrap items-center gap-3 border-b px-6 py-2"
            style={{ borderColor: 'var(--color-border)' }}>
            {data && (
              <div className="flex items-center gap-2 text-xs">
                <span style={{ color: 'var(--color-muted)' }}>Current:</span>
                {data.current_items.length === 0 ? (
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    {data.has_current_gear ? '(empty)' : 'unknown'}
                  </span>
                ) : (
                  data.current_items.map((ci) => (
                    <button key={ci.id} onClick={() => openItem(ci.id)}
                      className="flex items-center gap-1 underline decoration-dotted"
                      style={{ color: 'var(--color-primary)' }}>
                      <ItemIcon id={ci.icon} name={ci.name} size={16} />
                      {ci.name}
                      {ci.focus_name && <Star size={10} style={{ color: '#eab308' }} />}
                    </button>
                  ))
                )}
              </div>
            )}
            <div className="ml-auto flex items-center gap-3 text-xs">
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}>
                <input type="checkbox" checked={showAll} onChange={(e) => setShowAll(e.target.checked)} />
                Show non-upgrades
              </label>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}>
                <input type="checkbox" checked={focusOnly} onChange={(e) => setFocusOnly(e.target.checked)} />
                Focus only
              </label>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}>
                <input type="checkbox" checked={hideNoDrop} onChange={(e) => setHideNoDrop(e.target.checked)} />
                Tradeable only
              </label>
              <button onClick={() => setShowWeights((v) => !v)}
                className="flex items-center gap-1 rounded px-2 py-1"
                style={{ border: '1px solid var(--color-border)',
                  color: showWeights ? 'var(--color-primary)' : 'var(--color-muted)' }}>
                <Sliders size={12} /> Weights{weightsCustom ? ' *' : ''}
              </button>
            </div>
          </div>

          {showWeights && weights && (
            <WeightsEditor
              weights={weights}
              isCustom={weightsCustom}
              onChange={updateWeight}
              onSave={saveWeights}
              onReset={resetWeights}
            />
          )}

          {/* Results */}
          <div className="flex-1 overflow-y-auto px-6 py-2">
            {!data?.has_current_gear && (
              <div className="mb-2 flex items-center gap-2 rounded px-3 py-1.5 text-xs"
                style={{ backgroundColor: 'var(--color-surface-2)', color: '#f59e0b' }}>
                <AlertTriangle size={12} />
                No Quarmy export found — ranking by stats only, with no current-item comparison.
                Export via Zeal to compare against what you're wearing.
              </div>
            )}
            {error ? (
              <div className="flex items-center gap-2 py-10 text-sm" style={{ color: 'var(--color-danger)' }}>
                <AlertTriangle size={16} /> {error}
              </div>
            ) : loading && !data ? (
              <div className="flex items-center justify-center gap-2 py-10 text-sm" style={{ color: 'var(--color-muted)' }}>
                <Loader2 size={16} className="animate-spin" /> Scoring items…
              </div>
            ) : visible.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-2 py-10 text-sm"
                style={{ color: 'var(--color-muted)' }}>
                <Star size={24} />
                {showAll ? 'No usable items found for this slot.'
                  : 'No upgrades found — looks like this slot is already strong.'}
              </div>
            ) : (
              <table className="w-full text-sm" style={{ borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ color: 'var(--color-muted)' }} className="text-left text-xs">
                    <th className="w-8 px-1 py-1">#</th>
                    <th className="px-2 py-1">Item</th>
                    <th className="w-16 px-2 py-1 text-right">Score</th>
                    <th className="px-2 py-1">Changes</th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((c, i) => (
                    <ResultRow key={c.id} rank={i + 1} cand={c} onOpen={openItem} />
                  ))}
                </tbody>
              </table>
            )}
            {data && (
              <div className="py-2 text-center text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {visible.length} shown · {data.considered} items considered for {data.slot_label}
              </div>
            )}
          </div>
        </div>
      )}

      <ItemDetailModal item={modalItem} open={modalItem !== null} onClose={() => setModalItem(null)} />
    </div>
  )
}

// ── Weights editor ─────────────────────────────────────────────────────────────

function WeightsEditor({
  weights, isCustom, onChange, onSave, onReset,
}: {
  weights: UpgradeWeights
  isCustom: boolean
  onChange: (key: keyof UpgradeWeights, value: number) => void
  onSave: () => void
  onReset: () => void
}): React.ReactElement {
  return (
    <div className="shrink-0 border-b px-6 py-2" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}>
      <div className="mb-2 flex items-center gap-3">
        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
          Stat weights (HP-equivalent — e.g. AC 5 means 1 AC counts as 5 HP). Edits re-rank live.
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button onClick={onSave} className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}>
            <Save size={11} /> Save
          </button>
          <button onClick={onReset} disabled={!isCustom}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ border: '1px solid var(--color-border)',
              color: isCustom ? 'var(--color-foreground)' : 'var(--color-muted)' }}>
            <RotateCcw size={11} /> Class default
          </button>
        </div>
      </div>
      <div className="grid grid-cols-5 gap-2 md:grid-cols-8">
        {STAT_KEYS.map((s) => (
          <label key={s.key} className="flex items-center gap-1 text-xs"
            style={{ color: 'var(--color-muted-foreground)' }}>
            <span className="w-8 shrink-0">{s.label}</span>
            <input
              type="number" step={0.1} min={0}
              value={weights[s.key]}
              onChange={(e) => onChange(s.key, Number(e.target.value))}
              style={{ ...inputStyle(), width: 56 }}
            />
          </label>
        ))}
      </div>
    </div>
  )
}

// ── Result row ─────────────────────────────────────────────────────────────────

function ResultRow({
  rank, cand, onOpen,
}: {
  rank: number
  cand: UpgradeCandidate
  onOpen: (id: number) => void
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [sources, setSources] = useState<ItemSources | null>(null)
  const [srcLoading, setSrcLoading] = useState(false)

  const toggle = (): void => {
    const next = !open
    setOpen(next)
    if (next && !sources && !srcLoading) {
      setSrcLoading(true)
      getItemSources(cand.id)
        .then(setSources)
        .catch(() => setSources({ drops: [], merchants: [], forage_zones: [], ground_spawns: [], tradeskills: [] }))
        .finally(() => setSrcLoading(false))
    }
  }

  return (
    <>
      <tr className="border-b align-top" style={{ borderColor: 'var(--color-border)' }}>
        <td className="px-1 py-1.5 text-xs" style={{ color: 'var(--color-muted)' }}>{rank}</td>
        <td className="px-2 py-1.5">
          <div className="flex items-center gap-2">
            <button onClick={toggle} style={{ color: 'var(--color-muted)' }}>
              {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            </button>
            <button onClick={() => onOpen(cand.id)}
              className="flex items-center gap-1.5 underline decoration-dotted"
              style={{ color: 'var(--color-primary)' }}>
              <ItemIcon id={cand.icon} name={cand.name} size={18} />
              {cand.name}
            </button>
            {cand.focus_name && (
              <span className="flex items-center gap-0.5 rounded px-1 text-[10px]"
                style={{ backgroundColor: 'rgba(234,179,8,0.15)', color: '#eab308' }}
                title={`Focus: ${cand.focus_name}`}>
                <Star size={9} /> {cand.focus_name}
              </span>
            )}
            {cand.nodrop !== 0 && (
              <span className="rounded px-1 text-[10px]"
                style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
                NO DROP
              </span>
            )}
            {cand.req_level > 0 && (
              <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
                req {cand.req_level}
              </span>
            )}
          </div>
        </td>
        <td className="px-2 py-1.5 text-right font-mono text-xs"
          style={{ color: cand.score > 0 ? '#22c55e' : 'var(--color-muted)' }}>
          {cand.score > 0 ? '+' : ''}{cand.score.toFixed(0)}
        </td>
        <td className="px-2 py-1.5">
          <DeltaChips cand={cand} />
        </td>
      </tr>
      {open && (
        <tr style={{ borderColor: 'var(--color-border)' }}>
          <td />
          <td colSpan={3} className="px-2 pb-2">
            <div className="rounded p-2 text-xs" style={{ backgroundColor: 'var(--color-surface-2)' }}>
              {srcLoading ? (
                <span className="flex items-center gap-1" style={{ color: 'var(--color-muted)' }}>
                  <Loader2 size={11} className="animate-spin" /> Loading sources…
                </span>
              ) : (
                <SourcesPanel sources={sources} />
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

// DeltaChips shows the per-stat effective changes vs the current item.
function DeltaChips({ cand }: { cand: UpgradeCandidate }): React.ReactElement {
  const chips = cand.deltas.filter((d) => d.effective !== 0 || d.cand !== d.current)
  if (chips.length === 0) {
    return <span className="text-xs" style={{ color: 'var(--color-muted)' }}>—</span>
  }
  return (
    <div className="flex flex-wrap gap-1">
      {chips.map((d) => {
        const up = d.effective > 0
        const flat = d.effective === 0
        const color = flat ? 'var(--color-muted)' : up ? '#22c55e' : '#ef4444'
        const raw = d.cand - d.current
        return (
          <span key={d.stat} className="rounded px-1 text-[10px]"
            style={{ backgroundColor: 'var(--color-surface-2)', color }}
            title={d.capped ? `${raw > 0 ? '+' : ''}${raw} on paper, but capped — only ${d.effective} counts` : undefined}>
            {STAT_LABEL[d.stat] ?? d.stat} {raw > 0 ? '+' : ''}{raw}
            {d.capped && <span style={{ color: 'var(--color-muted)' }}> (cap)</span>}
          </span>
        )
      })}
    </div>
  )
}

function SourcesPanel({ sources }: { sources: ItemSources | null }): React.ReactElement {
  if (!sources) return <span style={{ color: 'var(--color-muted)' }}>No source data.</span>
  const { drops, merchants, forage_zones, ground_spawns, tradeskills } = sources
  const empty =
    drops.length === 0 && merchants.length === 0 && forage_zones.length === 0 &&
    ground_spawns.length === 0 && tradeskills.length === 0
  if (empty) {
    return <span style={{ color: 'var(--color-muted)' }}>No known drop/vendor source (may be quest or unobtainable).</span>
  }
  return (
    <div className="space-y-1">
      {drops.length > 0 && (
        <div>
          <div className="mb-0.5 font-semibold" style={{ color: 'var(--color-muted-foreground)' }}>Drops</div>
          {drops.slice(0, 8).map((n) => <SourceNPCLink key={n.id} npc={n} showRate />)}
        </div>
      )}
      {merchants.length > 0 && (
        <div>
          <div className="mb-0.5 font-semibold" style={{ color: 'var(--color-muted-foreground)' }}>Merchants</div>
          {merchants.slice(0, 8).map((n) => <SourceNPCLink key={n.id} npc={n} />)}
        </div>
      )}
      {forage_zones.length > 0 && (
        <div style={{ color: 'var(--color-muted-foreground)' }}>
          Forage: {forage_zones.map((f) => f.zone_name).join(', ')}
        </div>
      )}
      {ground_spawns.length > 0 && (
        <div style={{ color: 'var(--color-muted-foreground)' }}>
          Ground spawn: {ground_spawns.map((g) => g.zone_name).join(', ')}
        </div>
      )}
      {tradeskills.length > 0 && (
        <div style={{ color: 'var(--color-muted-foreground)' }}>
          Tradeskill: {tradeskills.map((t) => t.recipe_name).join(', ')}
        </div>
      )}
    </div>
  )
}
