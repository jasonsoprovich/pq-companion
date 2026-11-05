import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Crosshair, Loader2, Search } from 'lucide-react'
import { getOverlayNPCPreview, listCharacters, searchNPCs } from '../../services/api'
import { NPCOverlayPreviewContext } from '../../lib/npcOverlayPreview'
import { useNPCOverlaySections } from '../../hooks/useNPCOverlaySections'
import { useNPCOutlineMatches } from '../../hooks/useNPCOutlineMatches'
import { useWishlistItemIds } from '../../hooks/useWishlistItemIds'
import { useActiveCharacterLevel } from '../../hooks/useActiveCharacterLevel'
import type { NPCOverlayView } from '../../hooks/useNPCOverlayView'
import { NPCOutlineFrame, NPCOutlineLabels } from '../overlays/NPCOutline'
import { NPCCard, ViewToggle as DashboardViewToggle } from '../overlays/NPCPanel'
import {
  NPCContent,
  ViewToggle as PopoutViewToggle,
} from '../../pages/NPCOverlayWindowPage'
import { className } from '../../lib/npcHelpers'
import type { Preferences } from '../../types/config'
import type { NPC } from '../../types/npc'
import type { TargetState } from '../../types/overlay'

type Surface = 'popout' | 'dashboard'

interface Props {
  // The unsaved, in-memory preferences being edited.
  prefs: Preferences
  // Character whose per-character lists the preview applies ('' = default).
  character: string
  onCharacterChange: (character: string) => void
}

// Lord Nagafen: a familiar target with plenty of tags, so the preview has
// something to show before the user searches.
const DEFAULT_NPC_ID = 32040
const NPC_KEY = 'npcOverlayPreview:npc'
const SURFACE_KEY = 'npcOverlayPreview:surface'

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

const displayName = (name: string): string => name.replace(/^#/, '').replace(/_/g, ' ').trim()

function NPCSearch({ onPick }: { onPick: (id: number) => void }): React.ReactElement {
  const [q, setQ] = useState('')
  const [results, setResults] = useState<NPC[]>([])
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
      searchNPCs(term, 12)
        .then((r) => {
          if (!cancelled) setResults(r.items ?? [])
        })
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
        placeholder="Search an NPC to preview…"
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
          {results.map((n) => (
            <button
              key={n.id}
              onClick={() => {
                onPick(n.id)
                setOpen(false)
                setQ('')
              }}
              className="flex w-full items-baseline justify-between gap-2 px-2 py-1 text-left text-xs hover:bg-black/20"
              style={{ color: 'var(--color-foreground)' }}
            >
              <span className="truncate">{displayName(n.name)}</span>
              <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>
                L{n.level} {className(n.class)}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// The overlay body itself. Rendered inside the preview context so every
// display hook below reads the unsaved preferences.
function PreviewWindow({
  surface,
  target,
  pinned,
}: {
  surface: Surface
  target: TargetState
  pinned: boolean
}): React.ReactElement {
  const [view, setView] = useState<NPCOverlayView>('stats')
  const sections = useNPCOverlaySections(surface)
  const outlineMatches = useNPCOutlineMatches(target)
  const wishlistItemIds = useWishlistItemIds()
  const charLevel = useActiveCharacterLevel()
  const pinRing = pinned && outlineMatches.length === 0 ? '2px solid #c9a84c' : undefined

  if (surface === 'popout') {
    return (
      <div
        style={{
          position: 'relative',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          backgroundColor: 'rgba(10,10,12,0.92)',
          color: 'rgba(255,255,255,0.85)',
          fontFamily: 'system-ui, sans-serif',
          overflow: 'hidden',
          borderRadius: 8,
          border: '1px solid rgba(255,255,255,0.1)',
          outline: pinRing,
          outlineOffset: pinRing ? -2 : undefined,
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '6px 10px',
            borderBottom: '1px solid rgba(255,255,255,0.08)',
            flexShrink: 0,
          }}
        >
          <Crosshair size={13} style={{ color: '#c9a84c' }} />
          <PopoutViewToggle view={view} onChange={setView} />
        </div>
        <NPCOutlineFrame rule={outlineMatches[0]} pinned={pinned} />
        <NPCOutlineLabels rules={outlineMatches} />
        <NPCContent
          state={target}
          view={view}
          sections={sections}
          wishlistItemIds={wishlistItemIds}
          charLevel={charLevel}
          dist={null}
        />
      </div>
    )
  }

  return (
    <div
      className="flex flex-col"
      style={{
        position: 'relative',
        height: '100%',
        overflow: 'hidden',
        borderRadius: 8,
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        outline: pinRing,
        outlineOffset: pinRing ? -2 : undefined,
      }}
    >
      <div
        className="flex shrink-0 items-center gap-2 px-3 py-1.5"
        style={{ borderBottom: '1px solid var(--color-border)' }}
      >
        <Crosshair size={13} style={{ color: 'var(--color-primary)' }} />
        <DashboardViewToggle view={view} onChange={setView} />
      </div>
      <NPCOutlineFrame rule={outlineMatches[0]} pinned={pinned} />
      <NPCOutlineLabels rules={outlineMatches} />
      <NPCCard
        state={target}
        view={view}
        sections={sections}
        onItemClick={() => {}}
        wishlistItemIds={wishlistItemIds}
        charLevel={charLevel}
        dist={null}
      />
    </div>
  )
}

// In-game /con words by faction level, for the preview's sample reading.
const SAMPLE_STANDINGS: Record<number, string> = {
  1: 'ally',
  2: 'warmly',
  3: 'kindly',
  4: 'amiable',
  5: 'indifferent',
  6: 'apprehensive',
  7: 'dubious',
  8: 'threatening',
  9: 'scowls',
}

/**
 * Live preview of the NPC overlay for the Settings editor. Pick any NPC and
 * every change in the cards beside it shows here immediately — before the
 * debounced save, and without needing a live target in game.
 */
export default function NPCOverlayPreview({
  prefs,
  character,
  onCharacterChange,
}: Props): React.ReactElement {
  const [npcId, setNpcId] = useState<number>(() => {
    const stored = Number(readStored(NPC_KEY))
    return Number.isInteger(stored) && stored > 0 ? stored : DEFAULT_NPC_ID
  })
  const [surface, setSurface] = useState<Surface>(() =>
    readStored(SURFACE_KEY) === 'dashboard' ? 'dashboard' : 'popout',
  )
  const [target, setTarget] = useState<TargetState | null>(null)
  const [error, setError] = useState(false)
  const [pinned, setPinned] = useState(false)
  // Sample /con standing (faction level, 0 = none) to try the border on.
  const [conLevel, setConLevel] = useState(0)
  const [characters, setCharacters] = useState<string[]>([])

  useEffect(() => {
    listCharacters()
      .then((r) => setCharacters(r.characters.map((c) => c.name)))
      .catch(() => {})
  }, [])

  useEffect(() => {
    let cancelled = false
    setError(false)
    getOverlayNPCPreview(npcId)
      .then((t) => {
        if (!cancelled) setTarget(t)
      })
      .catch(() => {
        if (!cancelled) setError(true)
      })
    return () => {
      cancelled = true
    }
  }, [npcId])

  const pick = (id: number): void => {
    setNpcId(id)
    writeStored(NPC_KEY, String(id))
  }

  const ctx = useMemo(() => ({ prefs, character }), [prefs, character])
  const shown = useMemo<TargetState | null>(
    () =>
      target && conLevel > 0
        ? {
            ...target,
            consider: {
              faction: conLevel,
              standing: SAMPLE_STANDINGS[conLevel],
              at: new Date().toISOString(),
              source: 'pipe',
            },
          }
        : target,
    [target, conLevel],
  )

  const segment = (active: boolean): React.CSSProperties => ({
    backgroundColor: active ? 'var(--color-primary)' : 'transparent',
    color: active ? 'var(--color-primary-foreground, #000)' : 'var(--color-muted-foreground)',
  })
  const selectStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
  }

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
          Changes show here as you make them. Pick any NPC to try your settings on.
        </p>
      </div>

      <NPCSearch onPick={pick} />

      <div className="flex flex-wrap items-center gap-2 text-xs">
        <div
          className="flex overflow-hidden rounded"
          style={{ border: '1px solid var(--color-border)' }}
        >
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
        <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}>
          <input type="checkbox" checked={pinned} onChange={(e) => setPinned(e.target.checked)} />
          Pinned
        </label>
        <label className="flex items-center gap-1" style={{ color: 'var(--color-muted-foreground)' }}>
          /con
          <select
            value={conLevel}
            onChange={(e) => setConLevel(Number(e.target.value))}
            className="rounded px-1.5 py-0.5 text-xs"
            style={selectStyle}
          >
            <option value={0}>none</option>
            {[9, 8, 7, 6, 5, 4, 1].map((n) => (
              <option key={n} value={n}>
                {SAMPLE_STANDINGS[n]}
              </option>
            ))}
          </select>
        </label>
      </div>

      <label className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Preview as
        <select
          value={character}
          onChange={(e) => onCharacterChange(e.target.value)}
          className="min-w-0 flex-1 rounded px-1.5 py-0.5 text-xs"
          style={selectStyle}
        >
          <option value="">All characters (default)</option>
          {characters.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
      </label>

      <div style={{ height: 560 }}>
        {error ? (
          <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
            Couldn&rsquo;t load that NPC. Try another.
          </p>
        ) : shown === null ? (
          <div className="flex h-full items-center justify-center">
            <Loader2 size={18} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        ) : (
          <NPCOverlayPreviewContext.Provider value={ctx}>
            <PreviewWindow
              // Remount on surface change so each starts on the Stats view.
              key={surface}
              surface={surface}
              target={shown}
              pinned={pinned}
            />
          </NPCOverlayPreviewContext.Provider>
        )}
      </div>
    </section>
  )
}
