/**
 * Helpers for the global overlay hotkeys (Settings → Hotkeys). Bindings are
 * stored and registered by the Electron main process as Electron accelerator
 * strings ("Ctrl+Alt+H"); this file turns key presses into those strings and
 * flags combos that are likely to clash with EverQuest.
 */
import { OVERLAY_DEFS, type OverlayName } from './overlays'

export const TOGGLE_ALL_ACTION = 'toggleAll'
export const overlayAction = (name: OverlayName): string => `overlay:${name}`
export const layoutAction = (id: string): string => `layout:${id}`

export interface HotkeyActionDef {
  id: string
  label: string
  kind: 'all' | 'layout' | 'overlay'
}

// Layout actions switch to that layout (or show/hide everything when it's
// already active), so they're listed with the show/hide-all action.
export function hotkeyActions(layouts: Array<{ id: string; name: string }> = []): HotkeyActionDef[] {
  return [
    { id: TOGGLE_ALL_ACTION, label: 'Show / hide all overlays', kind: 'all' },
    ...layouts.map((l) => ({ id: layoutAction(l.id), label: l.name, kind: 'layout' as const })),
    ...OVERLAY_DEFS.map((d) => ({
      id: overlayAction(d.name),
      label: d.label,
      kind: 'overlay' as const,
    })),
  ]
}

// Filled in by "Use suggested keys" for actions that are still unbound.
// Ctrl+Alt combos are rarely bound in EQ.
export const SUGGESTED_HOTKEYS: Record<string, string> = {
  [TOGGLE_ALL_ACTION]: 'Ctrl+Alt+H',
  [overlayAction('buffTimer')]: 'Ctrl+Alt+B',
  [overlayAction('detrimTimer')]: 'Ctrl+Alt+E',
  [overlayAction('dps')]: 'Ctrl+Alt+D',
  [overlayAction('threat')]: 'Ctrl+Alt+T',
  [overlayAction('npc')]: 'Ctrl+Alt+N',
}

// KeyboardEvent.code → accelerator key. Using `code` keeps bindings tied to
// the physical key regardless of keyboard layout.
const NAMED_CODES: Record<string, string> = {
  Space: 'Space',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  Insert: 'Insert',
  Delete: 'Delete',
  Backquote: '`',
  Minus: '-',
  Equal: '=',
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Semicolon: ';',
  Quote: "'",
  Comma: ',',
  Period: '.',
  Slash: '/',
  NumpadAdd: 'numadd',
  NumpadSubtract: 'numsub',
  NumpadMultiply: 'nummult',
  NumpadDivide: 'numdiv',
  NumpadDecimal: 'numdec',
}

function keyFromCode(code: string): string | null {
  let m = /^Key([A-Z])$/.exec(code)
  if (m) return m[1]
  m = /^Digit(\d)$/.exec(code)
  if (m) return m[1]
  m = /^Numpad(\d)$/.exec(code)
  if (m) return `num${m[1]}`
  if (/^F([1-9]|1\d|2[0-4])$/.test(code)) return code
  return NAMED_CODES[code] ?? null
}

export type CaptureResult =
  | { kind: 'pending' } // only modifiers held so far
  | { kind: 'invalid'; reason: string }
  | { kind: 'ok'; accelerator: string }

/**
 * acceleratorFromEvent converts a keydown into an accelerator. Letters,
 * digits and punctuation need Ctrl, Alt or Win — on their own they'd stop
 * working for typing everywhere, EQ chat included. F-keys and the numpad may
 * be bound bare (with a warning, since EQ uses them).
 */
export function acceleratorFromEvent(e: KeyboardEvent): CaptureResult {
  if (['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) return { kind: 'pending' }
  const key = keyFromCode(e.code)
  if (!key) return { kind: 'invalid', reason: "That key can't be used for a hotkey." }
  const mods: string[] = []
  if (e.ctrlKey) mods.push('Ctrl')
  if (e.altKey) mods.push('Alt')
  if (e.shiftKey) mods.push('Shift')
  if (e.metaKey) mods.push('Super')
  const bareOk = /^F\d+$/.test(key) || key.startsWith('num')
  if (!bareOk && !e.ctrlKey && !e.altKey && !e.metaKey) {
    return {
      kind: 'invalid',
      reason: 'Add Ctrl or Alt — a plain key would stop working for typing everywhere.',
    }
  }
  return { kind: 'ok', accelerator: [...mods, key].join('+') }
}

/**
 * eqConflictWarning returns a warning for combos EverQuest is likely to use
 * itself, or null for the safer Ctrl+Alt / Ctrl+Shift families.
 */
export function eqConflictWarning(accelerator: string): string | null {
  const parts = accelerator.split('+')
  const has = (m: string): boolean => parts.includes(m)
  if (has('Ctrl') && (has('Alt') || has('Shift'))) return null
  const key = parts[parts.length - 1]
  if (/^F\d+$/.test(key)) return 'EverQuest uses F-keys for targeting — check your EQ key bindings.'
  if (key.startsWith('num')) return 'EverQuest uses the numpad for movement — check your EQ key bindings.'
  return 'This combo may already do something in EverQuest. Ctrl+Alt combos are safest.'
}

export function acceleratorParts(accelerator: string): string[] {
  return accelerator.split('+').map((p) => (p === 'Super' ? 'Win' : p))
}
