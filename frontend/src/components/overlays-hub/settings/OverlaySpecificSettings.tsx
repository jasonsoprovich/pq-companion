/**
 * Each overlay's own settings, shown in its drawer in the Overlays hub below
 * the Window section. Overlays with a live preview show it first, so changes
 * can be seen as they're made. Overlays without settings of their own have
 * no entry.
 */
import React, { useState } from 'react'
import type { Config } from '../../../types/config'
import type { DashboardPanelKey } from '../../../services/dashboardLayout'
import DiscordVoiceOverlaySettings from '../../settings/DiscordVoiceOverlaySettings'
import NPCOverlayDisplayCard from '../../settings/NPCOverlayDisplayCard'
import NPCOverlayTagStyleCard from '../../settings/NPCOverlayTagStyleCard'
import NPCOverlayLayoutCard from '../../settings/NPCOverlayLayoutCard'
import NPCOverlayOutlineRulesCard from '../../settings/NPCOverlayOutlineRulesCard'
import NPCOverlayPreview from '../../settings/NPCOverlayPreview'
import ItemInfoDisplayCard from '../../settings/ItemInfoDisplayCard'
import ItemInfoLayoutCard from '../../settings/ItemInfoLayoutCard'
import ItemInfoPreview from '../../settings/ItemInfoPreview'
import { PetAlertsCard, PetGeneralCard, PetPreview, PetWatchListCard } from '../../settings/PetOverlaySettings'

export interface OverlaySettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

function NPCSettings({ config, setConfig }: OverlaySettingsProps): React.ReactElement {
  // The character whose per-character lists the live preview applies
  // ('' = defaults). Follows the cards' "Editing" selectors.
  const [npcPreviewCharacter, setNpcPreviewCharacter] = useState('')
  return (
    <>
      <NPCOverlayPreview
        prefs={config.preferences}
        character={npcPreviewCharacter}
        onCharacterChange={setNpcPreviewCharacter}
      />
      {/* ── NPC Overlay display (sections + items, per surface) ─────── */}
      <NPCOverlayDisplayCard
        onScopeChange={setNpcPreviewCharacter}
        hidden={config.preferences.npc_overlay_hidden_items ?? []}
        byCharacter={config.preferences.npc_overlay_hidden_items_by_character ?? {}}
        onChange={(hidden, byCharacter) =>
          setConfig({
            ...config,
            preferences: {
              ...config.preferences,
              npc_overlay_hidden_items: hidden,
              npc_overlay_hidden_items_by_character: byCharacter,
            },
          })
        }
        sectionOrder={config.preferences.npc_overlay_section_order ?? []}
        castRange={config.preferences.npc_overlay_cast_range || 200}
        rangedRange={config.preferences.npc_overlay_ranged_range || 0}
        rangesManual={Boolean(config.preferences.npc_overlay_ranges_manual)}
        onChangeRanges={(cast, ranged, manual) =>
          setConfig({
            ...config,
            preferences: {
              ...config.preferences,
              npc_overlay_cast_range: cast,
              npc_overlay_ranged_range: ranged,
              npc_overlay_ranges_manual: manual,
            },
          })
        }
        considerBorder={!config.preferences.npc_overlay_consider_border_off}
        onChangeConsiderBorder={(on) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, npc_overlay_consider_border_off: !on },
          })
        }
      />

      {/* ── NPC Overlay tag colors ─────────────────────────────────────── */}
      <NPCOverlayTagStyleCard
        style={config.preferences.npc_overlay_tag_style ?? {}}
        onChange={(tagStyle) =>
          setConfig({
            ...config,
            preferences: { ...config.preferences, npc_overlay_tag_style: tagStyle },
          })
        }
      />

      {/* ── NPC Overlay layout (section + tag order) ───────────────────── */}
      <NPCOverlayLayoutCard
        sectionOrder={config.preferences.npc_overlay_section_order ?? []}
        tagOrder={config.preferences.npc_overlay_tag_order ?? []}
        tagStyle={config.preferences.npc_overlay_tag_style ?? {}}
        onChange={(sectionOrder, tagOrder) =>
          setConfig({
            ...config,
            preferences: {
              ...config.preferences,
              npc_overlay_section_order: sectionOrder,
              npc_overlay_tag_order: tagOrder,
            },
          })
        }
      />

      {/* ── NPC Overlay outline rules ──────────────────────────────────── */}
      <NPCOverlayOutlineRulesCard
        onScopeChange={setNpcPreviewCharacter}
        rules={config.preferences.npc_overlay_outline_rules ?? []}
        sets={config.preferences.npc_overlay_outline_rule_sets ?? []}
        onChange={(rules, sets) =>
          setConfig({
            ...config,
            preferences: {
              ...config.preferences,
              npc_overlay_outline_rules: rules,
              npc_overlay_outline_rule_sets: sets,
            },
          })
        }
      />
    </>
  )
}

function ItemInfoSettings({ config, setConfig }: OverlaySettingsProps): React.ReactElement {
  return (
    <>
      <ItemInfoPreview settings={config.preferences.item_info_overlay ?? {}} />
      <ItemInfoDisplayCard
        settings={config.preferences.item_info_overlay ?? {}}
        onChange={(next) =>
          setConfig({ ...config, preferences: { ...config.preferences, item_info_overlay: next } })
        }
      />
      <ItemInfoLayoutCard
        settings={config.preferences.item_info_overlay ?? {}}
        onChange={(next) =>
          setConfig({ ...config, preferences: { ...config.preferences, item_info_overlay: next } })
        }
      />
    </>
  )
}

function PetSettings({ config, setConfig }: OverlaySettingsProps): React.ReactElement {
  return (
    <>
      <PetPreview />
      <PetGeneralCard
        settings={config.preferences.pet_overlay ?? {}}
        onChange={(next) =>
          setConfig({ ...config, preferences: { ...config.preferences, pet_overlay: next } })
        }
      />
      <PetAlertsCard
        settings={config.preferences.pet_overlay ?? {}}
        onChange={(next) =>
          setConfig({ ...config, preferences: { ...config.preferences, pet_overlay: next } })
        }
      />
      <PetWatchListCard
        settings={config.preferences.pet_overlay ?? {}}
        onChange={(next) =>
          setConfig({ ...config, preferences: { ...config.preferences, pet_overlay: next } })
        }
      />
    </>
  )
}

function DiscordVoiceSettings({ config, setConfig }: OverlaySettingsProps): React.ReactElement {
  return <DiscordVoiceOverlaySettings config={config} setConfig={setConfig} />
}

export const OVERLAY_SETTINGS: Partial<Record<DashboardPanelKey, React.ComponentType<OverlaySettingsProps>>> = {
  npc: NPCSettings,
  itemInfo: ItemInfoSettings,
  pet: PetSettings,
  discordVoice: DiscordVoiceSettings,
}
