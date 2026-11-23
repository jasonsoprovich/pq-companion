// MainApp is the main window's whole UI: the sidebar Layout, every main page
// route, the setup wizards and the audio/alert hooks. App.tsx lazy-loads it
// at "/*", so overlay windows (which only ever render their own small page)
// never download, parse or hold any of this. Within the main window it loads
// as one chunk, so the routes below stay synchronous and the first paint
// doesn't gain a loading step.
import React, { useEffect, useRef, useState, lazy } from 'react'
import { FeatureDotsProvider } from './hooks/useFeatureDots'
import { migratePopoutSelection } from './services/overlayPopoutSelection'
import { useLayoutAutoSwitch } from './hooks/useLayoutAutoSwitch'
import { initialFeatureDots } from './lib/featureRegistry'
import { Routes, Route, Navigate, useNavigate } from 'react-router-dom'
import Layout from './components/Layout'
import OnboardingWizard from './components/OnboardingWizard'
import OverlaySetupWizard from './components/overlays-hub/OverlaySetupWizard'
import WhatsNewModal from './components/WhatsNewModal'
import { GuideProvider } from './components/guide/GuideProvider'
import { getConfig, getChangelog, updateConfig, saveConfigChanges, type ChangelogEntry } from './services/api'
import { setAudioOwner } from './services/audio'
import { useAudioEngine } from './hooks/useAudioEngine'
import { useTriggerClipboard } from './hooks/useTriggerClipboard'
import { useTimerAlerts } from './hooks/useTimerAlerts'
import { useRespawnAlerts } from './hooks/useRespawnAlerts'
import { useItemInfoAutoShow } from './hooks/useItemDisplay'
import { useMetronomeAlerts } from './hooks/useMetronomeAlerts'
import { useAudioPrefs } from './hooks/useAudioPrefs'
import { useHighContrast } from './hooks/useHighContrast'
import { useZoom } from './hooks/useZoom'
import { useLogFeedSubscriber } from './hooks/useLogFeed'
// ItemsPage is the default landing route, so it ships in this chunk for an
// instant first paint. Every other page is code-split below so it loads on
// demand. The structural layout components (Layout/CombatLayout/
// CharactersLayout/RaidsLayout) are also in this chunk since the shell
// renders immediately.
import ItemsPage from './pages/ItemsPage'
import CombatLayout from './components/CombatLayout'
import CharactersLayout from './components/CharactersLayout'
import RaidsLayout from './components/RaidsLayout'
import { ActiveCharacterProvider } from './contexts/ActiveCharacterContext'
import { BackfillProvider } from './contexts/BackfillContext'

const SpellsPage = lazy(() => import('./pages/SpellsPage'))
const NpcsPage = lazy(() => import('./pages/NpcsPage'))
const ResistCalcPage = lazy(() => import('./pages/ResistCalcPage'))
const TraderTrackerPage = lazy(() => import('./pages/TraderTrackerPage'))
const CharmPetFinderPage = lazy(() => import('./pages/CharmPetFinderPage'))
const PoPFlaggingPage = lazy(() => import('./pages/PoPFlaggingPage'))
const ZonesPage = lazy(() => import('./pages/ZonesPage'))
const MapsPage = lazy(() => import('./pages/MapsPage'))
const LiveMapPage = lazy(() => import('./pages/LiveMapPage'))
const RaidReplayPage = lazy(() => import('./pages/RaidReplayPage'))
const DressingRoomPage = lazy(() => import('./pages/DressingRoomPage'))
const RecipesPage = lazy(() => import('./pages/RecipesPage'))
const TradeskillLevelingPage = lazy(() => import('./pages/TradeskillLevelingPage'))
const QuestsPage = lazy(() => import('./pages/QuestsPage'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const HelpPage = lazy(() => import('./pages/HelpPage'))
const InventoryPage = lazy(() => import('./pages/InventoryPage'))
const SpellChecklistPage = lazy(() => import('./pages/SpellChecklistPage'))
const InventoryTrackerPage = lazy(() => import('./pages/InventoryTrackerPage'))
const KeyTrackerPage = lazy(() => import('./pages/KeyTrackerPage'))
const LockoutTrackerPage = lazy(() => import('./pages/LockoutTrackerPage'))
const RaidSummaryPage = lazy(() => import('./pages/RaidSummaryPage'))
const RaidCheckPage = lazy(() => import('./pages/RaidCheckPage'))
const RaidEditorPage = lazy(() => import('./pages/RaidEditorPage'))
const WishlistPage = lazy(() => import('./pages/WishlistPage'))
const FactionsPage = lazy(() => import('./pages/FactionsPage'))
const LogFeedPage = lazy(() => import('./pages/LogFeedPage'))
const KillTrackerPage = lazy(() => import('./pages/KillTrackerPage'))
const OverlaysHub = lazy(() => import('./pages/OverlaysHub'))
const CombatLogPage = lazy(() => import('./pages/CombatLogPage'))
const CombatHistoryPage = lazy(() => import('./pages/CombatHistoryPage'))
const TriggersPage = lazy(() => import('./pages/TriggersPage'))
const RollTrackerPage = lazy(() => import('./pages/RollTrackerPage'))
const PlayersPage = lazy(() => import('./pages/PlayersPage'))
const ChatHistoryPage = lazy(() => import('./pages/ChatHistoryPage'))
const LootTrackerPage = lazy(() => import('./pages/LootTrackerPage'))
const CharactersPage = lazy(() => import('./pages/CharactersPage'))
const CharacterProgressPage = lazy(() => import('./pages/CharacterProgressPage'))
const CharacterTasksPage = lazy(() => import('./pages/CharacterTasksPage'))
const GearUpgradeFinderPage = lazy(() => import('./pages/GearUpgradeFinderPage'))
const CharacterSpellsetsPage = lazy(() => import('./pages/CharacterSpellsetsPage'))
const CharacterBandolierPage = lazy(() => import('./pages/CharacterBandolierPage'))
const CharacterMacrosPage = lazy(() => import('./pages/CharacterMacrosPage'))
const CharacterBlockedBuffsPage = lazy(() => import('./pages/CharacterBlockedBuffsPage'))

// MainWindowLayout mounts the audio/alert hooks and renders the main Layout.
// It is only rendered for the "/" route so overlay windows never run these hooks
// and never fire duplicate TTS or sound alerts.
function MainWindowLayout(): React.ReactElement {
  useAudioPrefs()
  useHighContrast()
  useZoom()
  useAudioEngine()
  useTriggerClipboard()
  useTimerAlerts()
  useRespawnAlerts()
  useMetronomeAlerts()
  useItemInfoAutoShow()
  // Keep the Log Feed populating in the background so it persists across tab
  // navigation. Clearing only happens via the user's Trash button or restart.
  useLogFeedSubscriber()

  // Only the one canonical main window may emit trigger/alert audio. Resolve
  // this window's primacy once and hand it to the audio service; a duplicate /
  // ghost main renderer resolves false and stays silent, so a single fire can't
  // play the same sound from two renderers at once. Defaults to owner=true, so
  // a browser dev preview (no IPC) is unaffected.
  useEffect(() => {
    let cancelled = false
    window.electron?.window?.isPrimary?.()
      .then((primary) => { if (!cancelled) setAudioOwner(Boolean(primary)) })
      .catch(() => { /* no IPC (dev preview): stay owner */ })
    return () => { cancelled = true }
  }, [])

  // One-time: fold the old saved Pop Out All set into the Default layout.
  useEffect(() => {
    migratePopoutSelection()
  }, [])

  // Logging in on a character linked to a layout switches to it.
  useLayoutAutoSwitch()

  // Deep links pushed from overlay windows (e.g. an NPC overlay loot row)
  // arrive here as hash routes; navigate the main window to them.
  const navigate = useNavigate()
  useEffect(() => {
    return window.electron?.app?.onNavigate?.((route) => navigate(route))
  }, [navigate])

  // 'unknown' until config has been loaded; then either true (skip wizard)
  // or false (show wizard before mounting the main Layout).
  const [onboardingDone, setOnboardingDone] = useState<boolean | 'unknown'>('unknown')
  // Overlay setup follows first-launch setup (not a rerun from Settings),
  // and opens from the Overlays hub via 'pq:open-overlay-setup'.
  const firstRunRef = useRef(false)
  const [overlaySetup, setOverlaySetup] = useState(false)

  // "What's new" popup: null until a version bump (with unseen changelog
  // entries) is confirmed. appVersion is captured alongside the entries so
  // the dismiss handler can stamp it without a second IPC round-trip.
  const [whatsNew, setWhatsNew] = useState<{ entries: ChangelogEntry[]; appVersion: string } | null>(null)

  useEffect(() => {
    getConfig()
      .then((c) => {
        setOnboardingDone(Boolean(c.onboarding_completed))
        firstRunRef.current = !c.onboarding_completed
        // Mirror the persisted "Minimize to Tray" preference to the main
        // process so the close ('X') behaviour and tray icon match the saved
        // setting from the moment the app loads.
        void window.electron?.window?.setMinimizeToTray(
          Boolean(c.preferences?.minimize_to_tray)
        )

        window.electron?.app?.getVersion?.()
          .then((appVersion) => {
            if (!appVersion) return
            // First run of the "new feature" dots: pick the baseline from
            // the last-seen version as loaded (empty = fresh install) before
            // the stamp below overwrites it. Saved together with the stamp
            // so the two writes can't race.
            const dotsPatch = c.feature_dots?.baseline
              ? {}
              : { feature_dots: initialFeatureDots(c.last_seen_changelog_version, appVersion) }
            if (!c.last_seen_changelog_version) {
              // Fresh install: stamp the current version silently — the
              // wizard above already covers first-run orientation, so
              // there's no "what's new" to show yet.
              void saveConfigChanges(
                { ...c, ...dotsPatch, last_seen_changelog_version: appVersion },
                c,
              )
              return
            }
            if ('feature_dots' in dotsPatch) {
              void saveConfigChanges({ ...c, ...dotsPatch }, c)
            }
            if (c.changelog_popup_disabled) return
            if (c.last_seen_changelog_version === appVersion) return
            getChangelog()
              .then(({ entries }) => {
                // CHANGELOG.md is newest-first; slice down to (but not
                // including) the last-seen version. If that version isn't
                // found (very old install, or a pruned changelog), fall
                // back to just the newest entry rather than dumping the
                // whole history.
                const idx = entries.findIndex((e) => e.version === c.last_seen_changelog_version)
                const since = idx === -1 ? entries.slice(0, 1) : entries.slice(0, idx)
                if (since.length > 0) setWhatsNew({ entries: since, appVersion })
              })
              .catch(() => undefined)
          })
          .catch(() => undefined)
      })
      // If the backend is briefly unreachable on first launch, default to
      // showing the wizard rather than the main UI in an unconfigured state.
      .catch(() => setOnboardingDone(false))

    function handleReopen(): void {
      setOnboardingDone(false)
    }
    function handleOverlaySetup(): void {
      setOverlaySetup(true)
    }
    window.addEventListener('pq:open-onboarding', handleReopen)
    window.addEventListener('pq:open-overlay-setup', handleOverlaySetup)
    return () => {
      window.removeEventListener('pq:open-onboarding', handleReopen)
      window.removeEventListener('pq:open-overlay-setup', handleOverlaySetup)
    }
  }, [])

  function dismissWhatsNew(disablePopup: boolean): void {
    if (!whatsNew) return
    const { appVersion } = whatsNew
    setWhatsNew(null)
    getConfig()
      .then((c) =>
        updateConfig({
          ...c,
          last_seen_changelog_version: appVersion,
          changelog_popup_disabled: c.changelog_popup_disabled || disablePopup,
        })
      )
      .catch(() => undefined)
  }

  if (onboardingDone === 'unknown') return <></>

  return (
    <ActiveCharacterProvider>
      <BackfillProvider>
        {onboardingDone === false && (
          <OnboardingWizard
            allowCancel
            onCancel={() => setOnboardingDone(true)}
            onComplete={() => {
              setOnboardingDone(true)
              if (firstRunRef.current && window.electron?.overlay) setOverlaySetup(true)
              firstRunRef.current = false
            }}
          />
        )}
        {onboardingDone === true && overlaySetup && (
          <OverlaySetupWizard onClose={() => setOverlaySetup(false)} />
        )}
        {onboardingDone === true && whatsNew && (
          <WhatsNewModal entries={whatsNew.entries} onDismiss={dismissWhatsNew} />
        )}
        <FeatureDotsProvider>
          {/* Tours wait while any setup modal is open. */}
          <GuideProvider busy={onboardingDone === false || overlaySetup || !!whatsNew}>
            <Layout />
          </GuideProvider>
        </FeatureDotsProvider>
      </BackfillProvider>
    </ActiveCharacterProvider>
  )
}

export default function MainApp(): React.ReactElement {
  return (
    <Routes>
      {/* Main app routes — wrapped in full Layout with audio/alert hooks */}
      <Route path="/" element={<MainWindowLayout />}>
        <Route index element={<Navigate to="/items" replace />} />
        <Route path="items" element={<ItemsPage />} />
        <Route path="spells" element={<SpellsPage />} />
        <Route path="npcs" element={<NpcsPage />} />
        <Route path="resist-calc" element={<ResistCalcPage />} />
        <Route path="trader-tracker" element={<TraderTrackerPage />} />
        <Route path="charm-pet-finder" element={<CharmPetFinderPage />} />
        <Route path="pop-flags" element={<PoPFlaggingPage />} />
        <Route path="zones" element={<ZonesPage />} />
        <Route path="maps" element={<MapsPage />} />
        <Route path="live-map" element={<LiveMapPage />} />
        <Route path="raid-replay" element={<RaidReplayPage />} />
        <Route path="recipes" element={<RecipesPage />} />
        <Route path="tradeskill-leveling" element={<TradeskillLevelingPage />} />
        <Route path="quests" element={<QuestsPage />} />
        <Route path="inventory" element={<InventoryPage />} />
        <Route path="backup-manager" element={<Navigate to="/settings" replace />} />
        <Route path="log-feed" element={<LogFeedPage />} />
        <Route path="overlays" element={<OverlaysHub />} />
        <Route path="overlays/npc" element={<Navigate to="/overlays" replace />} />
        <Route path="overlays/dps" element={<Navigate to="/overlays" replace />} />
        <Route path="overlays/timers" element={<Navigate to="/overlays" replace />} />
        <Route path="npc-overlay" element={<Navigate to="/overlays" replace />} />
        <Route path="dps-overlay" element={<Navigate to="/overlays" replace />} />
        <Route path="spell-timers" element={<Navigate to="/overlays" replace />} />
        <Route path="combat" element={<CombatLayout />}>
          <Route index element={<Navigate to="/combat/log" replace />} />
          <Route path="log" element={<CombatLogPage />} />
          <Route path="history" element={<CombatHistoryPage />} />
        </Route>
        <Route path="combat-log" element={<Navigate to="/combat/log" replace />} />
        <Route path="combat-history" element={<Navigate to="/combat/history" replace />} />
        <Route path="triggers" element={<TriggersPage />} />
        <Route path="rolls" element={<RollTrackerPage />} />
        <Route path="kills" element={<KillTrackerPage />} />
        <Route path="players" element={<PlayersPage />} />
        <Route path="chat" element={<ChatHistoryPage />} />
        <Route path="loot" element={<LootTrackerPage />} />
        <Route path="characters" element={<CharactersLayout />}>
          <Route index element={<Navigate to="/characters/overview" replace />} />
          <Route path="overview" element={<CharactersPage />} />
          <Route path="progress" element={<CharacterProgressPage />} />
          <Route path="inventory" element={<InventoryTrackerPage />} />
          <Route path="spells" element={<SpellChecklistPage />} />
          <Route path="spellsets" element={<CharacterSpellsetsPage />} />
          <Route path="bandolier" element={<CharacterBandolierPage />} />
          <Route path="macros" element={<CharacterMacrosPage />} />
          <Route path="blocked-buffs" element={<CharacterBlockedBuffsPage />} />
          <Route path="keys" element={<KeyTrackerPage />} />
          <Route path="lockouts" element={<LockoutTrackerPage />} />
          <Route path="wishlist" element={<WishlistPage />} />
          <Route path="factions" element={<FactionsPage />} />
          <Route path="upgrades" element={<GearUpgradeFinderPage />} />
          <Route path="dressing-room" element={<DressingRoomPage />} />
          <Route path="tasks" element={<CharacterTasksPage />} />
        </Route>
        <Route path="raids" element={<RaidsLayout />}>
          <Route index element={<RaidSummaryPage />} />
          <Route path="check" element={<RaidCheckPage />} />
          <Route path="editor" element={<RaidEditorPage />} />
        </Route>
        <Route path="character-progress" element={<Navigate to="/characters/progress" replace />} />
        <Route path="inventory-tracker" element={<Navigate to="/characters/inventory" replace />} />
        <Route path="spell-checklist" element={<Navigate to="/characters/spells" replace />} />
        <Route path="key-tracker" element={<Navigate to="/characters/keys" replace />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="help" element={<HelpPage />} />
        <Route path="help/:slug" element={<HelpPage />} />
      </Route>
    </Routes>
  )
}
