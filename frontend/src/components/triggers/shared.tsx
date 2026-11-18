import React from 'react'
import type { TriggerCategory } from '../../types/trigger'

// Helpers shared by the Triggers page and the trigger editor.

export const CLASS_NAMES = [
  'Warrior', 'Cleric', 'Paladin', 'Ranger', 'Shadow Knight',
  'Druid', 'Monk', 'Bard', 'Rogue', 'Shaman',
  'Necromancer', 'Wizard', 'Magician', 'Enchanter', 'Beastlord',
]

// flattenCategoryTree returns categories in tree render order: each top-level
// category immediately followed by its children — both already sorted by
// sort_order/name within their own parent, since that's how the backend
// returns them (ListCategories sorts per-parent). Nesting is capped at one
// level (MAX_CATEGORY_DEPTH), so this two-pass grouping is exhaustive.
export function flattenCategoryTree(categories: TriggerCategory[]): TriggerCategory[] {
  const children = new Map<string, TriggerCategory[]>()
  for (const c of categories) {
    if (!c.parent_id) continue
    if (!children.has(c.parent_id)) children.set(c.parent_id, [])
    children.get(c.parent_id)!.push(c)
  }
  const out: TriggerCategory[] = []
  for (const c of categories) {
    if (c.parent_id) continue
    out.push(c)
    for (const child of children.get(c.id) ?? []) out.push(child)
  }
  return out
}

export function Toggle({
  checked,
  onChange,
}: {
  checked: boolean
  onChange: (v: boolean) => void
}): React.ReactElement {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className="relative inline-flex h-4 w-7 items-center rounded-full transition-colors shrink-0"
      style={{
        backgroundColor: checked ? 'var(--color-primary)' : 'var(--color-surface-3)',
      }}
    >
      <span
        className="inline-block h-3 w-3 rounded-full bg-white shadow transition-transform"
        style={{ transform: checked ? 'translateX(14px)' : 'translateX(2px)' }}
      />
    </button>
  )
}
