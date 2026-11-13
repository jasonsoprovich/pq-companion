/**
 * OverlaysHub — the /overlays page. Library lists every overlay as a card
 * (pop out, dashboard panel, hotkey, details); Dashboard is the in-app panel
 * grid. The layout chips sit above both, since a layout covers every popout.
 * The page remembers which tab you were on.
 */
import React from 'react'
import { LayoutGrid, LayoutDashboard, MonitorPlay } from 'lucide-react'
import { usePersistentState } from '../hooks/usePersistentState'
import LayoutChips from '../components/LayoutChips'
import OverlayLibrary from '../components/overlays-hub/OverlayLibrary'
import OverlaysDashboard from './OverlaysDashboard'

type HubTab = 'library' | 'dashboard'
const HUB_TABS: readonly HubTab[] = ['library', 'dashboard']

const TAB_DEFS: Array<{ id: HubTab; label: string; icon: React.ElementType }> = [
  { id: 'library', label: 'Library', icon: LayoutGrid },
  { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
]

export default function OverlaysHub(): React.ReactElement {
  const [tab, setTab] = usePersistentState<HubTab>('pq-tab:overlays', 'library', HUB_TABS)

  return (
    <div className="flex h-full flex-col" style={{ backgroundColor: 'var(--color-background)' }}>
      <div
        className="flex shrink-0 items-center gap-4 border-b px-4"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex shrink-0 items-center gap-2 py-2">
          <MonitorPlay size={14} style={{ color: 'var(--color-primary)' }} />
          <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Overlays
          </span>
        </div>
        <div className="flex shrink-0 items-stretch self-stretch">
          {TAB_DEFS.map(({ id, label, icon: Icon }) => {
            const active = tab === id
            return (
              <button
                key={id}
                onClick={() => setTab(id)}
                className="flex items-center gap-1.5 px-3 text-xs"
                style={{
                  color: active ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
                  borderBottom: `2px solid ${active ? 'var(--color-primary)' : 'transparent'}`,
                  fontWeight: active ? 600 : 400,
                }}
              >
                <Icon size={12} />
                {label}
              </button>
            )
          })}
        </div>
        <div className="min-w-0 flex-1 py-2">
          <LayoutChips />
        </div>
      </div>
      <div className="min-h-0 flex-1">
        {tab === 'library' ? <OverlayLibrary /> : <OverlaysDashboard />}
      </div>
    </div>
  )
}
