import React, { useEffect, useState } from 'react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import {
  Sparkles,
  FolderOpen,
  CheckCircle2,
  AlertTriangle,
  Loader2,
  ArrowRight,
  ArrowLeft,
  Users,
  Info,
  X,
} from 'lucide-react'
import {
  detectZeal,
  getConfig,
  updateConfig,
  validateEQPath,
  getBackfillInfo,
  runBackfill,
  type DiscoveredCharacter,
  type EqDiagnostics,
} from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { Config } from '../types/config'
import type { ZealInstallStatus } from '../types/zeal'
import { charClassLabel, charClassOptions } from '../lib/enumsCache'

const ZEAL_RELEASE_URL = 'https://github.com/CoastalRedwood/Zeal/releases/latest'

type Step = 'welcome' | 'eq-path' | 'character' | 'zeal' | 'confirm'

const STEPS: Step[] = ['welcome', 'eq-path', 'character', 'zeal', 'confirm']

const STEP_TITLES: Record<Step, string> = {
  welcome: 'Welcome',
  'eq-path': 'EverQuest Folder',
  character: 'Character',
  zeal: 'Zeal Integration',
  confirm: 'All Set',
}

interface OnboardingWizardProps {
  onComplete: () => void
  onCancel?: () => void
  allowCancel?: boolean
}

export default function OnboardingWizard({
  onComplete,
  onCancel,
  allowCancel = false,
}: OnboardingWizardProps): React.ReactElement {
  const [step, setStep] = useState<Step>('welcome')
  const [config, setConfig] = useState<Config | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)

  // Only allow Esc to dismiss when cancelling is permitted — first-run setup
  // is mandatory and shouldn't be escapable.
  useEscapeToClose(() => onCancel?.(), allowCancel && !!onCancel)

  const [eqPath, setEqPath] = useState('')
  const [validating, setValidating] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)
  const [eqDiag, setEqDiag] = useState<EqDiagnostics | null>(null)
  const [discovered, setDiscovered] = useState<DiscoveredCharacter[]>([])
  const [pathConfirmed, setPathConfirmed] = useState(false)

  const [character, setCharacter] = useState('')
  const [characterClass, setCharacterClass] = useState<number>(-1)

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Optional log backfill on finish (only offered when logs were detected).
  const [backfillOptIn, setBackfillOptIn] = useState(true)
  const [backfilling, setBackfilling] = useState(false)
  const [backfillPct, setBackfillPct] = useState(0)

  const [zealStatus, setZealStatus] = useState<ZealInstallStatus | null>(null)
  const [zealChecking, setZealChecking] = useState(false)
  const [zealError, setZealError] = useState<string | null>(null)

  useEffect(() => {
    getConfig()
      .then((c) => {
        setConfig(c)
        if (c.eq_path) setEqPath(c.eq_path)
        if (c.character) setCharacter(c.character)
        if (typeof c.character_class === 'number') setCharacterClass(c.character_class)
      })
      .catch((err: Error) => setLoadError(err.message))
  }, [])

  async function checkZeal(path: string): Promise<void> {
    if (!path.trim()) {
      setZealStatus(null)
      return
    }
    setZealChecking(true)
    setZealError(null)
    try {
      setZealStatus(await detectZeal(path.trim()))
    } catch (err) {
      setZealError((err as Error).message)
      setZealStatus(null)
    } finally {
      setZealChecking(false)
    }
  }

  useEffect(() => {
    if (step === 'zeal') {
      void checkZeal(eqPath)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step])

  const stepIndex = STEPS.indexOf(step)
  const hasElectronDialog = Boolean(window.electron?.dialog)

  function goNext(): void {
    const next = STEPS[stepIndex + 1]
    if (next) setStep(next)
  }

  function goBack(): void {
    const prev = STEPS[stepIndex - 1]
    if (prev) setStep(prev)
  }

  async function handleBrowse(): Promise<void> {
    if (!window.electron?.dialog) return
    const folder = await window.electron.dialog.selectFolder()
    if (folder) {
      setEqPath(folder)
      setPathConfirmed(false)
      setValidationError(null)
      setDiscovered([])
    }
  }

  async function handleValidate(): Promise<void> {
    if (!eqPath.trim()) {
      setValidationError('Please enter or browse to your EverQuest folder')
      return
    }
    setValidating(true)
    setValidationError(null)
    try {
      const result = await validateEQPath(eqPath.trim())
      setEqDiag(result.diagnostics ?? null)
      if (!result.valid) {
        setValidationError(result.error ?? 'Folder is not a valid EverQuest installation')
        setDiscovered(result.characters)
        setPathConfirmed(false)
      } else {
        setDiscovered(result.characters)
        setPathConfirmed(true)
        // Pre-select the most recent character if none chosen yet
        if (!character && result.characters.length > 0) {
          setCharacter(result.characters[0].name)
        }
      }
    } catch (err) {
      setValidationError((err as Error).message)
      setPathConfirmed(false)
    } finally {
      setValidating(false)
    }
  }

  // Live backfill progress while finishing.
  useWebSocket((msg) => {
    if (msg.type !== WSEvent.BackfillProgress) return
    const d = msg.data as { done: number; total: number }
    if (d && d.total > 0) setBackfillPct(Math.min(100, Math.round((d.done / d.total) * 100)))
  })

  async function handleFinish(): Promise<void> {
    if (!config) return
    setSaving(true)
    setSaveError(null)
    try {
      const updated: Config = {
        ...config,
        eq_path: eqPath.trim(),
        character: character.trim(),
        character_class: characterClass,
        onboarding_completed: true,
      }
      await updateConfig(updated)

      // Optional: backfill the trackers from this character's existing log.
      // Non-fatal — a failure here shouldn't block finishing setup.
      if (backfillOptIn && discovered.length > 0 && character.trim()) {
        try {
          setBackfilling(true)
          setBackfillPct(0)
          const info = await getBackfillInfo()
          const sections = info.sections.map((s) => s.key)
          if (sections.length > 0) {
            await runBackfill(character.trim(), sections)
          }
        } catch {
          // ignore — user can backfill later from Settings → Logs
        } finally {
          setBackfilling(false)
        }
      }
      onComplete()
    } catch (err) {
      setSaveError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const canAdvance = ((): boolean => {
    switch (step) {
      case 'welcome':
        return true
      case 'eq-path':
        return pathConfirmed
      case 'character':
        return character.trim().length > 0
      case 'zeal':
        return true
      case 'confirm':
        return true
    }
  })()

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(2px)' }}
    >
      <div
        className="flex w-full max-w-2xl flex-col rounded-lg shadow-2xl"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          maxHeight: '90vh',
        }}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between border-b px-6 py-4"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <div className="flex items-center gap-3">
            <Sparkles size={20} style={{ color: 'var(--color-primary)' }} />
            <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
              PQ Companion Setup — {STEP_TITLES[step]}
            </h2>
          </div>
          {allowCancel && onCancel && (
            <button
              onClick={onCancel}
              className="rounded p-1"
              style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
              title="Close"
            >
              <X size={16} />
            </button>
          )}
        </div>

        {/* Step indicator */}
        <div
          className="flex items-center gap-2 border-b px-6 py-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {STEPS.map((s, i) => (
            <React.Fragment key={s}>
              <div
                className="flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold"
                style={{
                  backgroundColor:
                    i <= stepIndex ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: i <= stepIndex ? '#fff' : 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {i + 1}
              </div>
              {i < STEPS.length - 1 && (
                <div
                  className="h-px flex-1"
                  style={{
                    backgroundColor:
                      i < stepIndex ? 'var(--color-primary)' : 'var(--color-border)',
                  }}
                />
              )}
            </React.Fragment>
          ))}
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-6 py-6">
          {loadError && (
            <div
              className="mb-4 flex items-center gap-2 rounded p-3 text-sm"
              style={{ backgroundColor: 'rgba(248,113,113,0.1)', color: '#f87171' }}
            >
              <AlertTriangle size={14} />
              Failed to load config: {loadError}
            </div>
          )}

          {step === 'welcome' && (
            <div className="space-y-4">
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Welcome to PQ Companion — a desktop helper for the Project Quarm
                EverQuest emulated server.
              </p>
              <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                This quick setup will:
              </p>
              <ul
                className="ml-4 list-disc space-y-1 text-sm"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                <li>Locate your EverQuest installation folder</li>
                <li>Detect your characters from EQ log files</li>
                <li>Optionally configure Zeal integration</li>
              </ul>
              <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                You can re-run this wizard anytime from the Settings tab.
              </p>
            </div>
          )}

          {step === 'eq-path' && (
            <div className="space-y-4">
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Choose the folder where EverQuest is installed (e.g. <code>C:\EverQuest</code>).
                We&apos;ll look for <code>eqlog_*_pq.proj.txt</code> files inside it.
              </p>
              <div className="flex gap-2">
                <input
                  type="text"
                  value={eqPath}
                  onChange={(e) => {
                    setEqPath(e.target.value)
                    setPathConfirmed(false)
                    setValidationError(null)
                  }}
                  placeholder="e.g. C:\EverQuest"
                  className="flex-1 rounded px-3 py-2 text-sm"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                    outline: 'none',
                  }}
                />
                {hasElectronDialog && (
                  <button
                    onClick={handleBrowse}
                    className="flex items-center gap-1.5 rounded px-3 py-2 text-sm font-medium"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--color-foreground)',
                      cursor: 'pointer',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    <FolderOpen size={14} />
                    Browse
                  </button>
                )}
                <button
                  onClick={handleValidate}
                  disabled={validating || !eqPath.trim()}
                  className="flex items-center gap-1.5 rounded px-3 py-2 text-sm font-semibold"
                  style={{
                    backgroundColor: 'var(--color-primary)',
                    color: '#fff',
                    border: 'none',
                    cursor: validating || !eqPath.trim() ? 'not-allowed' : 'pointer',
                    opacity: validating || !eqPath.trim() ? 0.6 : 1,
                  }}
                >
                  {validating ? <Loader2 size={14} className="animate-spin" /> : null}
                  {validating ? 'Checking…' : 'Validate'}
                </button>
              </div>

              {pathConfirmed && (
                <div
                  className="flex items-start gap-2 rounded p-3 text-sm"
                  style={{ backgroundColor: 'rgba(34,197,94,0.1)', color: '#22c55e' }}
                >
                  <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                  <div>
                    <p>Folder looks good!</p>
                    <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      Found {discovered.length} character{discovered.length === 1 ? '' : 's'}:{' '}
                      {discovered.slice(0, 5).map((d) => d.name).join(', ')}
                      {discovered.length > 5 ? ', …' : ''}
                    </p>
                  </div>
                </div>
              )}

              {validationError && (
                <div
                  className="flex items-start gap-2 rounded p-3 text-sm"
                  style={{ backgroundColor: 'rgba(248,113,113,0.1)', color: '#f87171' }}
                >
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                  <p>{validationError}</p>
                </div>
              )}

              {/* Read-only diagnostics so the user can see exactly what's
                  wrong; the actual toggles live in Settings (the EQ path must
                  be saved first). */}
              {eqDiag && !pathConfirmed && (
                <div className="rounded p-3 text-xs" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
                  <DiagRow label="EverQuest logging" on={eqDiag.log_enabled} offText={eqDiag.log_found ? 'off' : 'not set'} />
                  {eqDiag.zeal_installed && (
                    <DiagRow label="Zeal output on camp" on={eqDiag.export_on_camp} offText="off" />
                  )}
                  {eqDiag.zeal_installed && (
                    <DiagRow label={`Zeal ${eqDiag.zeal_version || 'version'}`} on={eqDiag.zeal_version_ok} offText="update recommended" />
                  )}
                  <p className="mt-2" style={{ color: 'var(--color-muted)' }}>
                    You can turn logging and Zeal output on from Settings once setup is complete.
                  </p>
                </div>
              )}
            </div>
          )}

          {step === 'character' && (
            <div className="space-y-4">
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Pick the character you play most often. PQ Companion will use this
                to tail the right log file and pre-select your class in spell views.
              </p>

              {discovered.length > 0 && (
                <div className="space-y-2">
                  <label className="text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                    Detected from log files
                  </label>
                  <div className="flex flex-wrap gap-2">
                    {discovered.map((d) => {
                      const selected = character === d.name
                      return (
                        <button
                          key={d.name}
                          onClick={() => setCharacter(d.name)}
                          className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm"
                          style={{
                            backgroundColor: selected
                              ? 'var(--color-primary)'
                              : 'var(--color-surface-2)',
                            color: selected ? '#fff' : 'var(--color-foreground)',
                            border: '1px solid var(--color-border)',
                            cursor: 'pointer',
                          }}
                        >
                          <Users size={12} />
                          {d.name}
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}

              <div className="space-y-2">
                <label className="text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                  Character name
                </label>
                <input
                  type="text"
                  value={character}
                  onChange={(e) => setCharacter(e.target.value)}
                  placeholder="e.g. Osui"
                  className="w-full rounded px-3 py-2 text-sm"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                    outline: 'none',
                  }}
                />
              </div>

              <div className="space-y-2">
                <label className="text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
                  Class (optional)
                </label>
                <select
                  value={characterClass}
                  onChange={(e) => setCharacterClass(Number(e.target.value))}
                  className="w-full rounded px-3 py-2 text-sm"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                    outline: 'none',
                  }}
                >
                  {charClassOptions().map(({ value: v, label }) => (
                    <option key={v} value={v}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
          )}

          {step === 'zeal' && (
            <div className="space-y-4">
              <div
                className="flex items-start gap-2 rounded p-3 text-sm"
                style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
              >
                <Info size={14} className="mt-0.5 shrink-0" style={{ color: 'var(--color-primary)' }} />
                <div className="space-y-2">
                  <p>
                    <strong>Zeal</strong> is a community EverQuest add-on that exports
                    inventory and spellbook data and exposes live game state over a
                    local pipe. PQ Companion uses it as an optional enhancement — the
                    app works fully without it and falls back to log-file parsing.
                  </p>
                  <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    With Zeal installed and running, you also get:
                  </p>
                  <ul
                    className="ml-4 list-disc space-y-0.5 text-xs"
                    style={{ color: 'var(--color-muted-foreground)' }}
                  >
                    <li>Real-time target detection (no <code>/con</code> needed)</li>
                    <li>Live target HP bar in the NPC overlay</li>
                    <li>&quot;Pet of X&quot; attribution for charmed/summoned pets</li>
                    <li>Authoritative DPS attribution for ambiguous fights</li>
                    <li>Trigger conditions like &quot;target HP &lt; 20%&quot; and <code>/pipe</code> alerts</li>
                  </ul>
                </div>
              </div>

              <div
                className="rounded p-3 text-sm"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <div className="mb-2 flex items-center justify-between">
                  <span
                    className="text-xs font-semibold uppercase tracking-wide"
                    style={{ color: 'var(--color-muted)' }}
                  >
                    Detection
                  </span>
                  <button
                    onClick={() => void checkZeal(eqPath)}
                    disabled={zealChecking || !eqPath.trim()}
                    className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
                    style={{
                      backgroundColor: 'var(--color-surface)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--color-foreground)',
                      cursor: zealChecking || !eqPath.trim() ? 'not-allowed' : 'pointer',
                      opacity: zealChecking || !eqPath.trim() ? 0.5 : 1,
                    }}
                  >
                    {zealChecking ? <Loader2 size={12} className="animate-spin" /> : null}
                    {zealChecking ? 'Checking…' : 'Re-check'}
                  </button>
                </div>

                {zealChecking && !zealStatus && (
                  <p style={{ color: 'var(--color-muted-foreground)' }}>
                    Looking for Zeal.asi in your EverQuest folder…
                  </p>
                )}

                {!zealChecking && zealStatus?.installed && (
                  <div className="flex items-start gap-2" style={{ color: '#22c55e' }}>
                    <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                    <div>
                      <p>Zeal is installed.</p>
                      {zealStatus.asi_path && (
                        <p
                          className="mt-1 text-xs"
                          style={{ color: 'var(--color-muted-foreground)' }}
                        >
                          Found <code>{zealStatus.asi_path}</code>
                        </p>
                      )}
                    </div>
                  </div>
                )}

                {!zealChecking && zealStatus && !zealStatus.installed && (
                  <div className="space-y-2">
                    <div className="flex items-start gap-2" style={{ color: 'var(--color-muted-foreground)' }}>
                      <Info size={14} className="mt-0.5 shrink-0" />
                      <div>
                        <p>
                          Zeal is not installed in this folder
                          {zealStatus.eqgame_present ? '' : ' (eqgame.exe also not found here — double-check the path on the previous step)'}.
                        </p>
                        <p className="mt-1 text-xs">
                          You can skip this step and install Zeal later — every Zeal
                          feature in PQ Companion is optional.
                        </p>
                      </div>
                    </div>
                    <a
                      href={ZEAL_RELEASE_URL}
                      target="_blank"
                      rel="noreferrer noopener"
                      className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                      style={{
                        backgroundColor: 'var(--color-primary)',
                        color: '#fff',
                        textDecoration: 'none',
                      }}
                    >
                      Get Zeal (GitHub releases)
                    </a>
                  </div>
                )}

                {zealError && (
                  <div
                    className="flex items-start gap-2 text-xs"
                    style={{ color: '#f87171' }}
                  >
                    <AlertTriangle size={12} className="mt-0.5 shrink-0" />
                    <p>Couldn&apos;t check for Zeal: {zealError}</p>
                  </div>
                )}
              </div>
            </div>
          )}

          {step === 'confirm' && (
            <div className="space-y-4">
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                You&apos;re all set! Review your configuration below and click Finish to start using PQ Companion.
              </p>
              <div
                className="space-y-2 rounded p-4"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
              >
                <div className="flex justify-between text-sm">
                  <span style={{ color: 'var(--color-muted-foreground)' }}>EverQuest folder</span>
                  <code style={{ color: 'var(--color-foreground)' }}>{eqPath || '—'}</code>
                </div>
                <div className="flex justify-between text-sm">
                  <span style={{ color: 'var(--color-muted-foreground)' }}>Character</span>
                  <span style={{ color: 'var(--color-foreground)' }}>{character || '—'}</span>
                </div>
                <div className="flex justify-between text-sm">
                  <span style={{ color: 'var(--color-muted-foreground)' }}>Class</span>
                  <span style={{ color: 'var(--color-foreground)' }}>
                    {charClassLabel(characterClass)}
                  </span>
                </div>
              </div>

              {/* Optional backfill — only when this character has a log to read. */}
              {discovered.length > 0 && (
                <div className="rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}>
                  <label className="flex cursor-pointer items-start gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
                    <input
                      type="checkbox"
                      checked={backfillOptIn}
                      disabled={backfilling}
                      onChange={(e) => setBackfillOptIn(e.target.checked)}
                      className="mt-0.5"
                    />
                    <span>
                      Backfill chat, loot, and player history from {character || 'this character'}&apos;s existing log
                      <span className="ml-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                        (recommended for existing players — can take a few minutes)
                      </span>
                    </span>
                  </label>
                  {backfilling && (
                    <div className="mt-3">
                      <div className="h-2 w-full overflow-hidden rounded-full" style={{ backgroundColor: 'var(--color-surface)' }}>
                        <div className="h-full rounded-full transition-all" style={{ width: `${backfillPct}%`, backgroundColor: 'var(--color-primary)' }} />
                      </div>
                      <p className="mt-1 text-[11px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
                        Backfilling… {backfillPct}%
                      </p>
                    </div>
                  )}
                </div>
              )}
              {saveError && (
                <div
                  className="flex items-center gap-2 rounded p-3 text-sm"
                  style={{ backgroundColor: 'rgba(248,113,113,0.1)', color: '#f87171' }}
                >
                  <AlertTriangle size={14} />
                  {saveError}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-between border-t px-6 py-4"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <button
            onClick={goBack}
            disabled={stepIndex === 0}
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
              cursor: stepIndex === 0 ? 'not-allowed' : 'pointer',
              opacity: stepIndex === 0 ? 0.4 : 1,
            }}
          >
            <ArrowLeft size={14} />
            Back
          </button>

          {step !== 'confirm' ? (
            <button
              onClick={goNext}
              disabled={!canAdvance}
              className="flex items-center gap-1.5 rounded px-4 py-1.5 text-sm font-semibold"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: '#fff',
                border: 'none',
                cursor: canAdvance ? 'pointer' : 'not-allowed',
                opacity: canAdvance ? 1 : 0.5,
              }}
            >
              Next
              <ArrowRight size={14} />
            </button>
          ) : (
            <button
              onClick={handleFinish}
              disabled={saving || !character.trim() || !eqPath.trim()}
              className="flex items-center gap-1.5 rounded px-4 py-1.5 text-sm font-semibold"
              style={{
                backgroundColor: '#22c55e',
                color: '#fff',
                border: 'none',
                cursor: saving ? 'not-allowed' : 'pointer',
                opacity: saving ? 0.7 : 1,
              }}
            >
              {saving ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle2 size={14} />}
              {backfilling ? 'Backfilling…' : saving ? 'Saving…' : 'Finish'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

// DiagRow is one green/red status line in the wizard's path diagnostics.
function DiagRow({ label, on, offText }: { label: string; on: boolean; offText: string }): React.ReactElement {
  return (
    <div className="flex items-center gap-2 py-0.5">
      <span className="inline-block h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: on ? '#22c55e' : '#ef4444' }} />
      <span style={{ color: 'var(--color-foreground)' }}>{label}</span>
      <span className="ml-auto" style={{ color: on ? '#22c55e' : '#ef4444' }}>{on ? 'on' : offText}</span>
    </div>
  )
}
