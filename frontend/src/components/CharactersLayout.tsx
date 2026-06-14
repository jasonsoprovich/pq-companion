import React from 'react'
import { Outlet } from 'react-router-dom'

// CharactersLayout is now a thin wrapper around the character sub-pages. The
// per-page navigation that used to live here as a horizontal tab strip moved
// into the left sidebar's collapsible "Characters" section, so this just hosts
// the routed page.
export default function CharactersLayout(): React.ReactElement {
  return (
    <div className="flex h-full flex-col">
      <Outlet />
    </div>
  )
}
