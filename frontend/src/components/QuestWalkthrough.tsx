import React from 'react'
import { useNavigate } from 'react-router-dom'
import type { QuestDialogueLine, ItemRef } from '../types/item'

// QuestWalkthrough renders a quest's dialogue branches as a step-by-step
// walkthrough: each line is a player action (say a keyword / turn in items /
// hail) with the NPC's response text and any items it grants. Item names link
// to their own pages so cross-NPC prerequisites are reachable in-app. All data
// is derived from the Project Quarm quest scripts and bundled with the app —
// nothing is fetched from an external site.

function ItemLinks({
  items,
  onNavigate,
}: {
  items: ItemRef[]
  onNavigate?: () => void
}): React.ReactElement {
  const navigate = useNavigate()
  return (
    <>
      {items.map((it, i) => (
        <React.Fragment key={`${it.id}-${i}`}>
          {i > 0 && ', '}
          <button
            onClick={() => {
              onNavigate?.()
              navigate(`/items?select=${it.id}`)
            }}
            className="underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
          >
            {it.name || `Item ${it.id}`}
          </button>
        </React.Fragment>
      ))}
    </>
  )
}

function DialogueLine({
  line,
  onNavigate,
}: {
  line: QuestDialogueLine
  onNavigate?: () => void
}): React.ReactElement {
  const triggers = line.triggers ?? []
  const turnin = line.turnin ?? []
  const grants = line.grants ?? []

  let action: React.ReactNode
  if (triggers.length > 0) {
    action = (
      <span style={{ color: 'var(--color-foreground)' }}>
        Say{' '}
        {triggers.map((t, i) => (
          <React.Fragment key={t}>
            {i > 0 && ' / '}
            <span className="font-mono" style={{ color: 'var(--color-primary)' }}>“{t}”</span>
          </React.Fragment>
        ))}
      </span>
    )
  } else if (turnin.length > 0) {
    action = (
      <span style={{ color: 'var(--color-foreground)' }}>
        Turn in <ItemLinks items={turnin} onNavigate={onNavigate} />
      </span>
    )
  } else {
    action = <span style={{ color: 'var(--color-foreground)' }}>Hail</span>
  }

  return (
    <li className="py-1 text-sm">
      <div className="flex items-baseline gap-1.5">
        <span style={{ color: 'var(--color-muted)' }}>•</span>
        <div className="min-w-0">
          {action}
          {grants.length > 0 && (
            <span style={{ color: 'var(--color-foreground)' }}>
              {' → receive '}
              <ItemLinks items={grants} onNavigate={onNavigate} />
            </span>
          )}
          {line.text && (
            <p className="mt-0.5 text-xs italic" style={{ color: 'var(--color-muted-foreground)' }}>
              {line.text}
            </p>
          )}
        </div>
      </div>
    </li>
  )
}

export default function QuestWalkthrough({
  dialogue,
  onNavigate,
}: {
  dialogue: QuestDialogueLine[]
  onNavigate?: () => void
}): React.ReactElement | null {
  if (dialogue.length === 0) return null
  return (
    <ul>
      {dialogue.map((line, i) => (
        <DialogueLine key={i} line={line} onNavigate={onNavigate} />
      ))}
    </ul>
  )
}
