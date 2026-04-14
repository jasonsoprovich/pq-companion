import React, { useEffect, useState } from 'react'
import { Outlet } from 'react-router-dom'
import TitleBar from './TitleBar'
import Sidebar from './Sidebar'
import GlobalSearch from './GlobalSearch'

export default function Layout(): React.ReactElement {
  const [searchOpen, setSearchOpen] = useState(false)

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setSearchOpen((prev) => !prev)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  return (
    <div
      className="flex h-full flex-col"
      style={{ backgroundColor: 'var(--color-background)' }}
    >
      <TitleBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <main
          className="selectable flex-1 overflow-auto"
          style={{ backgroundColor: 'var(--color-background)' }}
        >
          <Outlet />
        </main>
      </div>
      <GlobalSearch open={searchOpen} onClose={() => setSearchOpen(false)} />
    </div>
  )
}
