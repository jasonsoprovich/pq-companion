/**
 * Instance windows: overlay types that can have many popouts, one per
 * user-created item, keyed `<kind>:<id>` (mirrors INSTANCE_KINDS in
 * electron/main). Named Custom Timers windows are `customTimer:<groupId>`;
 * Custom Overlays are `customOverlay:<overlayId>`.
 */
export type InstanceKind = 'customTimer' | 'customOverlay'

const KINDS: readonly InstanceKind[] = ['customTimer', 'customOverlay']

export function instanceKind(key: string): InstanceKind | null {
  const i = key.indexOf(':')
  if (i <= 0 || i === key.length - 1) return null
  const kind = key.slice(0, i) as InstanceKind
  return KINDS.includes(kind) ? kind : null
}

export function isInstanceKey(key: string): boolean {
  return instanceKind(key) !== null
}

export function instanceId(key: string): string {
  return key.slice(key.indexOf(':') + 1)
}

// What to call an instance window when its own name isn't known.
export const INSTANCE_FALLBACK_LABEL: Record<InstanceKind, string> = {
  customTimer: 'Timer window',
  customOverlay: 'Custom overlay',
}

// Card / box accent per kind (Arrange view).
export const INSTANCE_COLOR: Record<InstanceKind, string> = {
  customTimer: '#38bdf8',
  customOverlay: '#a78bfa',
}
