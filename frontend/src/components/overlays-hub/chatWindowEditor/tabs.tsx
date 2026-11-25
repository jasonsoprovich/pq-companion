/**
 * The Chat Window editor's tabs: Sources, Keep / Hide rules, Appearance and
 * Test.
 */
import React, { useEffect, useState } from 'react'
import type {
  ChatMatchMode,
  ChatRule,
  ChatSimpleKind,
  ChatTestResult,
  ChatVerdict,
  ChatWindowDef,
} from '../../../types/chatWindow'
import { testChatWindowLines } from '../../../services/api'
import {
  AddButton,
  Box,
  Hint,
  ListControls,
  NumberInput,
  Row,
  Select,
  TextInput,
  inputStyle,
  move,
} from '../customOverlayEditor/fields'
import {
  CHAT_CHANNELS,
  DEFAULT_CHANNEL_COLORS,
  DEFAULT_SELF_COLOR,
  SELF_COLOR_NONE,
  blankRule,
  capitalize,
  channelColor,
  channelLabel,
  isStandardChannel,
  mentionsMeRule,
} from '../../../lib/chatWindow'
import { DEFAULT_MAX_TELL_TABS, tabsOffered } from '../../../lib/chatTabs'

function Check({
  checked,
  onChange,
  label,
  hint,
}: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
  hint?: string
}): React.ReactElement {
  return (
    <label className="flex items-start gap-1.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="mt-0.5" />
      <span>
        {label}
        {hint && (
          <span className="block text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
            {hint}
          </span>
        )}
      </span>
    </label>
  )
}

// A comma-separated list of names, committed on blur so typing ", " works.
function NameListInput({
  value,
  onChange,
  placeholder,
}: {
  value: string[] | undefined
  onChange: (v: string[]) => void
  placeholder: string
}): React.ReactElement {
  const joined = (value ?? []).join(', ')
  const [text, setText] = useState(joined)
  useEffect(() => setText(joined), [joined])
  return (
    <input
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => onChange(text.split(',').map((s) => s.trim()).filter(Boolean))}
      placeholder={placeholder}
      aria-label={placeholder}
      spellCheck={false}
      className="min-w-0 flex-1 rounded px-2 py-1 text-xs outline-none"
      style={inputStyle}
    />
  )
}

// ── Sources ─────────────────────────────────────────────────────────────────

export function SourcesTab({
  draft,
  setDraft,
  namedChannels,
  characters,
}: {
  draft: ChatWindowDef
  setDraft: (d: ChatWindowDef) => void
  namedChannels: string[]
  characters: string[]
}): React.ReactElement {
  const src = draft.sources
  const has = (c: string): boolean => src.channels.includes(c)
  const toggle = (c: string, on: boolean): void =>
    setDraft({
      ...draft,
      sources: { ...src, channels: on ? [...src.channels, c] : src.channels.filter((x) => x !== c) },
    })
  const named = namedChannels.filter((c) => !isStandardChannel(c))
  return (
    <div className="flex flex-col gap-4">
      <Box>
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Chat channels</span>
        <div className="grid grid-cols-4 gap-2">
          {CHAT_CHANNELS.map((c) => (
            <Check key={c.key} checked={has(c.key)} onChange={(v) => toggle(c.key, v)} label={c.label} />
          ))}
        </div>
        <Hint>
          Lines already in your chat history fill the window when it’s created. /say isn’t kept in
          history, so it starts empty.
        </Hint>
      </Box>
      <Box>
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Named channels</span>
        <Check
          checked={!!src.all_named}
          onChange={(v) => setDraft({ ...draft, sources: { ...src, all_named: v } })}
          label="Every named channel"
          hint="Includes channels you join later."
        />
        {!src.all_named && (
          <div className="grid grid-cols-4 gap-2">
            {named.map((c) => (
              <Check key={c} checked={has(c)} onChange={(v) => toggle(c, v)} label={capitalize(c)} />
            ))}
            {named.length === 0 && <Hint>No named channels in your chat history yet.</Hint>}
          </div>
        )}
      </Box>
      <Box>
        <Check
          checked={!!src.other}
          onChange={(v) => setDraft({ ...draft, sources: { ...src, other: v } })}
          label="Other log lines"
          hint="Everything that isn’t chat: combat, spells, loot, system messages. Busy; pair it with keep or hide rules. Starts empty."
        />
      </Box>
      <Box>
        <Row label="Characters">
          <NameListInput
            value={draft.characters}
            onChange={(v) => setDraft({ ...draft, characters: v })}
            placeholder={characters.length ? `All (e.g. ${characters.slice(0, 2).join(', ')})` : 'All characters'}
          />
        </Row>
        <Hint>Leave empty to show lines from whichever character you’re playing.</Hint>
        <Row label="Scrollback">
          <NumberInput
            value={draft.max_lines}
            min={50}
            max={1000}
            onChange={(v) => setDraft({ ...draft, max_lines: v ?? 300 })}
            ariaLabel="Lines kept"
          />
          <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>lines (up to 1000)</span>
        </Row>
      </Box>
    </div>
  )
}

// ── Rules ───────────────────────────────────────────────────────────────────

type MatchKind = ChatSimpleKind | 'regex'

const MATCH_OPTIONS: { value: MatchKind; label: string }[] = [
  { value: 'contains', label: 'Contains' },
  { value: 'starts', label: 'Starts with' },
  { value: 'exact', label: 'Is exactly' },
  { value: 'regex', label: 'Regex' },
]

export function RulesTab({
  keep,
  draft,
  setDraft,
  channels,
  highlightId,
}: {
  keep: boolean
  draft: ChatWindowDef
  setDraft: (d: ChatWindowDef) => void
  channels: string[]
  highlightId?: string
}): React.ReactElement {
  const rules = keep ? draft.keep : draft.hide
  const set = (next: ChatRule[]): void => setDraft(keep ? { ...draft, keep: next } : { ...draft, hide: next })
  const patch = (i: number, p: Partial<ChatRule>): void => set(rules.map((r, j) => (j === i ? { ...r, ...p } : r)))
  const hasMentions = draft.keep.some((r) => r.text?.trim() === '{c}')

  const channelOptions = [
    { value: '', label: 'Whole line' },
    ...channels.map((c) => ({ value: c, label: `${channelLabel(c)} message` })),
  ]

  return (
    <div className="flex flex-col gap-3">
      <Hint>
        {keep
          ? 'Keep rules run first. A kept line always shows, even if a hide rule matches it, and can be highlighted and pinned to the top of the window. The first matching keep rule wins.'
          : 'Hide rules drop matching lines. Keep rules always win over them. Right-click a line in the window and pick “Hide lines like this” to make one quickly.'}
      </Hint>
      <Hint>
        Text: <code>*</code> matches anything, <code>{'{c}'}</code> is your character’s name,
        {' '}<code>{'{n:number}'}</code> and <code>{'{name:word}'}</code> match a number or one word.
        Matching ignores case.
      </Hint>
      {rules.map((r, i) => {
        const kind: MatchKind = r.mode === 'regex' ? 'regex' : (r.kind ?? 'contains')
        return (
          <div
            key={r.id}
            className="flex flex-col gap-2 rounded-lg p-3"
            style={{
              backgroundColor: 'var(--color-surface)',
              border: `1px solid ${r.id === highlightId ? 'var(--color-primary)' : 'var(--color-border)'}`,
              opacity: r.enabled ? 1 : 0.6,
            }}
          >
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={r.enabled}
                onChange={(e) => patch(i, { enabled: e.target.checked })}
                title={r.enabled ? 'Turn this rule off' : 'Turn this rule on'}
              />
              <TextInput
                value={r.name ?? ''}
                onChange={(v) => patch(i, { name: v })}
                placeholder={`${keep ? 'Keep' : 'Hide'} rule ${i + 1}`}
                className="flex-1"
              />
              <ListControls
                index={i}
                count={rules.length}
                what="rule"
                onMove={(dir) => set(move(rules, i, dir))}
                onDelete={() => set(rules.filter((_, j) => j !== i))}
              />
            </div>
            <Row label="Match">
              <Select
                value={r.channel ?? ''}
                options={
                  r.channel && !channels.includes(r.channel)
                    ? [...channelOptions, { value: r.channel, label: `${channelLabel(r.channel)} message` }]
                    : channelOptions
                }
                onChange={(v) => patch(i, { channel: v || undefined })}
                ariaLabel="What to match"
              />
              <Select
                value={kind}
                options={MATCH_OPTIONS}
                onChange={(v) =>
                  patch(i, v === 'regex'
                    ? { mode: 'regex' as ChatMatchMode, kind: undefined }
                    : { mode: 'simple' as ChatMatchMode, kind: v })
                }
                ariaLabel="Match type"
              />
              <TextInput
                value={r.text ?? ''}
                onChange={(v) => patch(i, { text: v })}
                placeholder={r.channel ? 'Text (blank = every line)' : 'Text'}
                mono
                className="min-w-[200px] flex-1"
              />
            </Row>
            <Row label="From">
              <NameListInput
                value={r.speakers}
                onChange={(v) => patch(i, { speakers: v.length ? v : undefined })}
                placeholder="Anyone (or names, comma-separated)"
              />
            </Row>
            {keep && (
              <Row label="Highlight">
                <input
                  type="color"
                  value={r.color || '#facc15'}
                  onChange={(e) => patch(i, { color: e.target.value })}
                  disabled={!r.color}
                  aria-label="Highlight color"
                  style={{ width: 28, height: 22, background: 'transparent', border: 'none', opacity: r.color ? 1 : 0.3 }}
                />
                <label className="flex items-center gap-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
                  <input
                    type="checkbox"
                    checked={!!r.color}
                    onChange={(e) => patch(i, { color: e.target.checked ? '#facc15' : undefined })}
                  />
                  Color
                </label>
                <span className="ml-3 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Pin for</span>
                <NumberInput
                  value={r.pin_minutes || undefined}
                  min={0}
                  max={60}
                  placeholder="0"
                  onChange={(v) => patch(i, { pin_minutes: v ? Math.max(0, Math.min(60, Math.round(v))) : undefined })}
                  ariaLabel="Pin minutes"
                />
                <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>min (0 = don’t pin)</span>
              </Row>
            )}
          </div>
        )
      })}
      <div className="flex flex-wrap gap-2">
        <AddButton label={keep ? 'Add keep rule' : 'Add hide rule'} onClick={() => set([...rules, blankRule(keep)])} />
        {keep && !hasMentions && (
          <AddButton
            label="Mentions me"
            title="Keep, highlight and pin for 2 minutes any line with your character’s name"
            onClick={() => set([...rules, mentionsMeRule()])}
          />
        )}
      </div>
    </div>
  )
}

// ── Appearance ──────────────────────────────────────────────────────────────

export function AppearanceTab({
  draft,
  setDraft,
}: {
  draft: ChatWindowDef
  setDraft: (d: ChatWindowDef) => void
}): React.ReactElement {
  const a = draft.appearance
  const setA = (p: Partial<ChatWindowDef['appearance']>): void =>
    setDraft({ ...draft, appearance: { ...a, ...p } })
  const colorKeys = [
    ...CHAT_CHANNELS.map((c) => c.key),
    'named',
    'other',
  ]
  const setColor = (key: string, color: string | null): void => {
    const next = { ...(a.channel_colors ?? {}) }
    if (color) next[key] = color
    else delete next[key]
    setA({ channel_colors: Object.keys(next).length ? next : undefined })
  }
  return (
    <div className="flex flex-col gap-4">
      <Box>
        <Row label="Font size">
          <input
            type="range"
            min={0.5}
            max={3}
            step={0.1}
            value={a.font_scale}
            onChange={(e) => setA({ font_scale: Number(e.target.value) })}
            aria-label="Font size"
          />
          <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>{Math.round(a.font_scale * 100)}%</span>
        </Row>
        <Row label="Background">
          <input
            type="range"
            min={0.1}
            max={1}
            step={0.05}
            value={a.opacity}
            onChange={(e) => setA({ opacity: Number(e.target.value) })}
            aria-label="Background opacity"
          />
          <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>{Math.round(a.opacity * 100)}%</span>
        </Row>
        <Hint>
          These belong to this window. Ctrl+scroll over the window changes its font size too. 100%
          background is fully solid.
        </Hint>
        <div className="flex gap-4">
          <Check checked={a.show_timestamps} onChange={(v) => setA({ show_timestamps: v })} label="Show times" />
          <Check checked={a.show_channel} onChange={(v) => setA({ show_channel: v })} label="Show channel tags" />
        </div>
      </Box>
      <Box>
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Layout</span>
        <div className="flex flex-col gap-1.5">
          <label className="flex items-start gap-1.5 text-xs" style={{ color: 'var(--color-foreground)' }}>
            <input type="radio" checked={a.layout !== 'tabs'} onChange={() => setA({ layout: undefined })} className="mt-0.5" />
            <span>
              One list
              <span className="block text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                Every channel together, each in its own color.
              </span>
            </span>
          </label>
          <label
            className="flex items-start gap-1.5 text-xs"
            style={{ color: 'var(--color-foreground)', opacity: tabsOffered(draft) || a.layout === 'tabs' ? 1 : 0.5 }}
          >
            <input
              type="radio"
              checked={a.layout === 'tabs'}
              disabled={!tabsOffered(draft) && a.layout !== 'tabs'}
              onChange={() => setA({ layout: 'tabs' })}
              className="mt-0.5"
            />
            <span>
              Tabs per channel
              <span className="block text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                {tabsOffered(draft)
                  ? 'A tab for each channel. A tab with new lines fills with its color and pulses, with a count.'
                  : 'Pick two or more sources (Sources tab) to use tabs.'}
              </span>
            </span>
          </label>
        </div>
        {a.layout === 'tabs' && draft.sources.channels.includes('tell') && (
          <div className="flex flex-col gap-1.5 border-t pt-2" style={{ borderColor: 'var(--color-border)' }}>
            <Check
              checked={!!a.tell_tabs}
              onChange={(v) => setA({ tell_tabs: v || undefined })}
              label="A tab for each tell conversation"
              hint="Each player who sends you a tell (or you send one to) gets a tab; Tells still shows them all. Close a conversation with its × or a middle-click; it comes back on their next tell."
            />
            {a.tell_tabs && (
              <Row label="Open at most">
                <NumberInput
                  value={a.max_tell_tabs ?? DEFAULT_MAX_TELL_TABS}
                  onChange={(v) => setA({ max_tell_tabs: v })}
                  min={2}
                  max={30}
                  ariaLabel="Most conversation tabs"
                />
                <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  conversations. Past that the oldest read one closes; tabs that don’t fit go in a +N menu.
                </span>
              </Row>
            )}
          </div>
        )}
      </Box>
      <Box>
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Channel colors</span>
        <div className="grid grid-cols-2 gap-x-4 gap-y-1.5">
          {colorKeys.map((k) => {
            const own = a.channel_colors?.[k]
            const shown = own ?? (k === 'named' ? DEFAULT_CHANNEL_COLORS.named : channelColor(draft, k))
            return (
              <div key={k} className="flex items-center gap-2">
                <input
                  type="color"
                  value={shown}
                  onChange={(e) => setColor(k, e.target.value)}
                  aria-label={`${k} color`}
                  style={{ width: 26, height: 20, background: 'transparent', border: 'none' }}
                />
                <span className="flex-1 text-xs" style={{ color: shown }}>
                  {k === 'named' ? 'Named channels' : channelLabel(k)}
                </span>
                {own && (
                  <button onClick={() => setColor(k, null)} className="text-[10px] underline" style={{ color: 'var(--color-muted-foreground)' }}>
                    default
                  </button>
                )}
              </div>
            )
          })}
        </div>
        <div className="flex items-center gap-2 border-t pt-2" style={{ borderColor: 'var(--color-border)' }}>
          <input
            type="color"
            value={a.self_color && a.self_color !== SELF_COLOR_NONE ? a.self_color : DEFAULT_SELF_COLOR}
            disabled={a.self_color === SELF_COLOR_NONE}
            onChange={(e) => setA({ self_color: e.target.value })}
            aria-label="Your messages color"
            style={{ width: 26, height: 20, background: 'transparent', border: 'none', opacity: a.self_color === SELF_COLOR_NONE ? 0.4 : 1 }}
          />
          <span
            className="flex-1 text-xs"
            style={{ color: a.self_color === SELF_COLOR_NONE ? 'var(--color-muted-foreground)' : a.self_color || DEFAULT_SELF_COLOR }}
          >
            Your messages
          </span>
          <Check
            checked={a.self_color === SELF_COLOR_NONE}
            onChange={(v) => setA({ self_color: v ? SELF_COLOR_NONE : undefined })}
            label="Same as channel"
          />
          {a.self_color && a.self_color !== SELF_COLOR_NONE && (
            <button onClick={() => setA({ self_color: undefined })} className="text-[10px] underline" style={{ color: 'var(--color-muted-foreground)' }}>
              default
            </button>
          )}
        </div>
        <Hint>
          Lines you send (tells, guild, raid…) use your color so they stand out; the channel tag keeps
          the channel color. A keep rule’s highlight color wins over both.
        </Hint>
      </Box>
    </div>
  )
}

// ── Test ────────────────────────────────────────────────────────────────────

const VERDICT_STYLE: Record<ChatVerdict, { label: string; color: string }> = {
  kept: { label: 'kept', color: '#facc15' },
  shown: { label: 'shown', color: '#4ade80' },
  hidden: { label: 'hidden', color: '#f87171' },
  filtered: { label: 'not a source', color: 'var(--color-muted-foreground)' },
}

export function TestTab({
  draft,
  defaultCharacter,
}: {
  draft: ChatWindowDef
  defaultCharacter: string
}): React.ReactElement {
  const [text, setText] = useState('')
  const [character, setCharacter] = useState('')
  const [result, setResult] = useState<ChatTestResult | null>(null)
  const [running, setRunning] = useState(false)

  // Re-run on every draft change once a test has run.
  useEffect(() => {
    if (!result) return
    const t = setTimeout(() => run(), 300)
    return () => clearTimeout(t)
  }, [draft]) // eslint-disable-line react-hooks/exhaustive-deps

  const run = (): void => {
    setRunning(true)
    testChatWindowLines(draft, text.split('\n').filter((l) => l.trim()), character || defaultCharacter)
      .then(setResult)
      .catch((e: Error) => setResult({ steps: [], error: e.message }))
      .finally(() => setRunning(false))
  }

  return (
    <div className="flex flex-col gap-3">
      <Hint>
        Paste lines from your log (with or without the [timestamp]) and see what this window would do
        with each one, using your unsaved changes.
      </Hint>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={8}
        spellCheck={false}
        placeholder={"[Fri Nov 13 20:01:00 2026] Raidlead tells the raid,  'Osui mez the adds'\n[Fri Nov 13 20:01:05 2026] Soandso tells General:1, 'WTS stuff'"}
        className="w-full resize-y rounded p-2 font-mono text-[11px] outline-none"
        style={inputStyle}
      />
      <div className="flex flex-wrap items-center gap-3">
        <button
          onClick={run}
          disabled={!text.trim() || running}
          className="rounded px-3 py-1 text-xs font-semibold"
          style={{
            backgroundColor: 'var(--color-primary)',
            color: 'var(--color-background)',
            opacity: text.trim() && !running ? 1 : 0.5,
          }}
        >
          Run test
        </button>
        <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>As</span>
        <TextInput
          value={character}
          onChange={setCharacter}
          placeholder={defaultCharacter || 'Character'}
          className="w-36"
          ariaLabel="Test as character"
        />
      </div>
      {result?.error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{result.error}</p>}
      {result && !result.error && (
        <div className="flex flex-col gap-1">
          {result.steps.length === 0 && <Hint>No lines to run.</Hint>}
          {result.steps.map((s, i) => {
            const v = VERDICT_STYLE[s.verdict]
            return (
              <div key={i} className="flex items-start gap-2 rounded px-2 py-1" style={{ backgroundColor: 'var(--color-surface)' }}>
                <span
                  className="min-w-0 flex-1 break-words font-mono text-[11px]"
                  style={{ color: s.color || (s.verdict === 'kept' || s.verdict === 'shown' ? 'var(--color-foreground)' : 'var(--color-muted-foreground)') }}
                >
                  {s.text}
                </span>
                <span className="flex shrink-0 items-center gap-1">
                  {s.pin && <span className="text-[10px]" style={{ color: '#facc15' }}>pinned</span>}
                  <span className="rounded px-1.5 py-px text-[10px]" style={{ backgroundColor: 'var(--color-surface-2)', color: v.color }}>
                    {v.label}{s.rule ? ` · ${s.rule}` : ''}
                  </span>
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
