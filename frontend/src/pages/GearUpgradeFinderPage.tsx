import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Wand2, ChevronDown, ChevronUp, ChevronRight, Star, Loader2, AlertTriangle, Sliders, RotateCcw, Save, Check, LayoutGrid, List, Target, Search, Info } from 'lucide-react'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ItemIcon } from '../components/Icon'
import { SourceNPCLink } from '../components/SourceNPCLink'
import ItemDetailModal from '../components/ItemDetailModal'
import SpellHoverCard from '../components/SpellHoverCard'
import {
  listCharacters,
  getItem,
  getItemSources,
  getCharacterUpgrades,
  getCharacterUpgradesOverview,
  getCharacterUpgradeWeights,
  setCharacterUpgradeWeights,
  resetCharacterUpgradeWeights,
  getCharacterFocusOptions,
  getCharacterPriorityFocus,
  setCharacterPriorityFocus,
  listWishlist,
  addWishlistEntries,
  deleteWishlistEntry,
  type Character,
  type UpgradeWeights,
  type UpgradeCandidate,
  type UpgradesResponse,
  type UpgradesOverviewResponse,
  type FocusOption,
} from '../services/api'
import type { Item, ItemSources } from '../types/item'
import type { WishlistEntry } from '../types/wishlist'

// Logical worn slots, keyed to the backend slot keys. `bucket` is the wishlist
// slot-bucket name (singular Shoulder/Finger) that an item from this slot is
// wishlisted under — see lib/wishlistSlots WISHLIST_SLOT_ORDER.
// The paired slots (Ear/Wrist/Finger) are split into two targets each so each
// is ranked against the item actually worn in that physical slot. Both share
// one wishlist `bucket` (singular Ear/Wrist/Finger) — see lib/wishlistSlots
// WISHLIST_SLOT_ORDER. Keys mirror the backend upgradeSlots keys.
const SLOTS: { key: string; label: string; bucket: string }[] = [
  { key: 'ear1', label: 'Ear 1', bucket: 'Ear' },
  { key: 'ear2', label: 'Ear 2', bucket: 'Ear' },
  { key: 'head', label: 'Head', bucket: 'Head' },
  { key: 'face', label: 'Face', bucket: 'Face' },
  { key: 'neck', label: 'Neck', bucket: 'Neck' },
  { key: 'shoulders', label: 'Shoulders', bucket: 'Shoulder' },
  { key: 'arms', label: 'Arms', bucket: 'Arms' },
  { key: 'back', label: 'Back', bucket: 'Back' },
  { key: 'wrist1', label: 'Wrist 1', bucket: 'Wrist' },
  { key: 'wrist2', label: 'Wrist 2', bucket: 'Wrist' },
  { key: 'hands', label: 'Hands', bucket: 'Hands' },
  { key: 'finger1', label: 'Finger 1', bucket: 'Finger' },
  { key: 'finger2', label: 'Finger 2', bucket: 'Finger' },
  { key: 'chest', label: 'Chest', bucket: 'Chest' },
  { key: 'legs', label: 'Legs', bucket: 'Legs' },
  { key: 'feet', label: 'Feet', bucket: 'Feet' },
  { key: 'waist', label: 'Waist', bucket: 'Waist' },
  { key: 'primary', label: 'Primary', bucket: 'Primary' },
  { key: 'secondary', label: 'Secondary', bucket: 'Secondary' },
  { key: 'range', label: 'Range', bucket: 'Range' },
  // No Charm slot — Project Quarm (TAKP/EQMac client) has no charm slot.
  { key: 'ammo', label: 'Ammo', bucket: 'Ammo' },
]

const BUCKET_FOR_SLOT: Record<string, string> = Object.fromEntries(
  SLOTS.map((s) => [s.key, s.bucket]),
)

// The Ammo slot never confers stats or worn effects in game (EQMacEmu's
// CalcItemBonuses stops before SLOT_AMMO), so upgrade scores are meaningless
// there. Items are still listed for convenience (throwing weapons, tradeskill
// trophies), but the score column is blanked and this note explains why.
const AMMO_NO_STATS_NOTE =
  'Items in the Ammo slot grant no stats or effects in game, so upgrade scores don’t apply here.'

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
  { key: 'atk', label: 'ATK' }, { key: 'haste', label: 'Haste' },
  { key: 'mana_regen', label: 'ManaReg' },
]

const STAT_LABEL: Record<string, string> = Object.fromEntries(
  STAT_KEYS.map((s) => [s.key, s.label]),
)

// Every editable weight, in display order: the STAT_KEYS grid plus the two
// weights that only live in the editor (DPS, focus bonus), not the delta chips.
const WEIGHT_ROWS: { key: keyof UpgradeWeights; label: string }[] = [
  ...STAT_KEYS,
  { key: 'dps', label: 'Weapon DPS' },
  { key: 'focus_bonus', label: 'Focus bonus' },
]

// Importance presets per weight. Each weight lives on its own scale (1 AC is
// worth more per point than 1 STR, weapon DPS is a big ratio multiplier, etc.),
// so the Off/Low/Med/High/Critical anchors differ by stat. Clicking a preset
// fills the number; the number stays hand-editable for fine tuning. Stats not
// listed use DEFAULT_PRESET (the per-point attribute/resist scale).
const PRESET_LABELS = ['Off', 'Low', 'Med', 'High', 'Crit']
const DEFAULT_PRESET = [0, 0.2, 0.5, 1, 2]
const WEIGHT_PRESETS: Partial<Record<keyof UpgradeWeights, number[]>> = {
  hp: [0, 0.5, 1, 2, 3],
  mana: [0, 0.5, 1, 2, 3],
  ac: [0, 2, 5, 10, 20],
  atk: [0, 0.3, 0.6, 1, 2],
  haste: [0, 5, 10, 12, 15],
  mana_regen: [0, 5, 15, 25, 40],
  dps: [0, 40, 100, 180, 250],
  focus_bonus: [0, 50, 100, 200, 400],
}

// Caveats for the conditional weights, so they don't read as broken when a
// change appears to do nothing.
const WEIGHT_NOTE: Partial<Record<keyof UpgradeWeights, string>> = {
  atk: 'soft-capped ~250 — no gain once capped; ~0 for casters',
  haste: 'best worn haste wins, level-capped',
  mana_regen: 'capped at 15 — no gain once capped; 0 for pure melee',
  dps: 'weapon slots only (Primary / Secondary / Range)',
  focus_bonus: 'only applies when a Priority focus is set',
}

function presetsFor(key: keyof UpgradeWeights): number[] {
  return WEIGHT_PRESETS[key] ?? DEFAULT_PRESET
}

// stepFor picks a sensible number-input step for each weight's scale.
function stepFor(key: keyof UpgradeWeights): number {
  if (key === 'dps' || key === 'focus_bonus') return 10
  if (key === 'mana_regen' || key === 'haste') return 1
  return 0.1
}

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
  // NO DROP gear is shown by default — you can still farm it yourself — but the
  // toggle lets you hide it when you only care about tradeable upgrades.
  const [hideNoDrop, setHideNoDrop] = useState(false)
  // Planes of Power gear is hidden by default (not yet obtainable on Quarm).
  const [showPoP, setShowPoP] = useState(false)
  // Crafted (tradeskill-made) gear is hidden by default — it's chased
  // deliberately, so it tends to be noise in a "what drops can I upgrade" list.
  const [hideCrafted, setHideCrafted] = useState(true)
  // NO RENT gear (expires on camp/zone) is always excluded server-side — it's
  // throwaway and never a real upgrade target, so there's no toggle for it.

  const [weights, setWeights] = useState<UpgradeWeights | null>(null)
  const [weightsCustom, setWeightsCustom] = useState(false)
  const [showWeights, setShowWeights] = useState(false)

  const [modalItem, setModalItem] = useState<Item | null>(null)

  const [mode, setMode] = useState<'slot' | 'overview'>('slot')
  const [overview, setOverview] = useState<UpgradesOverviewResponse | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(false)

  // The viewed character's wishlist (all buckets), for the star toggles.
  const [wishlist, setWishlist] = useState<WishlistEntry[]>([])

  // Priority focus effects (per character): boost items carrying them.
  const [focusOptions, setFocusOptions] = useState<FocusOption[]>([])
  const [priorityFocus, setPriorityFocus] = useState<number[]>([])
  const [showFocus, setShowFocus] = useState(false)
  // Bumped after a priority-focus change to force a re-rank (scoring reads the
  // stored set, so we refetch once the PUT lands).
  const [reload, setReload] = useState(0)

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
      getCharacterUpgrades(selected.id, { slot, showAll, showPoP, hideCrafted, hideNoDrop, weights, limit: 100 })
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
  }, [selected, slot, showAll, showPoP, hideCrafted, hideNoDrop, weights, reload])

  // Fetch the all-slots overview on demand (entering overview mode, or weights
  // change while in it).
  useEffect(() => {
    if (mode !== 'overview' || !selected || !weights) return
    let cancelled = false
    const id = setTimeout(() => {
      setOverviewLoading(true)
      getCharacterUpgradesOverview(selected.id, weights, showPoP, hideCrafted, hideNoDrop)
        .then((r) => {
          if (!cancelled) setOverview(r)
        })
        .catch(() => {})
        .finally(() => {
          if (!cancelled) setOverviewLoading(false)
        })
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(id)
    }
  }, [mode, selected, weights, showPoP, hideCrafted, hideNoDrop, reload])

  // Load the viewed character's wishlist for the star toggles.
  const refreshWishlist = useCallback((charID: number) => {
    listWishlist(charID).then((r) => setWishlist(r.entries)).catch(() => {})
  }, [])
  useEffect(() => {
    if (!selected) {
      setWishlist([])
      return
    }
    refreshWishlist(selected.id)
  }, [selected, refreshWishlist])

  // Load focus options + the saved priority set when the character changes.
  useEffect(() => {
    if (!selected) {
      setFocusOptions([])
      setPriorityFocus([])
      return
    }
    getCharacterFocusOptions(selected.id).then(setFocusOptions).catch(() => setFocusOptions([]))
    getCharacterPriorityFocus(selected.id).then((r) => setPriorityFocus(r.spell_ids)).catch(() => setPriorityFocus([]))
  }, [selected])

  // Toggle a focus effect in the character's priority set, persist, and re-rank.
  const togglePriorityFocus = useCallback(
    (spellID: number) => {
      if (!selected) return
      setPriorityFocus((prev) => {
        const next = prev.includes(spellID)
          ? prev.filter((x) => x !== spellID)
          : [...prev, spellID]
        setCharacterPriorityFocus(selected.id, next)
          .then(() => setReload((n) => n + 1))
          .catch(() => {})
        return next
      })
    },
    [selected],
  )

  const wishEntry = useCallback(
    (itemID: number, bucket: string): WishlistEntry | undefined =>
      wishlist.find((e) => e.item_id === itemID && e.slot_bucket === bucket),
    [wishlist],
  )

  // Toggle an item on/off the viewed character's wishlist under one slot bucket.
  const toggleWish = useCallback(
    (itemID: number, bucket: string) => {
      if (!selected) return
      const existing = wishEntry(itemID, bucket)
      const op = existing
        ? deleteWishlistEntry(selected.id, existing.id)
        : addWishlistEntries(selected.id, itemID, [bucket])
      op.then(() => refreshWishlist(selected.id)).catch(() => {})
    },
    [selected, wishEntry, refreshWishlist],
  )

  const openItem = useCallback((id: number) => {
    getItem(id).then(setModalItem).catch(() => {})
  }, [])

  const updateWeight = (key: keyof UpgradeWeights, value: number): void => {
    setWeights((w) => (w ? { ...w, [key]: value } : w))
    setWeightsCustom(true)
  }

  const saveWeights = (): Promise<void> => {
    if (!selected || !weights) return Promise.resolve()
    return setCharacterUpgradeWeights(selected.id, weights).then(() => {
      setWeightsCustom(true)
    })
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
    return list
  }, [data, focusOnly])

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
          {selected && (
            <div className="ml-auto flex items-center rounded text-xs"
              style={{ border: '1px solid var(--color-border)', overflow: 'hidden' }}>
              <button onClick={() => setMode('slot')}
                className="flex items-center gap-1 px-2 py-1"
                style={{ backgroundColor: mode === 'slot' ? 'var(--color-primary)' : 'transparent',
                  color: mode === 'slot' ? 'var(--color-background)' : 'var(--color-muted)' }}>
                <List size={12} /> By slot
              </button>
              <button onClick={() => setMode('overview')}
                className="flex items-center gap-1 px-2 py-1"
                style={{ backgroundColor: mode === 'overview' ? 'var(--color-primary)' : 'transparent',
                  color: mode === 'overview' ? 'var(--color-background)' : 'var(--color-muted)' }}>
                <LayoutGrid size={12} /> Overview
              </button>
            </div>
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
          {mode === 'slot' && (
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
          )}

          {/* Controls + current item */}
          {mode === 'slot' && (
          <div className="shrink-0 flex flex-wrap items-center gap-3 border-b px-6 py-2"
            style={{ borderColor: 'var(--color-border)' }}>
            {data && (
              <div className="flex items-center gap-2 text-xs">
                <span style={{ color: 'var(--color-muted)' }}>Current:</span>
                {(data.current_items ?? []).length === 0 ? (
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    {data.has_current_gear ? '(empty)' : 'unknown'}
                  </span>
                ) : (
                  (data.current_items ?? []).map((ci) => (
                    <button key={ci.id} onClick={() => openItem(ci.id)}
                      className="flex items-center gap-1 underline decoration-dotted"
                      style={{ color: 'var(--color-primary)' }}>
                      <ItemIcon id={ci.icon} name={ci.name} size={16} />
                      {ci.name}
                      {ci.focus_name && ci.focus_effect > 0 && (
                        <SpellHoverCard spellId={ci.focus_effect} effectsOnly clickHint={false}>
                          <span className="rounded px-1 text-[9px]"
                            style={{ backgroundColor: 'rgba(234,179,8,0.15)', color: '#eab308' }}>focus</span>
                        </SpellHoverCard>
                      )}
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
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}
                title="Hide NO DROP items (can't be traded for — must farm yourself)">
                <input type="checkbox" checked={hideNoDrop} onChange={(e) => setHideNoDrop(e.target.checked)} />
                Hide no-drop
              </label>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}
                title="Hide tradeskill-made (crafted) items">
                <input type="checkbox" checked={hideCrafted} onChange={(e) => setHideCrafted(e.target.checked)} />
                Hide crafted
              </label>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}
                title="Planes of Power gear isn't obtainable on Quarm yet">
                <input type="checkbox" checked={showPoP} onChange={(e) => setShowPoP(e.target.checked)} />
                Show PoP gear
              </label>
              <button onClick={() => setShowFocus((v) => !v)}
                className="flex items-center gap-1 rounded px-2 py-1"
                style={{ border: '1px solid var(--color-border)',
                  color: showFocus || priorityFocus.length ? 'var(--color-primary)' : 'var(--color-muted)' }}>
                <Target size={12} /> Focus{priorityFocus.length ? ` (${priorityFocus.length})` : ''}
              </button>
              <button onClick={() => setShowWeights((v) => !v)}
                className="flex items-center gap-1 rounded px-2 py-1"
                style={{ border: '1px solid var(--color-border)',
                  color: showWeights ? 'var(--color-primary)' : 'var(--color-muted)' }}>
                <Sliders size={12} /> Weights{weightsCustom ? ' *' : ''}
              </button>
            </div>
          </div>
          )}

          {/* Weights/focus toggles when in overview mode (no per-slot bar) */}
          {mode === 'overview' && (
            <div className="shrink-0 flex items-center justify-end gap-2 border-b px-6 py-2 text-xs"
              style={{ borderColor: 'var(--color-border)' }}>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}
                title="Hide NO DROP items (can't be traded for — must farm yourself)">
                <input type="checkbox" checked={hideNoDrop} onChange={(e) => setHideNoDrop(e.target.checked)} />
                Hide no-drop
              </label>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}
                title="Hide tradeskill-made (crafted) items">
                <input type="checkbox" checked={hideCrafted} onChange={(e) => setHideCrafted(e.target.checked)} />
                Hide crafted
              </label>
              <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}
                title="Planes of Power gear isn't obtainable on Quarm yet">
                <input type="checkbox" checked={showPoP} onChange={(e) => setShowPoP(e.target.checked)} />
                Show PoP gear
              </label>
              <button onClick={() => setShowFocus((v) => !v)}
                className="flex items-center gap-1 rounded px-2 py-1"
                style={{ border: '1px solid var(--color-border)',
                  color: showFocus || priorityFocus.length ? 'var(--color-primary)' : 'var(--color-muted)' }}>
                <Target size={12} /> Focus{priorityFocus.length ? ` (${priorityFocus.length})` : ''}
              </button>
              <button onClick={() => setShowWeights((v) => !v)}
                className="flex items-center gap-1 rounded px-2 py-1"
                style={{ border: '1px solid var(--color-border)',
                  color: showWeights ? 'var(--color-primary)' : 'var(--color-muted)' }}>
                <Sliders size={12} /> Weights{weightsCustom ? ' *' : ''}
              </button>
            </div>
          )}

          {showFocus && (
            <FocusPanel
              options={focusOptions}
              selected={priorityFocus}
              onToggle={togglePriorityFocus}
            />
          )}

          {showWeights && weights && (
            <WeightsEditor
              weights={weights}
              isCustom={weightsCustom}
              onChange={updateWeight}
              onSave={saveWeights}
              onReset={resetWeights}
            />
          )}

          {mode === 'overview' ? (
            <OverviewView
              overview={overview}
              loading={overviewLoading}
              onOpen={openItem}
              onPickSlot={(key) => { setSlot(key); setMode('slot') }}
              isWishlisted={(id, bucket) => wishEntry(id, bucket) !== undefined}
              onToggleWish={toggleWish}
            />
          ) : (
          /* Results */
          <div className="flex-1 overflow-y-auto px-6 py-2">
            {!data?.has_current_gear && (
              <div className="mb-2 flex items-center gap-2 rounded px-3 py-1.5 text-xs"
                style={{ backgroundColor: 'var(--color-surface-2)', color: '#f59e0b' }}>
                <AlertTriangle size={12} />
                No Quarmy export found — ranking by stats only, with no current-item comparison.
                Export via Zeal to compare against what you're wearing.
              </div>
            )}
            {slot === 'ammo' && (
              <div className="mb-2 flex items-center gap-2 rounded px-3 py-1.5 text-xs"
                style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
                <Info size={12} />
                {AMMO_NO_STATS_NOTE}
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
                    <ResultRow key={c.id} rank={i + 1} cand={c} onOpen={openItem}
                      wishlisted={wishEntry(c.id, BUCKET_FOR_SLOT[slot]) !== undefined}
                      onStar={() => toggleWish(c.id, BUCKET_FOR_SLOT[slot])}
                      hideScore={slot === 'ammo'} />
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
          )}
        </div>
      )}

      <ItemDetailModal item={modalItem} open={modalItem !== null} onClose={() => setModalItem(null)} />
    </div>
  )
}

// ── Weights editor ─────────────────────────────────────────────────────────────

// WeightRow is one stat's importance presets + editable number + caveat.
function WeightRow({
  weightKey, label, value, onChange,
}: {
  weightKey: keyof UpgradeWeights
  label: string
  value: number
  onChange: (key: keyof UpgradeWeights, value: number) => void
}): React.ReactElement {
  const presets = presetsFor(weightKey)
  const note = WEIGHT_NOTE[weightKey]
  return (
    <div className="flex items-center gap-2 py-0.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
      <span className="w-20 shrink-0">{label}</span>
      <div className="flex shrink-0 gap-0.5">
        {presets.map((v, i) => {
          const active = Math.abs(value - v) < 1e-9
          return (
            <button key={i} onClick={() => onChange(weightKey, v)}
              className="rounded px-1.5 py-0.5 text-[10px]"
              title={`${PRESET_LABELS[i]} = ${v}`}
              style={{
                backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                color: active ? 'var(--color-background)' : 'var(--color-muted)',
              }}>
              {PRESET_LABELS[i]}
            </button>
          )
        })}
      </div>
      <input type="number" step={stepFor(weightKey)} min={0}
        value={value}
        onChange={(e) => onChange(weightKey, Number(e.target.value))}
        style={{ ...inputStyle(), width: 60 }} />
      {note && (
        <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>{note}</span>
      )}
    </div>
  )
}

function WeightsEditor({
  weights, isCustom, onChange, onSave, onReset,
}: {
  weights: UpgradeWeights
  isCustom: boolean
  onChange: (key: keyof UpgradeWeights, value: number) => void
  onSave: () => Promise<void>
  onReset: () => void
}): React.ReactElement {
  const [saved, setSaved] = useState(false)
  const handleSave = (): void => {
    onSave()
      .then(() => {
        setSaved(true)
        setTimeout(() => setSaved(false), 2000)
      })
      .catch(() => {})
  }
  return (
    <div className="shrink-0 border-b px-6 py-2" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}>
      <div className="mb-2 flex items-start gap-3">
        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
          How important is each stat to this character? Higher = weighed more
          when ranking upgrades. Values are independent (not a % of any total),
          and a point of one stat isn&apos;t a point of another — they&apos;re just
          relative importance. Edits re-rank live; Save keeps them for this character.
        </span>
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <button onClick={handleSave} className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ backgroundColor: saved ? '#22c55e' : 'var(--color-primary)', color: 'var(--color-background)' }}>
            {saved ? <Check size={11} /> : <Save size={11} />} {saved ? 'Saved' : 'Save'}
          </button>
          <button onClick={onReset} disabled={!isCustom}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ border: '1px solid var(--color-border)',
              color: isCustom ? 'var(--color-foreground)' : 'var(--color-muted)' }}>
            <RotateCcw size={11} /> Class default
          </button>
        </div>
      </div>
      <div className="flex flex-col" style={{ maxHeight: '42vh', overflowY: 'auto' }}>
        {WEIGHT_ROWS.map((s) => (
          <WeightRow key={s.key} weightKey={s.key} label={s.label}
            value={weights[s.key]} onChange={onChange} />
        ))}
      </div>
    </div>
  )
}

// ── Priority focus picker ──────────────────────────────────────────────────────

function FocusPanel({
  options, selected, onToggle,
}: {
  options: FocusOption[]
  selected: number[]
  onToggle: (spellID: number) => void
}): React.ReactElement {
  const [filter, setFilter] = useState('')
  const sel = new Set(selected)
  const q = filter.trim().toLowerCase()
  const shown = q ? options.filter((o) => o.name.toLowerCase().includes(q)) : options
  return (
    <div className="shrink-0 border-b px-6 py-2"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Target size={12} style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Priority focus effects
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          Click any below to prioritize it — an upgrade carrying one you don't
          already wear is boosted and flagged. This is the list of focuses found
          on gear your class can use ({options.length}); type to filter it, not to
          search for new ones.
        </span>
        {options.length > 0 && (
          <div className="ml-auto" style={{ position: 'relative' }}>
            <Search size={11} style={{ position: 'absolute', left: 7, top: '50%',
              transform: 'translateY(-50%)', color: 'var(--color-muted)', pointerEvents: 'none' }} />
            <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Filter the list…"
              style={{ ...inputStyle(), paddingLeft: 22, width: 180 }} />
          </div>
        )}
      </div>
      {options.length === 0 ? (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          No focus effects are carried by gear this class can use, so there's
          nothing to prioritize here.
        </p>
      ) : shown.length === 0 ? (
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          None of this class's {options.length} focus effects match “{filter.trim()}”.
          Clear the filter to see them all — the list only covers focuses your
          class's gear actually carries.
        </p>
      ) : (
        <div className="flex flex-wrap gap-1" style={{ maxHeight: 150, overflowY: 'auto' }}>
          {shown.map((o) => {
            const on = sel.has(o.spell_id)
            return (
              <button key={o.spell_id} onClick={() => onToggle(o.spell_id)}
                className="flex items-center gap-1 rounded px-2 py-0.5 text-[11px]"
                title={`${o.count} item${o.count === 1 ? '' : 's'} carry this focus`}
                style={{ border: '1px solid var(--color-border)',
                  backgroundColor: on ? 'rgba(234,179,8,0.18)' : 'var(--color-surface-2)',
                  color: on ? '#eab308' : 'var(--color-muted-foreground)' }}>
                {on && <Check size={10} />}
                {o.name}
                <span style={{ color: 'var(--color-muted)' }}>·{o.count}</span>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

// EffectPills renders a candidate's focus / click / proc spell badges. The
// focus pill carries no star icon — stars are reserved for the wishlist toggle,
// and a star on a focus pill was being mistaken for one. Colors differ per
// effect type (focus = amber, click = sky, proc = violet) so they read apart.
// Each pill is a link into the Spells explorer, with a SpellHoverCard quick
// view on hover (EFFECTS only for focus — worn foci have no cast/targeting).
function EffectPills({ cand }: { cand: UpgradeCandidate }): React.ReactElement | null {
  const navigate = useNavigate()
  const pill = (
    key: string, spellId: number, text: string, effectsOnly: boolean,
    style: React.CSSProperties,
  ): React.ReactElement => (
    <SpellHoverCard key={key} spellId={spellId} effectsOnly={effectsOnly}>
      <button
        onClick={() => navigate(`/spells?select=${spellId}`)}
        className="shrink-0 whitespace-nowrap rounded px-1 text-[10px]"
        style={style}>
        {text}
      </button>
    </SpellHoverCard>
  )

  const pills: React.ReactElement[] = []
  if (cand.focus_name && cand.focus_effect > 0) {
    pills.push(pill('focus', cand.focus_effect,
      cand.priority_focus ? `Priority: ${cand.focus_name}` : cand.focus_name, true, {
        backgroundColor: cand.priority_focus ? '#eab308' : 'rgba(234,179,8,0.15)',
        color: cand.priority_focus ? '#1a1a1a' : '#eab308',
        fontWeight: cand.priority_focus ? 600 : 400,
      }))
  }
  if (cand.click_name && cand.click_effect > 0) {
    pills.push(pill('click', cand.click_effect, `Click: ${cand.click_name}`, false,
      { backgroundColor: 'rgba(56,189,248,0.16)', color: '#38bdf8' }))
  }
  if (cand.proc_name && cand.proc_effect > 0) {
    pills.push(pill('proc', cand.proc_effect, `Proc: ${cand.proc_name}`, false,
      { backgroundColor: 'rgba(167,139,250,0.18)', color: '#a78bfa' }))
  }
  if (pills.length === 0) return null
  return <>{pills}</>
}

// ── Result row ─────────────────────────────────────────────────────────────────

function ResultRow({
  rank, cand, onOpen, wishlisted, onStar, hideScore,
}: {
  rank: number
  cand: UpgradeCandidate
  onOpen: (id: number) => void
  wishlisted: boolean
  onStar: () => void
  /** Ammo slot: stats don't apply in game, so the score is meaningless. */
  hideScore?: boolean
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
          <div className="flex flex-wrap items-center gap-2">
            <button onClick={toggle} style={{ color: 'var(--color-muted)' }}>
              {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            </button>
            <WishStar on={wishlisted} onClick={onStar} />
            <button onClick={() => onOpen(cand.id)}
              className="flex shrink-0 items-center gap-1.5 text-left underline decoration-dotted"
              style={{ color: 'var(--color-primary)' }}>
              <ItemIcon id={cand.icon} name={cand.name} size={18} />
              {cand.name}
            </button>
            <EffectPills cand={cand} />
            {cand.nodrop === 0 && (
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
          style={{ color: !hideScore && cand.score > 0 ? '#22c55e' : 'var(--color-muted)' }}>
          {hideScore ? (
            <span title={AMMO_NO_STATS_NOTE}>—</span>
          ) : (
            <>{cand.score > 0 ? '+' : ''}{cand.score.toFixed(0)}</>
          )}
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
        // The dps delta carries weapon ratio (damage/delay) at x100 in the int
        // fields, so render it as a ratio with two decimals; everything else is
        // a plain integer stat delta.
        if (d.stat === 'dps') {
          const r = (d.cand - d.current) / 100
          return (
            <span key={d.stat} className="rounded px-1 text-[10px]"
              style={{ backgroundColor: 'var(--color-surface-2)', color }}
              title={`weapon ratio ${(d.current / 100).toFixed(2)} → ${(d.cand / 100).toFixed(2)}`}>
              Ratio {r > 0 ? '+' : ''}{r.toFixed(2)}
            </span>
          )
        }
        if (d.stat === 'haste') {
          // Worn haste is best-of/capped: show the EFFECTIVE % gained, not the
          // item's raw haste (which can differ from what actually helps).
          return (
            <span key={d.stat} className="rounded px-1 text-[10px]"
              style={{ backgroundColor: 'var(--color-surface-2)', color }}
              title={d.capped ? 'clipped at the melee haste cap' : "worn haste (best-of — only your highest item counts)"}>
              Haste {d.effective > 0 ? '+' : ''}{d.effective}%
              {d.capped && <span style={{ color: 'var(--color-muted)' }}> (cap)</span>}
            </span>
          )
        }
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
  // The backend marshals empty source categories as JSON null (Go nil slices),
  // so each array can be null even when sources itself is set — default them.
  const drops = sources.drops ?? []
  const merchants = sources.merchants ?? []
  const forage_zones = sources.forage_zones ?? []
  const ground_spawns = sources.ground_spawns ?? []
  const tradeskills = sources.tradeskills ?? []
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

// ── Wishlist star ──────────────────────────────────────────────────────────────

// WishStar is a slot-scoped wishlist toggle: it adds/removes the item for the
// *viewed* character under the current slot's bucket. (The shared
// WishlistStarButton targets the global active character, which isn't the
// character being viewed here, so the finder manages its own toggle.)
function WishStar({ on, onClick }: { on: boolean; onClick: () => void }): React.ReactElement {
  return (
    <button onClick={onClick} className="shrink-0"
      title={on ? 'Remove from wishlist' : 'Add to wishlist'}>
      <Star size={15} fill={on ? '#eab308' : 'none'}
        style={{ color: on ? '#eab308' : 'var(--color-muted)' }} />
    </button>
  )
}

// ── All-slots overview ─────────────────────────────────────────────────────────

function OverviewView({
  overview, loading, onOpen, onPickSlot, isWishlisted, onToggleWish,
}: {
  overview: UpgradesOverviewResponse | null
  loading: boolean
  onOpen: (id: number) => void
  onPickSlot: (key: string) => void
  isWishlisted: (id: number, bucket: string) => boolean
  onToggleWish: (id: number, bucket: string) => void
}): React.ReactElement {
  // Column sorting. `null` col = backend's natural slot order. Rows whose best
  // upgrade is null always sink to the bottom regardless of direction.
  const [sortCol, setSortCol] = useState<'slot' | 'best' | 'score' | null>(null)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')

  const handleSort = useCallback((col: 'slot' | 'best' | 'score') => {
    if (sortCol === col) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortCol(col)
      // Score is the headline use case — start high-to-low.
      setSortDir(col === 'score' ? 'desc' : 'asc')
    }
  }, [sortCol])

  const sortedSlots = useMemo(() => {
    const slots = overview?.slots ?? []
    if (!sortCol) return slots
    const dir = sortDir === 'asc' ? 1 : -1
    const indexed = slots.map((s, i) => ({ s, i }))
    indexed.sort((a, b) => {
      let cmp = 0
      if (sortCol === 'slot') {
        cmp = a.s.slot_label.localeCompare(b.s.slot_label)
      } else if (sortCol === 'best') {
        const an = a.s.best?.name ?? null
        const bn = b.s.best?.name ?? null
        if (an === null && bn === null) cmp = 0
        else if (an === null) return 1
        else if (bn === null) return -1
        else cmp = an.localeCompare(bn)
      } else {
        const av = a.s.best?.score ?? null
        const bv = b.s.best?.score ?? null
        if (av === null && bv === null) cmp = 0
        else if (av === null) return 1
        else if (bv === null) return -1
        else cmp = av - bv
      }
      if (cmp === 0) return a.i - b.i // stable: fall back to natural order
      return cmp * dir
    })
    return indexed.map((x) => x.s)
  }, [overview, sortCol, sortDir])

  if (loading && !overview) {
    return (
      <div className="flex flex-1 items-center justify-center gap-2 py-10 text-sm"
        style={{ color: 'var(--color-muted)' }}>
        <Loader2 size={16} className="animate-spin" /> Scanning all slots…
      </div>
    )
  }
  if (!overview) return <div className="flex-1" />

  const arrow = (col: 'slot' | 'best' | 'score'): React.ReactNode => {
    if (sortCol !== col) return null
    return sortDir === 'asc'
      ? <ChevronUp size={12} className="inline" />
      : <ChevronDown size={12} className="inline" />
  }

  return (
    <div className="flex-1 overflow-y-auto px-6 py-2">
      {!overview.has_current_gear && (
        <div className="mb-2 flex items-center gap-2 rounded px-3 py-1.5 text-xs"
          style={{ backgroundColor: 'var(--color-surface-2)', color: '#f59e0b' }}>
          <AlertTriangle size={12} />
          No Quarmy export found — ranking by stats only. Export via Zeal to compare against your gear.
        </div>
      )}
      <table className="w-full text-sm" style={{ borderCollapse: 'collapse' }}>
        <thead>
          <tr className="text-left text-xs" style={{ color: 'var(--color-muted)' }}>
            <th className="w-24 px-2 py-1">
              <button onClick={() => handleSort('slot')}
                className="inline-flex items-center gap-0.5 hover:text-[var(--color-foreground)]">
                Slot {arrow('slot')}
              </button>
            </th>
            <th className="px-2 py-1">Current</th>
            <th className="px-2 py-1">
              <button onClick={() => handleSort('best')}
                className="inline-flex items-center gap-0.5 hover:text-[var(--color-foreground)]">
                Best upgrade {arrow('best')}
              </button>
            </th>
            <th className="w-16 px-2 py-1 text-right">
              <button onClick={() => handleSort('score')}
                className="inline-flex items-center gap-0.5 hover:text-[var(--color-foreground)]">
                Score {arrow('score')}
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          {sortedSlots.map((s) => {
            const bucket = BUCKET_FOR_SLOT[s.slot]
            const best = s.best
            return (
              <tr key={s.slot} className="border-b align-top" style={{ borderColor: 'var(--color-border)' }}>
                <td className="px-2 py-1.5">
                  <button onClick={() => onPickSlot(s.slot)} className="underline decoration-dotted"
                    style={{ color: 'var(--color-primary)' }}>{s.slot_label}</button>
                </td>
                <td className="px-2 py-1.5">
                  {(s.current_items ?? []).length === 0 ? (
                    <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                      {overview.has_current_gear ? '(empty)' : '—'}
                    </span>
                  ) : (
                    <div className="flex flex-col gap-0.5">
                      {(s.current_items ?? []).map((ci) => (
                        <button key={ci.id} onClick={() => onOpen(ci.id)}
                          className="flex items-center gap-1 text-xs underline decoration-dotted"
                          style={{ color: 'var(--color-muted-foreground)' }}>
                          <ItemIcon id={ci.icon} name={ci.name} size={14} /> {ci.name}
                          {ci.focus_name && ci.focus_effect > 0 && (
                            <SpellHoverCard spellId={ci.focus_effect} effectsOnly clickHint={false}>
                              <span className="rounded px-1 text-[9px]"
                                style={{ backgroundColor: 'rgba(234,179,8,0.15)', color: '#eab308' }}>focus</span>
                            </SpellHoverCard>
                          )}
                        </button>
                      ))}
                    </div>
                  )}
                </td>
                <td className="px-2 py-1.5">
                  {best ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <WishStar on={isWishlisted(best.id, bucket)} onClick={() => onToggleWish(best.id, bucket)} />
                      <button onClick={() => onOpen(best.id)}
                        className="flex items-center gap-1.5 underline decoration-dotted"
                        style={{ color: 'var(--color-primary)' }}>
                        <ItemIcon id={best.icon} name={best.name} size={16} /> {best.name}
                      </button>
                      <EffectPills cand={best} />
                      {best.nodrop === 0 && (
                        <span className="rounded px-1 text-[10px]"
                          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
                          NO DROP
                        </span>
                      )}
                    </div>
                  ) : (
                    <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
                      {s.considered === 0 ? 'no usable items' : 'no upgrade'}
                    </span>
                  )}
                </td>
                <td className="px-2 py-1.5 text-right font-mono text-xs"
                  style={{ color: best && s.slot !== 'ammo' ? '#22c55e' : 'var(--color-muted)' }}>
                  {s.slot === 'ammo' ? (
                    <span title={AMMO_NO_STATS_NOTE}>—</span>
                  ) : best ? `+${best.score.toFixed(0)}` : ''}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
