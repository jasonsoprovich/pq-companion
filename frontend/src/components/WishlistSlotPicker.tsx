import React, { useEffect, useMemo, useState } from 'react'
import { X } from 'lucide-react'
import { validSlotsForItem } from '../lib/wishlistSlots'

interface WishlistSlotPickerProps {
  open: boolean
  itemName: string
  /** items.slots bitmask — used to compute which buckets are valid. */
  itemSlots: number
  /** Buckets that are currently on the wishlist for this item+character. */
  currentSlots: string[]
  /** Called with the new set when the user confirms. */
  onConfirm: (selected: string[]) => void
  onClose: () => void
}

/**
 * Modal used to pick which slot bucket(s) to wishlist an item under. When
 * `currentSlots` is non-empty the modal renders as an edit dialog (toggle
 * existing buckets on/off); when empty it's the initial add flow.
 *
 * Bypassed by the caller for single-slot items — those just commit straight
 * to the one valid bucket.
 */
export default function WishlistSlotPicker({
  open,
  itemName,
  itemSlots,
  currentSlots,
  onConfirm,
  onClose,
}: WishlistSlotPickerProps): React.ReactElement | null {
  const buckets = useMemo(() => validSlotsForItem(itemSlots), [itemSlots])
  const [selected, setSelected] = useState<Set<string>>(new Set(currentSlots))

  useEffect(() => {
    if (!open) return
    setSelected(new Set(currentSlots))
  }, [open, currentSlots])

  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  function toggle(bucket: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(bucket)) next.delete(bucket)
      else next.add(bucket)
      return next
    })
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="w-full max-w-sm rounded-lg shadow-2xl"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between border-b px-4 py-3" style={{ borderColor: 'var(--color-border)' }}>
          <div className="min-w-0">
            <div className="text-[10px] uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
              Wishlist slots
            </div>
            <div className="truncate text-sm font-semibold" style={{ color: 'var(--color-foreground)' }} title={itemName}>
              {itemName}
            </div>
          </div>
          <button onClick={onClose} title="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>
        <div className="max-h-[50vh] overflow-y-auto px-2 py-2">
          {buckets.map((b) => {
            const checked = selected.has(b)
            return (
              <label
                key={b}
                className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm"
                style={{ color: 'var(--color-foreground)' }}
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => toggle(b)}
                  className="h-4 w-4"
                />
                <span>{b}</span>
              </label>
            )
          })}
        </div>
        <div className="flex justify-end gap-2 border-t px-4 py-2.5" style={{ borderColor: 'var(--color-border)' }}>
          <button
            onClick={onClose}
            className="rounded border px-3 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface)',
              borderColor: 'var(--color-border)',
              color: 'var(--color-muted-foreground)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={() => onConfirm(Array.from(selected))}
            className="rounded border px-3 py-1 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-primary)',
              borderColor: 'var(--color-primary)',
              color: 'var(--color-surface)',
            }}
          >
            Save
          </button>
        </div>
      </div>
    </div>
  )
}
