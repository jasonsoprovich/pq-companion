/**
 * OverlaySetupWizard — picks a starting set of overlays and sets them up.
 * Runs straight after first-launch setup, and again from the Overlays hub.
 *
 * Steps:
 * 0. Layout: make a new layout or replace the current one's windows (only
 *    asked when the current layout already has windows).
 * 1. Overlays: every overlay in the Library, with a set pre-ticked for the
 *    character's class (see CLASS_PICKS).
 * 2. Monitor: which screen they go on (skipped with a single monitor).
 * 3. Options: the show/hide-all hotkey and collapse when idle.
 * 4. Arrange: the overlays open around that monitor's edges (main's
 *    overlay:setup:apply) and the Arrange map lets you move them.
 */
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Keyboard, Layers, LayoutGrid, Loader2, Monitor } from 'lucide-react'
import Wizard from '../guide/Wizard'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import { getConfig, listCharacters } from '../../services/api'
import { OVERLAY_CATALOG, OVERLAY_CATEGORIES } from '../../lib/overlayCatalog'
import { CLASS_NAMES } from '../../lib/spellHelpers'
import {
  SUGGESTED_HOTKEYS,
  TOGGLE_ALL_ACTION,
  acceleratorFromEvent,
  acceleratorParts,
  eqConflictWarning,
} from '../../lib/hotkeys'
import { setIdleCollapseForAll } from './HubActionsMenu'
import ArrangeView from './ArrangeView'
import type { OverlayName } from '../../lib/overlays'

type Step = 'layout' | 'pick' | 'monitor' | 'options' | 'arrange'

const STEP_TITLES: Record<Step, string> = {
  layout: 'Layout',
  pick: 'Overlays',
  monitor: 'Monitor',
  options: 'Options',
  arrange: 'Arrange',
}

// Pre-ticked for everyone, then by class (CLASS_NAMES order). HPS and Threat
// are never pre-ticked: both work from limited log data.
const BASE_PICKS: OverlayName[] = ['buffTimer', 'npc']
const CLASS_PICKS: Record<string, OverlayName[]> = {
  WAR: ['dps'],
  PAL: ['detrimTimer', 'dps'],
  SHD: ['detrimTimer', 'dps'],
  CLR: ['chChain', 'detrimTimer'],
  DRU: ['detrimTimer'],
  SHM: ['detrimTimer'],
  ENC: ['detrimTimer'],
  BRD: ['detrimTimer'],
  NEC: ['pet', 'detrimTimer', 'dps'],
  MAG: ['pet', 'detrimTimer', 'dps'],
  BST: ['pet', 'detrimTimer', 'dps'],
  MNK: ['dps'],
  ROG: ['dps'],
  RNG: ['dps'],
  WIZ: ['dps'],
}

const CLASS_FULL: Record<string, string> = {
  WAR: 'Warrior', CLR: 'Cleric', PAL: 'Paladin', RNG: 'Ranger', SHD: 'Shadow Knight',
  DRU: 'Druid', MNK: 'Monk', BRD: 'Bard', ROG: 'Rogue', SHM: 'Shaman',
  NEC: 'Necromancer', WIZ: 'Wizard', MAG: 'Magician', ENC: 'Enchanter', BST: 'Beastlord',
}

export function classPicks(classIndex: number): OverlayName[] {
  const abbr = CLASS_NAMES[classIndex]
  return [...BASE_PICKS, ...(abbr ? CLASS_PICKS[abbr] ?? [] : ['dps' as OverlayName])]
}

type DisplayInfo = { id: number; label: string; isPrimary: boolean; width: number; height: number }

function Kbd({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <kbd
      className="rounded px-1.5 py-px font-mono text-[11px]"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
        borderBottomWidth: 2,
        color: 'var(--color-foreground)',
      }}
    >
      {children}
    </kbd>
  )
}

// Records one key combo, releasing the app's hotkeys while it listens so a
// combo that's already bound reaches the wizard.
function HotkeyField({
  value,
  taken,
  onChange,
}: {
  value: string
  // Combos bound to other actions.
  taken: Record<string, string>
  onChange: (accelerator: string) => void
}): React.ReactElement {
  const api = window.electron?.overlay
  const [recording, setRecording] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const recordingRef = useRef(false)

  const stop = useCallback(() => {
    if (recordingRef.current) void api?.hotkeysSuspend(false)
    recordingRef.current = false
    setRecording(false)
  }, [api])

  useEffect(() => stop, [stop])

  useEffect(() => {
    if (!recording) return
    const onKey = (e: KeyboardEvent): void => {
      e.preventDefault()
      e.stopPropagation()
      if (e.key === 'Escape' && !e.ctrlKey && !e.altKey && !e.shiftKey) {
        stop()
        return
      }
      if ((e.key === 'Backspace' || e.key === 'Delete') && !e.ctrlKey && !e.altKey) {
        stop()
        onChange('')
        return
      }
      const result = acceleratorFromEvent(e)
      if (result.kind === 'pending') return
      if (result.kind === 'invalid') {
        setError(result.reason)
        return
      }
      if (taken[result.accelerator]) {
        setError(`${result.accelerator} is already used for "${taken[result.accelerator]}".`)
        return
      }
      setError(null)
      stop()
      onChange(result.accelerator)
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [recording, taken, onChange, stop])

  const warning = value ? eqConflictWarning(value) : null
  return (
    <div className="space-y-1.5">
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => {
            if (recording) {
              stop()
              return
            }
            void api?.hotkeysSuspend(true)
            recordingRef.current = true
            setRecording(true)
            setError(null)
          }}
          className="flex min-w-[9rem] items-center justify-center gap-1 rounded px-3 py-1.5 text-xs"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: `1px solid ${recording ? 'var(--color-primary)' : 'var(--color-border)'}`,
            color: 'var(--color-foreground)',
            cursor: 'pointer',
          }}
        >
          {recording ? (
            <span style={{ color: 'var(--color-primary)' }}>Press a key combo…</span>
          ) : value ? (
            acceleratorParts(value).map((p, i) => <Kbd key={i}>{p}</Kbd>)
          ) : (
            <span style={{ color: 'var(--color-muted-foreground)' }}>None — click to set</span>
          )}
        </button>
        {value && !recording && (
          <button
            type="button"
            onClick={() => onChange('')}
            className="text-xs underline"
            style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
          >
            Clear
          </button>
        )}
      </div>
      {recording && (
        <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Esc cancels, Backspace clears.
        </p>
      )}
      {error && <p className="text-[11px]" style={{ color: '#f87171' }}>{error}</p>}
      {warning && !error && <p className="text-[11px]" style={{ color: '#fbbf24' }}>{warning}</p>}
    </div>
  )
}

export default function OverlaySetupWizard({ onClose }: { onClose: () => void }): React.ReactElement {
  const api = window.electron?.overlay
  const [loading, setLoading] = useState(true)
  const [classIndex, setClassIndex] = useState(-1)
  const [picked, setPicked] = useState<Set<OverlayName>>(new Set())
  const [displays, setDisplays] = useState<DisplayInfo[]>([])
  const [displayId, setDisplayId] = useState<number | null>(null)
  const [hotkey, setHotkey] = useState('')
  // Other actions' combos, for the conflict check: accelerator → action.
  const [taken, setTaken] = useState<Record<string, string>>({})
  const [idleOn, setIdleOn] = useState(false)
  // The active layout, and whether to replace its windows or make a new one.
  const [current, setCurrent] = useState<{ name: string; windows: number } | null>(null)
  const [mode, setMode] = useState<'new' | 'replace'>('new')
  const [newName, setNewName] = useState('')
  const [step, setStep] = useState<Step | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEscapeToClose(onClose, !saving)

  useEffect(() => {
    const load = async (): Promise<void> => {
      const [cfg, chars, arrange, hotkeys, states, layouts] = await Promise.all([
        getConfig().catch(() => null),
        listCharacters().catch(() => null),
        api?.arrangeGet?.().catch(() => null),
        api?.hotkeysGet?.().catch(() => null),
        api?.popoutStates?.().catch(() => null),
        api?.layoutsGet?.().catch(() => null),
      ])
      const active = layouts?.layouts.find((l) => l.id === layouts.activeId)
      const hasWindows = !!active && active.windows.length > 0
      setCurrent(active ? { name: active.name, windows: active.windows.length } : null)
      setMode(hasWindows ? 'new' : 'replace')
      setStep(hasWindows ? 'layout' : 'pick')
      // The active character's class, else the one saved in first-launch setup.
      const activeChar = chars?.characters.find((c) => c.name === chars.active)
      const cls = activeChar && activeChar.class >= 0 ? activeChar.class : cfg?.character_class ?? -1
      setClassIndex(cls)
      const clsName = CLASS_FULL[CLASS_NAMES[cls] ?? '']
      setNewName(clsName ? `${clsName} setup` : 'My overlays')
      // Already-open overlays stay ticked alongside the class picks.
      const open = Object.entries(states ?? {}).filter(([, on]) => on).map(([k]) => k)
      const known = new Set(OVERLAY_CATALOG.map((e) => e.name as string))
      setPicked(new Set([...classPicks(cls), ...open.filter((k) => known.has(k))] as OverlayName[]))
      const ds = (arrange?.displays ?? []).map((d) => ({
        id: d.id,
        label: d.label,
        isPrimary: d.isPrimary,
        width: d.bounds.width,
        height: d.bounds.height,
      }))
      setDisplays(ds)
      setDisplayId((ds.find((d) => d.isPrimary) ?? ds[0])?.id ?? null)
      const bindings = hotkeys?.bindings ?? {}
      setHotkey(bindings[TOGGLE_ALL_ACTION] ?? SUGGESTED_HOTKEYS[TOGGLE_ALL_ACTION])
      setTaken(
        Object.fromEntries(
          Object.entries(bindings)
            .filter(([a]) => a !== TOGGLE_ALL_ACTION)
            .map(([a, acc]) => [acc, a.replace(/^overlay:/, '').replace(/^layout:/, 'layout ')]),
        ),
      )
      const idle = cfg?.preferences.overlay_idle_collapse ?? {}
      setIdleOn(Object.values(idle).some((v) => v > 0))
      setLoading(false)
    }
    void load()
  }, [api])

  const askLayout = !!current && current.windows > 0
  const steps: Step[] = useMemo(
    () => [
      ...(askLayout ? ['layout' as Step] : []),
      'pick',
      ...(displays.length > 1 ? ['monitor' as Step] : []),
      'options',
      'arrange',
    ],
    [askLayout, displays.length],
  )
  const stepIndex = step ? steps.indexOf(step) : 0
  const placed = step === 'arrange'
  const abbr = CLASS_NAMES[classIndex]
  const suggested = useMemo(() => new Set(classPicks(classIndex)), [classIndex])

  const toggle = (name: OverlayName): void =>
    setPicked((s) => {
      const next = new Set(s)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })

  // Saves the options and opens the picks in place, then shows Arrange.
  const apply = async (): Promise<void> => {
    if (!api) return
    setSaving(true)
    setError(null)
    try {
      await api.hotkeysSet(TOGGLE_ALL_ACTION, hotkey || null)
      await setIdleCollapseForAll(idleOn)
      // Trigger alerts go on the game's monitor too.
      if (displayId !== null && displays.length > 1) await api.setDisplay(displayId)
      await api.setupApply({
        keys: OVERLAY_CATALOG.filter((e) => picked.has(e.name)).map((e) => e.name),
        displayId,
        mode,
        name: newName,
      })
      setStep('arrange')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Wizard
      icon={<LayoutGrid size={20} />}
      title={`Overlay Setup${step ? ` — ${STEP_TITLES[step]}` : ''}`}
      steps={steps.map((s) => ({ id: s, label: STEP_TITLES[s] }))}
      currentStep={step}
      showStepLabels
      onClose={onClose}
      closeDisabled={saving}
      closeTitle="Skip — set up overlays later from the Overlays page"
      wide={placed}
      bodyClassName={placed ? 'flex min-h-0 flex-1 flex-col' : 'flex-1 overflow-y-auto px-6 py-5'}
      onBack={() => setStep(steps[stepIndex - 1])}
      backDisabled={saving || placed}
      backHidden={placed}
      footerCenter={
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          {picked.size} overlay{picked.size === 1 ? '' : 's'} picked
        </span>
      }
      primary={
        step !== 'options' && step !== 'arrange'
          ? {
              label: 'Next',
              onClick: () => setStep(steps[stepIndex + 1]),
              disabled: loading || !step || (step === 'pick' && picked.size === 0),
            }
          : {
              label: saving ? 'Opening…' : placed ? 'Done' : 'Place overlays',
              onClick: () => (placed ? onClose() : void apply()),
              busy: saving,
              variant: 'finish',
            }
      }
    >
      {loading || !step ? (
        <div className="flex items-center gap-2 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          <Loader2 size={14} className="animate-spin" /> Loading…
        </div>
      ) : step === 'layout' ? (
        <div className="space-y-3">
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Your current layout, {current?.name}, already has {current?.windows} window
            {current?.windows === 1 ? '' : 's'}.{' '}
            <span style={{ color: 'var(--color-muted-foreground)' }}>
              Switch between layouts any time from the Overlays page.
            </span>
          </p>
          {([
            ['new', 'Make a new layout', `Keeps ${current?.name} as it is and switches to the new one.`],
            ['replace', `Replace ${current?.name}`, 'Closes its windows and puts the new picks in their place.'],
          ] as const).map(([value, label, hint]) => {
            const on = mode === value
            return (
              <label
                key={value}
                className="flex cursor-pointer items-start gap-3 rounded-md px-3 py-2.5"
                style={{
                  backgroundColor: on ? 'var(--color-surface-2)' : 'transparent',
                  border: `1px solid ${on ? 'var(--color-primary)' : 'var(--color-border)'}`,
                }}
              >
                <input
                  type="radio"
                  name="overlay-setup-mode"
                  checked={on}
                  onChange={() => setMode(value)}
                  className="mt-1"
                  style={{ accentColor: 'var(--color-primary)' }}
                />
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
                    {label}
                  </span>
                  <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    {hint}
                  </span>
                  {value === 'new' && on && (
                    <input
                      value={newName}
                      onChange={(e) => setNewName(e.target.value)}
                      maxLength={40}
                      placeholder="Layout name"
                      className="mt-2 w-full max-w-xs rounded px-2 py-1 text-sm"
                      style={{
                        backgroundColor: 'var(--color-background)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                    />
                  )}
                </span>
              </label>
            )
          })}
        </div>
      ) : step === 'arrange' ? (
        <>
          <p className="flex items-center gap-2 px-6 pt-4 text-sm" style={{ color: 'var(--color-foreground)' }}>
            <span>
              Your overlays are open around the edges of the screen. Drag them here or on screen
              to where you want them.
            </span>
          </p>
          <p className="flex items-center gap-1.5 px-6 pt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <Layers size={12} className="shrink-0" />
            Tip: drop one overlay on another&rsquo;s title strip to stack them as tabs in one window.
          </p>
          <div className="h-[56vh] min-h-[320px]">
            <ArrangeView compact />
          </div>
        </>
      ) : step === 'pick' ? (
        <div className="space-y-4">
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Pick the overlays to start with.{' '}
            <span style={{ color: 'var(--color-muted-foreground)' }}>
              {abbr
                ? `The ticked ones suit ${/^[AEIOU]/.test(CLASS_FULL[abbr] ?? abbr) ? 'an' : 'a'} ${CLASS_FULL[abbr] ?? abbr}.`
                : 'Set your class in Settings for picks that suit it.'}{' '}
              You can add or remove any overlay later from the Overlays page.
            </span>
          </p>
          {OVERLAY_CATEGORIES.map((cat) => {
            const entries = OVERLAY_CATALOG.filter((e) => e.category === cat.id)
            if (entries.length === 0) return null
            return (
              <div key={cat.id}>
                <h3
                  className="mb-1.5 text-[11px] font-semibold uppercase tracking-wide"
                  style={{ color: 'var(--color-muted)' }}
                >
                  {cat.label}
                </h3>
                <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                  {entries.map((e) => {
                    const on = picked.has(e.name)
                    const Icon = e.icon
                    return (
                      <label
                        key={e.name}
                        className="flex cursor-pointer items-start gap-2 rounded-md px-2.5 py-2"
                        style={{
                          backgroundColor: on ? 'var(--color-surface-2)' : 'transparent',
                          border: `1px solid ${on ? e.color + '80' : 'var(--color-border)'}`,
                        }}
                      >
                        <input
                          type="checkbox"
                          checked={on}
                          onChange={() => toggle(e.name)}
                          className="mt-0.5 shrink-0"
                          style={{ accentColor: 'var(--color-primary)' }}
                        />
                        <Icon size={14} className="mt-0.5 shrink-0" style={{ color: e.color }} />
                        <span className="min-w-0">
                          <span
                            className="flex items-center gap-1.5 text-xs font-semibold"
                            style={{ color: 'var(--color-foreground)' }}
                          >
                            {e.label}
                            {suggested.has(e.name) && (
                              <span
                                className="rounded px-1 text-[9px] font-medium uppercase"
                                style={{ backgroundColor: 'rgba(201,168,76,0.15)', color: '#c9a84c' }}
                              >
                                Suggested
                              </span>
                            )}
                          </span>
                          <span
                            className="block text-[11px] leading-snug"
                            style={{ color: 'var(--color-muted-foreground)' }}
                          >
                            {e.blurb}
                          </span>
                        </span>
                      </label>
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      ) : step === 'monitor' ? (
        <div className="space-y-3">
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Which monitor is EverQuest on?{' '}
            <span style={{ color: 'var(--color-muted-foreground)' }}>
              The overlays open there. You can move any of them to another monitor later.
            </span>
          </p>
          <div className="space-y-1.5">
            {displays.map((d) => {
              const on = d.id === displayId
              return (
                <label
                  key={d.id}
                  className="flex cursor-pointer items-center gap-3 rounded-md px-3 py-2.5"
                  style={{
                    backgroundColor: on ? 'var(--color-surface-2)' : 'transparent',
                    border: `1px solid ${on ? 'var(--color-primary)' : 'var(--color-border)'}`,
                  }}
                >
                  <input
                    type="radio"
                    name="overlay-monitor"
                    checked={on}
                    onChange={() => setDisplayId(d.id)}
                    style={{ accentColor: 'var(--color-primary)' }}
                  />
                  <Monitor size={16} style={{ color: 'var(--color-muted-foreground)' }} />
                  <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                    {d.label}
                    {d.isPrimary ? ' (main)' : ''}
                  </span>
                  <span className="ml-auto text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    {d.width}×{d.height}
                  </span>
                </label>
              )
            })}
          </div>
        </div>
      ) : (
        <div className="space-y-5">
          <div className="space-y-2">
            <h3 className="flex items-center gap-2 text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
              <Keyboard size={14} /> Show/hide hotkey
            </h3>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Hides every overlay at once and brings them back, from in game. Change it or add
              hotkeys for single overlays in Settings → Hotkeys.
            </p>
            <HotkeyField value={hotkey} taken={taken} onChange={setHotkey} />
          </div>
          <label className="flex cursor-pointer items-start gap-2.5">
            <input
              type="checkbox"
              checked={idleOn}
              onChange={(e) => setIdleOn(e.target.checked)}
              className="mt-0.5"
              style={{ accentColor: 'var(--color-primary)' }}
            />
            <span>
              <span className="block text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
                Collapse overlays when idle
              </span>
              <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Overlays with nothing to show (no timers running, no target, no fight) fold to
                their title bar after a few seconds and open again when something happens.
                Change the delay per overlay on the Overlays page.
              </span>
            </span>
          </label>
          {error && <p className="text-xs" style={{ color: '#f87171' }}>{error}</p>}
        </div>
      )}
    </Wizard>
  )
}
