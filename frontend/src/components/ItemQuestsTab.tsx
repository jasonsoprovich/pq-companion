import React from 'react'
import { useNavigate } from 'react-router-dom'
import type { ItemQuests, ItemQuestRef } from '../types/item'

// ItemQuestsTab renders the quests that reward an item and the quests that
// consume it as a turn-in. The data comes from the Quarm quest scripts (parsed
// at build time), since EQEmu implements quests in scripts rather than the
// item DB tables — so this is often the only listed source for BiS quest gear.
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

function ItemLink({
  id,
  name,
  onNavigate,
}: {
  id: number
  name: string
  onNavigate?: () => void
}): React.ReactElement {
  const navigate = useNavigate()
  return (
    <button
      onClick={() => {
        onNavigate?.()
        navigate(`/items?select=${id}`)
      }}
      className="underline decoration-dotted"
      style={{ color: 'var(--color-primary)' }}
    >
      {name || `Item ${id}`}
    </button>
  )
}

// QuestRow shows one quest: the NPC + zone, and the related items under a label
// ("Turn in" for a reward quest, "Reward" for a turn-in quest).
function QuestRow({
  quest,
  relatedLabel,
  onNavigate,
}: {
  quest: ItemQuestRef
  relatedLabel: string
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
          {relatedLabel}:{' '}
          {quest.related_items.map((it, i) => (
            <React.Fragment key={it.id}>
              {i > 0 && ', '}
              <ItemLink id={it.id} name={it.name} onNavigate={onNavigate} />
            </React.Fragment>
          ))}
        </div>
      )}
    </div>
  )
}

function Section({
  title,
  quests,
  relatedLabel,
  onNavigate,
}: {
  title: string
  quests: ItemQuestRef[]
  relatedLabel: string
  onNavigate?: () => void
}): React.ReactElement | null {
  if (quests.length === 0) return null
  return (
    <div className="mb-3">
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      {quests.map((q, i) => (
        <QuestRow key={`${q.zone_short_name}-${q.npc}-${i}`} quest={q} relatedLabel={relatedLabel} onNavigate={onNavigate} />
      ))}
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
  const rewardedBy = quests?.rewarded_by ?? []
  const usedIn = quests?.used_in ?? []
  if (rewardedBy.length === 0 && usedIn.length === 0) {
    return (
      <div className="py-4 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
        No quests reference this item.
      </div>
    )
  }
  return (
    <div>
      <Section title="Rewarded by" quests={rewardedBy} relatedLabel="Turn in" onNavigate={onNavigate} />
      <Section title="Used in (turn-in)" quests={usedIn} relatedLabel="Reward" onNavigate={onNavigate} />
      <p className="mt-2 text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
        Quest data parsed from Project Quarm scripts; turn-in chains are approximate.
      </p>
    </div>
  )
}

// questsHaveContent reports whether a quests payload has anything to show — used
// by the detail panels to decide whether to render the Quests tab at all.
export function questsHaveContent(quests: ItemQuests | null): boolean {
  return (quests?.rewarded_by?.length ?? 0) > 0 || (quests?.used_in?.length ?? 0) > 0
}
