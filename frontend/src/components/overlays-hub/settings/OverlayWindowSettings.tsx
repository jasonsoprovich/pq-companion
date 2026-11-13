/**
 * Overlay window settings: lock mode, opacity and fade overrides, monitor.
 * OverlayLockModeCard is the all-overlays table (global overlay settings);
 * the selects are reused one overlay at a time in each overlay's settings.
 */
import React, { useEffect, useState } from 'react'
import { Crosshair, ExternalLink } from 'lucide-react'
import { OVERLAY_DEFS, resolveLockedMode } from '../../../lib/overlays'
import type { OverlayName, LockedMode, FadeMode } from '../../../lib/overlays'
import { DEV_HPS } from '../../../lib/devFlags'
import type { Config } from '../../../types/config'

// OverlayLockModeCard lets the user pick, per popout overlay, how it behaves
// while locked. The two modes trade off "click reaches the game" against
// "scroll / clear rows from the overlay" — see lib/overlays.ts. Rows are
// generated from OVERLAY_DEFS, so a newly registered overlay shows up here
// automatically with both options.
export const LOCK_MODE_OPTIONS: ReadonlyArray<{
  value: LockedMode
  label: string
  hint: string
}> = [
  {
    value: 'interactive',
    label: 'Interactive on hover',
    hint: 'Hover the overlay to scroll and clear individual rows; move away and clicks pass through to the game.',
  },
  {
    value: 'clickthrough',
    label: 'Click-through',
    hint: 'Only the title-bar buttons are clickable; scrolling and clicks everywhere else pass through to the game.',
  },
  {
    value: 'display-only',
    label: 'Display only',
    hint: 'Nothing ever captures the mouse — the title bar is hidden too. A pure HUD. To move it, use Reset position or the Monitor dropdown, or the Move button in the Dashboard tab’s “Manage overlays” menu.',
  },
]

// Maps each lockable overlay to the IPC toggle that opens/closes its floating
// popout window. Mirrors PANEL_POPOUT in OverlaysDashboard so popping out from
// Settings drives the exact same windows — and therefore the exact same
// persisted positions — as the Overlays tab.
const OVERLAY_POPOUT_TOGGLE: Record<OverlayName, () => void> = {
  dps:          () => { window.electron?.overlay?.toggleDPS() },
  hps:          () => { window.electron?.overlay?.toggleHPS() },
  buffTimer:    () => { window.electron?.overlay?.toggleBuffTimer() },
  filteredTimer: () => { window.electron?.overlay?.toggleFilteredTimer() },
  detrimTimer:  () => { window.electron?.overlay?.toggleDetrimTimer() },
  customTimer:  () => { window.electron?.overlay?.toggleCustomTimer() },
  npc:          () => { window.electron?.overlay?.toggleNPC() },
  threat:       () => { window.electron?.overlay?.toggleThreat() },
  rollTracker:  () => { window.electron?.overlay?.toggleRollTracker() },
  respawnTimer: () => { window.electron?.overlay?.toggleRespawnTimer() },
  liveMap: () => { window.electron?.overlay?.toggleLiveMap() },
  zoneLockouts: () => { window.electron?.overlay?.toggleZoneLockouts() },
  raidReadiness: () => { window.electron?.overlay?.toggleRaidReadiness() },
  chChain:      () => { window.electron?.overlay?.toggleCHChain() },
  chMetronome:  () => { window.electron?.overlay?.toggleCHMetronome() },
  discordVoice: () => { window.electron?.overlay?.toggleDiscordVoice() },
  killCounter:  () => { window.electron?.overlay?.toggleKillCounter() },
  itemInfo:     () => { window.electron?.overlay?.toggleItemInfo() },
  pet:          () => { window.electron?.overlay?.togglePet() },
  deathRecap:   () => { window.electron?.overlay?.toggleDeathRecap() },
}

// The HPS overlay can't be implemented correctly until EQ logs expose real
// heal data, so it's hidden everywhere (see DEV_HPS / SHOW_HPS) — including
// this lock-behaviour list. Flip VITE_DEV_HPS to surface it during dev.
const VISIBLE_OVERLAY_DEFS = DEV_HPS
  ? OVERLAY_DEFS
  : OVERLAY_DEFS.filter((def) => def.name !== 'hps')

// Per-overlay opacity steps. The global slider runs 10–100%, so these match.
const OVERLAY_OPACITY_STEPS = [10, 20, 30, 40, 50, 60, 70, 80, 90, 100]

// OverlayOpacitySelect overrides the global overlay opacity for one overlay.
// "Default" is stored as -1 rather than deleting the key, for the same reason
// the Fade dropdown stores '': the config PUT decodes over the existing map,
// so a removed key would survive server-side.
export function OverlayOpacitySelect({
  label,
  value,
  globalValue,
  onChange,
}: {
  label: string
  value: number | undefined
  globalValue: number
  onChange: (v: number) => void
}): React.ReactElement {
  const pct = value != null && value >= 0 ? Math.round(value * 100) : null
  // A value set some other way (an imported config) still shows as itself.
  const steps =
    pct != null && !OVERLAY_OPACITY_STEPS.includes(pct)
      ? [...OVERLAY_OPACITY_STEPS, pct].sort((a, b) => a - b)
      : OVERLAY_OPACITY_STEPS
  return (
    <select
      value={pct ?? ''}
      onChange={(e) => onChange(e.target.value === '' ? -1 : Number(e.target.value) / 100)}
      className="rounded px-1.5 py-1 outline-none"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-foreground)',
        cursor: 'pointer',
      }}
      title={`Background opacity of the ${label} overlay — Default follows the global Overlay Opacity setting`}
    >
      <option value="">Default ({Math.round(globalValue * 100)}%)</option>
      {steps.map((n) => (
        <option key={n} value={n}>{n}%</option>
      ))}
    </select>
  )
}

export function OverlayLockModeCard({
  offerItemInfo,
  modes,
  onChange,
  fadeModes,
  fadeGlobal,
  onFadeChange,
  opacities,
  opacityGlobal,
  onOpacityChange,
}: {
  offerItemInfo: boolean
  modes: Partial<Record<OverlayName, LockedMode>>
  onChange: (next: Partial<Record<OverlayName, LockedMode>>) => void
  fadeModes: Partial<Record<OverlayName, FadeMode | ''>>
  fadeGlobal: boolean
  onFadeChange: (next: Partial<Record<OverlayName, FadeMode | ''>>) => void
  opacities: Partial<Record<OverlayName, number>>
  opacityGlobal: number
  onOpacityChange: (next: Partial<Record<OverlayName, number>>) => void
}): React.ReactElement {
  const defs = VISIBLE_OVERLAY_DEFS.filter(
    (def) => def.name !== 'itemInfo' || offerItemInfo,
  )

  // Per-overlay popout open-state + the monitor each overlay currently sits on,
  // keyed by canonical overlay name. Polled like the dashboard's manager since
  // Electron doesn't push window-state changes.
  const [popoutStates, setPopoutStates] = useState<Record<string, boolean>>({})
  const [overlayDisplays, setOverlayDisplays] = useState<Record<string, number>>({})
  useEffect(() => {
    if (!window.electron?.overlay?.popoutStates) return
    let cancelled = false
    const check = (): void => {
      window.electron.overlay
        .popoutStates()
        .then((s) => { if (!cancelled) setPopoutStates(s) })
        .catch(() => {})
      window.electron?.overlay?.displayIds?.()
        .then((m) => { if (!cancelled && m) setOverlayDisplays(m) })
        .catch(() => {})
    }
    check()
    const id = setInterval(check, 1500)
    return () => { cancelled = true; clearInterval(id) }
  }, [])

  // Available monitors. Fetched once on mount (and the overlay-display poll
  // above keeps the per-overlay selection fresh). The Monitor dropdown only
  // appears when more than one display is present.
  const [displays, setDisplays] = useState<Array<{ id: number; label: string; isPrimary: boolean }>>([])
  useEffect(() => {
    let cancelled = false
    window.electron?.screen?.listDisplays?.()
      .then((list) => { if (!cancelled) setDisplays(list ?? []) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])
  const multiMonitor = displays.length > 1

  const moveToDisplay = (name: OverlayName, id: number): void => {
    window.electron?.overlay?.moveToDisplay?.(name, id)
    setOverlayDisplays((m) => ({ ...m, [name]: id })) // optimistic; poll reconciles
  }

  const togglePopout = (name: OverlayName): void => {
    OVERLAY_POPOUT_TOGGLE[name]()
    // Optimistic flip; the poll reconciles shortly after.
    setPopoutStates((s) => ({ ...s, [name]: !s[name] }))
  }

  const setAll = (mode: LockedMode): void => {
    const next: Partial<Record<OverlayName, LockedMode>> = {}
    for (const def of defs) next[def.name] = mode
    onChange(next)
  }

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        Overlay Windows
      </h2>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        When an overlay is locked it can&rsquo;t be moved or resized. Choose how
        each one reacts to the mouse while locked. To position one, use its{' '}
        <span style={{ color: 'var(--color-foreground)' }}>Pop out</span> button
        to open the window and drag it where you want. On a multi-monitor setup,
        use its <span style={{ color: 'var(--color-foreground)' }}>Monitor</span>{' '}
        dropdown to send it to another screen.{' '}
        <span style={{ color: 'var(--color-foreground)' }}>Reset position</span>{' '}
        recenters a drifted overlay on your primary monitor.{' '}
        <span style={{ color: 'var(--color-foreground)' }}>Opacity</span> and{' '}
        <span style={{ color: 'var(--color-foreground)' }}>Fade</span> override
        the global Overlay Opacity and Fade When Inactive settings for just
        that overlay — handy for keeping overlays over the game see-through
        while ones on another screen stay solid.
      </p>

      {/* Mode legend */}
      <div className="mb-3 flex flex-col gap-1">
        {LOCK_MODE_OPTIONS.map((opt) => (
          <p key={opt.value} className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span className="font-semibold" style={{ color: 'var(--color-foreground)' }}>
              {opt.label}:
            </span>{' '}
            {opt.hint}
          </p>
        ))}
      </div>

      {/* Set-all shortcut */}
      <div className="mb-3 flex items-center gap-2">
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Set all to
        </span>
        {LOCK_MODE_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            onClick={() => setAll(opt.value)}
            className="rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: 'pointer',
            }}
          >
            {opt.label}
          </button>
        ))}
      </div>

      {/* Per-overlay rows — a single grid so the Pop out / Reset / Monitor /
          Opacity / Fade / lock-mode columns line up cleanly across rows regardless of label
          length. The Monitor column only appears on multi-monitor setups. */}
      {/* Scrolls sideways rather than clipping when the window is too narrow
          for every column (the Monitor column adds one on multi-monitor
          setups). */}
      <div className="overflow-x-auto">
      <div
        className="text-xs"
        style={{
          display: 'grid',
          gridTemplateColumns: multiMonitor
            ? 'minmax(80px,1fr) auto auto auto auto auto auto'
            : 'minmax(80px,1fr) auto auto auto auto auto',
          columnGap: 10,
          rowGap: 8,
          alignItems: 'center',
        }}
      >
        {/* Column labels, so each dropdown can show just its value — with a
            "Opacity:" style prefix in every option the row ran past the
            card's edge. */}
        {['', '', '', ...(multiMonitor ? ['Monitor'] : []), 'Opacity', 'Fade', 'When locked'].map(
          (h, i) => (
            <span
              key={i}
              className="text-[10px] uppercase tracking-wide"
              style={{ color: 'var(--color-muted)' }}
            >
              {h}
            </span>
          ),
        )}
        {defs.map((def) => {
          const mode = resolveLockedMode(modes, def.name)
          return (
            <React.Fragment key={def.name}>
              <span className="text-sm truncate" style={{ color: 'var(--color-foreground)' }}>
                {def.label}
              </span>
              <button
                onClick={() => togglePopout(def.name)}
                className="flex items-center justify-center gap-1 rounded px-2 py-1"
                style={{
                  width: 84,
                  backgroundColor: popoutStates[def.name] ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: popoutStates[def.name] ? '#fff' : 'var(--color-muted-foreground)',
                  cursor: 'pointer',
                }}
                title={
                  popoutStates[def.name]
                    ? `Close the ${def.label} pop-out window`
                    : `Pop out ${def.label} as a floating window so you can position it`
                }
              >
                <ExternalLink size={11} />
                {popoutStates[def.name] ? 'Close' : 'Pop out'}
              </button>
              {/* "Reset" + Crosshair — the label makes the button's purpose
                  obvious; it stays compact enough to leave room for the
                  lock-mode toggle and Monitor dropdown without clipping. */}
              <button
                onClick={() => window.electron?.overlay?.resetPosition?.(def.name)}
                className="flex items-center justify-center gap-1 rounded px-2 py-1"
                style={{
                  width: 84,
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-muted-foreground)',
                  cursor: 'pointer',
                }}
                title={`Reset position — recenter the ${def.label} overlay on the primary monitor and unlock it`}
              >
                <Crosshair size={11} /> Reset
              </button>
              {multiMonitor && (
                <select
                  value={overlayDisplays[def.name] ?? ''}
                  onChange={(e) => moveToDisplay(def.name, Number(e.target.value))}
                  className="rounded px-1.5 py-1 outline-none"
                  style={{
                    maxWidth: 120,
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                    cursor: 'pointer',
                  }}
                  title={`Send the ${def.label} overlay to a specific monitor`}
                >
                  {displays.map((d, i) => (
                    <option key={d.id} value={d.id}>
                      {d.label || `Display ${i + 1}`}{d.isPrimary ? ' (primary)' : ''}
                    </option>
                  ))}
                </select>
              )}
              <OverlayOpacitySelect
                label={def.label}
                value={opacities[def.name]}
                globalValue={opacityGlobal}
                onChange={(v) => onOpacityChange({ ...opacities, [def.name]: v })}
              />
              <select
                value={fadeModes[def.name] ?? ''}
                // "Default" is stored as '' rather than deleting the key: the
                // config PUT decodes over the existing map, so a removed key
                // would survive server-side.
                onChange={(e) =>
                  onFadeChange({ ...fadeModes, [def.name]: e.target.value as FadeMode | '' })
                }
                className="rounded px-1.5 py-1 outline-none"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  cursor: 'pointer',
                }}
                title={`Fade the ${def.label} overlay when inactive — Default follows the global Fade When Inactive setting`}
              >
                <option value="">Default ({fadeGlobal ? 'on' : 'off'})</option>
                <option value="on">On</option>
                <option value="off">Off</option>
              </select>
              <LockModeSelect
                label={def.label}
                value={mode}
                onChange={(next) => onChange({ ...modes, [def.name]: next })}
              />
            </React.Fragment>
          )
        })}
      </div>
      </div>
    </section>
  )
}

// LockModeToggle is a two-segment control choosing between the locked modes.
// LockModeSelect picks an overlay's locked behaviour. A dropdown rather than
// three side-by-side buttons so the row fits beside the Opacity and Fade
// dropdowns; the buttons pushed it past the card's edge. The legend above
// the table still explains each mode.
export function LockModeSelect({
  label,
  value,
  onChange,
}: {
  label: string
  value: LockedMode
  onChange: (next: LockedMode) => void
}): React.ReactElement {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as LockedMode)}
      className="rounded px-1.5 py-1 outline-none"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-foreground)',
        cursor: 'pointer',
      }}
      title={`How the ${label} overlay reacts to the mouse while locked`}
    >
      {LOCK_MODE_OPTIONS.map((opt) => (
        <option key={opt.value} value={opt.value}>{opt.label}</option>
      ))}
    </select>
  )
}

// FadeSelect overrides Fade When Inactive for one overlay. "Default" is
// stored as '' rather than deleting the key (see OverlayOpacitySelect).
export function FadeSelect({
  label,
  value,
  globalOn,
  onChange,
}: {
  label: string
  value: FadeMode | '' | undefined
  globalOn: boolean
  onChange: (v: FadeMode | '') => void
}): React.ReactElement {
  return (
    <select
      value={value ?? ''}
      onChange={(e) => onChange(e.target.value as FadeMode | '')}
      className="rounded px-1.5 py-1 outline-none"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-foreground)',
        cursor: 'pointer',
      }}
      title={`Fade the ${label} overlay when inactive — Default follows the global Fade When Inactive setting`}
    >
      <option value="">Default ({globalOn ? 'on' : 'off'})</option>
      <option value="on">On</option>
      <option value="off">Off</option>
    </select>
  )
}

/**
 * OverlayWindowControls — one overlay's row of the Overlay Windows table, laid
 * out for its own settings drawer: lock mode, opacity, fade and (with more
 * than one monitor) which screen it's on.
 */
export function OverlayWindowControls({
  name,
  label,
  config,
  setConfig,
}: {
  name: OverlayName
  label: string
  config: Config
  setConfig: (c: Config) => void
}): React.ReactElement {
  const prefs = config.preferences
  const [displays, setDisplays] = useState<Array<{ id: number; label: string; isPrimary: boolean }>>([])
  const [displayId, setDisplayId] = useState<number | undefined>(undefined)
  useEffect(() => {
    let cancelled = false
    window.electron?.screen?.listDisplays?.()
      .then((list) => { if (!cancelled) setDisplays(list ?? []) })
      .catch(() => {})
    window.electron?.overlay?.displayIds?.()
      .then((m) => { if (!cancelled && m) setDisplayId(m[name]) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [name])

  const patch = (p: Partial<Config['preferences']>): void =>
    setConfig({ ...config, preferences: { ...prefs, ...p } })
  const mode = resolveLockedMode(prefs.overlay_locked_modes ?? {}, name)

  const row = (title: string, control: React.ReactNode, hint?: string): React.ReactElement => (
    <div className="flex items-center justify-between gap-3 py-1.5">
      <div className="min-w-0">
        <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>{title}</p>
        {hint && <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{hint}</p>}
      </div>
      <div className="shrink-0 text-xs">{control}</div>
    </div>
  )

  return (
    <div className="flex flex-col">
      {row(
        'When locked',
        <LockModeSelect
          label={label}
          value={mode}
          onChange={(next) => patch({ overlay_locked_modes: { ...(prefs.overlay_locked_modes ?? {}), [name]: next } })}
        />,
        LOCK_MODE_OPTIONS.find((o) => o.value === mode)?.hint,
      )}
      {row(
        'Opacity',
        <OverlayOpacitySelect
          label={label}
          value={prefs.overlay_opacities?.[name]}
          globalValue={prefs.overlay_opacity}
          onChange={(v) => patch({ overlay_opacities: { ...(prefs.overlay_opacities ?? {}), [name]: v } })}
        />,
      )}
      {row(
        'Fade when inactive',
        <FadeSelect
          label={label}
          value={prefs.overlay_fade_modes?.[name]}
          globalOn={Boolean(prefs.overlay_fade_enabled)}
          onChange={(v) => patch({ overlay_fade_modes: { ...(prefs.overlay_fade_modes ?? {}), [name]: v } })}
        />,
      )}
      {displays.length > 1 &&
        row(
          'Monitor',
          <select
            value={displayId ?? ''}
            onChange={(e) => {
              const id = Number(e.target.value)
              window.electron?.overlay?.moveToDisplay?.(name, id)
              setDisplayId(id)
            }}
            className="rounded px-1.5 py-1 outline-none"
            style={{
              maxWidth: 160,
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
            title={`Send the ${label} overlay to a specific monitor`}
          >
            {displays.map((d, i) => (
              <option key={d.id} value={d.id}>
                {d.label || `Display ${i + 1}`}{d.isPrimary ? ' (primary)' : ''}
              </option>
            ))}
          </select>,
        )}
    </div>
  )
}
