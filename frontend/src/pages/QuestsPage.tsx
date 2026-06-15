import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ScrollText, Search, ChevronRight, ChevronDown } from 'lucide-react'
import { searchQuests } from '../services/api'
import QuestWalkthrough from '../components/QuestWalkthrough'
import type { QuestSummary, ItemRef } from '../types/item'

const PAGE_SIZE = 50

function ItemLinks({ items }: { items: ItemRef[] }): React.ReactElement {
  const navigate = useNavigate()
  if (items.length === 0) return <span style={{ color: 'var(--color-muted)' }}>—</span>
  return (
    <>
      {items.map((it, i) => (
        <React.Fragment key={it.id}>
          {i > 0 && ', '}
          <button
            onClick={() => navigate(`/items?select=${it.id}`)}
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

function QuestCard({ quest }: { quest: QuestSummary }): React.ReactElement {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const hasWalkthrough = (quest.dialogue?.length ?? 0) > 0
  return (
    <div
      className="rounded-lg p-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center justify-between gap-3">
        <button
          onClick={() => hasWalkthrough && setOpen((o) => !o)}
          className="flex min-w-0 items-center gap-1.5 text-left"
          style={{ cursor: hasWalkthrough ? 'pointer' : 'default' }}
        >
          {hasWalkthrough &&
            (open ? (
              <ChevronDown size={13} style={{ color: 'var(--color-muted)' }} />
            ) : (
              <ChevronRight size={13} style={{ color: 'var(--color-muted)' }} />
            ))}
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            {quest.npc}
          </span>
        </button>
        <button
          onClick={() => navigate(`/zones?select=${quest.zone_short_name}`)}
          className="shrink-0 text-xs underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          {quest.zone_name || quest.zone_short_name}
        </button>
      </div>
      {!open && quest.rewards.length > 0 && (
        <div className="mt-1 text-xs" style={{ color: 'var(--color-muted)' }}>
          Rewards: <ItemLinks items={quest.rewards} />
        </div>
      )}
      {!open && quest.turnins.length > 0 && (
        <div className="mt-0.5 text-xs" style={{ color: 'var(--color-muted)' }}>
          Turn-ins: <ItemLinks items={quest.turnins} />
        </div>
      )}
      {open && hasWalkthrough && (
        <div className="mt-1.5 border-t pt-1.5" style={{ borderColor: 'var(--color-border)' }}>
          <QuestWalkthrough dialogue={quest.dialogue ?? []} />
        </div>
      )}
    </div>
  )
}

export default function QuestsPage(): React.ReactElement {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<QuestSummary[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Monotonic token so a slow earlier request can't clobber a newer one
  // (out-of-order responses). Each run bumps it; only the latest applies.
  const seqRef = useRef(0)

  const run = useCallback((q: string, offset: number) => {
    const more = offset > 0
    const seq = ++seqRef.current
    if (more) setLoadingMore(true)
    else setLoading(true)
    setError(null)
    searchQuests(q, { limit: PAGE_SIZE, offset })
      .then((res) => {
        if (seq !== seqRef.current) return
        setTotal(res.total)
        setResults((prev) => (more ? [...prev, ...res.items] : res.items))
      })
      .catch((e) => { if (seq === seqRef.current) setError(String(e)) })
      .finally(() => { if (seq === seqRef.current) { setLoading(false); setLoadingMore(false) } })
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => run(query, 0), 200)
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current) }
  }, [query, run])

  return (
    <div className="flex h-full flex-col" style={{ backgroundColor: 'var(--color-background)' }}>
      {/* Header */}
      <div className="shrink-0 border-b px-5 py-3" style={{ borderColor: 'var(--color-border)' }}>
        <div className="flex items-center gap-2">
          <ScrollText size={16} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Quests
          </h1>
          <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
            {total.toLocaleString()} {total === 1 ? 'quest' : 'quests'}
          </span>
        </div>
        <div
          className="mt-2 flex items-center gap-2 rounded px-2 py-1.5"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <Search size={13} style={{ color: 'var(--color-muted)' }} />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by NPC, zone, or related item…"
            className="w-full bg-transparent text-sm outline-none"
            style={{ color: 'var(--color-foreground)' }}
          />
        </div>
        <p className="mt-1.5 text-[10px] italic" style={{ color: 'var(--color-muted)' }}>
          Derived from Project Quarm quest data. Expand a quest to see its full walkthrough.
        </p>
      </div>

      {/* Results */}
      <div className="flex-1 overflow-y-auto px-5 py-3">
        {loading && results.length === 0 ? (
          <p className="text-sm" style={{ color: 'var(--color-muted)' }}>Loading…</p>
        ) : error ? (
          <p className="text-sm" style={{ color: 'var(--color-danger, #ef4444)' }}>{error}</p>
        ) : results.length === 0 ? (
          <p className="text-sm" style={{ color: 'var(--color-muted)' }}>No quests match your search.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {results.map((q, i) => (
              <QuestCard key={`${q.zone_short_name}-${q.npc}-${i}`} quest={q} />
            ))}
            {results.length < total && (
              <button
                onClick={() => run(query, results.length)}
                disabled={loadingMore}
                className="mt-1 self-center rounded px-3 py-1.5 text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                }}
              >
                {loadingMore ? 'Loading…' : `Load more (${results.length} of ${total})`}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
