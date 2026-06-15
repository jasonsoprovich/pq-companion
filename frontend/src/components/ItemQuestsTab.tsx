import React from 'react'
import { useNavigate } from 'react-router-dom'
import QuestWalkthrough from './QuestWalkthrough'
import type { ItemQuests, ItemQuestRef, ItemRef } from '../types/item'

// ItemQuestsTab shows the full quest walkthrough for the NPC(s) that reward an
// item (say a keyword / turn in items → NPC response + items), plus the quests
// that consume the item as a turn-in. All data is derived from the Project
// Quarm quest scripts and bundled with the app — nothing is fetched externally.
//
// `onNavigate` is fired before any in-app navigation so a host modal can close
// itself (mirrors the Tradeskills tab).

function ZoneLink({
  shortName,
  name,
  onNavigate,
}: {
  shortName: string
  name: string
  onNavigate?: () => void
}): React.ReactElement {
  const navigate = useNavigate()
  return (
    <button
      onClick={() => {
        onNavigate?.()
        navigate(`/zones?select=${shortName}`)
      }}
      className="underline decoration-dotted"
      style={{ color: 'var(--color-primary)' }}
    >
      {name || shortName}
    </button>
  )
}

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
        <React.Fragment key={it.id}>
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

function UsedInRow({
  quest,
  onNavigate,
}: {
  quest: ItemQuestRef
  onNavigate?: () => void
}): React.ReactElement {
  return (
    <div className="py-1 text-sm">
      <div className="flex items-center justify-between gap-3">
        <span style={{ color: 'var(--color-foreground)' }}>{quest.npc}</span>
        <span className="shrink-0 text-xs">
          <ZoneLink shortName={quest.zone_short_name} name={quest.zone_name} onNavigate={onNavigate} />
        </span>
      </div>
      {quest.related_items && quest.related_items.length > 0 && (
        <div className="mt-0.5 text-xs" style={{ color: 'var(--color-muted)' }}>
          Reward: <ItemLinks items={quest.related_items} onNavigate={onNavigate} />
        </div>
      )}
    </div>
  )
}

export default function ItemQuestsTab({
  quests,
  onNavigate,
}: {
  quests: ItemQuests | null
  onNavigate?: () => void
}): React.ReactElement {
  const walkthrough = quests?.walkthrough ?? []
  const usedIn = quests?.used_in ?? []
  if (walkthrough.length === 0 && usedIn.length === 0) {
    return (
      <div className="py-4 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
        No quests reference this item.
      </div>
    )
  }
  return (
    <div>
      {walkthrough.map((q, i) => (
        <div key={`${q.zone_short_name}-${q.npc}-${i}`} className="mb-3">
          <div className="mb-1 flex items-center justify-between gap-3">
            <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {q.npc}
            </span>
            <span className="shrink-0 text-xs">
              <ZoneLink shortName={q.zone_short_name} name={q.zone_name} onNavigate={onNavigate} />
            </span>
          </div>
          <QuestWalkthrough dialogue={q.dialogue} onNavigate={onNavigate} />
        </div>
      ))}

      {usedIn.length > 0 && (
        <div className="mb-3">
          <div
            className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Used in (turn-in)
          </div>
          {usedIn.map((q, i) => (
            <UsedInRow key={`${q.zone_short_name}-${q.npc}-${i}`} quest={q} onNavigate={onNavigate} />
          ))}
        </div>
      )}
      <p className="mt-2 text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
        Quest walkthrough derived from Project Quarm quest data. Open any item for
        its own sources and quests.
      </p>
    </div>
  )
}

// questsHaveContent reports whether a quests payload has anything to show — used
// by the detail panels to decide whether to render the Quests tab at all.
export function questsHaveContent(quests: ItemQuests | null): boolean {
  return (quests?.walkthrough?.length ?? 0) > 0 || (quests?.used_in?.length ?? 0) > 0
}
