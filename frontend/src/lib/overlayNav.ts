// Helpers for opening a database-explorer entity from an overlay window.
//
// Overlay windows are separate Electron BrowserWindows with their own router,
// so they can't navigate the main window directly. Instead they ask the main
// process (via the app:navigate-main IPC) to focus the main window and drive it
// to the entity's hash route. In a plain browser (no Electron) these no-op.

export type EntityKind = 'item' | 'spell' | 'npc' | 'zone'

const ROUTE: Record<EntityKind, string> = {
  item: '/items',
  spell: '/spells',
  npc: '/npcs',
  zone: '/zones',
}

export function openEntityInMain(kind: EntityKind, id: number | string): void {
  void window.electron?.app?.navigateMain?.(`${ROUTE[kind]}?select=${id}`)
}
