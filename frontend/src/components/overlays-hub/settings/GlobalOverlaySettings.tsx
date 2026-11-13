/**
 * GlobalOverlaySettings — everything that applies to all overlays, opened
 * from the gear in the Overlays hub header: appearance, trigger alert text
 * defaults and monitor, and the Overlay Windows table for changing many
 * overlays at once.
 */
import React from 'react'
import type { Config } from '../../../types/config'
import { itemInfoOffered } from '../../../lib/overlays'
import { OverlayTextDefaults } from '../../settings/AlertDefaultsSettings'
import OverlayAppearanceSettings from './OverlayAppearanceSettings'
import TriggerDisplayPicker from './TriggerDisplayPicker'
import { OverlayLockModeCard } from './OverlayWindowSettings'
import { SettingsSection } from './SettingsSection'

export function AlertTextSettings({
  config,
  setConfig,
}: {
  config: Config
  setConfig: (c: Config) => void
}): React.ReactElement {
  return (
    <SettingsSection title="Trigger alert text">
      <TriggerDisplayPicker />
      <OverlayTextDefaults config={config} setConfig={setConfig} />
    </SettingsSection>
  )
}

export default function GlobalOverlaySettings({
  config,
  setConfig,
}: {
  config: Config
  setConfig: (c: Config) => void
}): React.ReactElement {
  const prefs = config.preferences
  const patch = (p: Partial<Config['preferences']>): void =>
    setConfig({ ...config, preferences: { ...prefs, ...p } })
  return (
    <div className="flex flex-col gap-4">
      <OverlayAppearanceSettings config={config} setConfig={setConfig} />
      <AlertTextSettings config={config} setConfig={setConfig} />
      <OverlayLockModeCard
        offerItemInfo={itemInfoOffered(prefs)}
        modes={prefs.overlay_locked_modes ?? {}}
        onChange={(next) => patch({ overlay_locked_modes: next })}
        fadeModes={prefs.overlay_fade_modes ?? {}}
        fadeGlobal={Boolean(prefs.overlay_fade_enabled)}
        onFadeChange={(next) => patch({ overlay_fade_modes: next })}
        opacities={prefs.overlay_opacities ?? {}}
        opacityGlobal={prefs.overlay_opacity}
        onOpacityChange={(next) => patch({ overlay_opacities: next })}
      />
    </div>
  )
}
