/**
 * Timer overlay settings, formerly the Settings page's Spell Timers tab.
 * SharedTimerSettings covers every timer overlay (tracking, thresholds, bar
 * appearance) and lives in the hub's global Overlay settings; the rest are
 * each overlay's own alerts, shown in that overlay's drawer.
 */
import { useAutoTour } from '../../guide/GuideProvider'
import React from 'react'
import type { Config, TimerAlertPref } from '../../../types/config'
import TimerAlertPrefEditor from '../../settings/TimerAlertPrefEditor'
import DetrimSpellOverridesEditor from '../../settings/DetrimSpellOverridesEditor'
import ExpiredKeepOverridesEditor from '../../settings/ExpiredKeepOverridesEditor'
import { withTimerAlertDefaults } from '../../../lib/timerAlerts'
import { resolveFillOpacity } from '../../../hooks/useTimerAppearance'
import { useChainStyle } from '../../../lib/chChainStyle'
import { SettingsSection } from './SettingsSection'
import {
  CH_CHAIN_DEFAULT_PATTERN,
  CH_CHAIN_NUMERIC_PATTERN,
  CH_CHAIN_SECONDARY_PATTERN,
} from '../../../lib/chChainPatterns'

interface TimerSettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

// Used by Buff, Detrimental, Filtered and Custom timers alike.
export function SharedTimerSettings({ config, setConfig }: TimerSettingsProps): React.ReactElement {
  useAutoTour('timers')
  return (
    <SettingsSection
      title="Timers"
      description="Shared by every timer overlay: which spells are tracked, when timers show, and how the bars look."
    >
      <div data-tour="timers-tracking">
        <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
          Tracking mode
        </p>
        <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <b>Auto</b> (default) creates a timer for every recognised spell landing; triggers/packs add alerts and thresholds on top. <b>Triggers only</b> shows just the spells you've curated through trigger packs or custom triggers — useful if you want a focused overlay instead of full auto-coverage.
        </p>
        <div className="mb-4 flex gap-2">
          {([
            { value: 'auto' as const, label: 'Auto' },
            { value: 'triggers_only' as const, label: 'Triggers only' },
          ]).map(({ value, label }) => {
            const active = (config.spell_timer?.tracking_mode ?? 'auto') === value
            return (
              <button
                key={value}
                type="button"
                onClick={() =>
                  setConfig({
                    ...config,
                    spell_timer: { ...config.spell_timer, tracking_mode: value },
                  })
                }
                className="rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: active ? '#000' : 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                  cursor: 'pointer',
                  minWidth: 110,
                }}
              >
                {label}
              </button>
            )
          })}
        </div>

        <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
          Tracking scope
        </p>
        <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <b>Self only</b> tracks just buffs landing on you. <b>Cast by me</b> (default) also includes anything you cast on others — without the noise of other players buffing each other. <b>Anyone</b> tracks every recognised land, useful when you want to see e.g. another enchanter's debuff on a raid mob. (Ignored in <b>Triggers only</b> mode.)
        </p>
        <div className="flex gap-2">
          {([
            { value: 'self' as const, label: 'Self only' },
            { value: 'cast_by_me' as const, label: 'Cast by me' },
            { value: 'anyone' as const, label: 'Anyone' },
          ]).map(({ value, label }) => {
            const active = (config.spell_timer?.tracking_scope ?? 'cast_by_me') === value
            return (
              <button
                key={value}
                type="button"
                onClick={() =>
                  setConfig({
                    ...config,
                    spell_timer: { ...config.spell_timer, tracking_scope: value },
                  })
                }
                className="rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color: active ? '#000' : 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                  cursor: 'pointer',
                  minWidth: 90,
                }}
              >
                {label}
              </button>
            )
          })}
        </div>
      </div>

      <div className="mt-4">
        <label className="flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.class_filter ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: { ...config.spell_timer, class_filter: e.target.checked },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Filter to my class
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Drops buffs and detrimentals your class can't cast (e.g. paladin Spiritual Purity, shaman Talisman, bard songs) so an enchanter's overlay isn't cluttered with other classes' casts. Your own detrimentals are always tracked regardless of scope. Combine with <b>Anyone</b> scope to see other same-class casters' buffs and detrimentals (e.g. another enchanter's mez or Tashan) across the raid.
            </span>
          </span>
        </label>
      </div>

      <div className="mt-4">
        <label className="flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.hide_other_character_timers ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: { ...config.spell_timer, hide_other_character_timers: e.target.checked },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Hide timers from other characters
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              When on, the Buff and Detrimental overlays only show timers cast by the currently active character — e.g. swapping from your enchanter to your necromancer hides the enchanter's buffs instead of leaving them visible. Timers aren't cleared or lost: switch back and they reappear right where they left off. Off by default.
            </span>
          </span>
        </label>
      </div>

      <div className="mt-4" data-tour="timers-thresholds">
        <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
          Display thresholds
        </p>
        <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Hide overlay rows whose remaining time exceeds the threshold (in seconds). Set to <b>0</b> to always show — useful for keeping long-duration raid buffs out of view until they're close to expiring. Per-trigger overrides take precedence.
        </p>
        <div className="flex gap-3">
          {([
            { label: 'Buff', key: 'buff_display_threshold_secs' as const },
            { label: 'Detrimental', key: 'detrim_display_threshold_secs' as const },
          ]).map(({ label, key }) => (
            <label key={key} className="flex flex-col gap-1" style={{ flex: 1 }}>
              <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
              <input
                type="number"
                min={0}
                step={1}
                value={config.spell_timer?.[key] ?? 0}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    spell_timer: {
                      ...config.spell_timer,
                      [key]: Math.max(0, Number(e.target.value) || 0),
                    },
                  })
                }
                className="rounded px-2 py-1 text-sm"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              />
            </label>
          ))}
        </div>
      </div>

      <div className="mt-4">
        <label className="flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.keep_expired_timers ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: { ...config.spell_timer, keep_expired_timers: e.target.checked },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Keep expired timers until refreshed
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Instead of vanishing when it runs out, a buff or detrimental stays in the overlay as an overdue row (full red bar, counting up) until you recast it or dismiss it with the row's <b>✕</b>. Turns the overlay into a "buffs I still need to refresh" checklist — handy for pet haste, Fero, and the like. Overdue rows clear themselves after the time set below (an hour by default).
            </span>
          </span>
        </label>
        {config.spell_timer?.keep_expired_timers && (
          <div className="mt-3 ml-6">
            <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              How long an expired row stays before it's cleared automatically (in minutes). Set to <b>0</b> for the default of 60.
            </p>
            <div className="flex gap-3">
              {([
                { label: 'Buff', key: 'buff_expired_keep_mins' as const },
                { label: 'Detrimental', key: 'detrim_expired_keep_mins' as const },
              ]).map(({ label, key }) => (
                <label key={key} className="flex flex-col gap-1" style={{ flex: 1 }}>
                  <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
                  <input
                    type="number"
                    min={0}
                    step={1}
                    value={config.spell_timer?.[key] ?? 0}
                    onChange={(e) =>
                      setConfig({
                        ...config,
                        spell_timer: {
                          ...config.spell_timer,
                          [key]: Math.max(0, Math.floor(Number(e.target.value) || 0)),
                        },
                      })
                    }
                    className="rounded px-2 py-1 text-sm"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      color: 'var(--color-foreground)',
                      border: '1px solid var(--color-border)',
                    }}
                  />
                </label>
              ))}
            </div>
            <ExpiredKeepOverridesEditor
              value={config.spell_timer?.expired_keep_overrides}
              onChange={(next) =>
                setConfig({
                  ...config,
                  spell_timer: { ...config.spell_timer, expired_keep_overrides: next },
                })
              }
            />
          </div>
        )}
      </div>

      <div className="mt-6">
        <p className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
          Timer bar appearance
        </p>
        <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Tune how timer rows look across the Buff, Detrimental, Custom, Respawn, and CH Chain overlays. Per-trigger bar colors (set in the Triggers tab) still override the automatic color. Each of those overlay windows also has its own bell icon in its header to mute per-trigger alerts without editing the triggers themselves.
        </p>
        <div className="flex items-center gap-2">
          <span className="text-xs w-20" style={{ color: 'var(--color-muted-foreground)' }}>Bar fill</span>
          <div className="flex gap-1">
            {([
              { value: 'faded' as const, label: 'Faded' },
              { value: 'solid' as const, label: 'Solid' },
              { value: 'none' as const, label: 'None' },
            ]).map(({ value, label }) => {
              const active = (config.spell_timer?.timer_bar_fill ?? 'faded') === value
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() =>
                    setConfig({
                      ...config,
                      spell_timer: { ...config.spell_timer, timer_bar_fill: value },
                    })
                  }
                  className="rounded px-3 py-1 text-xs"
                  style={{
                    backgroundColor: active ? 'var(--color-primary)' : 'var(--color-surface-2)',
                    color: active ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                    border: '1px solid var(--color-border)',
                  }}
                >
                  {label}
                </button>
              )
            })}
          </div>
          <span className="text-[11px] italic" style={{ color: 'var(--color-muted)' }}>
            None = countdown text only (transparent)
          </span>
        </div>
        <div className="mt-2 flex items-center gap-2">
          <span className="text-xs w-20" style={{ color: 'var(--color-muted-foreground)' }}>Fill opacity</span>
          <input
            type="range"
            min={0}
            max={100}
            value={Math.round(resolveFillOpacity(config.spell_timer) * 100)}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: {
                  ...config.spell_timer,
                  timer_bar_fill: 'custom',
                  timer_bar_fill_pct: Number(e.target.value),
                },
              })
            }
            style={{ width: 160 }}
          />
          <span className="text-xs w-9 text-right" style={{ color: 'var(--color-muted-foreground)' }}>
            {Math.round(resolveFillOpacity(config.spell_timer) * 100)}%
          </span>
          <span className="text-[11px] italic" style={{ color: 'var(--color-muted)' }}>
            drag to fine-tune between the presets above
          </span>
        </div>
        <div className="mt-3 flex gap-3">
          {([
            { label: 'Name font (px)', key: 'timer_name_font_size' as const, def: 12, min: 8, max: 28 },
            { label: 'Time font (px)', key: 'timer_time_font_size' as const, def: 11, min: 8, max: 28 },
            { label: 'Row padding (px)', key: 'timer_row_padding' as const, def: 3, min: 0, max: 16 },
          ]).map(({ label, key, def, min, max }) => (
            <label key={key} className="flex flex-col gap-1" style={{ flex: 1 }}>
              <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
              <input
                type="number"
                min={min}
                max={max}
                value={config.spell_timer?.[key] || def}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    spell_timer: {
                      ...config.spell_timer,
                      [key]: Math.max(min, Math.min(max, Number(e.target.value) || def)),
                    },
                  })
                }
                className="rounded px-2 py-1 text-sm"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              />
            </label>
          ))}
        </div>

        <label className="mt-3 flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.timer_bar_stack_from_bottom ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: { ...config.spell_timer, timer_bar_stack_from_bottom: e.target.checked },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Stack timer rows from the bottom
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              New rows appear at the bottom of the window and push older ones up, instead of stacking
              downward from underneath the header. The header stays put either way.
            </span>
          </span>
        </label>

        <label className="mt-3 flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.timer_bar_urgent_text_white ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: { ...config.spell_timer, timer_bar_urgent_text_white: e.target.checked },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Keep row text white when a timer is running low
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              By default the spell name and countdown turn red under 20% time remaining. Enable this to
              keep them in their normal color for legibility.
            </span>
          </span>
        </label>

        <label data-tour="timers-seconds" className="mt-3 flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.timer_show_seconds ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: { ...config.spell_timer, timer_show_seconds: e.target.checked },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Show seconds on buff/detrimental timers
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              By default countdowns over a minute round up to the nearest minute (e.g. "5m"). Enable
              this to show minutes and seconds instead (e.g. "4m 32s").
            </span>
          </span>
        </label>

        <label className="mt-3 flex items-start gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={config.spell_timer?.timer_row_controls_always_show ?? false}
            onChange={(e) =>
              setConfig({
                ...config,
                spell_timer: {
                  ...config.spell_timer,
                  timer_row_controls_always_show: e.target.checked,
                },
              })
            }
            style={{ marginTop: 3 }}
          />
          <span>
            <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Always show row controls
            </span>
            <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              By default each timer row's mute / hide / remove buttons appear only while you hover
              the row. Enable this to keep them visible all the time.
            </span>
          </span>
        </label>
      </div>
    </SettingsSection>
  )
}

// Custom Timers: alert as a manually added timer finishes.
export function CustomTimerAlertSettings({ config, setConfig }: TimerSettingsProps): React.ReactElement {
  return (
    <SettingsSection title="Alerts">
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Plays a sound or speaks when a manually-added timer on the Custom Timers
        overlay is about to finish. New timers added from the overlay arm this alert
        by default — use the bell button in the quick-add form to silence an individual
        timer. Put <code className="font-mono">{'{spell}'}</code> in the spoken text to
        insert the timer's name. (Trigger-driven custom timers keep their own alerts.)
      </p>
      <TimerAlertPrefEditor
        value={withTimerAlertDefaults(config.preferences?.custom_timer_alert, 'custom')}
        onChange={(next: TimerAlertPref) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, custom_timer_alert: next },
          })
        }
        secondsLabel="Alert at"
        secondsUnit="s remaining"
        secondsHint="Fires when a timer crosses this many seconds left (0 = as it finishes)."
        ttsPlaceholder="{spell} done"
      />
    </SettingsSection>
  )
}

// Respawn Timers: alert before an NPC respawns.
export function RespawnAlertSettings({ config, setConfig }: TimerSettingsProps): React.ReactElement {
  return (
    <SettingsSection title="Alerts">
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Plays a sound or speaks as an NPC respawn timer is about to pop. Fires only
        for respawns in your current zone. Put <code className="font-mono">{'{npc}'}</code> in
        the spoken text to insert the mob's name. Can be muted from the bell icon in the
        Respawn overlay window's header without changing this setting.
      </p>
      <TimerAlertPrefEditor
        value={withTimerAlertDefaults(config.preferences?.respawn_alert, 'respawn')}
        onChange={(next: TimerAlertPref) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, respawn_alert: next },
          })
        }
        secondsLabel="Announce at"
        secondsUnit="s before respawn"
        secondsHint="Fires when a respawn crosses this many seconds left (0 = right as it pops)."
        ttsPlaceholder="{npc} has re-spawned"
      />

      <label className="mt-4 flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={config.preferences?.respawn_pin_handoff ?? false}
          onChange={(e) =>
            setConfig({
              ...config,
              preferences: { ...config.preferences, respawn_pin_handoff: e.target.checked },
            })
          }
          style={{ marginTop: 3 }}
        />
        <span>
          <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Auto-hand off pinned respawns
          </span>
          <span className="mt-0.5 block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            When a pinned respawn pops, move the pin to the next kill at the same
            spot. It's a guess (placeholders and rotating spawn ids can send it to
            the wrong mob), so it's off by default — a pin otherwise stays on its
            timer until you unpin it or remove the timer.
          </span>
        </span>
      </label>
    </SettingsSection>
  )
}

// Detrimental Timers: alert before a debuff wears off.
export function DetrimAlertSettings({ config, setConfig }: TimerSettingsProps): React.ReactElement {
  return (
    <SettingsSection title="Alerts">
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Plays a sound or speaks as a debuff/DoT/mez/stun timer is about to wear
        off. Applies to every timer the spell-timer engine detects automatically
        from the log — any mob, not just ones you've built a trigger for. A
        trigger's own "From spell…" fading alert takes priority when one exists.
        Put <code className="font-mono">{'{spell}'}</code> in the spoken text to
        insert the effect's name. Can be muted from the bell icon in the
        Detrimental overlay window's header without changing this setting.
      </p>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Same-named mobs: with a Zeal build that sends spell actions, run{' '}
        <code className="font-mono">/pipespells detrimental</code> once in game
        and each timer is tied to the exact mob it landed on, so it clears when
        that mob dies rather than when a namesake does.
      </p>
      <TimerAlertPrefEditor
        value={withTimerAlertDefaults(config.preferences?.detrim_timer_alert, 'detrim')}
        onChange={(next: TimerAlertPref) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, detrim_timer_alert: next },
          })
        }
        secondsLabel="Alert at"
        secondsUnit="s remaining"
        secondsHint="Fires when a timer crosses this many seconds left."
        ttsPlaceholder="{spell} fading soon"
      />
      <DetrimSpellOverridesEditor
        value={config.preferences?.detrim_spell_overrides}
        onChange={(next) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, detrim_spell_overrides: next },
          })
        }
      />
    </SettingsSection>
  )
}

// CH Chain: chain call tracking, possible misses and the secondary chain.
export function CHChainSettings({ config, setConfig }: TimerSettingsProps): React.ReactElement {
  useAutoTour('chchain')
  const [chainStyle, setChainStyle] = useChainStyle()
  return (
    <SettingsSection title="Chain tracking">
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Watches chat (raid, group, guild, custom channels, shout, OOC, auction) for
        Complete-Heal chain calls and shows one bar per chain position
        in a dedicated overlay window. Each bar runs the 10s CH cast, counting down to when
        that heal lands, and the header shows the live cadence measured between calls (turning
        red when the chain stalls). The pattern must capture the named groups
        <code className="font-mono"> caster</code>, <code className="font-mono">chainnum</code>,
        and <code className="font-mono">target</code>.
      </p>

      <label data-tour="chchain-enable" className="mb-3 flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={config.ch_chain?.enabled ?? false}
          onChange={(e) =>
            setConfig({
              ...config,
              ch_chain: { ...config.ch_chain, enabled: e.target.checked },
            })
          }
          style={{ marginTop: 3 }}
        />
        <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
          Enable CH chain tracking
        </span>
      </label>

      <div className="mb-3 flex flex-col gap-1">
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Overlay style</span>
        <div className="flex gap-3">
          {(['list', 'lanes'] as const).map((v) => (
            <label key={v} className="flex items-center gap-1.5 cursor-pointer text-sm" style={{ color: 'var(--color-foreground)' }}>
              <input
                type="radio"
                name="chchain-style"
                checked={chainStyle === v}
                onChange={() => setChainStyle(v)}
              />
              {v === 'list' ? 'List' : 'Lanes'}
            </label>
          ))}
        </div>
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          List shows one countdown bar per call. Lanes shows one row per heal target, with
          each call's number sliding toward the target's name; the heal lands when it
          reaches the name (works best in a wide, short window). Also switchable from the
          overlay's header.
        </span>
      </div>

      <label data-tour="chchain-miss" className="mb-3 flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={config.ch_chain?.possible_miss_enabled ?? true}
          onChange={(e) =>
            setConfig({
              ...config,
              ch_chain: { ...config.ch_chain, possible_miss_enabled: e.target.checked },
            })
          }
          style={{ marginTop: 3 }}
        />
        <span>
          <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Flag possible misses
          </span>
          <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Colors a chain bar red when that callout's caster is never seen starting a cast
            in time — a likely fizzle, interrupt (e.g. a Luclin fling), or skipped cast.
            Detected from the "begins to cast a spell" line anyone nearby sees when the
            caster starts casting anything (or "You begin casting …" for your own) — it
            never inspects which spell, so this covers Cleric Complete Healing and Druid
            Tunare's/Karana's Renewal the same way. Purely additive; turn off if it ever
            reads as noisy.
          </span>
        </span>
      </label>

      <label className="mb-3 flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={config.ch_chain?.interrupt_detection_enabled ?? false}
          disabled={!(config.ch_chain?.possible_miss_enabled ?? true)}
          onChange={(e) =>
            setConfig({
              ...config,
              ch_chain: { ...config.ch_chain, interrupt_detection_enabled: e.target.checked },
            })
          }
          style={{ marginTop: 3 }}
        />
        <span>
          <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Also flag a miss on interrupt
          </span>
          <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Extends "Flag possible misses" above: if a caster's cast was confirmed starting
            but then gets interrupted (a melee hit, stun, etc.) before it would have landed,
            the bar is un-confirmed and flagged red instead of quietly expiring as if the
            heal went out. Off by default — enable if your chain relies on catching
            interrupts specifically, not just no-casts.
          </span>
        </span>
      </label>

      <label className="mb-3 flex flex-col gap-1">
        <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Chain-call pattern (regex)</span>
        <input
          type="text"
          value={config.ch_chain?.pattern ?? ''}
          onChange={(e) =>
            setConfig({
              ...config,
              ch_chain: { ...config.ch_chain, pattern: e.target.value },
            })
          }
          spellCheck={false}
          className="rounded px-2 py-1 font-mono text-xs"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        />
      </label>

      <label data-tour="chchain-secondary" className="mb-3 flex items-start gap-2 cursor-pointer">
        <input
          type="checkbox"
          checked={config.ch_chain?.secondary_enabled ?? false}
          onChange={(e) => {
            // Toggling the secondary chain auto-splits / re-merges the
            // default regexes: the catch-all default matches both 001
            // and AAA markers, so when a second chain exists the
            // primary narrows to numbers-only and the secondary takes
            // letters-only. Patterns the user has hand-edited are
            // never touched — only exact defaults are swapped.
            const on = e.target.checked
            const ch = config.ch_chain
            const next = { ...ch, secondary_enabled: on }
            if (on) {
              if (!ch?.pattern || ch.pattern === CH_CHAIN_DEFAULT_PATTERN) {
                next.pattern = CH_CHAIN_NUMERIC_PATTERN
              }
              if (!ch?.secondary_pattern) {
                next.secondary_pattern = CH_CHAIN_SECONDARY_PATTERN
              }
            } else if (ch?.pattern === CH_CHAIN_NUMERIC_PATTERN) {
              next.pattern = CH_CHAIN_DEFAULT_PATTERN
            }
            setConfig({ ...config, ch_chain: next })
          }}
          style={{ marginTop: 3 }}
        />
        <span>
          <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Enable secondary chain (ramp / split heals)
          </span>
          <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Tracks a second, independent chain — e.g. AAA/BBB ramp-heal calls alongside the
            001/002 main tank chain. The overlay and metronome gain a Main&thinsp;/&thinsp;Secondary
            switch. Enabling splits the default patterns into numbers-only (main) and
            letters-only (secondary); disabling restores the combined default. Custom patterns are
            left untouched.
          </span>
        </span>
      </label>

      {(config.ch_chain?.secondary_enabled ?? false) && (
        <label className="mb-3 ml-6 flex flex-col gap-1">
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Secondary chain-call pattern (regex)</span>
          <input
            type="text"
            value={config.ch_chain?.secondary_pattern ?? ''}
            onChange={(e) =>
              setConfig({
                ...config,
                ch_chain: { ...config.ch_chain, secondary_pattern: e.target.value },
              })
            }
            spellCheck={false}
            className="rounded px-2 py-1 font-mono text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          />
        </label>
      )}

      <div className="flex items-end gap-3">
        <label className="flex flex-col gap-1" style={{ maxWidth: 160 }}>
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Expected cadence (seconds)</span>
          <input
            type="number"
            min={1}
            step={0.5}
            value={config.ch_chain?.interval_secs ?? 6}
            onChange={(e) =>
              setConfig({
                ...config,
                ch_chain: {
                  ...config.ch_chain,
                  interval_secs: Math.max(1, Number(e.target.value) || 1),
                },
              })
            }
            className="rounded px-2 py-1 text-sm"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
            }}
          />
        </label>
      </div>
    </SettingsSection>
  )
}

// CH Metronome: countdown and cast-now cues.
export function MetronomeAlertSettings({ config, setConfig }: TimerSettingsProps): React.ReactElement {
  return (
    <SettingsSection title="Alerts">
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Optional sound or spoken cues from the CH Metronome. "Countdown starts" fires when
        the cleric ahead of you in the chain calls their cast; "Cast now" fires at the
        moment you should press your heal key. Alerts only fire while the popped-out
        metronome overlay window is open, and can be muted from the bell icon in that
        window's header without changing these settings.
      </p>

      <p className="mb-1 text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
        Countdown starts
      </p>
      <TimerAlertPrefEditor
        value={withTimerAlertDefaults(config.preferences?.metronome_start_alert, 'metronome_start')}
        onChange={(next: TimerAlertPref) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, metronome_start_alert: next },
          })
        }
        showSeconds={false}
        ttsPlaceholder="Chain starting"
      />

      <p className="mb-1 mt-4 text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
        Cast now
      </p>
      <TimerAlertPrefEditor
        value={withTimerAlertDefaults(config.preferences?.metronome_cast_alert, 'metronome_cast')}
        onChange={(next: TimerAlertPref) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, metronome_cast_alert: next },
          })
        }
        showSeconds={false}
        ttsPlaceholder="Cast now"
      />
    </SettingsSection>
  )
}
