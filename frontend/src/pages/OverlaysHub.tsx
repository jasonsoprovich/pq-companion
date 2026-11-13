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
 *
 * While the active layout has no windows, a banner offers the overlay setup
 * wizard (until dismissed; the ⋯ menu always has it).
 */
import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { LayoutGrid, LayoutDashboard, Monitor, MonitorPlay, Settings2, Wand2, X } from 'lucide-react'
import { usePersistentState } from '../hooks/usePersistentState'
import { useOverlayLayouts } from '../hooks/useOverlayLayouts'
import LayoutChips from '../components/LayoutChips'
import OverlayLibrary from '../components/overlays-hub/OverlayLibrary'
import HubActionsMenu from '../components/overlays-hub/HubActionsMenu'
import ArrangeView from '../components/overlays-hub/ArrangeView'
import OverlaysDashboard from './OverlaysDashboard'
import { selectionFromParam, type HubSelection } from '../components/overlays-hub/selection'

type HubTab = 'library' | 'arrange' | 'dashboard'
const HUB_TABS: readonly HubTab[] = ['library', 'arrange', 'dashboard']

const SETUP_HINT_KEY = 'pq-overlay-setup-hint-dismissed'

function readHintDismissed(): boolean {
  try { return localStorage.getItem(SETUP_HINT_KEY) === '1' } catch { return false }
}

// Offers the setup wizard while the active layout is empty.
function SetupHint(): React.ReactElement | null {
  const { state, supported } = useOverlayLayouts()
  const [dismissed, setDismissed] = useState(readHintDismissed)
  const active = state?.layouts.find((l) => l.id === state.activeId)
  if (!supported || dismissed || !active || active.windows.length > 0) return null
  const dismiss = (): void => {
    setDismissed(true)
    try { localStorage.setItem(SETUP_HINT_KEY, '1') } catch { /* shows again next time */ }
  }
  return (
    <div
      className="mx-4 mt-3 flex shrink-0 items-center gap-3 rounded-lg px-3 py-2"
      style={{ backgroundColor: 'rgba(201,168,76,0.08)', border: '1px solid rgba(201,168,76,0.35)' }}
    >
      <Wand2 size={14} className="shrink-0" style={{ color: 'var(--color-primary)' }} />
      <span className="min-w-0 flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
        No overlays in {active.name} yet.{' '}
        <span style={{ color: 'var(--color-muted-foreground)' }}>
          Setup picks a set for your class and places them around your screen.
        </span>
      </span>
      <button
        onClick={() => window.dispatchEvent(new Event('pq:open-overlay-setup'))}
        className="shrink-0 rounded px-2.5 py-1 text-xs font-semibold"
        style={{ backgroundColor: 'var(--color-primary)', color: '#fff' }}
      >
        Set up overlays
      </button>
      <button
        onClick={dismiss}
        className="shrink-0 rounded p-1"
        style={{ color: 'var(--color-muted-foreground)' }}
        title="Hide this. Setup stays in the ⋯ menu."
        aria-label="Dismiss"
      >
        <X size={12} />
      </button>
    </div>
  )
}

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
      <SetupHint />
      <div className="min-h-0 flex-1">
        {tab === 'library' ? (
          <OverlayLibrary selected={selected} onSelect={setSelected} />
        ) : tab === 'arrange' ? (
          <ArrangeView />
        ) : (
          <OverlaysDashboard />
        )}
      </div>
    </div>
  )
}
