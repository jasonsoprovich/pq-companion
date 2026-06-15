import React from 'react'
import { useNavigate } from 'react-router-dom'
import { ExternalLink } from 'lucide-react'
import type { ItemQuests, ItemQuestRef, ItemQuestStep, ItemRef } from '../types/item'

// ItemQuestsTab renders the questline that yields an item as a step-by-step
// to-do list (turn in X → receive Y), plus the quests that consume the item as
// a turn-in. The data comes from the Quarm quest scripts (parsed at build
// time), since EQEmu implements quests in scripts rather than the item DB
// tables — so this is often the only listed source for BiS quest gear.
//
// A "full walkthrough on PQDI" deep-link covers the narrative steps (dialogue,
// drops, combines) that the script facts can't reconstruct on their own.
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

// ChainStep renders one questline step. A step with no requirements is a
// quest-start hand-out ("NPC gives X"); otherwise it's a turn-in exchange.
function ChainStep({
  step,
  index,
  onNavigate,
}: {
  step: ItemQuestStep
  index: number
  onNavigate?: () => void
}): React.ReactElement {
  const requires = step.requires ?? []
  const grants = step.grants ?? []
  return (
    <li className="flex gap-2 py-1 text-sm">
      <span className="shrink-0 tabular-nums" style={{ color: 'var(--color-muted)' }}>
        {index + 1}.
      </span>
      <span style={{ color: 'var(--color-foreground)' }}>
        {requires.length > 0 ? (
          <>
            Turn in <ItemLinks items={requires} onNavigate={onNavigate} /> to{' '}
          </>
        ) : (
          <>Receive from </>
        )}
        <span style={{ color: 'var(--color-foreground)' }}>{step.npc}</span>{' '}
        <span className="text-xs">
          (<ZoneLink shortName={step.zone_short_name} name={step.zone_name} onNavigate={onNavigate} />)
        </span>
        {grants.length > 0 && (
          <>
            {requires.length > 0 ? ' → receive ' : ': '}
            <ItemLinks items={grants} onNavigate={onNavigate} />
          </>
        )}
      </span>
    </li>
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
  itemId,
  quests,
  onNavigate,
}: {
  itemId: number
  quests: ItemQuests | null
  onNavigate?: () => void
}): React.ReactElement {
  const chain = quests?.chain ?? []
  const usedIn = quests?.used_in ?? []
  if (chain.length === 0 && usedIn.length === 0) {
    return (
      <div className="py-4 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
        No quests reference this item.
      </div>
    )
  }
  return (
    <div>
      {chain.length > 0 && (
        <div className="mb-3">
          <div
            className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
            style={{ color: 'var(--color-muted)' }}
          >
            Quest steps
          </div>
          <ol>
            {chain.map((s, i) => (
              <ChainStep key={`${s.zone_short_name}-${s.npc}-${i}`} step={s} index={i} onNavigate={onNavigate} />
            ))}
          </ol>
        </div>
      )}
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
      <div className="mt-2 flex items-center justify-between gap-2">
        <p className="text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
          Steps parsed from Project Quarm scripts; drop/combine prerequisites and
          the full walkthrough are on PQDI.
        </p>
        <a
          href={`https://www.pqdi.cc/item/${itemId}`}
          target="_blank"
          rel="noreferrer"
          className="inline-flex shrink-0 items-center gap-1 text-xs underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          Full walkthrough on PQDI
          <ExternalLink size={11} />
        </a>
      </div>
    </div>
  )
}

// questsHaveContent reports whether a quests payload has anything to show — used
// by the detail panels to decide whether to render the Quests tab at all.
export function questsHaveContent(quests: ItemQuests | null): boolean {
  return (quests?.chain?.length ?? 0) > 0 || (quests?.used_in?.length ?? 0) > 0
}
