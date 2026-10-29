import React, { useEffect, useRef, useState } from 'react'
import { Info, Loader2, Search } from 'lucide-react'
import { getItem, searchItems } from '../../services/api'
import { AppNavigateContext } from '../../hooks/useAppNavigate'
import { ItemInfoPreviewContext } from '../../hooks/useItemInfoSettings'
import ItemInfoCard, { DASHBOARD_THEME, POPOUT_THEME } from '../overlays/ItemInfoCard'
import type { ItemInfoOverlaySettings } from '../../types/config'
import type { Item } from '../../types/item'

type Surface = 'popout' | 'dashboard'

// Ethereal Silk Swatch: the feature request's own example, with tradeskill
// uses, vendors and drops, so every section has something to show.
const DEFAULT_ITEM_ID = 16286
const ITEM_KEY = 'itemInfoPreview:item'
const SURFACE_KEY = 'itemInfoPreview:surface'

function readStored(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeStored(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* noop */
  }
}

function ItemSearch({ onPick }: { onPick: (id: number) => void }): React.ReactElement {
  const [q, setQ] = useState('')
  const [results, setResults] = useState<Item[]>([])
  const [open, setOpen] = useState(false)
  const boxRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const term = q.trim()
    if (term.length < 2) {
      setResults([])
      return
    }
    let cancelled = false
    const t = setTimeout(() => {
      searchItems(term, 12)
        .then((r) => { if (!cancelled) setResults(r.items ?? []) })
        .catch(() => {})
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(t)
    }
  }, [q])

  // Close the result list on an outside click.
  useEffect(() => {
    const onDown = (e: MouseEvent): void => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [])

  return (
    <div ref={boxRef} className="relative">
      <Search
        size={12}
        className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2"
        style={{ color: 'var(--color-muted)' }}
      />
      <input
        value={q}
        onChange={(e) => {
          setQ(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        placeholder="Search an item to preview…"
        className="w-full rounded py-1 pl-7 pr-2 text-xs"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          color: 'var(--color-foreground)',
          border: '1px solid var(--color-border)',
        }}
      />
      {open && results.length > 0 && (
        <div
          className="absolute left-0 right-0 top-full z-50 mt-1 max-h-64 overflow-y-auto rounded py-1 shadow-lg"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
        >
          {results.map((it) => (
            <button
              key={it.id}
              onClick={() => {
                onPick(it.id)
                setOpen(false)
                setQ('')
              }}
              className="flex w-full items-baseline justify-between gap-2 px-2 py-1 text-left text-xs hover:bg-black/20"
              style={{ color: 'var(--color-foreground)' }}
            >
              <span className="truncate">{it.name}</span>
              <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>
                #{it.id}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

const noop = (): void => {}

/**
 * Live preview of the Item Info overlay for the Settings editor: pick any
 * item and every change in the cards beside it shows here immediately,
 * before the debounced save and without clicking anything in game.
 */
export default function ItemInfoPreview({ settings }: { settings: ItemInfoOverlaySettings }): React.ReactElement {
  const [itemId, setItemId] = useState<number>(() => {
    const stored = Number(readStored(ITEM_KEY))
    return Number.isInteger(stored) && stored > 0 ? stored : DEFAULT_ITEM_ID
  })
  const [surface, setSurface] = useState<Surface>(() =>
    readStored(SURFACE_KEY) === 'dashboard' ? 'dashboard' : 'popout',
  )
  const [item, setItem] = useState<Item | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    let cancelled = false
    setError(false)
    getItem(itemId)
      .then((it) => { if (!cancelled) setItem(it) })
      .catch(() => { if (!cancelled) setError(true) })
    return () => { cancelled = true }
  }, [itemId])

  const pick = (id: number): void => {
    setItemId(id)
    writeStored(ITEM_KEY, String(id))
  }

  const segment = (active: boolean): React.CSSProperties => ({
    backgroundColor: active ? 'var(--color-primary)' : 'transparent',
    color: active ? 'var(--color-primary-foreground, #000)' : 'var(--color-muted-foreground)',
  })

  const popout = surface === 'popout'
  const frame: React.CSSProperties = popout
    ? {
        backgroundColor: 'rgba(10,10,12,0.92)',
        border: '1px solid rgba(255,255,255,0.1)',
        fontFamily: 'system-ui, sans-serif',
      }
    : { backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }

  return (
    <section
      className="flex flex-col gap-3 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div>
        <h2
          className="mb-1 text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          Live Preview
        </h2>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Changes show here as you make them. Pick any item to try your settings on.
        </p>
      </div>

      <ItemSearch onPick={pick} />

      <div className="flex overflow-hidden rounded self-start text-xs" style={{ border: '1px solid var(--color-border)' }}>
        {(['popout', 'dashboard'] as const).map((s) => (
          <button
            key={s}
            onClick={() => {
              setSurface(s)
              writeStored(SURFACE_KEY, s)
            }}
            className="px-2 py-0.5"
            style={segment(surface === s)}
          >
            {s === 'popout' ? 'Popout' : 'Dashboard'}
          </button>
        ))}
      </div>

      <div style={{ height: 560 }}>
        {error ? (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            Couldn&rsquo;t load that item. Try another.
          </p>
        ) : item === null ? (
          <div className="flex h-full items-center justify-center">
            <Loader2 size={18} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        ) : (
          <div
            style={{ ...frame, height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden', borderRadius: 8 }}
          >
            <div
              style={{
                display: 'flex', alignItems: 'center', gap: 5, padding: '5px 8px', flexShrink: 0,
                borderBottom: `1px solid ${popout ? 'rgba(255,255,255,0.1)' : 'var(--color-border)'}`,
              }}
            >
              <Info size={11} style={{ color: '#38bdf8' }} />
              <span style={{ fontSize: 11, fontWeight: 700, color: popout ? 'rgba(255,255,255,0.8)' : 'var(--color-foreground)' }}>
                Item Info
              </span>
            </div>
            {/* Links stay inert here so clicking one doesn't leave Settings. */}
            <AppNavigateContext.Provider value={noop}>
              <ItemInfoPreviewContext.Provider value={settings}>
                <ItemInfoCard
                  key={`${surface}:${item.id}`}
                  item={item}
                  surface={surface}
                  theme={popout ? POPOUT_THEME : DASHBOARD_THEME}
                />
              </ItemInfoPreviewContext.Provider>
            </AppNavigateContext.Provider>
          </div>
        )}
      </div>
    </section>
  )
}
