/**
 * HelpPage — the built-in help center at /help and /help/:slug.
 *
 * - Quick Start: the quickstart article (plus the Getting Started
 *   checklist once it lands).
 * - Reference: every docs/help article with a searchable, sectioned table
 *   of contents. /help/:slug opens an article; ?a=<heading-id> scrolls to a
 *   heading inside it.
 * - Tours: replay any guided tour, or rerun the setup wizards.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { BookOpen, Compass, LifeBuoy, PlayCircle, Rocket, Search, Sparkles, LayoutGrid } from 'lucide-react'
import { usePersistentState } from '../hooks/usePersistentState'
import { HELP_ARTICLES, helpArticle, searchHelp } from '../lib/help'
import { TOURS } from '../lib/tours'
import { useGuide, useAutoTour } from '../components/guide/GuideProvider'
import HelpMarkdown from '../components/guide/HelpMarkdown'

type Tab = 'quickstart' | 'reference' | 'tours'
const TABS: readonly Tab[] = ['quickstart', 'reference', 'tours']
const DEFAULT_ARTICLE = 'getting-started'

export default function HelpPage(): React.ReactElement {
  const { slug } = useParams<{ slug?: string }>()
  const [searchParams] = useSearchParams()
  const anchor = searchParams.get('a') ?? undefined
  const navigate = useNavigate()
  const [tab, setTab] = usePersistentState<Tab>('pq-tab:help', 'quickstart', TABS)

  // A deep link to an article always lands on Reference.
  useEffect(() => {
    if (slug) setTab('reference')
  }, [slug, setTab])

  useAutoTour('help')

  const openArticle = useCallback(
    (s: string, a?: string) => {
      navigate(`/help/${s}${a ? `?a=${encodeURIComponent(a)}` : ''}`)
    },
    [navigate],
  )

  const tabBtn = (key: Tab, label: string, icon: React.ReactNode): React.ReactElement => {
    const active = tab === key
    return (
      <button
        onClick={() => setTab(key)}
        className="flex items-center gap-1.5 rounded-t px-3 py-1.5 text-xs font-medium whitespace-nowrap"
        style={{
          backgroundColor: active ? 'var(--color-surface)' : 'transparent',
          color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
          border: '1px solid',
          borderColor: active ? 'var(--color-border)' : 'transparent',
          borderBottom: active ? '1px solid var(--color-surface)' : '1px solid transparent',
        }}
      >
        {icon}
        {label}
      </button>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-3 px-4 pt-3">
        <LifeBuoy size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Help
        </span>
      </div>
      <div className="shrink-0 border-b" style={{ borderColor: 'var(--color-border)' }}>
        <div className="flex items-end gap-1 px-3 pt-2" style={{ marginBottom: -1 }} data-tour="help-tabs">
          {tabBtn('quickstart', 'Quick Start', <Rocket size={12} />)}
          {tabBtn('reference', 'Reference', <BookOpen size={12} />)}
          {tabBtn('tours', 'Tours', <Compass size={12} />)}
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col">
        {tab === 'quickstart' && <QuickStartTab onOpenArticle={openArticle} />}
        {tab === 'reference' && (
          <ReferenceTab slug={slug ?? DEFAULT_ARTICLE} anchor={anchor} onOpenArticle={openArticle} />
        )}
        {tab === 'tours' && <ToursTab />}
      </div>
    </div>
  )
}

function QuickStartTab({
  onOpenArticle,
}: {
  onOpenArticle: (slug: string, anchor?: string) => void
}): React.ReactElement {
  const qs = helpArticle('quickstart')
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-3xl px-6 py-5">
        <h1 className="mb-1 text-xl font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {qs?.title ?? 'Quick Start'}
        </h1>
        {qs ? <HelpMarkdown body={qs.body} onOpenArticle={onOpenArticle} /> : null}
      </div>
    </div>
  )
}

function ReferenceTab({
  slug,
  anchor,
  onOpenArticle,
}: {
  slug: string
  anchor?: string
  onOpenArticle: (slug: string, anchor?: string) => void
}): React.ReactElement {
  const [query, setQuery] = useState('')
  const article = helpArticle(slug) ?? helpArticle(DEFAULT_ARTICLE)
  const hits = useMemo(() => (query.trim() ? searchHelp(query) : null), [query])
  const scrollRef = useRef<HTMLDivElement>(null)

  // Scroll to the requested heading, or to the top on a new article.
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    if (!anchor) {
      el.scrollTop = 0
      return
    }
    requestAnimationFrame(() => {
      document.getElementById(anchor)?.scrollIntoView({ block: 'start' })
    })
  }, [article?.slug, anchor])

  const grouped = useMemo(() => {
    const out: { section: string; items: typeof HELP_ARTICLES }[] = []
    for (const a of HELP_ARTICLES) {
      if (a.slug === 'quickstart') continue
      let g = out.find((x) => x.section === a.section)
      if (!g) {
        g = { section: a.section, items: [] }
        out.push(g)
      }
      g.items.push(a)
    }
    // HELP_ARTICLES is already in section order.
    return out
  }, [])

  const tocLink = (s: string, title: string, sub?: string): React.ReactElement => {
    const active = s === article?.slug
    return (
      <button
        key={s}
        onClick={() => onOpenArticle(s)}
        className="block w-full rounded px-2 py-1 text-left text-xs"
        style={{
          backgroundColor: active ? 'var(--color-surface-2)' : 'transparent',
          color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
          cursor: 'pointer',
        }}
      >
        <span className="block" style={{ color: active ? 'var(--color-primary)' : 'var(--color-foreground)' }}>
          {title}
        </span>
        {sub && <span className="mt-0.5 block line-clamp-2 text-[11px]">{sub}</span>}
      </button>
    )
  }

  return (
    <div className="flex min-h-0 flex-1">
      <aside
        className="flex w-64 shrink-0 flex-col border-r"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="p-3" data-tour="help-search">
          <div className="relative">
            <Search
              size={13}
              className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2"
              style={{ color: 'var(--color-muted-foreground)' }}
            />
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search help…"
              className="w-full rounded py-1.5 pl-7 pr-2 text-xs outline-none"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            />
          </div>
        </div>
        <nav className="flex-1 overflow-y-auto px-2 pb-3">
          {hits ? (
            hits.length === 0 ? (
              <p className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                No matching articles.
              </p>
            ) : (
              hits.map((h) => tocLink(h.article.slug, h.article.title, h.snippet))
            )
          ) : (
            grouped.map((g) => (
              <div key={g.section} className="mb-3">
                <p
                  className="px-2 pb-1 text-[10px] font-semibold uppercase tracking-wider"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  {g.section}
                </p>
                {g.items.map((a) => tocLink(a.slug, a.title))}
              </div>
            ))
          )}
        </nav>
      </aside>
      <div ref={scrollRef} className="min-w-0 flex-1 overflow-y-auto">
        {article && (
          <article className="mx-auto max-w-3xl px-6 py-5">
            <p
              className="mb-1 text-[10px] font-semibold uppercase tracking-wider"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              {article.section}
            </p>
            <h1 className="mb-1 text-xl font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {article.title}
            </h1>
            <HelpMarkdown body={article.body} onOpenArticle={onOpenArticle} />
          </article>
        )}
      </div>
    </div>
  )
}

function ToursTab(): React.ReactElement {
  const { startTour, seen } = useGuide()
  const row = (
    key: string,
    icon: React.ReactNode,
    title: string,
    description: string,
    action: string,
    onClick: () => void,
    done?: boolean,
  ): React.ReactElement => (
    <div
      key={key}
      className="flex items-center gap-3 rounded-lg px-4 py-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <span style={{ color: 'var(--color-primary)' }}>{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {title}
          {done && (
            <span className="ml-2 text-[10px] font-normal" style={{ color: 'var(--color-muted-foreground)' }}>
              seen
            </span>
          )}
        </p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {description}
        </p>
      </div>
      <button
        onClick={onClick}
        className="shrink-0 rounded px-3 py-1.5 text-xs font-semibold"
        style={{ backgroundColor: 'var(--color-primary)', color: '#fff', cursor: 'pointer' }}
      >
        {action}
      </button>
    </div>
  )
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-3xl space-y-6 px-6 py-5">
        <section className="space-y-2">
          <h2 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Setup wizards
          </h2>
          {row(
            'onboarding',
            <Sparkles size={18} />,
            'Re-run setup wizard',
            'EverQuest folder, character, logging and Zeal.',
            'Open',
            () => window.dispatchEvent(new Event('pq:open-onboarding')),
          )}
          {window.electron?.overlay &&
            row(
              'overlay-setup',
              <LayoutGrid size={18} />,
              'Overlay setup',
              'Pick overlays for your class and place them on your game monitor.',
              'Open',
              () => window.dispatchEvent(new Event('pq:open-overlay-setup')),
            )}
        </section>
        <section className="space-y-2">
          <h2 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Guided tours
          </h2>
          {TOURS.map((t) =>
            row(
              t.id,
              <PlayCircle size={18} />,
              t.title,
              t.description,
              'Start tour',
              () => startTour(t.id),
              seen.includes(t.id),
            ),
          )}
        </section>
      </div>
    </div>
  )
}
