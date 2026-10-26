import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  ChevronDown, ChevronRight, RotateCcw, Save, Search, Star, Trash2, X,
} from 'lucide-react'
import {
  getMockBase, resolveMockSpell, listMockLoadouts, createMockLoadout,
  updateMockLoadout, deleteMockLoadout, listWishlist, addWishlistEntries,
  deleteWishlistEntry, getItem,
  type MockBaseResponse, type MockBaseSlot, type MockItemBrief, type MockLoadout,
  type MockOverrides, type MockResolveResponse, type SpellModifierResolution,
} from '../services/api'
import type { WishlistEntry } from '../types/wishlist'
import type { Item } from '../types/item'
import { usePersistentState } from '../hooks/usePersistentState'
import { spaLabel, formatDuration } from '../lib/spellModHelpers'
import ItemSearchModal from './ItemSearchModal'
import { ConfirmModal } from './ConfirmModal'
import { ItemIcon } from './Icon'

// MockLoadoutPanel is the "what-if" half of the Spell Modifiers Test
// Resolution panel: swap worn items / AA ranks, see the picked spell resolve
// against the mock next to the actual gear, save named loadouts, and push the
// mocked items onto the character's wishlist. Mocks never change the
// character's data — the rest of Character Info keeps showing real gear.

const EMPTY_OVERRIDES: MockOverrides = { gear: {}, aas: {} }

function sameOverrides(a: MockOverrides, b: MockOverrides): boolean {
  const eq = (x: Record<string, number>, y: Record<string, number>): boolean => {
    const kx = Object.keys(x)
    if (kx.length !== Object.keys(y).length) return false
    return kx.every((k) => k in y && y[k] === x[k])
  }
  return eq(a.gear, b.gear) && eq(a.aas, b.aas)
}

function briefFromItem(it: Item): MockItemBrief {
  return {
    id: it.id, name: it.name, icon: it.icon, slots: it.slots,
    focus_id: it.focus_effect > 0 ? it.focus_effect : undefined,
    focus_name: it.focus_effect > 0 ? it.focus_name : undefined,
  }
}

function overrideCount(o: MockOverrides): number {
  return Object.keys(o.gear).length + Object.keys(o.aas).length
}

interface MockLoadoutPanelProps {
  characterID: number
  // Spell picked in the Test Resolution search; null until one is chosen.
  spellID: number | null
}

export default function MockLoadoutPanel({
  characterID, spellID,
}: MockLoadoutPanelProps): React.ReactElement {
  const navigate = useNavigate()
  const [expanded, setExpanded] = usePersistentState<'open' | 'closed'>(
    'spellmods.whatif', 'closed', ['open', 'closed'],
  )
  const [base, setBase] = useState<MockBaseResponse | null>(null)
  const [baseError, setBaseError] = useState<string | null>(null)
  const [overrides, setOverrides] = useState<MockOverrides>(EMPTY_OVERRIDES)
  // Briefs for items picked into the mock (the base only knows worn items).
  const [mockItems, setMockItems] = useState<Record<number, MockItemBrief>>({})
  const [pickerSlot, setPickerSlot] = useState<MockBaseSlot | null>(null)

  const [loadouts, setLoadouts] = useState<MockLoadout[]>([])
  const [activeID, setActiveID] = useState<number | null>(null)
  const [nameDraft, setNameDraft] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const [wishlist, setWishlist] = useState<WishlistEntry[]>([])
  const [compare, setCompare] = useState<MockResolveResponse | null>(null)
  const [compareError, setCompareError] = useState<string | null>(null)
  const [comparing, setComparing] = useState(false)

  const open = expanded === 'open'

  // Reset everything when the viewed character changes.
  useEffect(() => {
    setOverrides(EMPTY_OVERRIDES)
    setActiveID(null)
    setNameDraft(null)
    setCompare(null)
    setBase(null)
    setLoadouts([])
    setWishlist([])
  }, [characterID])

  useEffect(() => {
    if (!open) return
    let cancelled = false
    setBaseError(null)
    getMockBase(characterID)
      .then((b) => { if (!cancelled) setBase(b) })
      .catch(() => { if (!cancelled) setBaseError('Could not load equipped gear.') })
    listMockLoadouts(characterID)
      .then((r) => { if (!cancelled) setLoadouts(r.loadouts ?? []) })
      .catch(() => {})
    listWishlist(characterID)
      .then((r) => { if (!cancelled) setWishlist(r.entries ?? []) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [characterID, open])

  // A saved loadout may reference items we haven't fetched briefs for yet.
  useEffect(() => {
    const missing = Object.values(overrides.gear)
      .filter((id) => id > 0 && !mockItems[id])
    if (missing.length === 0) return
    let cancelled = false
    Promise.all(missing.map((id) => getItem(id).catch(() => null))).then((items) => {
      if (cancelled) return
      setMockItems((prev) => {
        const next = { ...prev }
        items.forEach((it, i) => {
          // A failed lookup still gets a placeholder so it isn't refetched.
          const id = missing[i]
          next[id] = it ? briefFromItem(it) : { id, name: `Item ${id}`, icon: 0, slots: 0 }
        })
        return next
      })
    })
    return () => { cancelled = true }
  }, [overrides.gear, mockItems])

  // Re-resolve the picked spell against the mock whenever either changes.
  useEffect(() => {
    if (!open || spellID === null || overrideCount(overrides) === 0) {
      setCompare(null)
      setCompareError(null)
      return
    }
    let cancelled = false
    setComparing(true)
    const t = setTimeout(() => {
      resolveMockSpell(characterID, spellID, overrides)
        .then((r) => { if (!cancelled) { setCompare(r); setCompareError(null) } })
        .catch((err) => {
          if (!cancelled) {
            setCompare(null)
            setCompareError(err instanceof Error ? err.message : 'Could not resolve the mock.')
          }
        })
        .finally(() => { if (!cancelled) setComparing(false) })
    }, 250)
    return () => { cancelled = true; clearTimeout(t) }
  }, [characterID, spellID, overrides, open])

  const actualAARank = useCallback(
    (id: number) => base?.aas.find((a) => a.id === id)?.rank ?? 0,
    [base],
  )

  const setSlot = (slot: MockBaseSlot, itemID: number | null): void => {
    setOverrides((o) => {
      const gear = { ...o.gear }
      const actual = slot.item?.id ?? 0
      // null = revert; picking the actually worn item is also a revert.
      if (itemID === null || itemID === actual) delete gear[slot.key]
      else gear[slot.key] = itemID
      return { ...o, gear }
    })
  }

  const setAARank = (id: number, rank: number): void => {
    setOverrides((o) => {
      const aas = { ...o.aas }
      if (rank === actualAARank(id)) delete aas[String(id)]
      else aas[String(id)] = rank
      return { ...o, aas }
    })
  }

  const handlePick = (item: Item): void => {
    if (!pickerSlot) return
    setMockItems((prev) => ({ ...prev, [item.id]: briefFromItem(item) }))
    setSlot(pickerSlot, item.id)
    setPickerSlot(null)
  }

  // ── Saved loadouts ──────────────────────────────────────────────────────

  const active = loadouts.find((l) => l.id === activeID) ?? null
  const dirty = active
    ? !sameOverrides(overrides, { gear: active.gear ?? {}, aas: active.aas ?? {} })
    : overrideCount(overrides) > 0

  const loadLoadout = (id: number | null): void => {
    setSaveError(null)
    setNameDraft(null)
    const l = loadouts.find((x) => x.id === id)
    setActiveID(l ? l.id : null)
    setOverrides(l ? { gear: { ...(l.gear ?? {}) }, aas: { ...(l.aas ?? {}) } } : EMPTY_OVERRIDES)
  }

  const replaceLoadout = (l: MockLoadout): void => {
    setLoadouts((prev) => [...prev.filter((x) => x.id !== l.id), l]
      .sort((a, b) => a.name.localeCompare(b.name)))
    setActiveID(l.id)
  }

  const saveActive = (): void => {
    if (!active) return
    setSaveError(null)
    updateMockLoadout(characterID, active.id, active.name, overrides)
      .then(replaceLoadout)
      .catch((err) => setSaveError(err instanceof Error ? err.message : 'Save failed.'))
  }

  const saveAsNew = (): void => {
    const name = (nameDraft ?? '').trim()
    if (!name) return
    setSaveError(null)
    createMockLoadout(characterID, name, overrides)
      .then((l) => { replaceLoadout(l); setNameDraft(null) })
      .catch((err) => setSaveError(err instanceof Error ? err.message : 'Save failed.'))
  }

  const removeActive = (): void => {
    setConfirmDelete(false)
    if (!active) return
    deleteMockLoadout(characterID, active.id)
      .then(() => {
        setLoadouts((prev) => prev.filter((x) => x.id !== active.id))
        setActiveID(null)
      })
      .catch((err) => setSaveError(err instanceof Error ? err.message : 'Delete failed.'))
  }

  // ── Wishlist ────────────────────────────────────────────────────────────

  const wishEntry = useCallback(
    (itemID: number, bucket: string) =>
      wishlist.find((e) => e.item_id === itemID && e.slot_bucket === bucket),
    [wishlist],
  )

  const refreshWishlist = (): Promise<void> =>
    listWishlist(characterID).then((r) => setWishlist(r.entries ?? [])).catch(() => {})

  const toggleWish = (itemID: number, bucket: string): void => {
    const existing = wishEntry(itemID, bucket)
    const op = existing
      ? deleteWishlistEntry(characterID, existing.id)
      : addWishlistEntries(characterID, itemID, [bucket])
    op.then(refreshWishlist).catch(() => {})
  }

  // Mocked items not already wishlisted under their slot's bucket.
  const unwishedMocked = useMemo(() => {
    if (!base) return []
    const seen = new Set<string>()
    const out: { itemID: number; bucket: string }[] = []
    for (const s of base.slots) {
      const id = overrides.gear[s.key]
      if (!id) continue
      const k = `${id}:${s.bucket}`
      if (seen.has(k) || wishEntry(id, s.bucket)) continue
      seen.add(k)
      out.push({ itemID: id, bucket: s.bucket })
    }
    return out
  }, [base, overrides.gear, wishEntry])

  const wishAll = (): void => {
    Promise.all(unwishedMocked.map((w) => addWishlistEntries(characterID, w.itemID, [w.bucket])))
      .then(refreshWishlist)
      .catch(() => {})
  }

  // ── Render ──────────────────────────────────────────────────────────────

  const n = overrideCount(overrides)

  return (
    <div className="mt-4 rounded" style={{ border: '1px solid var(--color-border)' }}>
      <button
        onClick={() => setExpanded(open ? 'closed' : 'open')}
        className="flex w-full items-center justify-between px-3 py-2 text-left"
        style={{ backgroundColor: 'transparent', cursor: 'pointer' }}
      >
        <span className="flex items-center gap-1.5 text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          What-if loadout
          {n > 0 && (
            <span
              className="rounded px-1.5 py-0.5 text-[10px] font-medium"
              style={{ backgroundColor: 'var(--color-surface-3)', color: 'var(--color-primary)' }}
            >
              {n} change{n === 1 ? '' : 's'}
            </span>
          )}
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Try other gear or AA ranks without equipping them
        </span>
      </button>

      {open && (
        <div className="space-y-3 px-3 pb-3">
          {baseError && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{baseError}</p>}
          {base && !base.has_quarmy && (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              No Quarmy export for this character yet, so every slot starts empty and no AAs are trained.
            </p>
          )}

          {/* Saved loadouts bar */}
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <select
              value={activeID ?? ''}
              onChange={(e) => loadLoadout(e.target.value ? Number(e.target.value) : null)}
              className="rounded px-2 py-1"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            >
              <option value="">{loadouts.length ? 'Unsaved loadout' : 'No saved loadouts'}</option>
              {loadouts.map((l) => (
                <option key={l.id} value={l.id}>{l.name}</option>
              ))}
            </select>
            {active && dirty && (
              <BarButton onClick={saveActive} title={`Overwrite "${active.name}"`}>
                <Save size={12} /> Save
              </BarButton>
            )}
            {nameDraft === null ? (
              <BarButton onClick={() => setNameDraft(active ? `${active.name} copy` : '')} disabled={n === 0}>
                <Save size={12} /> Save as…
              </BarButton>
            ) : (
              <span className="flex items-center gap-1">
                <input
                  autoFocus
                  value={nameDraft}
                  maxLength={64}
                  onChange={(e) => setNameDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') saveAsNew()
                    if (e.key === 'Escape') setNameDraft(null)
                  }}
                  placeholder="Loadout name"
                  className="w-40 rounded px-2 py-1"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                  }}
                />
                <BarButton onClick={saveAsNew} disabled={!nameDraft.trim()}>Save</BarButton>
                <BarButton onClick={() => setNameDraft(null)}>Cancel</BarButton>
              </span>
            )}
            {active && (
              <BarButton onClick={() => setConfirmDelete(true)} title={`Delete "${active.name}"`}>
                <Trash2 size={12} />
              </BarButton>
            )}
            <BarButton
              onClick={() => { setOverrides(EMPTY_OVERRIDES); setActiveID(null) }}
              disabled={n === 0 && !active}
              title="Back to the character's actual gear and AAs"
            >
              <RotateCcw size={12} /> Actual gear
            </BarButton>
            {unwishedMocked.length > 0 && (
              <BarButton onClick={wishAll} title="Add every mocked item to this character's wishlist">
                <Star size={12} /> Wishlist all ({unwishedMocked.length})
              </BarButton>
            )}
          </div>
          {saveError && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{saveError}</p>}

          {/* Comparison */}
          {spellID === null ? (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Pick a spell above to compare it against this loadout.
            </p>
          ) : n === 0 ? (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Swap an item or change an AA rank below to compare against your actual gear.
            </p>
          ) : compareError ? (
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{compareError}</p>
          ) : compare ? (
            <CompareCard actual={compare.actual} mock={compare.mock} stale={comparing} />
          ) : (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Resolving…</p>
          )}

          {/* Gear slots */}
          {base && (
            <div className="grid grid-cols-1 gap-1.5 lg:grid-cols-2">
              {base.slots.map((s) => {
                const mockedID = s.key in overrides.gear ? overrides.gear[s.key] : undefined
                const shown = mockedID === undefined
                  ? s.item
                  : mockedID === 0 ? null : (mockItems[mockedID] ?? { id: mockedID, name: `Item ${mockedID}`, icon: 0, slots: 0 })
                return (
                  <SlotRow
                    key={s.key}
                    slot={s}
                    shown={shown}
                    mocked={mockedID !== undefined}
                    wishlisted={shown ? !!wishEntry(shown.id, s.bucket) : false}
                    onOpenItem={(id) => navigate(`/items?select=${id}`)}
                    onPick={() => setPickerSlot(s)}
                    onEmpty={() => setSlot(s, 0)}
                    onRevert={() => setSlot(s, null)}
                    onToggleWish={shown ? () => toggleWish(shown.id, s.bucket) : undefined}
                  />
                )
              })}
            </div>
          )}

          {/* AA ranks */}
          {base && base.aas.length > 0 && (
            <div>
              <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                AAs that affect spell modifiers
              </p>
              <div className="flex flex-wrap gap-3">
                {base.aas.map((aa) => {
                  const key = String(aa.id)
                  const rank = key in overrides.aas ? overrides.aas[key] : aa.rank
                  const changed = key in overrides.aas
                  return (
                    <label key={aa.id} className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
                      {aa.name}
                      <select
                        value={rank}
                        onChange={(e) => setAARank(aa.id, Number(e.target.value))}
                        className="rounded px-1.5 py-0.5"
                        style={{
                          backgroundColor: 'var(--color-surface-2)',
                          border: `1px solid ${changed ? 'var(--color-primary)' : 'var(--color-border)'}`,
                          color: 'var(--color-foreground)',
                        }}
                      >
                        {Array.from({ length: aa.max_rank + 1 }, (_, r) => (
                          <option key={r} value={r}>
                            {r === 0 ? 'Untrained' : `Rank ${r}`}{r === aa.rank ? ' (actual)' : ''}
                          </option>
                        ))}
                      </select>
                    </label>
                  )
                })}
              </div>
            </div>
          )}

          <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            Only an item's focus effect counts here: Duration and Cast Time resolve per spell, the same as the actual
            Test Resolution above. Stats and other worn effects on mocked items are ignored.
          </p>
        </div>
      )}

      <ItemSearchModal
        open={pickerSlot !== null}
        title={pickerSlot ? `Try an item in ${pickerSlot.label}` : undefined}
        slotFilter={pickerSlot?.mask}
        onSelect={handlePick}
        onClose={() => setPickerSlot(null)}
      />
      {confirmDelete && active && (
        <ConfirmModal
          title="Delete loadout"
          message={<>Delete the saved loadout <strong>{active.name}</strong>? Your current changes stay on screen.</>}
          confirmLabel="Delete"
          tone="danger"
          onConfirm={removeActive}
          onCancel={() => setConfirmDelete(false)}
        />
      )}
    </div>
  )
}

function BarButton({
  onClick, disabled, title, children,
}: {
  onClick: () => void
  disabled?: boolean
  title?: string
  children: React.ReactNode
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      className="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        color: disabled ? 'var(--color-muted)' : 'var(--color-foreground)',
        cursor: disabled ? 'default' : 'pointer',
      }}
    >
      {children}
    </button>
  )
}

interface SlotRowProps {
  slot: MockBaseSlot
  shown: MockItemBrief | null
  mocked: boolean
  wishlisted: boolean
  onOpenItem: (id: number) => void
  onPick: () => void
  onEmpty: () => void
  onRevert: () => void
  onToggleWish?: () => void
}

function SlotRow({
  slot, shown, mocked, wishlisted, onOpenItem, onPick, onEmpty, onRevert, onToggleWish,
}: SlotRowProps): React.ReactElement {
  const iconBtn = 'rounded p-1'
  return (
    <div
      className="flex items-center gap-2 rounded px-2 py-1.5 text-xs"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: `1px solid ${mocked ? 'var(--color-primary)' : 'var(--color-border)'}`,
      }}
    >
      <span className="w-16 shrink-0 text-[11px]" style={{ color: 'var(--color-muted)' }}>{slot.label}</span>
      <span className="flex min-w-0 flex-1 items-center gap-1.5">
        {shown ? (
          <>
            <ItemIcon id={shown.icon} size={18} name={shown.name} />
            <span className="min-w-0">
              <button
                onClick={() => onOpenItem(shown.id)}
                className="truncate underline decoration-dotted"
                style={{ color: mocked ? 'var(--color-primary)' : 'var(--color-foreground)' }}
              >
                {shown.name}
              </button>
              {(shown.focus_name || (mocked && slot.item)) && (
                <span className="block truncate text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                  {[shown.focus_name, mocked && slot.item ? `was ${slot.item.name}` : '']
                    .filter(Boolean).join(' · ')}
                </span>
              )}
            </span>
          </>
        ) : (
          <span style={{ color: 'var(--color-muted)' }}>
            {mocked ? 'Empty (mock)' : 'Empty'}
            {mocked && slot.item && (
              <span className="ml-1 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                was {slot.item.name}
              </span>
            )}
          </span>
        )}
      </span>
      <span className="flex shrink-0 items-center gap-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
        <button onClick={onPick} className={iconBtn} title="Try a different item in this slot" style={{ cursor: 'pointer' }}>
          <Search size={13} />
        </button>
        {shown && (
          <button onClick={onEmpty} className={iconBtn} title="Leave this slot empty" style={{ cursor: 'pointer' }}>
            <X size={13} />
          </button>
        )}
        {mocked && (
          <button onClick={onRevert} className={iconBtn} title="Back to the actually worn item" style={{ cursor: 'pointer' }}>
            <RotateCcw size={13} />
          </button>
        )}
        {mocked && onToggleWish && (
          <button
            onClick={onToggleWish}
            className={iconBtn}
            title={wishlisted ? `Remove from wishlist (${slot.bucket})` : `Add to wishlist (${slot.bucket})`}
            style={{ cursor: 'pointer', color: wishlisted ? 'var(--color-warning)' : undefined }}
          >
            <Star size={13} fill={wishlisted ? 'currentColor' : 'none'} />
          </button>
        )}
      </span>
    </div>
  )
}

function durationText(r: SpellModifierResolution): string {
  if (r.permanent_illusion) return 'until death/dispel'
  return r.base_duration_sec > 0 ? formatDuration(r.extended_duration_sec) : 'instant'
}

function CompareCard({
  actual, mock, stale,
}: {
  actual: SpellModifierResolution
  mock: SpellModifierResolution
  stale: boolean
}): React.ReactElement {
  const durDelta = mock.extended_duration_sec - actual.extended_duration_sec
  const ctDelta = mock.cast_time_percent - actual.cast_time_percent
  const applied = mock.applied ?? []
  const deltaColor = (d: number): string =>
    d > 0 ? 'var(--color-success)' : d < 0 ? 'var(--color-danger)' : 'var(--color-muted)'
  const cell = 'font-mono'
  return (
    <div
      className="rounded p-3 text-xs"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        opacity: stale ? 0.6 : 1,
      }}
    >
      <p className="mb-2 font-semibold" style={{ color: 'var(--color-foreground)' }}>{mock.spell_name}</p>
      <div className="grid grid-cols-4 gap-x-3 gap-y-1">
        <span />
        <span className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>Actual</span>
        <span className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>What-if</span>
        <span className="text-[10px] uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>Change</span>

        <span style={{ color: 'var(--color-muted-foreground)' }}>Duration</span>
        <span className={cell} style={{ color: 'var(--color-foreground)' }}>
          {durationText(actual)}
          {actual.duration_percent > 0 && !actual.permanent_illusion ? ` (+${actual.duration_percent}%)` : ''}
        </span>
        <span className={cell} style={{ color: 'var(--color-primary)' }}>
          {durationText(mock)}
          {mock.duration_percent > 0 && !mock.permanent_illusion ? ` (+${mock.duration_percent}%)` : ''}
        </span>
        <span className={cell} style={{ color: deltaColor(durDelta) }}>
          {durDelta === 0 ? '—' : `${durDelta > 0 ? '+' : '−'}${formatDuration(Math.abs(durDelta))}`}
        </span>

        <span style={{ color: 'var(--color-muted-foreground)' }}>Cast time</span>
        <span className={cell} style={{ color: 'var(--color-foreground)' }}>−{actual.cast_time_percent}%</span>
        <span className={cell} style={{ color: 'var(--color-primary)' }}>−{mock.cast_time_percent}%</span>
        <span className={cell} style={{ color: deltaColor(ctDelta) }}>
          {ctDelta === 0 ? '—' : `${ctDelta > 0 ? '−' : '+'}${Math.abs(ctDelta)} pts`}
        </span>
      </div>
      <p className="mt-2 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
        {applied.length === 0
          ? 'No focus from the what-if loadout applies to this spell.'
          : 'What-if applies: ' + applied
            .map((m) => `${m.source === 'item' ? m.source_item_name : `${m.source_aa_name} ${m.source_aa_rank}`}`
              + ` (${m.spa === 127 ? '−' : '+'}${m.percent}% ${spaLabel(m.spa)})`)
            .join(', ')}
      </p>
    </div>
  )
}
