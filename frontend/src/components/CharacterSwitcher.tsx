import React, { useCallback, useEffect, useRef, useState } from 'react'
import { User, ChevronDown, Check, Wand2 } from 'lucide-react'
import {
  getConfig,
  listCharacters,
  updateConfig,
  type CharactersResponse,
} from '../services/api'
import type { Config } from '../types/config'

const POLL_MS = 30_000

export default function CharacterSwitcher(): React.ReactElement | null {
  const [data, setData] = useState<CharactersResponse | null>(null)
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const refresh = useCallback(() => {
    listCharacters()
      .then(setData)
      .catch(() => setData(null))
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, POLL_MS)
    return () => clearInterval(id)
  }, [refresh])

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [open])

  const selectCharacter = useCallback(
    async (name: string) => {
      if (busy) return
      setBusy(true)
      try {
        const cfg: Config = await getConfig()
        await updateConfig({ ...cfg, character: name })
        await refresh()
        setOpen(false)
      } catch {
        // Swallow — UI stays as-is; next poll will recover state.
      } finally {
        setBusy(false)
      }
    },
    [busy, refresh],
  )

  if (!data || data.characters.length === 0) return null

  const activeLabel = data.active || 'No character'

  return (
    <div ref={containerRef} className="no-drag relative px-2 pt-1 pb-2">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between rounded px-3 py-1.5 transition-colors hover:bg-(--color-surface-3)"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
        }}
        title={data.manual ? 'Character override (click to change)' : 'Auto-detected character (click to change)'}
      >
        <div className="flex min-w-0 items-center gap-2">
          <User size={12} style={{ color: 'var(--color-muted)', flexShrink: 0 }} />
          <span
            className="truncate text-[11px]"
            style={{ color: 'var(--color-foreground)' }}
          >
            {activeLabel}
          </span>
          {!data.manual && (
            <span
              className="shrink-0 rounded px-1 py-px text-[9px] uppercase tracking-wider"
              style={{ color: 'var(--color-muted)', backgroundColor: 'var(--color-surface-3)' }}
            >
              auto
            </span>
          )}
        </div>
        <ChevronDown
          size={11}
          style={{
            color: 'var(--color-muted)',
            transform: open ? 'rotate(180deg)' : 'none',
            transition: 'transform 120ms',
          }}
        />
      </button>

      {open && (
        <div
          className="absolute left-2 right-2 z-40 mt-1 overflow-hidden rounded shadow-lg"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-border)',
          }}
        >
          <button
            onClick={() => selectCharacter('')}
            disabled={busy}
            className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] hover:bg-(--color-surface-2)"
            style={{ color: 'var(--color-foreground)' }}
          >
            <Wand2 size={11} style={{ color: 'var(--color-muted)' }} />
            <span className="flex-1">Auto (most recent)</span>
            {!data.manual && <Check size={11} style={{ color: 'var(--color-primary)' }} />}
          </button>
          <div style={{ borderTop: '1px solid var(--color-border)' }} />
          {data.characters.map((c) => {
            const selected = data.manual && data.active === c.name
            return (
              <button
                key={c.name}
                onClick={() => selectCharacter(c.name)}
                disabled={busy}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] hover:bg-(--color-surface-2)"
                style={{ color: 'var(--color-foreground)' }}
              >
                <User size={11} style={{ color: 'var(--color-muted)' }} />
                <span className="flex-1 truncate">{c.name}</span>
                {selected && <Check size={11} style={{ color: 'var(--color-primary)' }} />}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
