/**
 * HotkeysSettings — Settings → Hotkeys. Global key combos for showing and
 * hiding overlays. Bindings live in the Electron main process (it registers
 * them with the OS at startup); this tab records new combos and shows
 * whether each one registered.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { AlertTriangle, Info, Keyboard, Wand2, X } from 'lucide-react'
import type { HotkeyState } from '../../types/electron'
import {
  SUGGESTED_HOTKEYS,
  acceleratorFromEvent,
  acceleratorParts,
  eqConflictWarning,
  hotkeyActions,
} from '../../lib/hotkeys'

function Kbd({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <kbd
      className="rounded px-1.5 py-px font-mono text-[11px]"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        borderBottomWidth: 2,
        color: 'var(--color-foreground)',
      }}
    >
      {children}
    </kbd>
  )
}

export default function HotkeysSettings(): React.ReactElement {
  const api = window.electron?.overlay
  const supported = typeof api?.hotkeysGet === 'function'
  const [state, setState] = useState<HotkeyState>({ bindings: {}, status: {} })
  // Action currently waiting for a key press, and any error from that press.
  const [recording, setRecording] = useState<string | null>(null)
  const [recordError, setRecordError] = useState<string | null>(null)
  const recordingRef = useRef<string | null>(null)

  useEffect(() => {
    if (supported) api.hotkeysGet().then(setState).catch(() => {})
  }, [api, supported])

  const stopRecording = useCallback(() => {
    if (recordingRef.current) void api?.hotkeysSuspend(false)
    recordingRef.current = null
    setRecording(null)
    setRecordError(null)
  }, [api])

  const startRecording = (action: string): void => {
    // Release every hotkey while recording, so pressing a combo that's
    // already bound reaches this page instead of firing its action.
    void api?.hotkeysSuspend(true)
    recordingRef.current = action
    setRecording(action)
    setRecordError(null)
  }

  // Leaving the tab mid-recording must not leave hotkeys suspended.
  useEffect(() => stopRecording, [stopRecording])

  useEffect(() => {
    if (!recording) return
    const onKey = (e: KeyboardEvent): void => {
      e.preventDefault()
      e.stopPropagation()
      if (e.key === 'Escape' && !e.ctrlKey && !e.altKey && !e.shiftKey) {
        stopRecording()
        return
      }
      if ((e.key === 'Backspace' || e.key === 'Delete') && !e.ctrlKey && !e.altKey) {
        const action = recording
        stopRecording()
        api?.hotkeysSet(action, null).then(setState).catch(() => {})
        return
      }
      const result = acceleratorFromEvent(e)
      if (result.kind === 'pending') return
      if (result.kind === 'invalid') {
        setRecordError(result.reason)
        return
      }
      const owner = Object.entries(state.bindings).find(
        ([a, acc]) => acc === result.accelerator && a !== recording,
      )
      if (owner) {
        const label = hotkeyActions().find((x) => x.id === owner[0])?.label ?? owner[0]
        setRecordError(`${result.accelerator} is already used for "${label}".`)
        return
      }
      const action = recording
      stopRecording()
      api?.hotkeysSet(action, result.accelerator).then(setState).catch(() => {})
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [recording, state.bindings, api, stopRecording])

  const applySuggested = (): void => {
    const used = new Set(Object.values(state.bindings))
    const next: Record<string, string> = {}
    for (const [action, acc] of Object.entries(SUGGESTED_HOTKEYS)) {
      if (!state.bindings[action] && !used.has(acc)) next[action] = acc
    }
    if (Object.keys(next).length > 0) api?.hotkeysSetMany(next).then(setState).catch(() => {})
  }

  if (!supported) {
    return (
      <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Hotkeys are only available in the desktop app.
      </p>
    )
  }

  const unboundSuggestions = Object.entries(SUGGESTED_HOTKEYS).some(
    ([action, acc]) => !state.bindings[action] && !Object.values(state.bindings).includes(acc),
  )

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2
          className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          <Keyboard size={13} /> Overlay Hotkeys
        </h2>
        {unboundSuggestions && (
          <button
            onClick={applySuggested}
            className="flex items-center gap-1.5 rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
            title={Object.entries(SUGGESTED_HOTKEYS)
              .map(([a, acc]) => `${hotkeyActions().find((x) => x.id === a)?.label}: ${acc}`)
              .join('\n')}
          >
            <Wand2 size={11} /> Use suggested keys
          </button>
        )}
      </div>

      <div
        className="mb-4 flex gap-2 rounded p-3 text-xs leading-relaxed"
        style={{
          border: '1px solid rgba(59,130,246,0.4)',
          backgroundColor: 'rgba(59,130,246,0.08)',
          color: 'var(--color-muted-foreground)',
        }}
      >
        <Info size={14} className="mt-0.5 shrink-0" style={{ color: 'var(--color-info)' }} />
        <div>
          <p style={{ color: 'var(--color-foreground)' }}>
            These work while EverQuest has focus, and they never pull focus away from it.
          </p>
          <p className="mt-1">
            PQ Companion registers each combo with Windows. EQ stays the active window and
            keeps getting every other key you press. The trade-off: while a combo is bound
            here, EQ won't receive it, so avoid combos you use in EQ's key bindings or
            hotbars. Hiding an overlay keeps it in place; trigger alert text is never hidden.
          </p>
        </div>
      </div>

      <div className="flex flex-col">
        {hotkeyActions().map((a, i) => {
          const acc = state.bindings[a.id]
          const failed = acc && state.status[a.id] === 'failed'
          const warning = acc && !failed ? eqConflictWarning(acc) : null
          const isRecording = recording === a.id
          return (
            <div
              key={a.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2"
              style={{ borderTop: i === 0 ? undefined : '1px solid var(--color-border)' }}
            >
              <span
                className="min-w-0 flex-1 text-xs"
                style={{
                  color: 'var(--color-foreground)',
                  fontWeight: i === 0 ? 600 : undefined,
                }}
              >
                {i === 0 ? a.label : `Show / hide · ${a.label}`}
              </span>
              <button
                onClick={() => (isRecording ? stopRecording() : startRecording(a.id))}
                className="flex min-w-[140px] items-center justify-center gap-1 rounded px-2 py-1 text-xs"
                style={{
                  backgroundColor: isRecording ? 'rgba(201,168,76,0.12)' : 'var(--color-surface-2)',
                  border: `1px solid ${isRecording ? 'var(--color-primary)' : 'var(--color-border)'}`,
                  color: 'var(--color-muted-foreground)',
                }}
                title={isRecording ? 'Press Esc to cancel, Backspace to clear' : 'Click, then press a key combo'}
              >
                {isRecording ? (
                  <span style={{ color: 'var(--color-primary)' }}>Press a combo…</span>
                ) : acc ? (
                  acceleratorParts(acc).map((p, j) => <Kbd key={j}>{p}</Kbd>)
                ) : (
                  <span className="italic">Click to set</span>
                )}
              </button>
              <button
                onClick={() => api?.hotkeysSet(a.id, null).then(setState).catch(() => {})}
                className="rounded p-1"
                style={{
                  color: 'var(--color-muted)',
                  visibility: acc && !isRecording ? 'visible' : 'hidden',
                }}
                title="Clear this hotkey"
                aria-label={`Clear hotkey for ${a.label}`}
              >
                <X size={12} />
              </button>
              {(isRecording && recordError) || failed || warning ? (
                <span
                  className="flex w-full items-center gap-1 text-[11px]"
                  style={{
                    color: isRecording || failed ? 'var(--color-destructive)' : 'var(--color-warning)',
                  }}
                >
                  <AlertTriangle size={11} />
                  {isRecording && recordError
                    ? recordError
                    : failed
                      ? "Couldn't register this combo — another program is already using it. Pick a different one."
                      : warning}
                </span>
              ) : null}
            </div>
          )
        })}
      </div>
    </section>
  )
}
