import React, { useEffect, useMemo, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { listCharacters } from '../../services/api'
import { loadEnums } from '../../lib/enumsCache'
import {
  NPC_ITEM_GROUPS,
  abilityKey,
  isHiddenOn,
  sectionKey,
  setKeyVisibility,
  type NPCSurface,
} from '../../lib/npcItemFilters'
import { resolveSectionOrder } from '../../lib/npcOverlayLayout'
import DecimalInput from '../DecimalInput'
import { useCharacterRanges } from '../../hooks/useCharacterRanges'
import type { EnumsCatalog } from '../../types/enums'
import type { CharacterRanges } from '../../services/api'

interface Props {
  /** Global hidden keys (sections and items). */
  hidden: string[]
  /** Per-character replacement lists, keyed by lowercased name. */
  byCharacter: Record<string, string[]>
  onChange: (hidden: string[], byCharacter: Record<string, string[]>) => void
  /** Called when the "Editing" character changes, so a preview can follow. */
  onScopeChange?: (character: string) => void
  /** Saved section order, so the tree matches the Layout card. */
  sectionOrder: string[]
  castRange: number
  rangedRange: number
  /** True when the user typed their own ranges instead of the gear-based ones. */
  rangesManual: boolean
  onChangeRanges: (cast: number, ranged: number, manual: boolean) => void
  /** The /con border and standing label (both surfaces, all characters). */
  considerBorder: boolean
  onChangeConsiderBorder: (on: boolean) => void
}

interface Item {
  key: string
  label: string
  hint?: string
}

interface Section {
  id: string
  label: string
  hint: string
  // Items inside the section, optionally split into labelled sub-groups.
  groups: Array<{ label?: string; items: Item[] }>
}

const SURFACES: Array<{ id: NPCSurface; label: string }> = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'popout', label: 'Popout' },
]

const SECTION_HINTS: Record<string, { label: string; hint: string }> = {
  identity: { label: 'Identity', hint: 'Level, class, race, body type' },
  combat: { label: 'Combat', hint: 'HP, mana, AC, damage, attacks/round, PBAoE XP penalty' },
  behavior: { label: 'Behavior', hint: 'Attack delay, aggro and assist radius' },
  resists: { label: 'Resists', hint: 'MR, CR, FR, DR, PR' },
  attributes: { label: 'Attributes', hint: 'STR, STA, DEX, AGI, INT, WIS, CHA' },
  special_abilities: { label: 'Special Abilities', hint: 'Summon, rampage, immunities and more' },
  spells: { label: 'Spells & Abilities', hint: 'Caster highlights, procs, signature spells, class lists' },
  faction: { label: 'Faction', hint: 'Primary faction and the hits taken on a kill' },
  distance: {
    label: 'Distance',
    hint: 'Distance to target, colored against your range (needs a Zeal build that reports target info)',
  },
  debuffs: {
    label: 'Debuff strip',
    hint: "The target's debuff timers as icons under its name, on the Stats and Loot tabs",
  },
}

// Special abilities, bucketed so the long list is scannable: damaging specials
// (codes 1–7), "Immune to …" entries, and everything else.
function abilityGroups(catalog: EnumsCatalog | null): Section['groups'] {
  const attacks: Item[] = []
  const immunities: Item[] = []
  const other: Item[] = []
  const entries = Object.entries(catalog?.special_abilities ?? {})
    .map(([code, meta]) => ({ code: Number(code), meta }))
    .sort((a, b) => a.code - b.code)
  for (const { code, meta } of entries) {
    const item = { key: abilityKey(code), label: meta.name, hint: meta.description || undefined }
    if (code >= 1 && code <= 7) attacks.push(item)
    else if (/^immune/i.test(meta.name)) immunities.push(item)
    else other.push(item)
  }
  return [
    { label: 'Attacks', items: attacks },
    { label: 'Immunities', items: immunities },
    { label: 'Other', items: other },
  ].filter((g) => g.items.length > 0)
}

function buildSections(catalog: EnumsCatalog | null, order: string[]): Section[] {
  const itemsFor = (id: string): Section['groups'] => {
    if (id === 'special_abilities') return abilityGroups(catalog)
    if (id === 'spells') {
      return [
        {
          items: [
            { key: sectionKey('spells_procs'), label: 'Procs' },
            { key: sectionKey('spells_signature'), label: 'Signature spells' },
            { key: sectionKey('spells_class'), label: 'Class spell summary' },
          ],
        },
      ]
    }
    const g = NPC_ITEM_GROUPS.find((x) => x.section === id)
    return g ? [{ items: g.items }] : []
  }
  // Same order as the overlay (Layout card); Distance and the debuff strip
  // live in the header area, so they go last.
  return [...resolveSectionOrder(order), 'distance', 'debuffs'].map((id) => ({
    id,
    ...SECTION_HINTS[id],
    groups: itemsFor(id),
  }))
}

const GRID = 'grid grid-cols-[minmax(0,1fr)_5.5rem_5.5rem] items-center'

export default function NPCOverlayDisplayCard({
  hidden,
  byCharacter,
  onChange,
  onScopeChange,
  sectionOrder,
  castRange,
  rangedRange,
  rangesManual,
  onChangeRanges,
  considerBorder,
  onChangeConsiderBorder,
}: Props): React.ReactElement {
  const auto = useCharacterRanges()
  const [catalog, setCatalog] = useState<EnumsCatalog | null>(null)
  const [characters, setCharacters] = useState<string[]>([])
  const [scope, setScope] = useState('') // '' = all characters (global list)
  const [open, setOpen] = useState<Set<string>>(new Set())

  useEffect(() => {
    loadEnums().then(setCatalog).catch(() => {})
    listCharacters()
      .then((r) => setCharacters(r.characters.map((c) => c.name)))
      .catch(() => {})
  }, [])

  const sections = useMemo(() => buildSections(catalog, sectionOrder), [catalog, sectionOrder])

  const lc = scope.toLowerCase()
  const hasOverride = scope !== '' && byCharacter[lc] !== undefined
  // The list the editor below reads and writes for the selected scope.
  const current = scope === '' ? hidden : (byCharacter[lc] ?? hidden)
  const editable = scope === '' || hasOverride

  const write = (next: string[]): void => {
    if (scope === '') onChange(next, byCharacter)
    else onChange(hidden, { ...byCharacter, [lc]: next })
  }

  const setVisible = (keys: string[], surfaces: NPCSurface[], show: boolean): void => {
    let next = current
    for (const k of keys) next = setKeyVisibility(next, k, surfaces, show)
    write(next)
  }

  const toggleOverride = (on: boolean): void => {
    if (on) {
      onChange(hidden, { ...byCharacter, [lc]: [...hidden] })
    } else {
      const rest = { ...byCharacter }
      delete rest[lc]
      onChange(hidden, rest)
    }
  }

  const toggleOpen = (id: string): void =>
    setOpen((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const shown = (key: string, s: NPCSurface): boolean => !isHiddenOn(current, key, s)

  const muted = { color: 'var(--color-muted-foreground)' }
  const inputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
  }

  // Summary for a collapsed section: how many of its items are hidden.
  const summary = (sec: Section): string => {
    const keys = sec.groups.flatMap((g) => g.items.map((i) => i.key))
    if (keys.length === 0) return ''
    const counts = SURFACES.map(
      (s) => keys.filter((k) => !shown(k, s.id)).length,
    )
    if (counts[0] === 0 && counts[1] === 0) return 'all items shown'
    if (counts[0] === counts[1]) return `${counts[0]} hidden`
    return `${counts[0]} hidden on dashboard, ${counts[1]} on popout`
  }

  const checkbox = (
    key: string,
    s: NPCSurface,
    disabled = false,
  ): React.ReactElement => (
    <span className="flex justify-center">
      <input
        type="checkbox"
        aria-label={`${key} on ${s}`}
        checked={shown(key, s)}
        disabled={disabled}
        onChange={(e) => setVisible([key], [s], e.target.checked)}
        style={{ opacity: disabled ? 0.35 : 1 }}
      />
    </span>
  )

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        NPC Overlay Display
      </h2>
      <p className="mb-4 text-xs" style={muted}>
        Choose what each overlay shows. Untick a section to hide all of it, or open it to hide
        single items, separately for the dashboard panel and the popout window. The target name
        and HP bar always show. A character can keep their own list, handy when a shaman and an
        enchanter care about different things.
      </p>

      <div className="mb-3 flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-foreground)' }}>
          Editing
          <select
            value={scope}
            onChange={(e) => {
              setScope(e.target.value)
              onScopeChange?.(e.target.value)
            }}
            className="rounded px-2 py-1 text-sm"
            style={{ ...inputStyle, backgroundColor: 'var(--color-surface-2)' }}
          >
            <option value="">All characters (default)</option>
            {characters.map((n) => (
              <option key={n} value={n}>
                {n}
                {byCharacter[n.toLowerCase()] !== undefined ? ' — custom list' : ''}
              </option>
            ))}
          </select>
        </label>
        {scope !== '' && (
          <label
            className="flex cursor-pointer items-center gap-2 text-xs"
            style={{ color: 'var(--color-foreground)' }}
          >
            <input
              type="checkbox"
              checked={hasOverride}
              onChange={(e) => toggleOverride(e.target.checked)}
            />
            Use a custom list for {scope}
          </label>
        )}
      </div>

      {!editable ? (
        <p className="text-xs" style={muted}>
          {scope} uses the default list. Turn on a custom list to give them their own.
        </p>
      ) : (
        <div className="flex flex-col gap-1.5">
          <div className={`${GRID} px-3 text-[11px] font-semibold uppercase tracking-wide`} style={muted}>
            <span />
            {SURFACES.map((s) => (
              <span key={s.id} className="text-center">
                {s.label}
              </span>
            ))}
          </div>

          {sections.map((sec) => {
            const sk = sectionKey(sec.id)
            const expandable = sec.groups.length > 0 || sec.id === 'distance'
            const isOpen = open.has(sec.id)
            const itemKeys = sec.groups.flatMap((g) => g.items.map((i) => i.key))
            return (
              <div
                key={sec.id}
                className="rounded"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <div className={`${GRID} px-3 py-2`}>
                  <button
                    onClick={() => expandable && toggleOpen(sec.id)}
                    className="flex min-w-0 items-start gap-2 text-left"
                    style={{ cursor: expandable ? 'pointer' : 'default' }}
                  >
                    <span className="mt-0.5 w-[13px] shrink-0" style={{ color: 'var(--color-muted)' }}>
                      {expandable && (isOpen ? <ChevronDown size={13} /> : <ChevronRight size={13} />)}
                    </span>
                    <span className="min-w-0">
                      <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                        {sec.label}
                      </span>
                      {summary(sec) && (
                        <span className="ml-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                          {summary(sec)}
                        </span>
                      )}
                      <span className="block text-xs" style={muted}>
                        {sec.hint}
                      </span>
                    </span>
                  </button>
                  {SURFACES.map((s) => (
                    <React.Fragment key={s.id}>{checkbox(sk, s.id)}</React.Fragment>
                  ))}
                </div>

                {isOpen && (
                  <div className="border-t px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
                    {itemKeys.length > 0 && (
                      <div className="mb-1 flex gap-3 pl-[21px] text-xs">
                        <button
                          onClick={() => setVisible(itemKeys, ['dashboard', 'popout'], true)}
                          style={{ color: 'var(--color-primary)' }}
                        >
                          Show all items
                        </button>
                      </div>
                    )}
                    {sec.groups.map((g, gi) => (
                      <div key={g.label ?? gi}>
                        {g.label && (
                          <p
                            className="mt-2 pl-[21px] text-[11px] font-semibold uppercase tracking-wide"
                            style={{ color: 'var(--color-muted)' }}
                          >
                            {g.label}
                          </p>
                        )}
                        {g.items.map((item) => (
                          <div
                            key={item.key}
                            className={`${GRID} rounded py-0.5 hover:bg-black/10`}
                            title={item.hint}
                          >
                            <span className="truncate pl-[21px] text-sm" style={{ color: 'var(--color-foreground)' }}>
                              {item.label}
                            </span>
                            {SURFACES.map((s) => (
                              <React.Fragment key={s.id}>
                                {checkbox(item.key, s.id, !shown(sk, s.id))}
                              </React.Fragment>
                            ))}
                          </div>
                        ))}
                      </div>
                    ))}

                    {sec.id === 'distance' && (
                      <RangeSettings
                        auto={auto}
                        manual={rangesManual}
                        castRange={castRange}
                        rangedRange={rangedRange}
                        onChange={onChangeRanges}
                        inputStyle={inputStyle}
                      />
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      <label
        className="mt-4 flex cursor-pointer items-start gap-2 rounded px-3 py-2 text-sm"
        style={{
          color: 'var(--color-foreground)',
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
        }}
      >
        <input
          type="checkbox"
          checked={considerBorder}
          onChange={(e) => onChangeConsiderBorder(e.target.checked)}
          style={{ marginTop: 3 }}
        />
        <span>
          Faction border from /con
          <span className="block text-xs" style={muted}>
            After you consider the target, both overlays show its standing and outline red for
            scowls or amber for threatening (a faint frame for dubious and apprehensive). It&rsquo;s
            the server&rsquo;s answer at that moment, so it already allows for your invisibility,
            illusion and faction buffs, and it&rsquo;s cleared when you retarget, zone, or your
            invis, illusion or feign state changes. With Zeal&rsquo;s{' '}
            <code>/leftclickcon on</code>, clicking an NPC cons it, so the border updates with
            every click. Works from the log /con line too, best with a Zeal build that sends
            consider events.
          </span>
        </span>
      </label>
    </section>
  )
}

function RangeSettings({
  auto,
  manual,
  castRange,
  rangedRange,
  onChange,
  inputStyle,
}: {
  auto: CharacterRanges | null
  manual: boolean
  castRange: number
  rangedRange: number
  onChange: (cast: number, ranged: number, manual: boolean) => void
  inputStyle: React.CSSProperties
}): React.ReactElement {
  const muted = { color: 'var(--color-muted-foreground)' }
  const fg = { color: 'var(--color-foreground)' }
  const autoOk = auto?.has_export === true
  return (
    <div className="flex flex-col gap-2 pl-[21px] pt-1">
      <p className="text-xs" style={muted}>
        The distance readout is green within your range, yellow in its last 10%, and red
        beyond it. Zeal only reports a target&rsquo;s position within 250 units, so past that
        it reads n/a.
      </p>

      <label className="flex cursor-pointer items-start gap-2 text-sm" style={fg}>
        <input
          type="radio"
          checked={!manual}
          onChange={() => onChange(castRange, rangedRange, false)}
          style={{ marginTop: 3 }}
        />
        <span>
          From your character&rsquo;s gear
          {autoOk ? (
            <span className="block text-xs" style={muted}>
              {auto.character}: cast range <b style={fg}>{auto.cast_range}</b>
              {auto.cast_focus_percent > 0
                ? ` (200 + ${auto.cast_focus_percent}% from ${auto.cast_focus_item ?? 'a focus'})`
                : ' (no Extended Range focus worn)'}
              {', ranged '}
              {auto.ranged_range > 0 ? (
                <>
                  <b style={fg}>{auto.ranged_range}</b> ({auto.ranged_item}
                  {auto.ammo_item ? ` + ${auto.ammo_item}` : ''})
                </>
              ) : (
                'off (no ranged weapon)'
              )}
              . Updates when you camp or run /outputfile.
            </span>
          ) : (
            <span className="block text-xs" style={muted}>
              Needs a Zeal inventory export for the active character (made when you camp, or
              with /outputfile). Until then the values below are used.
            </span>
          )}
        </span>
      </label>

      <label className="flex cursor-pointer items-start gap-2 text-sm" style={fg}>
        <input
          type="radio"
          checked={manual}
          onChange={() =>
            // Start manual mode from the gear values, so switching doesn't jump.
            autoOk
              ? onChange(auto.cast_range, auto.ranged_range, true)
              : onChange(castRange, rangedRange, true)
          }
          style={{ marginTop: 3 }}
        />
        <span>Set manually</span>
      </label>

      {(manual || !autoOk) && (
        <div className="flex flex-wrap items-end gap-4 pl-6">
          <label className="flex flex-col gap-1">
            <span className="text-xs" style={fg}>
              Cast range
            </span>
            <DecimalInput
              value={castRange}
              onValue={(v) => onChange(Math.round(v), rangedRange, manual)}
              min={1}
              max={2000}
              fallback={200}
              className="w-20 rounded px-2 py-1 text-sm"
              style={inputStyle}
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs" style={fg}>
              Ranged attack range (0 = off)
            </span>
            <DecimalInput
              value={rangedRange}
              onValue={(v) => onChange(castRange, Math.round(v), manual)}
              min={0}
              max={2000}
              fallback={0}
              className="w-20 rounded px-2 py-1 text-sm"
              style={inputStyle}
            />
          </label>
        </div>
      )}
    </div>
  )
}
