/**
 * Threat Meter and DPS Meter settings, shown in their drawers in the
 * Overlays hub. Moved from the Settings page's Overlays and DPS Meters tabs.
 */
import React, { useEffect, useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import type { Config, DPSClassColors } from '../../../types/config'
import { DEFAULT_DPS_CLASS_COLORS } from '../../../types/config'
import { SettingsSection } from './SettingsSection'

interface CombatSettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

// ThreatSettings: the manual hatemod for the Threat Meter, plus raid-mode
// tuning once raid mode is enabled from the Developer tab (the gate lives
// there so normal users never see the experimental card).
export function ThreatSettings({ config, setConfig }: CombatSettingsProps): React.ReactElement {
  return (
    <>
      <SettingsSection
        title="Threat Meter"
        description={
          <>
            The Threat Meter estimates your own per-mob hate from your
            character&rsquo;s combat log. The Spell Casting Subtlety AA is
            auto-detected from your trained AAs, so use this only for a hate
            modifier the logs and DB can&rsquo;t reveal &mdash; e.g.
            hate-reduction gear. Leave it <code>0</code> otherwise, and
            don&rsquo;t re-enter Spell Casting Subtlety here.
          </>
        }
      >
        <div className="flex items-center gap-2">
          <label className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Manual hatemod %
          </label>
          <input
            type="number"
            min={-100}
            max={500}
            step={1}
            defaultValue={config.preferences.threat_hatemod_pct ?? 0}
            key={config.preferences.threat_hatemod_pct ?? 0}
            onBlur={(e) => {
              const v = Number(e.target.value)
              const cur = config.preferences.threat_hatemod_pct ?? 0
              if (!Number.isNaN(v) && v !== cur) {
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    threat_hatemod_pct: Math.max(-100, Math.min(500, Math.round(v))),
                  },
                })
              }
            }}
            className="w-20 rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          />
          <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
            gear + AA hate bonus the log can&rsquo;t see
          </span>
        </div>
      </SettingsSection>
      {(config.preferences.raid_threat_enabled ?? false) && (
        <RaidThreatCard
          classMods={config.preferences.raid_threat_class_mods ?? {}}
          playerMods={config.preferences.raid_threat_player_mods ?? {}}
          onPatch={(patch) =>
            setConfig({ ...config, preferences: { ...config.preferences, ...patch } })
          }
        />
      )}
    </>
  )
}

// DPSSettings: the fight inactivity timeout and per-class bar colours.
export function DPSSettings({ config, setConfig }: CombatSettingsProps): React.ReactElement {
  return (
    <>
      <SettingsSection
        title="Combat Meter"
        description="A parse stays on the meter through zoning and death — handy when you evac/Abscond to drop aggro and run back, or die and get rezzed. It ends only when the mob dies, you clear it with the meter’s ✕ button (which saves it to Combat History), or it sits idle for the timeout below. Raid-sized fights always get at least a 2-minute window."
      >
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Fight Inactivity Timeout
            </p>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              How long a fight can go without any damage before it’s archived. Higher values survive longer zone trips; lower values close out parses faster. Default 60.
            </p>
          </div>
          <div className="flex items-center gap-1 shrink-0 ml-3">
            <input
              type="number"
              min={10}
              max={1800}
              step={5}
              value={config.combat?.fight_timeout_seconds ?? 60}
              onChange={(e) =>
                setConfig({
                  ...config,
                  combat: {
                    ...config.combat,
                    fight_timeout_seconds: Math.min(1800, Math.max(10, Number(e.target.value) || 60)),
                  },
                })
              }
              className="w-20 rounded px-2 py-1 text-sm text-right"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            />
            <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              sec
            </span>
          </div>
        </div>
      </SettingsSection>
      <DPSClassColorsSection
        value={config.dps_class_colors ?? DEFAULT_DPS_CLASS_COLORS}
        onChange={(next) => setConfig({ ...config, dps_class_colors: next })}
      />
    </>
  )
}

const DPS_CLASS_ROWS: { key: keyof DPSClassColors; label: string }[] = [
  { key: 'warrior', label: 'Warrior' },
  { key: 'cleric', label: 'Cleric' },
  { key: 'paladin', label: 'Paladin' },
  { key: 'ranger', label: 'Ranger' },
  { key: 'shadow_knight', label: 'Shadow Knight' },
  { key: 'druid', label: 'Druid' },
  { key: 'monk', label: 'Monk' },
  { key: 'bard', label: 'Bard' },
  { key: 'rogue', label: 'Rogue' },
  { key: 'shaman', label: 'Shaman' },
  { key: 'necromancer', label: 'Necromancer' },
  { key: 'wizard', label: 'Wizard' },
  { key: 'magician', label: 'Magician' },
  { key: 'enchanter', label: 'Enchanter' },
  { key: 'beastlord', label: 'Beastlord' },
  { key: 'unknown', label: 'Unknown / Default' },
]

function normalizeHex(input: string): string {
  let v = input.trim()
  if (!v.startsWith('#')) v = '#' + v
  return v.toUpperCase()
}

function isValidHex(input: string): boolean {
  return /^#([0-9A-Fa-f]{6})$/.test(input.trim())
}

function DPSClassColorsSection({
  value,
  onChange,
}: {
  value: DPSClassColors
  onChange: (next: DPSClassColors) => void
}): React.ReactElement {
  // Local mirror of the typed hex string per class so users can type freely
  // (e.g. while halfway through "#69CC") without the upper-case normaliser
  // jumping in mid-keystroke. We push to the parent config on blur or when
  // the value parses as a valid hex; the color picker writes through
  // immediately because it always produces a valid colour.
  const [drafts, setDrafts] = useState<Record<string, string>>(() => {
    const out: Record<string, string> = {}
    for (const r of DPS_CLASS_ROWS) out[r.key] = value[r.key] ?? ''
    return out
  })

  // Re-sync when the parent value changes (e.g. after Reset to defaults or a
  // load from the API). Without this, the local draft state would shadow
  // resets.
  useEffect(() => {
    const next: Record<string, string> = {}
    for (const r of DPS_CLASS_ROWS) next[r.key] = value[r.key] ?? ''
    setDrafts(next)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value])

  function setOne(key: keyof DPSClassColors, hex: string): void {
    onChange({ ...value, [key]: hex })
  }

  function handleTextChange(key: keyof DPSClassColors, raw: string): void {
    setDrafts((d) => ({ ...d, [key]: raw }))
    const norm = normalizeHex(raw)
    if (isValidHex(norm)) setOne(key, norm)
  }

  function handleTextBlur(key: keyof DPSClassColors): void {
    const norm = normalizeHex(drafts[key] ?? '')
    if (isValidHex(norm)) {
      setDrafts((d) => ({ ...d, [key]: norm }))
      setOne(key, norm)
    } else {
      // Revert to the last valid persisted value if the user typed garbage.
      setDrafts((d) => ({ ...d, [key]: value[key] }))
    }
  }

  function resetOne(key: keyof DPSClassColors): void {
    const def = DEFAULT_DPS_CLASS_COLORS[key]
    setDrafts((d) => ({ ...d, [key]: def }))
    setOne(key, def)
  }

  function resetAll(): void {
    setDrafts(() => {
      const out: Record<string, string> = {}
      for (const r of DPS_CLASS_ROWS) out[r.key] = DEFAULT_DPS_CLASS_COLORS[r.key]
      return out
    })
    onChange({ ...DEFAULT_DPS_CLASS_COLORS })
  }

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            DPS Class Colors
          </h2>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Per-class bar colour for the DPS meter and combat history. Pets
            inherit their owner's colour. Combatants whose class can't be
            resolved fall back to the Unknown / Default colour.
          </p>
        </div>
        <button
          type="button"
          onClick={resetAll}
          className="flex shrink-0 items-center gap-1.5 rounded border px-2.5 py-1 text-xs"
          style={{
            borderColor: 'var(--color-border)',
            color: 'var(--color-foreground)',
            backgroundColor: 'var(--color-surface-2)',
            cursor: 'pointer',
          }}
        >
          <RefreshCw size={11} />
          Reset all to defaults
        </button>
      </div>

      <div
        className="grid gap-2"
        style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}
      >
        {DPS_CLASS_ROWS.map((row) => {
          const draft = drafts[row.key] ?? ''
          const persisted = value[row.key] ?? ''
          const invalid = !isValidHex(normalizeHex(draft))
          const isDefault = persisted.toUpperCase() === DEFAULT_DPS_CLASS_COLORS[row.key].toUpperCase()
          return (
            <div
              key={row.key}
              className="flex items-center gap-2 rounded border p-2"
              style={{
                borderColor: 'var(--color-border)',
                backgroundColor: 'var(--color-surface-2)',
              }}
            >
              <input
                type="color"
                value={isValidHex(persisted) ? persisted : DEFAULT_DPS_CLASS_COLORS[row.key]}
                onChange={(e) => {
                  const hex = normalizeHex(e.target.value)
                  setDrafts((d) => ({ ...d, [row.key]: hex }))
                  setOne(row.key, hex)
                }}
                title={`Pick ${row.label} colour`}
                style={{
                  width: 32,
                  height: 24,
                  padding: 0,
                  border: '1px solid var(--color-border)',
                  borderRadius: 4,
                  cursor: 'pointer',
                  flexShrink: 0,
                  backgroundColor: 'transparent',
                }}
              />
              <div className="flex min-w-0 flex-1 flex-col">
                <span
                  className="truncate text-xs font-medium"
                  style={{ color: 'var(--color-foreground)' }}
                >
                  {row.label}
                </span>
                <input
                  type="text"
                  value={draft}
                  onChange={(e) => handleTextChange(row.key, e.target.value)}
                  onBlur={() => handleTextBlur(row.key)}
                  spellCheck={false}
                  maxLength={7}
                  className="mt-0.5 rounded border px-1.5 py-0.5 text-xs font-mono"
                  style={{
                    borderColor: invalid ? '#f97316' : 'var(--color-border)',
                    backgroundColor: 'var(--color-surface)',
                    color: 'var(--color-foreground)',
                  }}
                />
              </div>
              <button
                type="button"
                onClick={() => resetOne(row.key)}
                disabled={isDefault}
                title={isDefault ? 'Already at default' : 'Reset to default'}
                className="flex shrink-0 items-center justify-center rounded p-1"
                style={{
                  border: '1px solid var(--color-border)',
                  backgroundColor: 'transparent',
                  color: isDefault ? 'var(--color-muted)' : 'var(--color-muted-foreground)',
                  cursor: isDefault ? 'not-allowed' : 'pointer',
                  opacity: isDefault ? 0.4 : 1,
                }}
              >
                <RefreshCw size={11} />
              </button>
            </div>
          )
        })}
      </div>
    </section>
  )
}

// RAID_CLASSES mirrors internal/raidthreat. There is no built-in per-class
// boost (tanks are handled by modelling taunt); these knobs are opt-in for a
// known aggro-mod gear/AA setup, so every default is 0.
const RAID_CLASSES = [
  'Warrior', 'Shadow Knight', 'Paladin', 'Cleric', 'Druid', 'Shaman',
  'Ranger', 'Monk', 'Rogue', 'Bard', 'Beastlord',
  'Necromancer', 'Wizard', 'Magician', 'Enchanter',
]

// RaidThreatCard configures the Threat overlay's estimated Raid mode: optional
// per-class and per-player hate adjustments for known aggro-mod setups. It is
// only rendered once raid mode is enabled from the Developer tab (the enable
// toggle lives there), so this card is pure tuning.
function RaidThreatCard({
  classMods,
  playerMods,
  onPatch,
}: {
  classMods: Record<string, number>
  playerMods: Record<string, number>
  onPatch: (patch: Record<string, unknown>) => void
}): React.ReactElement {
  const [newPlayer, setNewPlayer] = useState('')
  const [newPlayerPct, setNewPlayerPct] = useState('')

  const setClassMod = (cls: string, raw: string): void => {
    const next = { ...classMods }
    if (raw.trim() === '') delete next[cls]
    else next[cls] = Math.max(-100, Math.min(500, Math.round(Number(raw) || 0)))
    onPatch({ raid_threat_class_mods: next })
  }
  const addPlayerMod = (name: string, raw: string): void => {
    const n = name.trim()
    if (n === '') return
    const next = { ...playerMods, [n]: Math.max(-100, Math.min(500, Math.round(Number(raw) || 0))) }
    onPatch({ raid_threat_player_mods: next })
  }
  const removePlayerMod = (name: string): void => {
    const next = { ...playerMods }
    delete next[name]
    onPatch({ raid_threat_player_mods: next })
  }

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2 className="mb-1 text-sm font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
        Raid threat (estimated)
      </h2>
      <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Raid mode is <strong>enabled</strong> (toggle it in Settings &rsaquo;
        Developer). It estimates every nearby player&rsquo;s hate from the damage
        they do — the same data the DPS meter uses — and pins a tank to the top
        when it sees their taunt land. It is approximate: out-of-range players,
        others&rsquo; DoTs, heals, and utility casts aren&rsquo;t in your log, so
        DoT/healer classes read low.
      </p>

      <>
          <div className="mt-4">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Per-class hate adjustment (%)
            </p>
            <div className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3">
              {RAID_CLASSES.map((cls) => {
                const set = classMods[cls]
                return (
                  <label key={cls} className="flex items-center justify-between gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    <span className="truncate">{cls}</span>
                    <input
                      type="number"
                      defaultValue={set ?? ''}
                      key={set ?? 'unset'}
                      placeholder="0"
                      onBlur={(e) => setClassMod(cls, e.target.value)}
                      className="w-14 rounded px-1 py-0.5 text-right text-xs"
                      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
                    />
                  </label>
                )
              })}
            </div>
            <p className="mt-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
              Blank = 0 (no adjustment). Applied to a player&rsquo;s observed damage —
              opt-in for known aggro-mod gear/AAs.
            </p>
          </div>

          <div className="mt-4">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
              Per-player override (%)
            </p>
            {Object.keys(playerMods).length > 0 && (
              <div className="mb-2 flex flex-col gap-1">
                {Object.entries(playerMods).map(([name, pct]) => (
                  <div key={name} className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    <span className="w-28 truncate">{name}</span>
                    <span className="w-12 text-right">{pct > 0 ? '+' : ''}{pct}%</span>
                    <button type="button" onClick={() => removePlayerMod(name)} title="Remove" style={{ color: '#f87171', cursor: 'pointer' }}>
                      <Trash2 size={12} />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <div className="flex items-center gap-2">
              <input
                value={newPlayer}
                onChange={(e) => setNewPlayer(e.target.value)}
                placeholder="Player name"
                className="w-28 rounded px-2 py-0.5 text-xs"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
              />
              <input
                value={newPlayerPct}
                onChange={(e) => setNewPlayerPct(e.target.value)}
                type="number"
                placeholder="%"
                className="w-14 rounded px-1 py-0.5 text-right text-xs"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
              />
              <button
                type="button"
                onClick={() => { addPlayerMod(newPlayer, newPlayerPct); setNewPlayer(''); setNewPlayerPct('') }}
                className="rounded px-2 py-0.5 text-xs"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
              >
                Add
              </button>
            </div>
          </div>
        </>
    </section>
  )
}
