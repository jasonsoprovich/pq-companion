import React from 'react'
import { Outlet } from 'react-router-dom'
import TitleBar from './TitleBar'
import Sidebar from './Sidebar'

export default function Layout(): React.ReactElement {
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
    </div>
  )
}
