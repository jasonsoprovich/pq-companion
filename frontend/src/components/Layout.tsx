import React, { lazy, useEffect, useState, Suspense } from 'react'
import { Outlet } from 'react-router-dom'
import TitleBar from './TitleBar'
import Sidebar from './Sidebar'
import GlobalSearch from './GlobalSearch'
import UpdateNotification from './UpdateNotification'
import BackfillProgressBar from './BackfillProgressBar'
import ZealNotification from './ZealNotification'
import ZealVersionWarning from './ZealVersionWarning'
import { HelpCircle } from 'lucide-react'

// Lazy so the bundled help articles stay out of the startup chunk.
const HelpDrawer = lazy(() => import('./guide/HelpDrawer'))

export default function Layout(): React.ReactElement {
  const [searchOpen, setSearchOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const nativeFrame = !!window.electron?.window.nativeFrame

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setSearchOpen((prev) => !prev)
      } else if (e.key === 'F1') {
        e.preventDefault()
        setHelpOpen((prev) => !prev)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  return (
    <div
      className="flex h-screen flex-col overflow-hidden"
      style={{ backgroundColor: 'var(--color-background)' }}
    >
      {/* Under Wine the window manager draws the frame (see main's
          useNativeMainFrame), so our own title bar would be a second one. */}
      {!nativeFrame && <TitleBar onHelp={() => setHelpOpen((v) => !v)} helpOpen={helpOpen} />}
      <div className="flex min-h-0 flex-1">
        <Sidebar onSearchClick={() => setSearchOpen(true)} />
        <main
          className="selectable flex-1 overflow-auto"
          style={{ backgroundColor: 'var(--color-background)' }}
        >
          {/* Pages are code-split (React.lazy in App.tsx), so the first visit
              to a tab fetches its chunk. This boundary covers every main-window
              page, including the nested combat/characters routes. */}
          <Suspense
            fallback={
              <div
                className="flex h-full items-center justify-center text-sm"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                Loading…
              </div>
            }
          >
            <Outlet />
          </Suspense>
        </main>
      </div>
      <GlobalSearch open={searchOpen} onClose={() => setSearchOpen(false)} />
      {helpOpen && (
        <Suspense fallback={null}>
          <HelpDrawer onClose={() => setHelpOpen(false)} />
        </Suspense>
      )}
      {/* Without our title bar (Wine), the help button floats instead. */}
      {nativeFrame && !helpOpen && (
        <button
          onClick={() => setHelpOpen(true)}
          className="fixed bottom-3 right-3 z-30 flex h-8 w-8 items-center justify-center rounded-full shadow-lg"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-muted-foreground)',
          }}
          title="Help for this page (F1)"
          data-tour="help-drawer-button"
        >
          <HelpCircle size={16} />
        </button>
      )}
      <ZealVersionWarning />
      <BackfillProgressBar />
      <UpdateNotification />
      <ZealNotification />
    </div>
  )
}
