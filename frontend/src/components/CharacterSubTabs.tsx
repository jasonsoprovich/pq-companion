import React, { useEffect, useState } from 'react'
import { listCharacters, type Character } from '../services/api'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'

interface CharacterSubTabsProps {
  /** Currently viewed character name. Empty string means "All" when allowAll. */
  value: string
  onChange: (next: string) => void
  /** When true, prepends an "All" tab whose value is the empty string. */
  allowAll?: boolean
  /** Optional right-aligned content (e.g. a refresh button). */
  rightSlot?: React.ReactNode
}

/**
 * Sub-tab strip rendered at the top of each Characters section to let the user
 * pick which character's data to view, independent of the global active
 * character. Defaults `value` to the active character when first mounted.
 */
export default function CharacterSubTabs({
  value,
  onChange,
  allowAll = false,
  rightSlot,
}: CharacterSubTabsProps): React.ReactElement {
  const { active } = useActiveCharacter()
  const [characters, setCharacters] = useState<Character[]>([])

  useEffect(() => {
    listCharacters()
      .then((res) => setCharacters(res.characters))
      .catch(() => setCharacters([]))
  }, [])

  // If the viewed value isn't valid for the current character set, fall back to
  // the active character (or All when allowed).
  useEffect(() => {
    if (characters.length === 0) return
    if (allowAll && value === '') return
    const exists = characters.some((c) => c.name === value)
    if (exists) return
    const fallback = characters.find((c) => c.name === active)?.name ?? characters[0].name
    onChange(allowAll && !active ? '' : fallback)
  }, [characters, value, active, allowAll, onChange])

  if (characters.length === 0) return <div />

  const tabs: Array<{ label: string; value: string }> = []
  if (allowAll) tabs.push({ label: 'All', value: '' })
  for (const c of characters) tabs.push({ label: c.name, value: c.name })

  return (
    <div
      className="flex items-center gap-1 border-b px-4 shrink-0 overflow-x-auto"
      style={{
        borderColor: 'var(--color-border)',
        backgroundColor: 'var(--color-surface)',
      }}
    >
      {tabs.map(({ label, value: v }) => {
        const activeTab = v === value
        const isActiveCharacter = v !== '' && v === active
        return (
          <button
            key={v || '__all'}
            onClick={() => onChange(v)}
            className="px-3 py-2 text-xs font-medium transition-colors whitespace-nowrap"
            style={{
              color: activeTab ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              borderBottom: activeTab
                ? '2px solid var(--color-primary)'
                : '2px solid transparent',
            }}
            title={isActiveCharacter ? `${label} (active character)` : label}
          >
            {label}
            {isActiveCharacter && (
              <span
                className="ml-1 text-[9px] uppercase tracking-wider"
                style={{
                  color: activeTab
                    ? 'var(--color-primary)'
                    : 'var(--color-muted)',
                }}
              >
                ●
              </span>
            )}
          </button>
        )
      })}
      {rightSlot && <div className="ml-auto pr-1">{rightSlot}</div>}
    </div>
  )
}
