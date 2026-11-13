/**
 * OverlaysHub — the /overlays page. Library lists every overlay as a card
 * (pop out, dashboard panel, hotkey, details); Arrange draws the layout's
 * windows on a miniature of the monitors to move them; Dashboard is the
 * in-app panel grid. The layout chips sit above all three, since a layout
 * covers every popout.
 * The page remembers which tab you were on.
 *
 * Settings open in a drawer over the Library: the gear for settings that
 * apply to every overlay, or a card for that overlay's own. /overlays?panel=
 * opens one directly (global, trigger, group:<id>, or an overlay's panel
 * key or name) — old Settings tab links redirect here. The ⋯ menu beside
 * the gear holds actions on every overlay at once.
 */
import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { LayoutGrid, LayoutDashboard, Monitor, MonitorPlay, Settings2 } from 'lucide-react'
import { usePersistentState } from '../hooks/usePersistentState'
import LayoutChips from '../components/LayoutChips'
import OverlayLibrary from '../components/overlays-hub/OverlayLibrary'
import HubActionsMenu from '../components/overlays-hub/HubActionsMenu'
import ArrangeView from '../components/overlays-hub/ArrangeView'
import OverlaysDashboard from './OverlaysDashboard'
import { selectionFromParam, type HubSelection } from '../components/overlays-hub/selection'

type HubTab = 'library' | 'arrange' | 'dashboard'
const HUB_TABS: readonly HubTab[] = ['library', 'arrange', 'dashboard']

const TAB_DEFS: Array<{ id: HubTab; label: string; icon: React.ElementType }> = [
  { id: 'library', label: 'Library', icon: LayoutGrid },
  { id: 'arrange', label: 'Arrange', icon: Monitor },
  { id: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
]

export default function OverlaysHub(): React.ReactElement {
  const [tab, setTab] = usePersistentState<HubTab>('pq-tab:overlays', 'library', HUB_TABS)
  const [selected, setSelected] = useState<HubSelection | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()
  const panelParam = searchParams.get('panel')

  useEffect(() => {
    if (!panelParam) return
    const sel = selectionFromParam(panelParam)
    if (sel) {
      setTab('library')
      setSelected(sel)
    }
    setSearchParams({}, { replace: true })
  }, [panelParam, setTab, setSearchParams])

  const openGlobal = (): void => {
    setTab('library')
    setSelected({ kind: 'global' })
  }

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
        <div className="flex shrink-0 items-center gap-2">
          <button
            onClick={openGlobal}
            className="flex shrink-0 items-center gap-1.5 rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: selected?.kind === 'global' ? 'var(--color-surface-2)' : 'transparent',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
            title="Settings for every overlay: opacity, fade, alert text, lock modes"
          >
            <Settings2 size={12} />
            Overlay settings
          </button>
          <HubActionsMenu />
        </div>
      </div>
      <div className="min-h-0 flex-1">
        {tab === 'library' ? (
          <OverlayLibrary selected={selected} onSelect={setSelected} />
        ) : tab === 'arrange' ? (
          <ArrangeView
            onOpenSettings={(sel) => {
              setTab('library')
              setSelected(sel)
            }}
          />
        ) : (
          <OverlaysDashboard />
        )}
      </div>
    </div>
  )
}
