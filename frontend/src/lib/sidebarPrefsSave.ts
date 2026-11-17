// saveSidebarPrefs writes sidebar order / hidden / favorites preferences.
// Shared by the sidebar's edit mode and the right-click menu; the save
// broadcasts config:updated, which useSidebarPrefs and Settings →
// Navigation both pick up.
import { getConfig, saveConfigChanges } from '../services/api'
import type { Preferences } from '../types/config'

type SidebarPatch = Partial<Pick<Preferences, 'sidebar_order' | 'sidebar_hidden' | 'sidebar_favorites'>>

export async function saveSidebarPrefs(patch: SidebarPatch): Promise<void> {
  try {
    const c = await getConfig()
    await saveConfigChanges({ ...c, preferences: { ...c.preferences, ...patch } }, c)
  } catch {
    /* the next config:updated refresh shows the saved state */
  }
}

function toggled(list: string[] | undefined, key: string): string[] {
  const set = new Set(list ?? [])
  if (set.has(key)) set.delete(key)
  else set.add(key)
  return Array.from(set)
}

export async function toggleSidebarFavorite(route: string): Promise<void> {
  const c = await getConfig().catch(() => null)
  if (!c) return
  await saveSidebarPrefs({ sidebar_favorites: toggled(c.preferences?.sidebar_favorites, route) })
}

export async function hideSidebarItem(route: string): Promise<void> {
  const c = await getConfig().catch(() => null)
  if (!c) return
  const hidden = new Set(c.preferences?.sidebar_hidden ?? [])
  hidden.add(route)
  await saveSidebarPrefs({ sidebar_hidden: Array.from(hidden) })
}
