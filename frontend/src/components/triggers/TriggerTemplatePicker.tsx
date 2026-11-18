/**
 * TriggerTemplatePicker — what "New Trigger" opens. Step one picks a
 * starting point; step two asks a few plain questions and either creates
 * the trigger or opens it in the full editor. Spell timer, Start blank and
 * Install a pack hand straight off to the existing flows.
 */
import React, { useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle,
  FilePlus2,
  Hourglass,
  MessageSquare,
  Megaphone,
  Package,
  Sparkles,
  Type,
  Zap,
} from 'lucide-react'
import Wizard from '../guide/Wizard'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import { convertTriggerPattern, createTrigger } from '../../services/api'
import type { Trigger } from '../../types/trigger'
import {
  CHAT_CHANNELS,
  TRIGGER_TEMPLATES,
  chatLine,
  overlayAction,
  speakAction,
  type ChatChannel,
  type TemplateId,
  type TriggerPrefill,
} from '../../lib/triggerTemplates'

const ICONS: Record<TemplateId, React.ReactNode> = {
  text: <Type size={16} />,
  spell: <Sparkles size={16} />,
  emote: <Megaphone size={16} />,
  chat: <MessageSquare size={16} />,
  countdown: <Hourglass size={16} />,
  blank: <FilePlus2 size={16} />,
  pack: <Package size={16} />,
}

const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
  outline: 'none',
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: React.ReactNode
  children: React.ReactNode
}): React.ReactElement {
  return (
    <div className="space-y-1">
      <label
        className="text-xs font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        {label}
      </label>
      {children}
      {hint && (
        <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {hint}
        </p>
      )}
    </div>
  )
}

function Check({
  checked,
  onChange,
  label,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
}): React.ReactElement {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm" style={{ color: 'var(--color-foreground)' }}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      {label}
    </label>
  )
}

const TOKEN_HINT = (
  <>
    Type the line as it appears in your log. <code>{'{name}'}</code> matches any
    text and can be used in the message; <code>{'{C}'}</code> is your
    character&apos;s name.
  </>
)

export interface TriggerTemplatePickerProps {
  onClose: () => void
  onCreated: (t: Trigger) => void
  onOpenEditor: (prefill: TriggerPrefill) => void
  onSpellTimer: () => void
  onBlank: () => void
  onPacks: () => void
}

export default function TriggerTemplatePicker({
  onClose,
  onCreated,
  onOpenEditor,
  onSpellTimer,
  onBlank,
  onPacks,
}: TriggerTemplatePickerProps): React.ReactElement {
  useEscapeToClose(onClose)
  const [template, setTemplate] = useState<TemplateId | null>(null)
  const [name, setName] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [line, setLine] = useState('')
  const [message, setMessage] = useState('')
  const [messageTouched, setMessageTouched] = useState(false)
  const [speak, setSpeak] = useState(true)
  const [show, setShow] = useState(true)
  const [channel, setChannel] = useState<ChatChannel>('tell')
  const [from, setFrom] = useState('')
  const [seconds, setSeconds] = useState(60)
  // The last conversion and the line it came from; a regex only counts
  // while it matches the line as currently typed.
  const [conv, setConv] = useState<{ source: string; regex: string }>({ source: '', regex: '' })
  const [patternError, setPatternError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const plainLine = template === 'chat' ? chatLine(channel, from) : line
  const regex = conv.source === plainLine ? conv.regex : ''

  // Convert the plain line to regex as the user types.
  useEffect(() => {
    if (!template || !plainLine.trim()) {
      setConv({ source: '', regex: '' })
      setPatternError(null)
      return
    }
    let stale = false
    const t = window.setTimeout(() => {
      convertTriggerPattern('regex', plainLine)
        .then((r) => {
          if (stale) return
          setConv({ source: plainLine, regex: r.ok ? (r.result ?? '') : '' })
          setPatternError(r.ok ? null : (r.error ?? 'Invalid pattern'))
        })
        .catch(() => {
          if (!stale) setPatternError("Couldn't check the pattern")
        })
    }, 200)
    return () => {
      stale = true
      window.clearTimeout(t)
    }
  }, [template, plainLine])

  // Sensible defaults for the name and message until the user edits them.
  const defaults = useMemo(() => {
    // Names read better without raw tokens: "{name} has been slain" →
    // "… has been slain".
    const short = line
      .replace(/\{\{/g, '{')
      .replace(/\}\}/g, '}')
      .replace(/\{[A-Za-z_][A-Za-z0-9_]*\}/g, '…')
      .trim()
      .slice(0, 40)
    switch (template) {
      case 'text':
        return { name: short || 'Text alert', message: short }
      case 'emote':
        return { name: short ? `Emote: ${short}` : 'Mob emote', message: 'Emote!' }
      case 'chat': {
        const c = CHAT_CHANNELS.find((x) => x.id === channel)!
        // With a fixed sender the line has no {name} capture, so say the
        // name itself.
        const who = from.trim()
        return {
          name: `${c.label} alert${who ? `: ${who}` : ''}`,
          message: who ? c.spoken.replace('{name}', who) : c.spoken,
        }
      }
      case 'countdown':
        return { name: short || 'Countdown', message: '' }
      default:
        return { name: '', message: '' }
    }
  }, [template, line, channel, from])
  const effName = nameTouched ? name : defaults.name
  const effMessage = messageTouched ? message : defaults.message

  function choose(id: TemplateId): void {
    if (id === 'spell') return onSpellTimer()
    if (id === 'blank') return onBlank()
    if (id === 'pack') return onPacks()
    setTemplate(id)
    setSpeak(true)
    setShow(true)
    setSaveError(null)
  }

  function buildPrefill(): TriggerPrefill {
    const actions = []
    if (template === 'emote') {
      actions.push(overlayAction(effMessage, '#ef4444'), speakAction(effMessage))
    } else if (template === 'text' || template === 'chat') {
      if (show) actions.push(overlayAction(effMessage))
      if (speak) actions.push(speakAction(effMessage))
    }
    return {
      name: effName.trim(),
      pattern: regex,
      actions,
      ...(template === 'countdown' ? { timerType: 'custom' as const, timerDurationSecs: seconds } : {}),
    }
  }

  const needsMessage = template === 'emote' || ((template === 'text' || template === 'chat') && (speak || show))
  const valid =
    !!regex &&
    !patternError &&
    !!effName.trim() &&
    (!needsMessage || !!effMessage.trim()) &&
    (template !== 'countdown' || seconds > 0) &&
    (template === 'countdown' || template === 'emote' || speak || show)

  async function create(): Promise<void> {
    const p = buildPrefill()
    setSaving(true)
    setSaveError(null)
    try {
      const t = await createTrigger({
        name: p.name,
        enabled: true,
        pattern: p.pattern,
        actions: p.actions ?? [],
        timer_type: p.timerType ?? 'none',
        timer_duration_secs: p.timerDurationSecs ?? 0,
      })
      onCreated(t)
    } catch (err) {
      setSaveError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const tpl = TRIGGER_TEMPLATES.find((t) => t.id === template)

  return (
    <Wizard
      icon={<Zap size={20} />}
      title={tpl ? `New trigger: ${tpl.label}` : 'New trigger'}
      steps={[
        { id: 'choose', label: 'Template' },
        { id: 'details', label: 'Details' },
      ]}
      currentStep={template ? 'details' : 'choose'}
      onClose={onClose}
      onBack={() => setTemplate(null)}
      backHidden={!template}
      footerCenter={
        template ? (
          <button
            onClick={() => onOpenEditor(buildPrefill())}
            disabled={!regex || !!patternError}
            className="rounded px-3 py-1.5 text-xs font-medium disabled:opacity-40"
            style={{ ...inputStyle, cursor: 'pointer' }}
            title="Open these settings in the full trigger editor before saving"
          >
            Open in full editor
          </button>
        ) : undefined
      }
      primary={
        template
          ? {
              label: saving ? 'Creating…' : 'Create trigger',
              onClick: () => void create(),
              disabled: !valid,
              busy: saving,
              variant: 'finish',
            }
          : { label: 'Pick a template', onClick: () => undefined, disabled: true }
      }
    >
      {!template ? (
        <div className="grid grid-cols-2 gap-2">
          {TRIGGER_TEMPLATES.map((t) => (
            <button
              key={t.id}
              onClick={() => choose(t.id)}
              className="flex items-start gap-3 rounded-lg p-3 text-left transition-colors hover:bg-(--color-surface-3)"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', cursor: 'pointer' }}
            >
              <span className="mt-0.5" style={{ color: 'var(--color-primary)' }}>
                {ICONS[t.id]}
              </span>
              <span>
                <span className="block text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
                  {t.label}
                </span>
                <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  {t.description}
                </span>
              </span>
            </button>
          ))}
        </div>
      ) : (
        <div className="space-y-4">
          {template === 'chat' ? (
            <>
              <Field label="Channel">
                <div className="flex flex-wrap gap-1.5">
                  {CHAT_CHANNELS.map((c) => (
                    <button
                      key={c.id}
                      onClick={() => setChannel(c.id)}
                      className="rounded px-2.5 py-1 text-xs"
                      style={{
                        ...inputStyle,
                        backgroundColor: channel === c.id ? 'var(--color-primary)' : 'var(--color-surface-2)',
                        color: channel === c.id ? 'var(--color-background)' : 'var(--color-foreground)',
                        cursor: 'pointer',
                      }}
                    >
                      {c.label}
                    </button>
                  ))}
                </div>
              </Field>
              <Field label="Only from (optional)" hint="Leave blank for anyone.">
                <input
                  value={from}
                  onChange={(e) => setFrom(e.target.value)}
                  placeholder="e.g. Soandso"
                  className="w-full rounded px-3 py-2 text-sm"
                  style={inputStyle}
                />
              </Field>
            </>
          ) : (
            <Field
              label={template === 'emote' ? 'Paste the emote' : 'When this line appears'}
              hint={
                template === 'emote' ? (
                  <>Copy it from your log or the Log Feed. Swap a changing player name for <code>{'{name}'}</code>.</>
                ) : (
                  TOKEN_HINT
                )
              }
            >
              <input
                value={line}
                onChange={(e) => setLine(e.target.value)}
                autoFocus
                placeholder={
                  template === 'emote'
                    ? 'e.g. The air around Vulak`Aerr begins to crackle.'
                    : template === 'countdown'
                      ? 'e.g. You feel yourself starting to appear.'
                      : 'e.g. {name} has been slain by'
                }
                className="w-full rounded px-3 py-2 font-mono text-sm"
                style={inputStyle}
              />
            </Field>
          )}

          {patternError && (
            <p className="flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} /> {patternError}
            </p>
          )}

          {template === 'countdown' && (
            <Field label="Timer length (seconds)">
              <input
                type="number"
                min={1}
                value={seconds}
                onChange={(e) => setSeconds(Math.max(0, Number(e.target.value) || 0))}
                className="w-32 rounded px-3 py-2 text-sm"
                style={inputStyle}
              />
            </Field>
          )}

          {(template === 'text' || template === 'chat') && (
            <div className="flex gap-4">
              <Check checked={speak} onChange={setSpeak} label="Speak it" />
              <Check checked={show} onChange={setShow} label="Show on screen" />
            </div>
          )}

          {needsMessage && (
            <Field label="Message" hint="Captured names like {name} are filled in when it fires.">
              <input
                value={effMessage}
                onChange={(e) => {
                  setMessage(e.target.value)
                  setMessageTouched(true)
                }}
                className="w-full rounded px-3 py-2 text-sm"
                style={inputStyle}
              />
            </Field>
          )}

          <Field label={template === 'countdown' ? 'Timer name' : 'Trigger name'}>
            <input
              value={effName}
              onChange={(e) => {
                setName(e.target.value)
                setNameTouched(true)
              }}
              className="w-full rounded px-3 py-2 text-sm"
              style={inputStyle}
            />
          </Field>

          {regex && (
            <p className="truncate text-[11px]" style={{ color: 'var(--color-muted)' }} title={regex}>
              Pattern: <code>{regex}</code>
            </p>
          )}
          {saveError && (
            <p className="flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} /> {saveError}
            </p>
          )}
        </div>
      )}
    </Wizard>
  )
}
