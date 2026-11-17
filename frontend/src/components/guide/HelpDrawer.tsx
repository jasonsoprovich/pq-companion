/**
 * HelpDrawer — a right-side panel with the help article for the current
 * page (matched by the article's `routes` frontmatter), a search box for
 * anything else, and a "Take the tour" link when the page has a tour.
 * Opened from the title bar's ? button or F1.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { ArrowLeft, ExternalLink, PlayCircle, Search, X } from 'lucide-react'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import { articleForPath, helpArticle, searchHelp, type HelpArticle } from '../../lib/help'
import { toursForPath } from '../../lib/tours'
import { useGuide } from './GuideProvider'
import HelpMarkdown from './HelpMarkdown'

const FALLBACK = ['quickstart', 'getting-started', 'troubleshooting']

export default function HelpDrawer({ onClose }: { onClose: () => void }): React.ReactElement {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { startTour } = useGuide()
  useEscapeToClose(onClose, true)

  const pageArticle = useMemo(() => articleForPath(pathname), [pathname])
  // An article picked from a link or search inside the drawer; cleared when
  // the page changes so the drawer follows navigation.
  const [picked, setPicked] = useState<HelpArticle | null>(null)
  const [query, setQuery] = useState('')
  useEffect(() => {
    setPicked(null)
    setQuery('')
  }, [pathname])

  const shown = picked ?? pageArticle
  const hits = useMemo(() => (query.trim() ? searchHelp(query, 12) : null), [query])
  const tours = toursForPath(pathname)
  const bodyRef = useRef<HTMLDivElement>(null)

  const open = useCallback((slug: string, anchor?: string) => {
    const a = helpArticle(slug)
    if (!a) return
    setPicked(a)
    setQuery('')
    requestAnimationFrame(() => {
      if (anchor) document.getElementById(anchor)?.scrollIntoView({ block: 'start' })
      else if (bodyRef.current) bodyRef.current.scrollTop = 0
    })
  }, [])

  return (
    <aside
      className="fixed bottom-0 right-0 z-40 flex w-[420px] max-w-full flex-col shadow-2xl"
      style={{
        top: window.electron?.window.nativeFrame ? 0 : 36,
        backgroundColor: 'var(--color-surface)',
        borderLeft: '1px solid var(--color-border)',
      }}
      aria-label="Help"
    >
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        {picked && pageArticle && picked !== pageArticle && (
          <button
            onClick={() => setPicked(null)}
            className="rounded p-1"
            style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
            title="Back to this page's help"
          >
            <ArrowLeft size={14} />
          </button>
        )}
        <div className="relative flex-1">
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
        <button
          onClick={onClose}
          className="rounded p-1"
          style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
          title="Close (Esc)"
        >
          <X size={16} />
        </button>
      </div>

      <div ref={bodyRef} className="flex-1 overflow-y-auto px-4 py-3">
        {hits ? (
          hits.length === 0 ? (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              No matching articles.
            </p>
          ) : (
            <div className="space-y-1">
              {hits.map((h) => (
                <button
                  key={h.article.slug}
                  onClick={() => open(h.article.slug)}
                  className="block w-full rounded px-2 py-1.5 text-left hover:bg-white/5"
                  style={{ cursor: 'pointer' }}
                >
                  <span className="block text-sm" style={{ color: 'var(--color-foreground)' }}>
                    {h.article.title}
                  </span>
                  {h.snippet && (
                    <span className="mt-0.5 block line-clamp-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      {h.snippet}
                    </span>
                  )}
                </button>
              ))}
            </div>
          )
        ) : shown ? (
          <>
            <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {shown.title}
            </h2>
            <div className="mt-2 flex flex-wrap gap-2">
              <button
                onClick={() => {
                  navigate(`/help/${shown.slug}`)
                  onClose()
                }}
                className="flex items-center gap-1 rounded px-2 py-1 text-xs"
                style={{ border: '1px solid var(--color-border)', color: 'var(--color-foreground)', cursor: 'pointer' }}
              >
                <ExternalLink size={11} /> Open in Help
              </button>
              {!picked &&
                tours.map((t) => (
                  <button
                    key={t.id}
                    onClick={() => {
                      onClose()
                      startTour(t.id)
                    }}
                    className="flex items-center gap-1 rounded px-2 py-1 text-xs font-semibold"
                    style={{ backgroundColor: 'var(--color-primary)', color: '#fff', cursor: 'pointer' }}
                  >
                    <PlayCircle size={11} /> Take the tour
                  </button>
                ))}
            </div>
            <HelpMarkdown body={shown.body} onOpenArticle={open} />
          </>
        ) : (
          <div className="space-y-3">
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              There's no dedicated article for this page yet. Search above, or
              start here:
            </p>
            {FALLBACK.map((s) => {
              const a = helpArticle(s)
              return a ? (
                <button
                  key={s}
                  onClick={() => open(s)}
                  className="block text-sm"
                  style={{ color: 'var(--color-primary)', cursor: 'pointer' }}
                >
                  {a.title}
                </button>
              ) : null
            })}
          </div>
        )}
      </div>
    </aside>
  )
}
