import React, { useEffect, useRef, useState } from 'react'
import { Plus, X } from 'lucide-react'
import type { Item } from '../types/item'
import {
  activeStatRows,
  combatRowLabel,
  deltaColor,
  deltaLabel,
  hasCombatRow,
  weaponRatio,
} from '../lib/itemCompare'
import { getEquippedInSlots, getItem, listCharacters, type Character } from '../services/api'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { ItemIcon } from './Icon'
import ItemSearchModal from './ItemSearchModal'

const MAX_COMPARE_ITEMS = 4

type EquippedState = 'idle' | 'loading' | 'loaded' | 'no-export' | 'error'

interface ItemCompareModalProps {
  open: boolean
  /** Item the user was viewing when they opened Compare — seeds column 1. */
  initialItem: Item | null
  onClose: () => void
}

export default function ItemCompareModal({ open, initialItem, onClose }: ItemCompareModalProps): React.ReactElement | null {
  const { active } = useActiveCharacter()
  const [items, setItems] = useState<Item[]>([])
  const [pickerOpen, setPickerOpen] = useState(false)

  const [characters, setCharacters] = useState<Character[]>([])
  const [selectedCharID, setSelectedCharID] = useState<number | null>(null)
  const [baselineItem, setBaselineItem] = useState<Item | null>(null)
  const [baselineLabel, setBaselineLabel] = useState<string | null>(null)
  const [equippedState, setEquippedState] = useState<EquippedState>('idle')
  const autoSelected = useRef(false)

  // Reset to just the seed item each time the modal is (re)opened for a new item.
  const [seededFor, setSeededFor] = useState<number | null>(null)
  if (open && initialItem && seededFor !== initialItem.id) {
    setSeededFor(initialItem.id)
    setItems([initialItem])
  }

  // Load the character list once, defaulting to the app's active character.
  useEffect(() => {
    if (!open) return
    let cancelled = false
    listCharacters().then((r) => {
      if (cancelled) return
      setCharacters(r.characters)
      if (!autoSelected.current) {
        autoSelected.current = true
        const match = r.characters.find((c) => c.name === active)
        if (match) setSelectedCharID(match.id)
      }
    }).catch(() => { if (!cancelled) setCharacters([]) })
    return () => { cancelled = true }
  }, [open, active])

  // Compare-vs-equipped baseline is keyed off the ORIGINAL viewed item's
  // slot(s) — the item the user opened Compare from — not every column
  // currently in the table, since candidates are expected to share its slot.
  useEffect(() => {
    if (!open || !selectedCharID || !initialItem) {
      setBaselineItem(null)
      setBaselineLabel(null)
      setEquippedState('idle')
      return
    }
    let cancelled = false
    setEquippedState('loading')
    getEquippedInSlots(selectedCharID, initialItem.slots)
      .then((resp) => {
        if (cancelled) return
        if (!resp.has_gear) {
          setEquippedState('no-export')
          setBaselineItem(null)
          setBaselineLabel(null)
          return
        }
        const entry = resp.slots.find((s) => s.item_ids.length > 0)
        if (!entry) {
          setEquippedState('loaded')
          setBaselineItem(null)
          setBaselineLabel(null)
          return
        }
        return getItem(entry.item_ids[0]).then((worn) => {
          if (cancelled) return
          setBaselineItem(worn)
          setBaselineLabel(entry.slot_label)
          setEquippedState('loaded')
        })
      })
      .catch(() => { if (!cancelled) setEquippedState('error') })
    return () => { cancelled = true }
  }, [open, selectedCharID, initialItem])

  useEffect(() => {
    if (!open) return
    function onKeyDown(e: KeyboardEvent) {
      // The nested ItemSearchModal owns Escape while it's open (its own
      // handler runs first and closes the picker, not this modal).
      if (e.key === 'Escape' && !pickerOpen) onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, pickerOpen, onClose])

  if (!open) return null

  function addItem(item: Item) {
    setItems((cur) => (cur.some((i) => i.id === item.id) ? cur : [...cur, item]))
    setPickerOpen(false)
  }

  function removeItem(id: number) {
    setItems((cur) => cur.filter((i) => i.id !== id))
  }

  const rows = activeStatRows(baselineItem ? [...items, baselineItem] : items)
  const showCombatRow = items.some(hasCombatRow) || (baselineItem != null && hasCombatRow(baselineItem))

  // "Best" highlighting only considers the candidate columns, not the
  // equipped baseline — the baseline is a reference point, not a contender.
  function bestValue(get: (item: Item) => number): number {
    return items.reduce((max, item) => Math.max(max, get(item)), -Infinity)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      // This modal can be opened from inside ItemDetailModal's own backdrop, so
      // a click here must not bubble up and close that parent modal too.
      onClick={(e) => { e.stopPropagation(); onClose() }}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-5xl flex-col rounded-lg shadow-2xl"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex shrink-0 items-center justify-between gap-2 border-b px-5 py-3" style={{ borderColor: 'var(--color-border)' }}>
          <h2 className="text-sm font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted-foreground)' }}>
            Compare Items
          </h2>
          <button onClick={onClose} title="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-5 py-2 text-xs" style={{ borderColor: 'var(--color-border)' }}>
          <span style={{ color: 'var(--color-muted-foreground)' }}>Compare vs equipped:</span>
          <select
            value={selectedCharID ?? ''}
            onChange={(e) => setSelectedCharID(e.target.value ? Number(e.target.value) : null)}
            className="rounded px-2 py-1 text-xs outline-none"
            style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
          >
            <option value="">— None —</option>
            {characters.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          {equippedState === 'loading' && (
            <span style={{ color: 'var(--color-muted)' }}>Loading…</span>
          )}
          {equippedState === 'no-export' && (
            <span style={{ color: 'var(--color-muted)' }}>
              No equipped data for this character — export inventory via Zeal on logout.
            </span>
          )}
          {equippedState === 'loaded' && selectedCharID && !baselineItem && (
            <span style={{ color: 'var(--color-muted)' }}>Nothing equipped in a matching slot.</span>
          )}
          {equippedState === 'error' && (
            <span style={{ color: 'var(--color-muted)' }}>Couldn't load equipped gear — is the EQ path configured in Settings?</span>
          )}
        </div>

        <div className="flex-1 overflow-auto px-5 py-4">
          {items.length === 0 && !baselineItem ? (
            <p className="py-8 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
              No items selected.
            </p>
          ) : (
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr>
                  <th className="w-32 shrink-0 px-2 pb-2 text-left text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
                    Stat
                  </th>
                  {baselineItem && (
                    <th className="min-w-[10rem] px-2 pb-2 text-left align-top">
                      <div className="flex items-center gap-2">
                        <ItemIcon id={baselineItem.icon} name={baselineItem.name} size={24} />
                        <div className="min-w-0">
                          <div className="truncate text-[9px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                            Equipped — {baselineLabel}
                          </div>
                          <div className="truncate text-xs font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
                            {baselineItem.name}
                          </div>
                        </div>
                      </div>
                    </th>
                  )}
                  {items.map((item) => (
                    <th key={item.id} className="min-w-[10rem] px-2 pb-2 text-left align-top">
                      <div className="flex items-start justify-between gap-1">
                        <div className="flex items-center gap-2">
                          <ItemIcon id={item.icon} name={item.name} size={24} />
                          <span className="text-xs font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
                            {item.name}
                          </span>
                        </div>
                        <button onClick={() => removeItem(item.id)} title="Remove from compare">
                          <X size={12} style={{ color: 'var(--color-muted)' }} />
                        </button>
                      </div>
                    </th>
                  ))}
                  {items.length < MAX_COMPARE_ITEMS && (
                    <th className="px-2 pb-2 align-top">
                      <button
                        onClick={() => setPickerOpen(true)}
                        className="flex items-center gap-1 rounded border px-2 py-1 text-[11px] font-medium"
                        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
                      >
                        <Plus size={12} /> Add Item
                      </button>
                    </th>
                  )}
                </tr>
              </thead>
              <tbody>
                {showCombatRow && (
                  <tr style={{ borderTop: '1px solid var(--color-border)' }}>
                    <td className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Damage / Delay</td>
                    {baselineItem && (
                      <td className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                        {hasCombatRow(baselineItem) ? combatRowLabel(baselineItem) : '—'}
                      </td>
                    )}
                    {items.map((item) => {
                      const ratio = weaponRatio(item)
                      const isBest = hasCombatRow(item) && ratio === bestValue(weaponRatio) && ratio > 0
                      return (
                        <td
                          key={item.id}
                          className="px-2 py-1 text-xs"
                          style={{ color: isBest ? 'var(--color-primary)' : 'var(--color-foreground)', fontWeight: isBest ? 600 : 400 }}
                        >
                          {hasCombatRow(item) ? combatRowLabel(item) : '—'}
                        </td>
                      )
                    })}
                    {items.length < MAX_COMPARE_ITEMS && <td />}
                  </tr>
                )}
                {rows.map((row) => {
                  const best = bestValue(row.get)
                  const baselineValue = baselineItem ? row.get(baselineItem) : null
                  return (
                    <tr key={row.key} style={{ borderTop: '1px solid var(--color-border)' }}>
                      <td className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{row.label}</td>
                      {baselineItem && (
                        <td className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                          {baselineValue !== 0 ? baselineValue : '—'}
                        </td>
                      )}
                      {items.map((item) => {
                        const value = row.get(item)
                        const isBest = value === best && value !== 0
                        const delta = baselineValue !== null ? value - baselineValue : null
                        return (
                          <td
                            key={item.id}
                            className="px-2 py-1 text-xs"
                            style={{ color: isBest ? 'var(--color-primary)' : 'var(--color-foreground)', fontWeight: isBest ? 600 : 400 }}
                          >
                            {value !== 0 ? value : '—'}
                            {delta !== null && delta !== 0 && (
                              <span className="ml-1" style={{ color: deltaColor(delta), fontWeight: 600 }}>
                                {deltaLabel(delta)}
                              </span>
                            )}
                          </td>
                        )
                      })}
                      {items.length < MAX_COMPARE_ITEMS && <td />}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <ItemSearchModal
        open={pickerOpen}
        title="Add item to compare"
        onSelect={addItem}
        onClose={() => setPickerOpen(false)}
      />
    </div>
  )
}
