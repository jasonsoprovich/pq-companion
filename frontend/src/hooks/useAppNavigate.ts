import { createContext, useContext } from 'react'
import { useNavigate } from 'react-router-dom'

// Route-string navigation that a popped-out overlay window can redirect.
//
// Overlay windows have their own router, so a plain useNavigate() inside
// shared components (the item detail tabs, source tables, …) would swap the
// overlay itself to the Items/NPCs page. An overlay window wraps its content in
// AppNavigateContext.Provider with a handler that drives the main window
// instead (see openRouteInMain); everywhere else this is just useNavigate().
export const AppNavigateContext = createContext<((to: string) => void) | null>(null)

export function useAppNavigate(): (to: string) => void {
  const override = useContext(AppNavigateContext)
  const navigate = useNavigate()
  return override ?? navigate
}
