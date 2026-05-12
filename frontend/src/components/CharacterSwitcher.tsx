import React, { useCallback, useEffect, useRef, useState } from 'react'
import { User, ChevronDown, Check, Wand2 } from 'lucide-react'
import {
  getConfig,
  listCharacters,
  updateConfig,
  type CharactersResponse,
} from '../services/api'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

const POLL_MS = 30_000

export default function CharacterSwitcher(): React.ReactElement | null {
  const [data, setData] = useState<CharactersResponse | null>(null)
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const { active, manual, setActive } = useActiveCharacter()

  const refresh = useCallback(() => {
    listCharacters()
      .then((d) => {
        setData(d)
        setActive(d.active, d.manual)
      })
      .catch(() => setData(null))
  }, [setActive])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, POLL_MS)
    return () => clearInterval(id)
  }, [refresh])

  // Subscribe to live auto-detection events from the backend tailer so the
  // selector reflects the active character as soon as a new log file becomes
  // active, without waiting for the 30s poll.
  const handleWsMessage = useCallback(
    (msg: WsMessage) => {
      if (msg.type !== WSEvent.ConfigCharacterDetected) return
      const payload = msg.data as { character?: string } | null
      const name = payload?.character ?? ''
      if (!name) return
      setData((prev) =>
        prev ? { ...prev, active: name, manual: false, detected: name } : prev,
      )
      setActive(name, false)
    },
    [setActive],
  )
  useWebSocket(handleWsMessage)

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
        const cfg = await getConfig()
        const chars = data?.characters ?? []
        const char = chars.find((c) => c.name === name)
        await updateConfig({
          ...cfg,
          character: name,
          character_class: char?.class ?? (name === '' ? -1 : cfg.character_class),
        })
        await refresh()
        setOpen(false)
      } catch {
        // Swallow — UI stays as-is; next poll will recover state.
      } finally {
        setBusy(false)
      }
    },
    [busy, data, refresh],
  )

  // Show switcher even with no characters so Auto option is always accessible.
  if (!data) return null

  const activeLabel = active || 'No character'
  const detected = data.detected
  const autoLabel = detected ? `Auto · ${detected}` : 'Auto (no character active)'

  return (
    <div ref={containerRef} className="no-drag relative px-2 pt-1 pb-2">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between rounded px-3 py-1.5 transition-colors hover:bg-(--color-surface-3)"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
        }}
        title={manual ? 'Manual character override (click to change)' : 'Auto-detected character (click to change)'}
      >
        <div className="flex min-w-0 items-center gap-2">
          {manual ? (
            <User size={12} style={{ color: 'var(--color-muted)', flexShrink: 0 }} />
          ) : (
            <Wand2 size={12} style={{ color: 'var(--color-primary)', flexShrink: 0 }} />
          )}
          <span
            className="truncate text-[11px]"
            style={{ color: 'var(--color-foreground)' }}
          >
            {activeLabel}
          </span>
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
            style={{
              color: 'var(--color-foreground)',
              backgroundColor: !manual ? 'var(--color-surface-2)' : undefined,
            }}
          >
            <Wand2 size={11} style={{ color: !manual ? 'var(--color-primary)' : 'var(--color-muted)' }} />
            <span className="flex-1 truncate">{autoLabel}</span>
            {!manual && <Check size={11} style={{ color: 'var(--color-primary)' }} />}
          </button>
          {data.characters.length > 0 && (
            <div style={{ borderTop: '1px solid var(--color-border)' }} />
          )}
          {data.characters.map((c) => {
            const selected = manual && active === c.name
            return (
              <button
                key={c.id}
                onClick={() => selectCharacter(c.name)}
                disabled={busy}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[11px] hover:bg-(--color-surface-2)"
                style={{
                  color: 'var(--color-foreground)',
                  backgroundColor: selected ? 'var(--color-surface-2)' : undefined,
                }}
              >
                <User size={11} style={{ color: selected ? 'var(--color-primary)' : 'var(--color-muted)' }} />
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
