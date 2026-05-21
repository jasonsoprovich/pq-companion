import React, { useCallback, useEffect, useState } from 'react'
import { Star } from 'lucide-react'
import {
  listCharacters,
  listWishlist,
  addWishlistEntries,
  deleteWishlistEntry,
} from '../services/api'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { isMultiSlotItem, validSlotsForItem } from '../lib/wishlistSlots'
import WishlistSlotPicker from './WishlistSlotPicker'
import type { Item } from '../types/item'

interface WishlistStarButtonProps {
  item: Item | null
  /** Pixel size of the star icon. Defaults to 18. */
  size?: number
}

/**
 * Self-contained star toggle for adding an item to the active character's
 * wishlist. Outline = not starred for active char; filled = ≥1 slot row.
 *
 * Single-slot or non-equippable items toggle directly; multi-slot items
 * open a small picker prefilled with the current state.
 *
 * Disabled (40% opacity) when no active character is set or the lookup
 * doesn't resolve to a stored character.
 */
export default function WishlistStarButton({
  item,
  size = 18,
}: WishlistStarButtonProps): React.ReactElement {
  const { active } = useActiveCharacter()
  const [activeCharID, setActiveCharID] = useState<number | null>(null)
  const [starredSlots, setStarredSlots] = useState<string[]>([])
  const [pickerOpen, setPickerOpen] = useState(false)

  // Resolve active character name → stored id.
  useEffect(() => {
    if (!active) {
      setActiveCharID(null)
      return
    }
    listCharacters()
      .then((r) => {
        const c = r.characters.find((c) => c.name.toLowerCase() === active.toLowerCase())
        setActiveCharID(c?.id ?? null)
      })
      .catch(() => setActiveCharID(null))
  }, [active])

  const refresh = useCallback(() => {
    if (!activeCharID || !item) {
      setStarredSlots([])
      return
    }
    listWishlist(activeCharID)
      .then((r) => {
        const slots = r.entries.filter((e) => e.item_id === item.id).map((e) => e.slot_bucket)
        setStarredSlots(slots)
      })
      .catch(() => setStarredSlots([]))
  }, [activeCharID, item])

  useEffect(() => {
    refresh()
  }, [refresh])

  if (!item) return <span />

  const isStarred = starredSlots.length > 0
  const disabled = !activeCharID
  const title = disabled
    ? 'Set an active character to wishlist items'
    : isStarred
      ? `Wishlisted in ${starredSlots.join(', ')} — click to edit`
      : 'Add to wishlist'

  function handleClick() {
    if (!item || !activeCharID) return
    // Currently starred → click removes every entry for this item, regardless
    // of how many slot buckets it spans. Fine-grained per-slot management lives
    // on the wishlist page itself (drag/delete).
    if (isStarred) {
      listWishlist(activeCharID)
        .then((r) =>
          Promise.all(
            r.entries
              .filter((e) => e.item_id === item.id)
              .map((e) => deleteWishlistEntry(activeCharID, e.id)),
          ),
        )
        .then(refresh)
        .catch(() => undefined)
      return
    }
    // Not yet starred → adding. Multi-slot items need the user to pick which
    // slot(s) they care about; single-slot / non-equippable items add straight
    // to their canonical bucket.
    if (isMultiSlotItem(item.slots)) {
      setPickerOpen(true)
      return
    }
    const slots = validSlotsForItem(item.slots)
    addWishlistEntries(activeCharID, item.id, slots).then(refresh).catch(() => undefined)
  }

  function handlePickerConfirm(selected: string[]) {
    if (!item || !activeCharID) return
    setPickerOpen(false)
    const toAdd = selected.filter((s) => !starredSlots.includes(s))
    const toRemove = starredSlots.filter((s) => !selected.includes(s))
    const work: Promise<unknown>[] = []
    if (toAdd.length > 0) work.push(addWishlistEntries(activeCharID, item.id, toAdd))
    if (toRemove.length > 0) {
      work.push(
        listWishlist(activeCharID).then((r) =>
          Promise.all(
            toRemove
              .map((slot) => r.entries.find((e) => e.item_id === item.id && e.slot_bucket === slot))
              .filter((e): e is NonNullable<typeof e> => !!e)
              .map((e) => deleteWishlistEntry(activeCharID, e.id)),
          ),
        ),
      )
    }
    Promise.all(work).then(refresh).catch(() => undefined)
  }

  return (
    <>
      <button
        onClick={handleClick}
        disabled={disabled}
        title={title}
        className="rounded p-0.5 disabled:opacity-40"
      >
        <Star
          size={size}
          style={{ color: isStarred ? 'var(--color-primary)' : 'var(--color-muted)' }}
          fill={isStarred ? 'currentColor' : 'none'}
        />
      </button>
      <WishlistSlotPicker
        open={pickerOpen}
        itemName={item.name}
        itemSlots={item.slots}
        currentSlots={starredSlots}
        onConfirm={handlePickerConfirm}
        onClose={() => setPickerOpen(false)}
      />
    </>
  )
}
