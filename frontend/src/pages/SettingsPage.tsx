import React, { useEffect, useRef, useState } from 'react'
import { useAutoTour } from '../components/guide/GuideProvider'
import { Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { Settings, Search, FolderOpen, Save, AlertTriangle, CheckCircle2, Loader2, X, RefreshCw, Trash2, HardDrive, Sparkles, Volume2, VolumeX, Wifi, FileText, Code2, PanelLeft, Info, Globe, MessageCircle, Coffee, Heart, Eye, Gauge, Download, Headphones, Map as MapIcon, Keyboard as KeyboardIcon } from 'lucide-react'
import BackfillPanel from '../components/settings/BackfillPanel'
import SidebarNavSettings from '../components/settings/SidebarNavSettings'
import EqLogStatusCard from '../components/settings/EqLogStatusCard'
import SetupHealthCard from '../components/settings/SetupHealthCard'
import { TtsVoiceDefault } from '../components/settings/AlertDefaultsSettings'
import PiperTtsSettings from '../components/settings/PiperTtsSettings'
import KokoroTtsSettings from '../components/settings/KokoroTtsSettings'
import DiscordWebhookSettings from '../components/settings/DiscordWebhookSettings'
import { getConfig, saveConfigChanges, getLogStatus, getLogFileInfo, cleanupLog, exportDebugLogs, getServerInfo, testPortAvailability, detectZeal, getZealPipeStatus, getQuarmClientStatus, getEqwStatus, getChangelog, type ServerInfo, type TestPortResult, type ChangelogEntry } from '../services/api'
import { searchSettings } from '../lib/settingsIndex'
import { renderChangelogBody } from '../components/WhatsNewModal'
import type { Config, LogTrimSettings } from '../types/config'
import { DEFAULT_LOG_TRIM } from '../types/config'
import { mergeConfigEdits } from '../lib/configMerge'
import { OVERLAY_DEFS, itemInfoOffered } from '../lib/overlays'
import type { LogFileInfo } from '../types/logEvent'
import { applyContrast } from '../hooks/useHighContrast'
import { applyZoom } from '../hooks/useZoom'
import { OVERLAY_ZOOM_MAX, OVERLAY_ZOOM_MIN } from '../hooks/useOverlayZoom'
import { speakTextForTest, stopTestPlayback } from '../services/audio'
import AudioOutputDeviceSetting from '../components/settings/AudioOutputDeviceSetting'
import type { ZealInstallStatus, ZealPipeStatus } from '../types/zeal'
import type { QuarmClientStatus, QuarmFileStatus, EqwStatus } from '../types/quarm'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

const ZEAL_RELEASE_URL = 'https://github.com/CoastalRedwood/Zeal/releases/latest'
const QUARM_PATCHER_RELEASE_URL = 'https://github.com/Pkelly668/QuarmPatcher/releases/latest'
const EQW_RELEASE_URL = 'https://github.com/CoastalRedwood/eqw_takp/releases/latest'

// Number of changelog versions shown by default on the Changelog tab before
// the user clicks "Show all" — history runs into the dozens of releases.
const CHANGELOG_PAGE_SIZE = 10

// Community / project links surfaced on the About tab.
const DISCORD_URL = 'https://discord.gg/Srj4FXcRaz'
const KOFI_URL = 'https://ko-fi.com/jasonsoprovich'
const GITHUB_URL = 'https://github.com/jasonsoprovich/pq-companion'
const WEBSITE_URL = 'https://pq-companion.com'

// formatManifestDate converts a YYYYMMDD manifest date into a readable form.
// Returns the original string if it doesn't match the expected layout — the
// upstream manifest format is stable but we'd rather show the raw value than
// throw away information on an unexpected shape.
function formatManifestDate(d: string): string {
  if (!/^\d{8}$/.test(d)) return d
  return `${d.slice(0, 4)}-${d.slice(4, 6)}-${d.slice(6, 8)}`
}

import BackupManagerPage from './BackupManagerPage'
import HotkeysSettings from '../components/settings/HotkeysSettings'
import { NewDot, useFeatureDots } from '../hooks/useFeatureDots'
import GameMapExportPanel from '../components/maps/GameMapExportPanel'
import MapSettingsPanel from '../components/settings/MapSettingsPanel'
import DeveloperTab from './DeveloperTab'


type SaveState = 'idle' | 'saving' | 'saved' | 'discarded' | 'error'
type UpdateState = 'idle' | 'checking' | 'up-to-date' | 'available' | 'downloading' | 'downloaded' | 'error'
type Tab = 'general' | 'audio' | 'accessibility' | 'navigation' | 'hotkeys' | 'discord' | 'logs' | 'backups' | 'maps' | 'advanced' | 'about' | 'changelog' | 'developer'

interface TabBarProps {
  tabs: { id: Tab; label: string; icon: React.ReactNode }[]
  active: Tab
  onChange: (t: Tab) => void
  // Tabs that need the user's attention get an orange dot; the string is the
  // hover text explaining why.
  alerts?: Partial<Record<Tab, string>>
}

// Settings tabs are grouped into labelled sections in the vertical sidebar so
// the (now ~11) tabs stay scannable instead of crowding a horizontal bar. Any
// tab not listed here (e.g. a future addition) still renders, ungrouped, at the
// end — so forgetting to register one degrades gracefully rather than hiding it.
const TAB_GROUPS: { label: string; ids: Tab[] }[] = [
  { label: 'General', ids: ['general', 'audio', 'accessibility'] },
  { label: 'Interface', ids: ['navigation', 'hotkeys', 'discord'] },
  { label: 'System', ids: ['logs', 'backups', 'maps', 'advanced'] },
  { label: 'About', ids: ['about', 'changelog', 'developer'] },
]

// TabBar renders the settings navigation as a vertical sidebar (a second nav
// beside the main app sidebar, scoped to the Settings page). Grouped by
// TAB_GROUPS; the provided `tabs` list still controls which entries exist
// (e.g. the developer tab only when unlocked).
function TabBar({ tabs, active, onChange, alerts }: TabBarProps): React.ReactElement {
  const byId = new Map(tabs.map((t) => [t.id, t]))
  const grouped = new Set(TAB_GROUPS.flatMap((g) => g.ids))
  const { settingsTabDot } = useFeatureDots()
  const leftovers = tabs.filter((t) => !grouped.has(t.id))
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const hits = query.trim() ? searchSettings(query, 12) : null

  const renderButton = ({ id, label, icon }: TabBarProps['tabs'][number]): React.ReactElement => (
    <button
      key={id}
      onClick={() => onChange(id)}
      title={alerts?.[id]}
      // text-left because a button centres its text by default, which only
      // shows up on a label long enough to wrap — "In-Game Map Markers" sat
      // centred while every one-line label above it was flush left.
      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs font-medium transition-colors"
      style={{
        backgroundColor: active === id ? 'var(--color-surface-2)' : 'transparent',
        color: active === id ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
      }}
    >
      {/* Without shrink-0 the icon is squeezed narrower on a wrapping row,
          which misaligns it with the icons above and below. */}
      <span className="shrink-0">{icon}</span>
      {label}
      {alerts?.[id] && (
        <span
          className="ml-auto h-2 w-2 shrink-0 rounded-full"
          style={{ backgroundColor: '#f97316' }}
          title={alerts[id]}
        />
      )}
      {!alerts?.[id] && active !== id && settingsTabDot(id) && <NewDot className="ml-auto" />}
    </button>
  )

  return (
    <nav
      data-tour="settings-tabs"
      className="flex w-44 shrink-0 flex-col gap-0.5 overflow-y-auto border-r p-2"
      style={{ borderColor: 'var(--color-border)' }}
    >
      <div className="mb-2 flex items-center gap-2 px-2 pt-1">
        <Settings size={16} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Settings
        </span>
      </div>
      <div className="relative mb-2" data-tour="settings-search">
        <Search
          size={12}
          className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2"
          style={{ color: 'var(--color-muted)' }}
        />
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search settings…"
          className="w-full rounded py-1 pl-6 pr-1 text-xs outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        />
      </div>
      {hits && (
        <div className="mb-1">
          {hits.length === 0 ? (
            <p className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>
              No matching settings.
            </p>
          ) : (
            hits.map((h) => (
              <button
                key={h.id}
                onClick={() => {
                  setQuery('')
                  navigate(h.to.path + (h.to.search ?? ''))
                }}
                className="block w-full rounded px-2 py-1.5 text-left text-xs hover:bg-white/5"
                style={{ cursor: 'pointer' }}
              >
                <span className="block" style={{ color: 'var(--color-foreground)' }}>{h.label}</span>
                <span className="block text-[10px]" style={{ color: 'var(--color-muted)' }}>{h.where}</span>
              </button>
            ))
          )}
        </div>
      )}
      {!hits && TAB_GROUPS.map((group) => {
        const items = group.ids
          .map((id) => byId.get(id))
          .filter((t): t is TabBarProps['tabs'][number] => Boolean(t))
        if (items.length === 0) return null
        return (
          <div key={group.label} className="mb-1">
            <p
              className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider"
              style={{ color: 'var(--color-muted)' }}
            >
              {group.label}
            </p>
            {items.map(renderButton)}
          </div>
        )
      })}
      {!hits && leftovers.length > 0 && <div className="mb-1">{leftovers.map(renderButton)}</div>}
    </nav>
  )
}

// QuarmFileRow renders one client DLL's status: a colored verdict pill plus
// detail lines for local version/build-date and the matching manifest entry.
// Used by the "EQ Client Version" section.
function QuarmFileRow({ file }: { file: QuarmFileStatus }): React.ReactElement {
  const verdict = (() => {
    switch (file.status) {
      case 'match':
        return { label: 'Up to date', color: '#22c55e', icon: <CheckCircle2 size={12} /> }
      case 'mismatch':
        return { label: 'Out of date', color: '#f59e0b', icon: <AlertTriangle size={12} /> }
      case 'missing':
        return { label: 'Missing', color: '#f87171', icon: <AlertTriangle size={12} /> }
      case 'unknown':
      default:
        return { label: 'Unknown', color: 'var(--color-muted)', icon: <FileText size={12} /> }
    }
  })()

  return (
    <div
      className="mb-2 rounded px-3 py-2 text-xs last:mb-0"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        border: '1px solid var(--color-border)',
      }}
    >
      <div className="flex items-center justify-between">
        <code style={{ color: 'var(--color-foreground)' }}>{file.name}</code>
        <span
          className="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-[11px]"
          style={{ color: verdict.color, border: `1px solid ${verdict.color}` }}
        >
          {verdict.icon}
          {verdict.label}
        </span>
      </div>

      {file.local && (
        <div className="mt-1 space-y-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
          {file.local.file_version && (
            <div>
              FileVersion: <span style={{ color: 'var(--color-foreground)' }}>{file.local.file_version}</span>
            </div>
          )}
          <div>
            Built: <span style={{ color: 'var(--color-foreground)' }}>
              {new Date(file.local.compiled_at).toLocaleDateString()}
            </span>
            {' · '}
            Size: <span style={{ color: 'var(--color-foreground)' }}>{file.local.size.toLocaleString()} bytes</span>
          </div>
          <div className="font-mono">
            MD5: <span style={{ color: 'var(--color-foreground)' }}>{file.local.md5}</span>
          </div>
        </div>
      )}

      {file.manifest && (
        <div className="mt-1 space-y-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
          <div>
            Manifest expects size{' '}
            <span style={{ color: 'var(--color-foreground)' }}>{file.manifest.size.toLocaleString()}</span>
            , dated{' '}
            <span style={{ color: 'var(--color-foreground)' }}>{formatManifestDate(file.manifest.date)}</span>
          </div>
          {file.manifest.ref_file_version && (
            <div>
              Manifest FileVersion:{' '}
              <span style={{ color: 'var(--color-foreground)' }}>{file.manifest.ref_file_version}</span>
            </div>
          )}
          {file.local && file.local.md5 !== file.manifest.md5 && (
            <div className="font-mono">
              Manifest MD5: <span style={{ color: 'var(--color-foreground)' }}>{file.manifest.md5}</span>
            </div>
          )}
        </div>
      )}

      {file.reason && (
        <p className="mt-1" style={{ color: 'var(--color-muted-foreground)' }}>
          {file.reason}
        </p>
      )}
    </div>
  )
}

// EqwFileRow renders the eqw.dll (EQW-TAKP) verdict alongside the eqgame.dll
// rows. Unlike eqgame.dll, eqw.dll is optional (windowed-mode wrapper) and has
// no manifest entry, so "not installed" is neutral, not an error, and the check
// is version-vs-latest rather than an MD5 match.
function EqwFileRow({ status }: { status: EqwStatus }): React.ReactElement {
  const verdict = (() => {
    if (!status.installed)
      return { label: 'Not installed', color: 'var(--color-muted)', icon: <FileText size={12} /> }
    if (status.update_available)
      return { label: 'Update available', color: '#f59e0b', icon: <AlertTriangle size={12} /> }
    if (!status.version)
      return { label: 'Unknown', color: 'var(--color-muted)', icon: <FileText size={12} /> }
    if (status.latest_version)
      return { label: 'Up to date', color: '#22c55e', icon: <CheckCircle2 size={12} /> }
    return { label: 'Installed', color: 'var(--color-muted)', icon: <FileText size={12} /> }
  })()

  return (
    <div
      className="mb-2 rounded px-3 py-2 text-xs last:mb-0"
      style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center justify-between">
        <code style={{ color: 'var(--color-foreground)' }}>eqw.dll</code>
        <span
          className="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-[11px]"
          style={{ color: verdict.color, border: `1px solid ${verdict.color}` }}
        >
          {verdict.icon}
          {verdict.label}
        </span>
      </div>

      {status.installed ? (
        <div className="mt-1 space-y-0.5" style={{ color: 'var(--color-muted-foreground)' }}>
          <div>
            Installed:{' '}
            <span style={{ color: 'var(--color-foreground)' }}>
              {status.version ? `v${status.version}` : 'version unreadable'}
            </span>
            {status.latest_version && (
              <>
                {' · '}Latest:{' '}
                <span style={{ color: 'var(--color-foreground)' }}>v{status.latest_version}</span>
              </>
            )}
          </div>
          {status.update_available && (
            <p>
              A newer EQW-TAKP is available.{' '}
              <a
                href={EQW_RELEASE_URL}
                target="_blank"
                rel="noreferrer noopener"
                className="underline"
                style={{ color: 'var(--color-primary)' }}
              >
                Get it here
              </a>
              .
            </p>
          )}
        </div>
      ) : (
        <p className="mt-1" style={{ color: 'var(--color-muted-foreground)' }}>
          The windowed-mode wrapper isn&apos;t present. This is optional — install{' '}
          <a
            href={EQW_RELEASE_URL}
            target="_blank"
            rel="noreferrer noopener"
            className="underline"
            style={{ color: 'var(--color-primary)' }}
          >
            EQW-TAKP
          </a>{' '}
          only if you run the client in windowed mode.
        </p>
      )}
    </div>
  )
}

const SETTINGS_TAB_KEY = 'pq-tab:settings'

// Settings tabs whose contents moved into the Overlays hub's settings drawer,
// mapped to the drawer that holds them now. Old ?tab= links redirect there.
const MOVED_TO_OVERLAYS_HUB: Record<string, string> = {
  npcoverlay: 'npc',
  iteminfo: 'itemInfo',
  pet: 'pet',
  spelltimers: 'timers',
  overlays: 'global',
  dpscolors: 'dps',
}

const VALID_TABS: Tab[] = ['general', 'audio', 'accessibility', 'navigation', 'hotkeys', 'discord', 'logs', 'backups', 'maps', 'advanced', 'about', 'changelog', 'developer']

export default function SettingsPage(): React.ReactElement {
  useAutoTour('settings')
  const [searchParams] = useSearchParams()
  // Allow deep-linking to a specific tab via ?tab= (e.g. links to the Logs tab
  // for backfill). Falls back to General for unknown/absent values.
  const [tab, setTab] = useState<Tab>(() => {
    const t = searchParams.get('tab') ?? ''
    // 'gamemaps' was its own tab until the two map tabs merged. Kept as an
    // alias so links written against the old name still land somewhere useful.
    if (t === 'gamemaps') return 'maps'
    if ((VALID_TABS as string[]).includes(t)) return t as Tab
    // No (valid) deep link: reopen the tab the user was last on. The hidden
    // Developer tab is never restored — it only shows while dev mode is on.
    try {
      const stored = localStorage.getItem(SETTINGS_TAB_KEY) ?? ''
      if (stored !== 'developer' && (VALID_TABS as string[]).includes(stored)) return stored as Tab
    } catch {
      /* fall through */
    }
    return 'general'
  })

  // Opening a tab clears its green "new feature" dot.
  const { markSettingsTabSeen } = useFeatureDots()
  useEffect(() => {
    markSettingsTabSeen(tab)
  }, [tab, markSettingsTabSeen])
  // ?tab= links (the large-log dot, Logs <-> Backup & Restore, Ctrl+K and
  // Settings search results). The initializer above only runs on mount, so
  // also honour the param when navigating here while the page is already
  // open (keyed on location.key so repeat clicks re-apply).
  const location = useLocation()
  useEffect(() => {
    const t = searchParams.get('tab') as Tab | null
    if (t && VALID_TABS.includes(t)) setTab(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.key])
  useEffect(() => {
    if (tab === 'developer') return
    try {
      localStorage.setItem(SETTINGS_TAB_KEY, tab)
    } catch {
      /* noop */
    }
  }, [tab])
  const [config, setConfig] = useState<Config | null>(null)
  const [originalConfig, setOriginalConfig] = useState<Config | null>(null)
  // Latest config / last-saved config mirrored into refs so the unmount flush
  // can read them without re-subscribing.
  const originalConfigRef = useRef<Config | null>(null)
  useEffect(() => {
    originalConfigRef.current = originalConfig
  }, [originalConfig])
  const configRef = useRef<Config | null>(null)
  useEffect(() => {
    configRef.current = config
  }, [config])
  const autosaveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Always-latest persist fn, kept in a ref so the debounce effect needn't list
  // it as a dependency.
  const persistRef = useRef<(c: Config) => void>(() => {})
  // Settings autosave: edits persist automatically. The UI reflects changes
  // immediately (local state); the PUT is debounced so dragging a slider or
  // typing doesn't spam the backend. No config field triggers an expensive
  // backend side effect, so a single whole-config debounce is safe. On leaving
  // Settings, any pending change is flushed (and high-contrast/zoom previews are
  // now permanent, so there's nothing to revert).
  useEffect(() => {
    return () => {
      if (autosaveTimer.current) clearTimeout(autosaveTimer.current)
      const c = configRef.current
      const oc = originalConfigRef.current
      if (c && oc && JSON.stringify(c) !== JSON.stringify(oc)) {
        void saveConfigChanges(c, oc)
      }
    }
  }, [])
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [saveError, setSaveError] = useState<string | null>(null)
  const [appVersion, setAppVersion] = useState<string | null>(null)
  const [updateState, setUpdateState] = useState<UpdateState>('idle')
  const [updateVersion, setUpdateVersion] = useState<string | null>(null)
  const [updateError, setUpdateError] = useState<string | null>(null)
  // Changelog tab: fetched lazily on first visit rather than at page load,
  // since most Settings visits never touch this tab. History runs into the
  // dozens of versions, so only the most recent CHANGELOG_PAGE_SIZE render by
  // default — "Show all" reveals the rest rather than dumping every release
  // note into one enormous scroll.
  const [changelogEntries, setChangelogEntries] = useState<ChangelogEntry[] | null>(null)
  const [showAllChangelog, setShowAllChangelog] = useState(false)
  useEffect(() => {
    if (tab === 'changelog' && changelogEntries === null) {
      getChangelog()
        .then(({ entries }) => setChangelogEntries(entries))
        .catch(() => setChangelogEntries([]))
    }
  }, [tab, changelogEntries])

  // Voice Rate preview: previews the live slider value (which may not be
  // saved yet) so the user can hear the speed before committing. Single
  // channel via the shared test-playback slot in services/audio.
  const [ttsPreviewPlaying, setTtsPreviewPlaying] = useState(false)
  useEffect(() => () => { stopTestPlayback() }, [])

  const [serverInfo, setServerInfo] = useState<ServerInfo | null>(null)
  const [portTestState, setPortTestState] = useState<'idle' | 'testing'>('idle')
  const [portTestResult, setPortTestResult] = useState<TestPortResult | null>(null)
  const [portTestPort, setPortTestPort] = useState<number | null>(null)
  const [configFolder, setConfigFolder] = useState<string | null>(null)

  const [zealStatus, setZealStatus] = useState<ZealInstallStatus | null>(null)
  const [zealChecking, setZealChecking] = useState(false)
  const [zealError, setZealError] = useState<string | null>(null)
  const [pipeStatus, setPipeStatus] = useState<ZealPipeStatus | null>(null)

  const [quarmStatus, setQuarmStatus] = useState<QuarmClientStatus | null>(null)
  const [quarmChecking, setQuarmChecking] = useState(false)
  const [quarmError, setQuarmError] = useState<string | null>(null)
  // eqw.dll (EQW-TAKP) rides in the same "EQ Client Version" section but has
  // its own endpoint (string-scan + GitHub tag, not the manifest). A failed
  // fetch is non-fatal — we just hide its row rather than error the section.
  const [eqwStatus, setEqwStatus] = useState<EqwStatus | null>(null)

  const [logLargeFile, setLogLargeFile] = useState(false)
  const [logFileInfo, setLogFileInfo] = useState<LogFileInfo | null>(null)
  const [logInfoLoading, setLogInfoLoading] = useState(false)
  const [cleanupState, setCleanupState] = useState<'idle' | 'running' | 'done' | 'error'>('idle')
  const [cleanupResult, setCleanupResult] = useState<string | null>(null)
  // Size of the live log right after Archive & Trim, so the result can say
  // whether the trim got it under the warning threshold.
  const [trimmedSize, setTrimmedSize] = useState<number | null>(null)
  const logPollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const saveStateClearRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const saveStateShowRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (saveStateClearRef.current) clearTimeout(saveStateClearRef.current)
      if (saveStateShowRef.current) clearTimeout(saveStateShowRef.current)
    }
  }, [])

  async function checkZeal(): Promise<void> {
    setZealChecking(true)
    setZealError(null)
    try {
      const [install, pipe] = await Promise.all([
        detectZeal(),
        getZealPipeStatus().catch(() => null),
      ])
      setZealStatus(install)
      setPipeStatus(pipe)
    } catch (err) {
      setZealError((err as Error).message)
      setZealStatus(null)
    } finally {
      setZealChecking(false)
    }
  }

  async function checkQuarmClient(): Promise<void> {
    setQuarmChecking(true)
    setQuarmError(null)
    try {
      const status = await getQuarmClientStatus()
      setQuarmStatus(status)
    } catch (err) {
      setQuarmError((err as Error).message)
      setQuarmStatus(null)
    } finally {
      setQuarmChecking(false)
    }
    // eqw.dll is a best-effort add-on to the same section; keep it out of the
    // main try so a failed eqw fetch never surfaces as an EQ-client error.
    try {
      setEqwStatus(await getEqwStatus())
    } catch {
      setEqwStatus(null)
    }
  }

  // Subscribe to backend pipe state changes so the status row reflects
  // connect/disconnect without the user having to click Re-check. The Re-check
  // button is still useful for re-running the filesystem detection when the
  // user has just installed Zeal — that signal doesn't come over the wire.
  useWebSocket((msg: WsMessage) => {
    if (msg.type !== WSEvent.ZealConnected && msg.type !== WSEvent.ZealDisconnected) return
    getZealPipeStatus().then(setPipeStatus).catch(() => null)
  })

  // Config also changes outside this page: overlay windows save their own
  // zoom (Ctrl+scroll — see useOverlayZoom) and other settings, and the
  // backend clears the manual character override on login. Rebase the draft
  // onto the server copy so those changes show here and so the next autosave
  // diffs against them instead of writing the old values back. An edit the
  // user has made here but not yet saved wins.
  useWebSocket((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((srv) => {
        const base = originalConfigRef.current
        if (!base) return
        setConfig((c) => c && mergeConfigEdits(srv, base, c))
        setOriginalConfig(srv)
      })
      .catch(() => null)
  })

  function flashSaveState(state: SaveState, ms = 2500): void {
    if (saveStateClearRef.current) clearTimeout(saveStateClearRef.current)
    setSaveState(state)
    saveStateClearRef.current = setTimeout(() => {
      setSaveState('idle')
      setSaveError(null)
    }, ms)
  }


  useEffect(() => {
    getConfig()
      .then((c) => {
        setConfig(c)
        setOriginalConfig(c)
      })
      .catch((err: Error) => setLoadError(err.message))

    if (window.electron?.app) {
      window.electron.app.getVersion().then(setAppVersion).catch(() => null)
    }

    // Pre-fetch the config folder path so the error fallback can show it
    // immediately if the backend fetch above fails.
    if (window.electron?.shell) {
      window.electron.shell.getConfigFolderPath().then(setConfigFolder).catch(() => null)
    }

    getServerInfo().then(setServerInfo).catch(() => null)

    void checkZeal()
    void checkQuarmClient()

    const pollLogSize = () => {
      getLogStatus()
        .then((s) => setLogLargeFile(s.large_file))
        .catch(() => null)
    }
    pollLogSize()
    logPollRef.current = setInterval(pollLogSize, 10 * 60 * 1000)
    return () => {
      if (logPollRef.current) clearInterval(logPollRef.current)
    }
  }, [])

  useEffect(() => {
    if (!window.electron?.updater) return
    const offAvailable = window.electron.updater.onAvailable((info) => {
      setUpdateState('available')
      setUpdateVersion(info.version)
    })
    const offProgress = window.electron.updater.onProgress(() => {
      setUpdateState('downloading')
    })
    const offDownloaded = window.electron.updater.onDownloaded((info) => {
      setUpdateState('downloaded')
      setUpdateVersion(info.version)
    })
    const offError = window.electron.updater.onError((msg) => {
      setUpdateState('error')
      setUpdateError(msg)
    })
    return () => {
      offAvailable()
      offProgress()
      offDownloaded()
      offError()
    }
  }, [])

  // Parses a configured listen address (e.g. ":17654", "127.0.0.1:17654") into
  // a port number. Returns 0 when the addr is the auto-assign sentinel ":0".
  function parsePortFromAddr(addr: string): number {
    const m = addr.match(/:(\d+)$/)
    if (m) return Number(m[1])
    const n = Number(addr)
    return Number.isFinite(n) ? n : 0
  }

  async function handlePortTest(): Promise<void> {
    if (!config) return
    const port = parsePortFromAddr(config.server_addr)
    if (port === 0) {
      // No specific port to test — auto-assign will always succeed.
      setPortTestResult({ available: true })
      setPortTestPort(0)
      return
    }
    setPortTestState('testing')
    setPortTestResult(null)
    try {
      const result = await testPortAvailability(port)
      setPortTestResult(result)
      setPortTestPort(port)
    } catch (err) {
      setPortTestResult({ available: false, error: (err as Error).message })
      setPortTestPort(port)
    } finally {
      setPortTestState('idle')
    }
  }

  async function handleBrowse(): Promise<void> {
    if (!window.electron?.dialog) return
    const folder = await window.electron.dialog.selectFolder()
    if (folder && config) {
      setConfig({ ...config, eq_path: folder })
    }
  }

  // persist writes the given config now. Used by both autosave and the explicit
  // Advanced-tab apply button. Only the fields that differ from the saved
  // baseline (originalConfig) are written, onto a fresh server copy, so values
  // changed elsewhere since this page loaded aren't reverted. Afterwards the
  // draft is rebased onto the saved config rather than replaced, so edits the
  // user makes while the save is in flight survive.
  function persist(c: Config): void {
    const base = originalConfigRef.current ?? c
    if (saveStateClearRef.current) clearTimeout(saveStateClearRef.current)
    if (saveStateShowRef.current) clearTimeout(saveStateShowRef.current)
    setSaveError(null)
    // The autosave PUT is a localhost round trip and usually resolves in a
    // few ms, so showing "Saving…" immediately just makes the indicator
    // flicker between "Saving…" and "Saved" as the user types with natural
    // pauses. Only surface "Saving…" if the request is still in flight after
    // a short delay; fast saves jump straight to "Saved".
    saveStateShowRef.current = setTimeout(() => setSaveState('saving'), 200)
    saveConfigChanges(c, base)
      .then((saved) => {
        if (saveStateShowRef.current) clearTimeout(saveStateShowRef.current)
        setConfig((cur) => cur && mergeConfigEdits(saved, base, cur))
        setOriginalConfig(saved)
        flashSaveState('saved')
      })
      .catch((err: Error) => {
        if (saveStateShowRef.current) clearTimeout(saveStateShowRef.current)
        setSaveError(err.message)
        setSaveState('error')
      })
  }
  persistRef.current = persist

  function handleSave(): void {
    if (config) persist(config)
  }

  // Debounced autosave: schedule a save whenever the draft differs from the
  // last-saved config.
  useEffect(() => {
    if (!config || !originalConfig) return
    if (JSON.stringify(config) === JSON.stringify(originalConfig)) return
    if (autosaveTimer.current) clearTimeout(autosaveTimer.current)
    autosaveTimer.current = setTimeout(() => persistRef.current(config), 600)
    return () => {
      if (autosaveTimer.current) clearTimeout(autosaveTimer.current)
    }
  }, [config, originalConfig])

  async function handleCheckForUpdates(): Promise<void> {
    if (!window.electron?.updater) return
    setUpdateState('checking')
    setUpdateError(null)
    await window.electron.updater.check()
    // If no event fires within 4s, assume up to date
    setTimeout(() => {
      setUpdateState((prev) => (prev === 'checking' ? 'up-to-date' : prev))
    }, 4_000)
  }

  function handleQuitAndInstall(): void {
    window.electron?.updater?.quitAndInstall()
  }

  function handleLoadLogInfo(): void {
    setLogInfoLoading(true)
    getLogFileInfo()
      .then((fi) => setLogFileInfo(fi))
      .catch(() => null)
      .finally(() => setLogInfoLoading(false))
  }

  async function handleCleanupLog(): Promise<void> {
    setCleanupState('running')
    setCleanupResult(null)
    try {
      const result = await cleanupLog()
      setCleanupResult(result.backup_path)
      setCleanupState('done')
      setLogLargeFile(false)
      setLogFileInfo(null)
      setTrimmedSize(null)
      // Re-poll size after purge
      getLogStatus()
        .then((s) => {
          setLogLargeFile(s.large_file)
          setTrimmedSize(s.size_bytes)
        })
        .catch(() => null)
    } catch (err) {
      setCleanupResult((err as Error).message)
      setCleanupState('error')
    }
  }

  // Mirrors the backend's liveWriteWindow (log.go): EverQuest still has the
  // file open while playing, so archiving a log written to this recently
  // risks corrupting the tail. The backend enforces this too; this is just
  // so the button doesn't invite the click in the first place.
  const logRecentlyActive =
    !!logFileInfo?.newest_entry && Date.now() - new Date(logFileInfo.newest_entry).getTime() < 2 * 60 * 1000

  const logTrim: LogTrimSettings = config?.log_trim ?? DEFAULT_LOG_TRIM
  function setLogTrim(patch: Partial<LogTrimSettings>): void {
    if (config) setConfig({ ...config, log_trim: { ...logTrim, ...patch } })
  }

  // The warning threshold is configurable, so re-check the large-file flag
  // once a changed threshold has been saved.
  const savedWarnMB = originalConfig?.log_trim?.warn_mb
  useEffect(() => {
    if (savedWarnMB === undefined) return
    getLogStatus()
      .then((s) => setLogLargeFile(s.large_file))
      .catch(() => null)
  }, [savedWarnMB])

  const developerMode = config?.preferences?.developer_mode ?? false

  // Ctrl+Shift+D toggles the hidden Developer tab while the Settings page is
  // focused. Persists via the preferences PUT so it survives restarts.
  // Deliberately a chord (not a single key) so it can't fire by accident.
  useEffect(() => {
    if (!config) return
    const handler = (e: KeyboardEvent): void => {
      if (!(e.ctrlKey || e.metaKey) || !e.shiftKey) return
      if (e.key !== 'D' && e.key !== 'd') return
      e.preventDefault()
      const next: Config = {
        ...config,
        preferences: { ...config.preferences, developer_mode: !developerMode },
      }
      setConfig(next)
      persist(next)
      // Hop straight to the tab the first time the user reveals it so the
      // unlock isn't silent. When hiding, fall back to General.
      setTab(developerMode ? 'general' : 'developer')
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [config, developerMode])

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'general', label: 'General', icon: <Settings size={13} /> },
    { id: 'audio', label: 'Audio', icon: <Volume2 size={13} /> },
    { id: 'accessibility', label: 'Accessibility', icon: <Eye size={13} /> },
    { id: 'navigation', label: 'Navigation', icon: <PanelLeft size={13} /> },
    { id: 'hotkeys', label: 'Hotkeys', icon: <KeyboardIcon size={13} /> },
    { id: 'discord', label: 'Discord', icon: <Headphones size={13} /> },
    { id: 'logs', label: 'Logs', icon: <FileText size={13} /> },
    { id: 'backups', label: 'Backup & Restore', icon: <HardDrive size={13} /> },
    { id: 'maps', label: 'Maps', icon: <MapIcon size={13} /> },
    { id: 'advanced', label: 'Advanced', icon: <Wifi size={13} /> },
    { id: 'about', label: 'About', icon: <Info size={13} /> },
    { id: 'changelog', label: 'Changelog', icon: <Sparkles size={13} /> },
    ...(developerMode
      ? [{ id: 'developer' as Tab, label: 'Developer', icon: <Code2 size={13} /> }]
      : []),
  ]

  const tabAlerts: Partial<Record<Tab, string>> = logLargeFile
    ? { logs: 'Your EverQuest log file is large — archive & trim it here' }
    : {}

  if (loadError && tab !== 'backups') {
    const configPath = configFolder ? `${configFolder}\\config.yaml` : null
    return (
      <div className="flex h-full">
        <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
        <div className="flex flex-1 items-start justify-center overflow-y-auto p-6">
          <div className="flex w-full max-w-xl flex-col gap-4">
            <div className="flex items-center gap-3">
              <AlertTriangle size={22} style={{ color: '#f97316' }} />
              <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
                Can't reach the backend
              </h2>
            </div>

            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Settings failed to load: <span className="font-mono">{loadError}</span>.
              The PQ Companion backend service isn't responding on its local port.
            </p>

            <div
              className="rounded border p-3 text-xs"
              style={{
                borderColor: 'var(--color-border)',
                backgroundColor: 'var(--color-muted-background, rgba(255,255,255,0.02))',
                color: 'var(--color-muted-foreground)',
              }}
            >
              <p className="mb-2 font-semibold" style={{ color: 'var(--color-foreground)' }}>
                Most common cause: antivirus
              </p>
              <p className="mb-2">
                Windows Defender or another antivirus may have quarantined
                <span className="font-mono"> pq-companion-server.exe</span>.
                Check Windows Security → Virus &amp; threat protection →
                Protection history. If the file is listed, restore it and add
                an exclusion, then relaunch PQ Companion.
              </p>
              <p className="mb-2">
                Other possibilities: a stuck listener on the configured port,
                or a firewall blocking loopback traffic. As a workaround you
                can change the port manually in <span className="font-mono">config.yaml</span>:
              </p>
              {configPath && (
                <p className="font-mono break-all" style={{ color: 'var(--color-foreground)' }}>
                  {configPath}
                </p>
              )}
            </div>

            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={() => {
                  if (window.electron?.shell) {
                    window.electron.shell.openConfigFolder().catch(() => null)
                  }
                }}
                disabled={!window.electron?.shell}
                className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-primary-foreground, #000)',
                }}
              >
                <FolderOpen size={13} />
                Open config folder
              </button>
              <button
                type="button"
                onClick={() => {
                  if (window.electron?.shell) {
                    window.electron.shell.openLogsFolder().catch(() => null)
                  }
                }}
                disabled={!window.electron?.shell}
                className="inline-flex items-center gap-1.5 rounded border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
                style={{
                  borderColor: 'var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <FileText size={13} />
                Open logs folder
              </button>
              <button
                type="button"
                onClick={() => window.location.reload()}
                className="inline-flex items-center gap-1.5 rounded border px-3 py-1.5 text-xs font-medium transition-colors"
                style={{
                  borderColor: 'var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <RefreshCw size={13} />
                Retry
              </button>
            </div>
          </div>
        </div>
      </div>
    )
  }

  if (!config && tab !== 'backups') {
    return (
      <div className="flex h-full">
        <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
        <div className="flex flex-1 flex-col items-center justify-center gap-3">
          <Loader2 size={24} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Loading settings…
          </p>
        </div>
      </div>
    )
  }

  // One tab for everything map-shaped. These were two tabs with near-identical
  // names ("Maps" / "In-Game Map Markers"), which asked the user to know the
  // difference between how we draw maps and what we write into the game's
  // before they could find either.
  if (tab === 'maps') {
    return (
      <div className="flex h-full">
        <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
        <div className="min-h-0 flex-1 overflow-y-auto">
          <MapSettingsPanel />
          <div
            className="mx-5 border-t"
            style={{ borderColor: 'var(--color-border)' }}
          />
          <GameMapExportPanel />
        </div>
      </div>
    )
  }

  if (tab === 'backups') {
    return (
      <div className="flex h-full">
        <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
        <div className="min-h-0 flex-1">
          <BackupManagerPage />
        </div>
      </div>
    )
  }

  if (tab === 'developer') {
    return (
      <div className="flex h-full">
        <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
        <div className="min-h-0 flex-1 overflow-y-auto">
          <DeveloperTab />
        </div>
      </div>
    )
  }

  if (tab === 'navigation') {
    return (
      <div className="flex h-full">
        <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
        <div className="min-h-0 flex-1 overflow-y-auto">
          <SidebarNavSettings />
        </div>
      </div>
    )
  }

  const movedTo = MOVED_TO_OVERLAYS_HUB[searchParams.get('tab') ?? '']
  if (movedTo) return <Navigate to={`/overlays?panel=${movedTo}`} replace />

  if (!config) return <></>


  const hasElectronDialog = Boolean(window.electron?.dialog)
  const hasElectronUpdater = Boolean(window.electron?.updater)

  return (
    <div className="flex h-full">
      <TabBar tabs={tabs} active={tab} onChange={setTab} alerts={tabAlerts} />
      <div className="flex-1 overflow-y-auto">
      <div className="mx-auto max-w-xl p-6">
      {/* Explains the orange dot on the main sidebar's Settings button, which
          otherwise gives no hint where to look. */}
      {logLargeFile && tab !== 'logs' && (
        <div
          className="mb-6 flex items-start gap-3 rounded-lg p-3 text-xs"
          style={{
            border: '1px solid #f97316',
            backgroundColor: 'rgba(249, 115, 22, 0.08)',
            color: 'var(--color-muted-foreground)',
          }}
        >
          <AlertTriangle size={16} className="mt-0.5 shrink-0" style={{ color: '#f97316' }} />
          <div className="flex-1">
            <p className="mb-0.5 font-semibold" style={{ color: 'var(--color-foreground)' }}>
              Your EverQuest log file is getting large
            </p>
            <p>
              That's what the orange dot on Settings means. Nothing is broken —
              archiving & trimming it from the Logs tab keeps backfills quick
              and frees disk space.
            </p>
          </div>
          <button
            type="button"
            onClick={() => setTab('logs')}
            className="shrink-0 rounded px-3 py-1.5 text-xs font-medium"
            style={{ backgroundColor: '#f97316', color: '#fff' }}
          >
            Go to Logs
          </button>
        </div>
      )}

      {/* Page header — shows the active tab's name; the sidebar carries the
          overall "Settings" branding. */}
      <div className="mb-6 flex items-center gap-3">
        <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {tabs.find((t) => t.id === tab)?.label ?? 'Settings'}
        </h1>
      </div>

      <div className="flex flex-col gap-6">
        {tab === 'general' && <SetupHealthCard />}

        {/* ── App ──────────────────────────────────────────────────────────── */}
        {tab === 'general' && (
        <section data-setting="app"
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-3 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            App
          </h2>

          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Version
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {appVersion ? `v${appVersion}` : 'Unknown'}
              </p>
            </div>

            {hasElectronUpdater && (
              <div className="flex items-center gap-2">
                {updateState === 'downloaded' && (
                  <button
                    onClick={handleQuitAndInstall}
                    className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
                    style={{
                      backgroundColor: '#22c55e',
                      color: '#fff',
                      border: 'none',
                      cursor: 'pointer',
                    }}
                  >
                    Install v{updateVersion} &amp; Restart
                  </button>
                )}
                {updateState !== 'downloaded' && (
                  <button
                    onClick={handleCheckForUpdates}
                    disabled={updateState === 'checking' || updateState === 'downloading'}
                    className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                    style={{
                      backgroundColor: 'var(--color-surface-2)',
                      border: '1px solid var(--color-border)',
                      color: 'var(--color-foreground)',
                      cursor: updateState === 'checking' || updateState === 'downloading' ? 'not-allowed' : 'pointer',
                      opacity: updateState === 'checking' || updateState === 'downloading' ? 0.7 : 1,
                    }}
                  >
                    {updateState === 'checking' || updateState === 'downloading' ? (
                      <Loader2 size={12} className="animate-spin" />
                    ) : (
                      <RefreshCw size={12} />
                    )}
                    {updateState === 'checking'
                      ? 'Checking…'
                      : updateState === 'downloading'
                        ? 'Downloading…'
                        : 'Check for Updates'}
                  </button>
                )}
              </div>
            )}
          </div>

          {updateState === 'up-to-date' && (
            <p className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: '#22c55e' }}>
              <CheckCircle2 size={12} />
              Up to date
            </p>
          )}
          {updateState === 'available' && (
            <p className="mt-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              v{updateVersion} available — downloading…
            </p>
          )}
          {updateState === 'error' && updateError && (
            <p className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} />
              {updateError}
            </p>
          )}
        </section>
        )}

        {/* ── Backend Network ────────────────────────────────────────────── */}
        {tab === 'advanced' && (
        <>
        <BackendNetworkSection
          config={config}
          setConfig={setConfig}
          originalServerAddr={originalConfig?.server_addr ?? config.server_addr}
          serverInfo={serverInfo}
          parsePortFromAddr={parsePortFromAddr}
          portTestState={portTestState}
          portTestResult={portTestResult}
          portTestPort={portTestPort}
          onTest={handlePortTest}
          onReset={() => {
            setConfig({ ...config, server_addr: ':0' })
            setPortTestResult(null)
          }}
          onSave={handleSave}
          saving={saveState === 'saving'}
        />
        <DiagnosticsSection
          debugLogging={config?.preferences?.debug_logging ?? false}
          onToggleDebugLogging={() => {
            const next: Config = {
              ...config,
              preferences: {
                ...config.preferences,
                debug_logging: !(config?.preferences?.debug_logging ?? false),
              },
            }
            setConfig(next)
            persist(next)
          }}
        />
        </>
        )}

        {/* ── About ──────────────────────────────────────────────────────── */}
        {tab === 'about' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <div className="mb-3 flex items-center gap-3">
            <span className="text-2xl" aria-hidden="true">⚔</span>
            <div>
              <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
                PQ Companion
              </h2>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {appVersion ? `v${appVersion}` : 'Desktop companion for Project Quarm'}
              </p>
            </div>
          </div>

          <p className="mb-4 text-sm leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
            An independent, fan-made desktop companion for the EverQuest
            emulated server Project Quarm. Built by players, for players.
          </p>

          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <a
              href={DISCORD_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center gap-2 rounded px-3 py-2 text-sm font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <MessageCircle size={15} style={{ color: '#5865F2' }} />
              Discord
            </a>
            <a
              href={KOFI_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center gap-2 rounded px-3 py-2 text-sm font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <Coffee size={15} style={{ color: '#FF5E5B' }} />
              Support on Ko-fi
            </a>
            <a
              href={GITHUB_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center gap-2 rounded px-3 py-2 text-sm font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <Code2 size={15} style={{ color: 'var(--color-muted-foreground)' }} />
              GitHub
            </a>
            <a
              href={WEBSITE_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center gap-2 rounded px-3 py-2 text-sm font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <Globe size={15} style={{ color: 'var(--color-primary)' }} />
              Website
            </a>
          </div>

          <div
            className="mt-4 flex items-center justify-center gap-1.5 border-t pt-4 text-xs"
            style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
          >
            <span>Made with</span>
            <Heart size={12} style={{ color: '#FF5E5B' }} fill="#FF5E5B" />
            <span>by Osui &lt;Seekers of Souls&gt;</span>
          </div>
        </section>
        )}

        {/* ── Changelog ──────────────────────────────────────────────────── */}
        {tab === 'changelog' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Changelog
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Release notes for every published version.
          </p>

          <label className="mb-4 flex cursor-pointer items-center justify-between border-b py-2" style={{ borderColor: 'var(--color-border)' }}>
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Show &ldquo;What&rsquo;s New&rdquo; popup after updates
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Pop up release notes on first launch after an update.
              </p>
            </div>
            <div
              onClick={() =>
                setConfig({ ...config, changelog_popup_disabled: !config.changelog_popup_disabled })
              }
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: !config.changelog_popup_disabled
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: !config.changelog_popup_disabled ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

          {changelogEntries === null && (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Loading…
            </p>
          )}
          {changelogEntries?.length === 0 && (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              No changelog data available.
            </p>
          )}
          {(showAllChangelog ? changelogEntries : changelogEntries?.slice(0, CHANGELOG_PAGE_SIZE))?.map((entry) => (
            <div key={entry.version} className="mb-4 border-b pb-4 last:mb-0 last:border-0" style={{ borderColor: 'var(--color-border)' }}>
              <p className="mb-1 text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
                v{entry.version} <span style={{ color: 'var(--color-muted-foreground)', fontWeight: 400 }}>— {entry.date}</span>
              </p>
              {renderChangelogBody(entry.body)}
            </div>
          ))}
          {!showAllChangelog && changelogEntries !== null && changelogEntries.length > CHANGELOG_PAGE_SIZE && (
            <button
              onClick={() => setShowAllChangelog(true)}
              className="w-full rounded px-3 py-2 text-center text-sm font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            >
              Show all {changelogEntries.length} versions
            </button>
          )}
        </section>
        )}

        {/* ── EverQuest Path ─────────────────────────────────────────────── */}
        {tab === 'general' && (
        <section data-setting="eq-install"
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            EverQuest Installation
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Path to your EverQuest folder — used to find log files and Zeal exports.
          </p>

          <div className="flex gap-2">
            <input
              type="text"
              value={config.eq_path}
              onChange={(e) => setConfig({ ...config, eq_path: e.target.value })}
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
          </div>

          {config.eq_path && (
            <p className="mt-2 text-xs" style={{ color: 'var(--color-muted)' }}>
              Log file: <code style={{ color: 'var(--color-foreground)' }}>
                {config.eq_path}/eqlog_{config.character || '<auto>'}_pq.proj.txt
              </code>
            </p>
          )}

          <div className="mt-3 flex items-center justify-between gap-2">
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Want to redo first-launch setup?
            </p>
            <button
              onClick={() => window.dispatchEvent(new Event('pq:open-onboarding'))}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
            >
              <Sparkles size={12} />
              Run Setup Wizard
            </button>
          </div>
        </section>
        )}

        {/* ── EverQuest logging & Zeal status/toggles ──────────────────────── */}
        {tab === 'general' && <EqLogStatusCard />}

        {/* ── Zeal integration ───────────────────────────────────────────── */}
        {tab === 'general' && (
        <section data-setting="zeal"
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <div className="mb-1 flex items-center justify-between">
            <h2
              className="text-sm font-semibold uppercase tracking-wide"
              style={{ color: 'var(--color-muted)' }}
            >
              Zeal Integration
            </h2>
            <button
              onClick={() => void checkZeal()}
              disabled={zealChecking}
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: zealChecking ? 'not-allowed' : 'pointer',
                opacity: zealChecking ? 0.5 : 1,
              }}
            >
              {zealChecking ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              {zealChecking ? 'Checking…' : 'Re-check'}
            </button>
          </div>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Zeal is a community EverQuest add-on that exposes live target, HP,
            buff, and group state. PQ Companion uses it for real-time overlays
            when installed and falls back to log parsing when not.
          </p>

          {zealStatus?.installed && (
            <div className="space-y-2">
              <div className="flex items-start gap-2 text-sm" style={{ color: '#22c55e' }}>
                <CheckCircle2 size={14} className="mt-0.5 shrink-0" />
                <div>
                  <p>
                    Zeal is installed
                    {zealStatus.version ? ` (v${zealStatus.version})` : ''}.
                  </p>
                  {zealStatus.asi_path && (
                    <p className="mt-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      Found <code>{zealStatus.asi_path}</code>
                    </p>
                  )}
                </div>
              </div>

              {zealStatus.update_available && zealStatus.latest_version && (
                <div
                  className="flex items-start gap-2 rounded px-3 py-2 text-xs"
                  style={{
                    backgroundColor: 'rgba(245, 158, 11, 0.10)',
                    border: '1px solid rgba(245, 158, 11, 0.35)',
                    color: 'var(--color-text)',
                  }}
                >
                  <AlertTriangle size={12} className="mt-0.5 shrink-0" style={{ color: '#f59e0b' }} />
                  <div className="flex-1">
                    A newer Zeal is available:{' '}
                    <span style={{ color: '#fcd34d' }}>v{zealStatus.latest_version}</span>{' '}
                    (you have v{zealStatus.version}). Pq-companion still works, but updating
                    keeps you on the latest fixes.
                  </div>
                  <a
                    href={ZEAL_RELEASE_URL}
                    target="_blank"
                    rel="noreferrer noopener"
                    style={{ color: '#fcd34d', textDecoration: 'underline' }}
                  >
                    Release notes
                  </a>
                </div>
              )}

              {zealStatus.export_on_camp_found && !zealStatus.export_on_camp && (
                <div
                  className="flex items-start gap-2 rounded px-3 py-2 text-xs"
                  style={{
                    backgroundColor: 'rgba(245, 158, 11, 0.10)',
                    border: '1px solid rgba(245, 158, 11, 0.35)',
                    color: 'var(--color-text)',
                  }}
                >
                  <AlertTriangle size={12} className="mt-0.5 shrink-0" style={{ color: '#f59e0b' }} />
                  <div className="flex-1">
                    Zeal&apos;s <code>ExportOnCamp</code> is disabled. Character inventory,
                    spellbook, and stats will not refresh on /camp.{' '}
                    <span style={{ color: 'var(--color-muted-foreground)' }}>
                      Fix: edit <code>zeal.ini</code>, set{' '}
                      <code>ExportOnCamp=TRUE</code> under <code>[Zeal]</code>, then relaunch EQ.
                    </span>
                  </div>
                </div>
              )}

              {zealStatus.export_on_camp_found && zealStatus.export_on_camp && (
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  <code>ExportOnCamp</code> is enabled — character data refreshes when you /camp.
                </p>
              )}
            </div>
          )}

          {zealStatus && !zealStatus.installed && !zealError && (
            <div className="space-y-2">
              <p className="text-sm font-medium" style={{ color: '#f87171' }}>
                Zeal is not installed in your configured EverQuest folder
                {zealStatus.eqgame_present ? '' : ' (eqgame.exe also not found — verify the path above)'}.
              </p>
              <p
                className="text-xs font-semibold uppercase tracking-wide"
                style={{ color: 'var(--color-muted)' }}
              >
                Installing Zeal unlocks
              </p>
              <ul
                className="ml-4 list-disc space-y-0.5 text-xs"
                style={{ color: 'var(--color-muted-foreground)' }}
              >
                <li>Real-time target detection without <code>/con</code></li>
                <li>Live target HP bar + pet-owner attribution</li>
                <li>Authoritative damage attribution in the DPS meter</li>
                <li>Trigger conditions on target HP, buff slots, and <code>/pipe</code> alerts</li>
              </ul>
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
            <div className="flex items-start gap-2 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              <p>Couldn&apos;t check for Zeal: {zealError}</p>
            </div>
          )}

          {!zealStatus && !zealError && !zealChecking && (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Set your EverQuest folder above, then click Re-check.
            </p>
          )}

          {pipeStatus && pipeStatus.state !== 'unsupported' && (
            <div
              className="mt-3 space-y-2 rounded px-3 py-2 text-xs"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
              }}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span
                    className="inline-block h-2 w-2 rounded-full"
                    style={{
                      backgroundColor:
                        pipeStatus.state === 'connected'
                          ? '#22c55e'
                          : pipeStatus.state === 'disconnected'
                            ? '#f87171'
                            : 'var(--color-muted)',
                    }}
                  />
                  <span style={{ color: 'var(--color-foreground)' }}>
                    Live pipe:{' '}
                    {pipeStatus.state === 'connected'
                      ? `connected${pipeStatus.character ? ` (${pipeStatus.character})` : ''}`
                      : pipeStatus.state === 'disconnected'
                        ? 'disconnected'
                        : 'waiting for EverQuest'}
                  </span>
                </div>
                {pipeStatus.state === 'connected' && pipeStatus.pid ? (
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    PID {pipeStatus.pid}
                  </span>
                ) : null}
              </div>
              {pipeStatus.state !== 'connected' && pipeStatus.last_error && (
                <pre
                  className="whitespace-pre-wrap break-words font-mono"
                  style={{ color: 'var(--color-muted-foreground)', fontSize: '11px' }}
                >
                  {pipeStatus.last_error}
                </pre>
              )}
            </div>
          )}
        </section>
        )}

        {/* ── EQ client version ──────────────────────────────────────────── */}
        {tab === 'general' && (
        <section data-setting="eq-client"
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <div className="mb-1 flex items-center justify-between">
            <h2
              className="text-sm font-semibold uppercase tracking-wide"
              style={{ color: 'var(--color-muted)' }}
            >
              EQ Client Version
            </h2>
            <button
              onClick={() => void checkQuarmClient()}
              disabled={quarmChecking}
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: quarmChecking ? 'not-allowed' : 'pointer',
                opacity: quarmChecking ? 0.5 : 1,
              }}
            >
              {quarmChecking ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              {quarmChecking ? 'Checking…' : 'Re-check'}
            </button>
          </div>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Compares <code>eqgame.dll</code> in your EverQuest folder against the
            public{' '}
            <a
              href={QUARM_PATCHER_RELEASE_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="underline"
              style={{ color: 'var(--color-primary)' }}
            >
              Quarm Patcher manifest
            </a>
            , and checks <code>eqw.dll</code> (the optional windowed-mode wrapper)
            against the latest{' '}
            <a
              href={EQW_RELEASE_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="underline"
              style={{ color: 'var(--color-primary)' }}
            >
              EQW-TAKP release
            </a>
            . Informational only — PQ Companion does not patch game files.
          </p>

          {quarmError && (
            <div className="flex items-start gap-2 text-xs" style={{ color: '#f87171' }}>
              <AlertTriangle size={12} className="mt-0.5 shrink-0" />
              <p>Couldn&apos;t check EQ client: {quarmError}</p>
            </div>
          )}

          {!quarmError && quarmStatus && quarmStatus.files.map((f) => (
            <QuarmFileRow key={f.name} file={f} />
          ))}

          {!quarmError && eqwStatus && <EqwFileRow status={eqwStatus} />}

          {!quarmStatus && !quarmError && !quarmChecking && (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Set your EverQuest folder above, then click Re-check.
            </p>
          )}

          {!quarmError && quarmStatus?.manifest_error && (
            <p
              className="mt-2 text-xs"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              Manifest unreachable ({quarmStatus.manifest_error}). Local DLL info
              is still shown below.
            </p>
          )}
        </section>
        )}

        {/* ── Preferences ────────────────────────────────────────────────── */}
        {tab === 'general' && (
        <section data-setting="preferences"
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-3 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Preferences
          </h2>

          <label className="flex cursor-pointer items-center justify-between py-1">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Parse Combat Log
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Enable real-time log parsing for DPS meter and overlays.
              </p>
            </div>
            <div
              onClick={() =>
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    parse_combat_log: !config.preferences.parse_combat_log,
                  },
                })
              }
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.parse_combat_log
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: config.preferences.parse_combat_log ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

          <label className="flex cursor-pointer items-center justify-between py-1 mt-4">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                Minimize to Tray
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Hide to system tray instead of closing when the window is dismissed.
              </p>
            </div>
            <div
              onClick={() => {
                const next = !config.preferences.minimize_to_tray
                setConfig({
                  ...config,
                  preferences: {
                    ...config.preferences,
                    minimize_to_tray: next,
                  },
                })
                // Apply immediately so the close behaviour / tray icon update
                // without waiting for a restart or config reload.
                void window.electron?.window?.setMinimizeToTray(next)
              }}
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.minimize_to_tray
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: config.preferences.minimize_to_tray ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

        </section>
        )}

        {/* ── Audio ──────────────────────────────────────────────────────── */}
        {tab === 'audio' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-3 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Audio
          </h2>

          <div>
            <div className="mb-1 flex items-center justify-between">
              <div>
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Master Volume
                </p>
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Scales all trigger sounds and TTS alerts. Each trigger keeps its own volume — this dampens everything together.
                </p>
              </div>
              <span className="text-xs font-mono shrink-0 ml-3" style={{ color: 'var(--color-muted-foreground)' }}>
                {config.preferences.master_volume ?? 100}%
              </span>
            </div>
            <div className="flex items-center gap-2 mt-2">
              {(config.preferences.master_volume ?? 100) === 0 ? (
                <VolumeX size={14} style={{ color: 'var(--color-muted)' }} />
              ) : (
                <Volume2 size={14} style={{ color: 'var(--color-muted)' }} />
              )}
              <input
                type="range"
                min={0}
                max={100}
                step={1}
                value={config.preferences.master_volume ?? 100}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    preferences: {
                      ...config.preferences,
                      master_volume: Number(e.target.value),
                    },
                  })
                }
                style={{ flex: 1 }}
              />
            </div>
          </div>

          <AudioOutputDeviceSetting
            deviceId={config.preferences.audio_output_device_id ?? ''}
            deviceLabel={config.preferences.audio_output_device_label ?? ''}
            defaultVoice={config.preferences.default_tts_voice ?? ''}
            onChange={(id, label) =>
              setConfig({
                ...config,
                preferences: {
                  ...config.preferences,
                  audio_output_device_id: id,
                  audio_output_device_label: label,
                },
              })
            }
          />

          <div className="mt-4">
            <div className="mb-1 flex items-center justify-between">
              <div>
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Voice Rate
                </p>
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Speaking speed for all text-to-speech alerts. 1.0× is normal; higher is faster.
                </p>
              </div>
              <span className="text-xs font-mono shrink-0 ml-3" style={{ color: 'var(--color-muted-foreground)' }}>
                {(config.preferences.tts_rate ?? 1).toFixed(1)}×
              </span>
            </div>
            <div className="flex items-center gap-2 mt-2">
              <Gauge size={14} style={{ color: 'var(--color-muted)' }} />
              <input
                type="range"
                min={0.5}
                max={2}
                step={0.1}
                value={config.preferences.tts_rate ?? 1}
                onChange={(e) =>
                  setConfig({
                    ...config,
                    preferences: {
                      ...config.preferences,
                      tts_rate: Number(e.target.value),
                    },
                  })
                }
                style={{ flex: 1 }}
              />
              <button
                type="button"
                onClick={() => {
                  if (ttsPreviewPlaying) {
                    stopTestPlayback()
                    setTtsPreviewPlaying(false)
                    return
                  }
                  setTtsPreviewPlaying(true)
                  speakTextForTest(
                    'This is a text to speech voice rate preview.',
                    config.preferences.default_tts_voice ?? '',
                    1.0,
                    () => setTtsPreviewPlaying(false),
                    config.preferences.tts_rate ?? 1,
                  )
                }}
                className="shrink-0 rounded px-2 py-1 text-xs"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-background)',
                  cursor: 'pointer',
                }}
                title="Preview the selected voice rate"
              >
                {ttsPreviewPlaying ? 'Stop' : 'Test'}
              </button>
            </div>
          </div>

          <div className="mt-4">
            <div className="mb-1 flex items-center justify-between">
              <div>
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Repeat-Audio Cooldown
                </p>
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  After a trigger plays a sound or TTS, mutes further audio from that same trigger for this long. Collapses rapid bursts (e.g. an AE mez wearing off several mobs at once) to a single alert. Overlay text, history, and timers are unaffected. 0 = off; try 1–2s.
                </p>
              </div>
              <div className="flex items-center gap-1 shrink-0 ml-3">
                <input
                  type="number"
                  min={0}
                  max={10}
                  step={0.5}
                  value={config.preferences.trigger_audio_cooldown_secs ?? 0}
                  onChange={(e) =>
                    setConfig({
                      ...config,
                      preferences: {
                        ...config.preferences,
                        trigger_audio_cooldown_secs: Math.max(0, Number(e.target.value) || 0),
                      },
                    })
                  }
                  className="w-16 rounded px-2 py-1 text-sm text-right"
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
          </div>

          <TtsVoiceDefault config={config} setConfig={setConfig} />

          <PiperTtsSettings config={config} setConfig={setConfig} />

          <KokoroTtsSettings config={config} setConfig={setConfig} />
        </section>
        )}

        {/* ── Accessibility ──────────────────────────────────────────────── */}
        {tab === 'accessibility' && (
        <section
          className="rounded-lg p-4"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide"
            style={{ color: 'var(--color-muted)' }}
          >
            Accessibility
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Readability options for different displays and resolutions.
          </p>

          <label className="flex cursor-pointer items-center justify-between py-1">
            <div>
              <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                High Contrast Text
              </p>
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Brighten muted text and borders for easier reading on
                high-resolution displays.
              </p>
            </div>
            <div
              onClick={() => {
                const next = !config.preferences.high_contrast
                // Apply immediately for instant feedback; the hook keeps it in
                // sync after the change is saved.
                applyContrast(next)
                setConfig({
                  ...config,
                  preferences: { ...config.preferences, high_contrast: next },
                })
              }}
              style={{
                width: 40,
                height: 22,
                borderRadius: 11,
                backgroundColor: config.preferences.high_contrast
                  ? 'var(--color-primary)'
                  : 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                position: 'relative',
                flexShrink: 0,
                transition: 'background-color 0.15s',
              }}
            >
              <div
                style={{
                  position: 'absolute',
                  top: 2,
                  left: config.preferences.high_contrast ? 20 : 2,
                  width: 16,
                  height: 16,
                  borderRadius: '50%',
                  backgroundColor: '#fff',
                  transition: 'left 0.15s',
                }}
              />
            </div>
          </label>

          <div className="py-1 mt-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  App Zoom
                </p>
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Scale the whole main app window up or down. Popout overlay
                  windows are sized separately below.
                </p>
              </div>
              <span className="text-sm font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
                {Math.round((config.preferences.zoom_factor || 1) * 100)}%
              </span>
            </div>
            <input
              type="range"
              min={80}
              max={150}
              step={5}
              value={Math.round((config.preferences.zoom_factor || 1) * 100)}
              onChange={(e) => {
                const factor = (parseInt(e.target.value) || 100) / 100
                // Apply immediately for live feedback; the hook keeps it in
                // sync after saving.
                applyZoom(factor)
                setConfig({
                  ...config,
                  preferences: { ...config.preferences, zoom_factor: factor },
                })
              }}
              className="w-full mt-2"
            />
          </div>

          <div
            className="mt-5 border-t pt-4"
            style={{ borderColor: 'var(--color-border)' }}
          >
            <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
              Overlay font size
            </p>
            <p className="mb-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Scale each popout overlay window independently — useful when an
              overlay is hard to read at your game resolution. Applies to the
              popped-out windows; dashboard cards follow the App Zoom above.
              You can also hold Ctrl and scroll over an overlay to resize it.
            </p>
            <div className="mt-2">
              {OVERLAY_DEFS.filter((o) => o.name !== 'itemInfo' || itemInfoOffered(config.preferences))
                .map((o) => {
                const factor = config.preferences.overlay_zoom_factors?.[o.name] ?? 1
                return (
                  <div key={o.name} className="py-1.5">
                    <div className="flex items-center justify-between">
                      <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                        {o.label}
                      </p>
                      <span
                        className="text-sm font-mono"
                        style={{ color: 'var(--color-muted-foreground)' }}
                      >
                        {Math.round(factor * 100)}%
                      </span>
                    </div>
                    <input
                      type="range"
                      min={OVERLAY_ZOOM_MIN * 100}
                      max={OVERLAY_ZOOM_MAX * 100}
                      step={5}
                      value={Math.round(factor * 100)}
                      onChange={(e) => {
                        const next = (parseInt(e.target.value) || 100) / 100
                        // No applyZoom() here — that would scale THIS (main)
                        // window. The change is saved and the overlay's own
                        // window picks it up live via the config:updated event
                        // (see useOverlayZoom).
                        const cur = config.preferences.overlay_zoom_factors ?? {}
                        setConfig({
                          ...config,
                          preferences: {
                            ...config.preferences,
                            overlay_zoom_factors: { ...cur, [o.name]: next },
                          },
                        })
                      }}
                      className="w-full mt-1"
                    />
                  </div>
                )
              })}
            </div>
          </div>
        </section>
        )}

        {/* ── Hotkeys / Discord ─────────────────────────────────────────── */}
        {tab === 'hotkeys' && <HotkeysSettings />}

        {tab === 'discord' && <DiscordWebhookSettings config={config} setConfig={setConfig} />}

        {/* ── Log Files ──────────────────────────────────────────────────── */}
        {tab === 'logs' && (
        <section
          className="rounded-lg p-4"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: logLargeFile
              ? '1px solid #f97316'
              : '1px solid var(--color-border)',
          }}
        >
          <h2
            className="mb-1 text-sm font-semibold uppercase tracking-wide flex items-center gap-2"
            style={{ color: 'var(--color-muted)' }}
          >
            Log Files
            {logLargeFile && (
              <span
                className="rounded-full px-2 py-0.5 text-[10px] font-semibold"
                style={{ backgroundColor: '#f97316', color: '#fff' }}
              >
                Large file detected
              </span>
            )}
          </h2>
          <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Archives the current EverQuest log file to a <code className="font-mono">.bak.zip</code> next to it, then trims the live log using the settings below. Archives stay in your EQ folder and Log Backfill still reads them. To keep a separate copy of your logs, make a snapshot with “Include log files” in{' '}
            <button className="underline" style={{ color: 'var(--color-primary)' }} onClick={() => setTab('backups')}>
              Backup &amp; Restore
            </button>
            ; your <code className="font-mono">.ini</code> settings files are backed up there too.
          </p>

          {/* Trim settings */}
          <div className="mb-3 space-y-2">
            <div className="flex flex-wrap items-center gap-2 text-xs" style={{ color: 'var(--color-foreground)' }}>
              <span>Keep the last</span>
              <input
                type="number"
                min={1}
                value={logTrim.mode === 'size' ? logTrim.keep_mb : logTrim.keep_days}
                onChange={(e) => {
                  const n = Math.max(1, parseInt(e.target.value, 10) || 1)
                  setLogTrim(logTrim.mode === 'size' ? { keep_mb: n } : { keep_days: n })
                }}
                className="w-20 rounded px-2 py-1 text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
              <select
                value={logTrim.mode}
                onChange={(e) => setLogTrim({ mode: e.target.value as LogTrimSettings['mode'] })}
                className="rounded px-2 py-1 text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <option value="days">days of entries</option>
                <option value="size">MB of newest entries</option>
              </select>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-xs" style={{ color: 'var(--color-foreground)' }}>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={logTrim.warn_mb > 0}
                  onChange={(e) => setLogTrim({ warn_mb: e.target.checked ? DEFAULT_LOG_TRIM.warn_mb : -1 })}
                />
                Flag log files larger than
              </label>
              <input
                type="number"
                min={1}
                value={logTrim.warn_mb > 0 ? logTrim.warn_mb : DEFAULT_LOG_TRIM.warn_mb}
                disabled={logTrim.warn_mb <= 0}
                onChange={(e) => setLogTrim({ warn_mb: Math.max(1, parseInt(e.target.value, 10) || 1) })}
                className="w-20 rounded px-2 py-1 text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  opacity: logTrim.warn_mb <= 0 ? 0.5 : 1,
                }}
              />
              <span>MB</span>
            </div>
            <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
              {logTrim.mode === 'days'
                ? 'Melee classes log many more lines per day than casters, so a day window can still leave a large file. Switch to MB to cap the size directly.'
                : 'Trimming by size may keep less than 30 days, which limits how far back Log Backfill can reach.'}
              {logTrim.mode === 'size' && logTrim.warn_mb > 0 && logTrim.keep_mb >= logTrim.warn_mb &&
                ' The kept size is at or above the warning threshold, so the log will stay flagged after trimming.'}
            </p>
          </div>

          {/* Load file info */}
          {!logFileInfo && cleanupState === 'idle' && (
            <button
              onClick={handleLoadLogInfo}
              disabled={logInfoLoading}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: logInfoLoading ? 'not-allowed' : 'pointer',
                opacity: logInfoLoading ? 0.7 : 1,
              }}
            >
              {logInfoLoading ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
              {logInfoLoading ? 'Loading…' : 'Check Log File'}
            </button>
          )}

          {/* File info display */}
          {logFileInfo && cleanupState === 'idle' && (
            <div className="mb-3 space-y-1">
              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Size: <span style={{ color: 'var(--color-foreground)' }}>{(logFileInfo.size_bytes / 1024 / 1024).toFixed(1)} MB</span>
              </p>
              {logFileInfo.oldest_entry && (
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Oldest entry: <span style={{ color: 'var(--color-foreground)' }}>{new Date(logFileInfo.oldest_entry).toLocaleDateString()}</span>
                </p>
              )}
              {logFileInfo.newest_entry && (
                <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                  Newest entry: <span style={{ color: 'var(--color-foreground)' }}>{new Date(logFileInfo.newest_entry).toLocaleDateString()}</span>
                </p>
              )}
              {logRecentlyActive && (
                <p className="flex items-center gap-1.5 text-xs" style={{ color: '#f97316' }}>
                  <AlertTriangle size={12} />
                  EverQuest still has this file open. Camp out first — archiving a live log can corrupt tailing until you restart PQ Companion.
                </p>
              )}
              <div className="flex gap-2 pt-2">
                <button
                  onClick={handleCleanupLog}
                  disabled={logRecentlyActive}
                  className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
                  style={{
                    backgroundColor: '#f97316',
                    color: '#fff',
                    border: 'none',
                    cursor: logRecentlyActive ? 'not-allowed' : 'pointer',
                    opacity: logRecentlyActive ? 0.5 : 1,
                  }}
                >
                  <Trash2 size={12} />
                  Archive &amp; Trim
                </button>
                <button
                  onClick={() => setLogFileInfo(null)}
                  className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                    cursor: 'pointer',
                  }}
                >
                  Cancel
                </button>
              </div>
            </div>
          )}

          {/* Running */}
          {cleanupState === 'running' && (
            <p className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              <Loader2 size={12} className="animate-spin" />
              Backing up and purging log…
            </p>
          )}

          {/* Done */}
          {cleanupState === 'done' && cleanupResult && (
            <div className="space-y-1">
              <p className="flex items-center gap-1.5 text-xs" style={{ color: '#22c55e' }}>
                <CheckCircle2 size={12} />
                Archive complete. Saved to:
              </p>
              <p className="text-xs font-mono break-all" style={{ color: 'var(--color-muted-foreground)' }}>
                {cleanupResult}
              </p>
              {trimmedSize !== null && (
                <p className="text-xs" style={{ color: logLargeFile ? '#f97316' : 'var(--color-muted-foreground)' }}>
                  Live log is now {(trimmedSize / 1024 / 1024).toFixed(1)} MB.
                  {logLargeFile && logTrim.mode === 'days' &&
                    ` That's still over the ${logTrim.warn_mb} MB warning — try fewer days, or keep a fixed number of MB instead.`}
                  {logLargeFile && logTrim.mode === 'size' &&
                    ` That's still over the ${logTrim.warn_mb} MB warning — lower the kept MB or raise the warning threshold.`}
                </p>
              )}
              <button
                onClick={() => { setCleanupState('idle'); setCleanupResult(null) }}
                className="mt-2 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  cursor: 'pointer',
                }}
              >
                <RefreshCw size={12} />
                Check Again
              </button>
            </div>
          )}

          {/* Error */}
          {cleanupState === 'error' && cleanupResult && (
            <div className="space-y-1">
              <p className="flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
                <AlertTriangle size={12} />
                {cleanupResult}
              </p>
              <button
                onClick={() => { setCleanupState('idle'); setCleanupResult(null) }}
                className="mt-2 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                  cursor: 'pointer',
                }}
              >
                Try Again
              </button>
            </div>
          )}
        </section>
        )}

        {/* ── Chat retention + Log Backfill (merged into the Logs tab) ─────── */}
        {tab === 'logs' && <BackfillPanel />}

        {/* ── Autosave status ────────────────────────────────────────────── */}
        <div className="flex items-center gap-1.5 text-sm">
          {saveState === 'saving' ? (
            <span className="flex items-center gap-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
              <Loader2 size={14} className="animate-spin" /> Saving…
            </span>
          ) : saveState === 'error' && saveError ? (
            <span className="flex items-center gap-1.5" style={{ color: '#f87171' }}>
              <AlertTriangle size={14} /> {saveError}
            </span>
          ) : saveState === 'saved' ? (
            <span className="flex items-center gap-1.5" style={{ color: '#22c55e' }}>
              <CheckCircle2 size={14} /> Saved
            </span>
          ) : (
            <span className="flex items-center gap-1.5" style={{ color: 'var(--color-muted)' }}>
              <CheckCircle2 size={14} /> Changes save automatically
            </span>
          )}
        </div>

        {/* ── Config file location note ──────────────────────────────────── */}
        <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
          Settings are stored at{' '}
          <code style={{ color: 'var(--color-foreground)' }}>~/.pq-companion/config.yaml</code>
        </p>
      </div>
      </div>
      </div>
    </div>
  )
}

// ── Backend Network section ──────────────────────────────────────────────────
// Shows the port the local API server is actually listening on, lets the user
// override the preferred port (used at next launch), and probes availability
// before they commit a change. `actual_port` reflects the running server;
// `preferred_addr` is whatever's saved in config — they only differ when the
// last startup fell back from a busy port to an OS-assigned one.

interface BackendNetworkSectionProps {
  config: Config
  setConfig: (c: Config) => void
  originalServerAddr: string
  serverInfo: ServerInfo | null
  parsePortFromAddr: (addr: string) => number
  portTestState: 'idle' | 'testing'
  portTestResult: TestPortResult | null
  portTestPort: number | null
  onTest: () => void
  onReset: () => void
  onSave: () => void
  saving: boolean
}

function BackendNetworkSection(props: BackendNetworkSectionProps): React.ReactElement {
  const {
    config, setConfig, originalServerAddr, serverInfo, parsePortFromAddr,
    portTestState, portTestResult, portTestPort, onTest, onReset, onSave, saving,
  } = props

  const preferredPort = parsePortFromAddr(config.server_addr)
  const isAuto = preferredPort === 0
  const actualPort = serverInfo?.actual_port ?? null
  const fellBack = serverInfo !== null
    && !isAuto
    && actualPort !== null
    && preferredPort !== actualPort
  const dirty = config.server_addr !== originalServerAddr

  return (
    <section data-setting="network"
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        <Wifi size={13} />
        Backend Network
      </h2>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        The companion app talks to its own local backend over a loopback port.
        The default is fine for most users — only change this if another local
        service is taking the port and the app can&rsquo;t start.
      </p>

      <div className="mb-3 grid grid-cols-2 gap-2 rounded p-3 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
      >
        <span style={{ color: 'var(--color-muted-foreground)' }}>Current port</span>
        <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>
          {actualPort ?? '—'}
        </span>
        <span style={{ color: 'var(--color-muted-foreground)' }}>Preferred</span>
        <span className="text-right font-mono" style={{ color: 'var(--color-foreground)' }}>
          {isAuto ? 'auto-assign' : preferredPort}
        </span>
      </div>

      {fellBack && (
        <p className="mb-3 flex items-start gap-1.5 text-xs" style={{ color: '#f97316' }}>
          <AlertTriangle size={12} style={{ marginTop: 2, flexShrink: 0 }} />
          <span>
            Preferred port <b>{preferredPort}</b> was unavailable at startup —
            another process on your machine is using it. The app fell back to
            port {actualPort}. To make this stable, either stop the conflicting
            service or pick a different preferred port below.
          </span>
        </p>
      )}

      <div className="mb-2">
        <label className="mb-1 block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Preferred port (used at next app launch)
        </label>
        <div className="flex gap-2">
          <input
            type="number"
            min={0}
            max={65535}
            step={1}
            value={preferredPort}
            onChange={(e) => {
              const n = Math.max(0, Math.min(65535, Number(e.target.value) || 0))
              setConfig({ ...config, server_addr: `:${n}` })
            }}
            placeholder="0 = auto"
            className="w-28 rounded px-2 py-1 text-sm font-mono"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-foreground)',
              border: '1px solid var(--color-border)',
              outline: 'none',
            }}
          />
          <button
            onClick={onTest}
            disabled={portTestState === 'testing'}
            className="flex items-center gap-1.5 rounded px-3 py-1 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
              cursor: portTestState === 'testing' ? 'not-allowed' : 'pointer',
              opacity: portTestState === 'testing' ? 0.7 : 1,
            }}
          >
            {portTestState === 'testing' ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
            {portTestState === 'testing' ? 'Testing…' : 'Test availability'}
          </button>
          {!isAuto && (
            <button
              onClick={onReset}
              className="flex items-center gap-1.5 rounded px-3 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                cursor: 'pointer',
              }}
            >
              Reset to auto
            </button>
          )}
        </div>
        <p className="mt-1 text-xs" style={{ color: 'var(--color-muted)' }}>
          Set to <code>0</code> (or click Reset to auto) to let the OS pick any
          free port at startup. Changes take effect the next time the app
          launches.
        </p>
      </div>

      {portTestResult && (
        <p
          className="mt-2 flex items-start gap-1.5 text-xs"
          style={{ color: portTestResult.available ? '#22c55e' : '#f87171' }}
        >
          {portTestResult.available ? (
            <CheckCircle2 size={12} style={{ marginTop: 2, flexShrink: 0 }} />
          ) : (
            <AlertTriangle size={12} style={{ marginTop: 2, flexShrink: 0 }} />
          )}
          <span>
            {portTestPort === 0 ? (
              <>Auto-assign always succeeds — the OS picks any free port.</>
            ) : portTestResult.available ? (
              portTestResult.in_use_by === 'pq-companion'
                ? <>Port {portTestPort} is currently used by this app — that&rsquo;s expected and fine.</>
                : <>Port {portTestPort} is available.</>
            ) : (
              <>Port {portTestPort} is unavailable{portTestResult.error ? <>: {portTestResult.error}</> : null}. Pick a different port or use auto-assign.</>
            )}
          </span>
        </p>
      )}

      {dirty && (
        <div
          className="mt-3 rounded p-3"
          style={{ backgroundColor: 'color-mix(in srgb, #f59e0b 18%, transparent)', border: '1px solid #f59e0b' }}
        >
          <p className="mb-2 flex items-start gap-1.5 text-xs" style={{ color: '#f59e0b' }}>
            <AlertTriangle size={12} style={{ marginTop: 2, flexShrink: 0 }} />
            <span>
              <b>Unsaved port change.</b> Testing alone does not save the change — click
              the button below (or <b>Save Settings</b> at the bottom of the page) to
              persist it, then restart the app for the new port to take effect.
            </span>
          </p>
          <button
            onClick={onSave}
            disabled={saving}
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-semibold"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: '#fff',
              border: 'none',
              cursor: saving ? 'not-allowed' : 'pointer',
              opacity: saving ? 0.7 : 1,
            }}
          >
            {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
            {saving ? 'Saving…' : 'Save port change'}
          </button>
        </div>
      )}
    </section>
  )
}

// DiagnosticsSection exposes the log folder so a user reporting an issue can
// hand back ~/.pq-companion/logs/ contents (server.log + electron.log, with
// rotated siblings). Lives on the Advanced tab — same place a power user
// would look when "the items page won't load."
function DiagnosticsSection({
  debugLogging,
  onToggleDebugLogging,
}: {
  debugLogging: boolean
  onToggleDebugLogging: () => void
}): React.ReactElement {
  const hasShell = typeof window !== 'undefined' && !!window.electron?.shell
  const hasSaveDialog = typeof window !== 'undefined' && !!window.electron?.dialog?.saveDebugLogs
  const hasSettingsBridge = typeof window !== 'undefined' && !!window.electron?.settings
  const [exportState, setExportState] = useState<'idle' | 'running' | 'done' | 'error'>('idle')
  const [exportResult, setExportResult] = useState<string | null>(null)
  const [hwAccelDisabled, setHwAccelDisabled] = useState(false)
  const [hwAccelPendingRestart, setHwAccelPendingRestart] = useState(false)

  async function handleExportDebugLogs() {
    if (!window.electron?.dialog?.saveDebugLogs) return
    const dest = await window.electron.dialog.saveDebugLogs()
    if (!dest) return
    setExportState('running')
    setExportResult(null)
    try {
      const res = await exportDebugLogs(dest)
      setExportResult(res.export_path)
      setExportState('done')
    } catch (err) {
      setExportResult(err instanceof Error ? err.message : String(err))
      setExportState('error')
    }
  }

  useEffect(() => {
    if (!window.electron?.settings) return
    window.electron.settings.getHardwareAcceleration()
      .then((res) => setHwAccelDisabled(res.disabled))
      .catch(() => null)
  }, [])

  async function handleToggleHwAccel() {
    if (!window.electron?.settings) return
    const next = !hwAccelDisabled
    setHwAccelDisabled(next)
    setHwAccelPendingRestart(true)
    await window.electron.settings.setHardwareAcceleration(next).catch(() => null)
  }

  async function handleHwAccelRestart() {
    if (!window.electron?.app?.relaunch) return
    await window.electron.app.relaunch()
  }

  return (
    <section data-setting="diagnostics"
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        <FileText size={13} />
        Diagnostics
      </h2>
      <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        If something isn&rsquo;t working, the log folder contains the last few
        sessions of <code>server.log</code> (backend) and <code>electron.log</code>{' '}
        (UI shell). Attach those to a bug report so the cause can be diagnosed.
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => {
            if (window.electron?.shell) {
              window.electron.shell.openLogsFolder().catch(() => null)
            }
          }}
          disabled={!hasShell}
          className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          <FolderOpen size={13} />
          Open logs folder
        </button>
        <button
          type="button"
          onClick={handleExportDebugLogs}
          disabled={!hasSaveDialog || exportState === 'running'}
          className="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          {exportState === 'running' ? <Loader2 size={13} className="animate-spin" /> : <Download size={13} />}
          Export debug logs (.zip)
        </button>
      </div>

      {exportState === 'done' && exportResult && (
        <p className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: '#22c55e' }}>
          <CheckCircle2 size={12} />
          Exported to {exportResult}
        </p>
      )}
      {exportState === 'error' && exportResult && (
        <p className="mt-2 flex items-center gap-1.5 text-xs" style={{ color: '#f87171' }}>
          <AlertTriangle size={12} />
          {exportResult}
        </p>
      )}

      {/* Verbose logging: raises the backend log to debug level so an
          intermittent issue (e.g. a trigger alert firing more than once) leaves
          a detailed trail in server.log. Off by default to keep the log small
          and readable; turn it on, reproduce, then attach the log. */}
      <label className="mt-4 flex cursor-pointer items-start gap-2">
        <input
          type="checkbox"
          checked={debugLogging}
          onChange={onToggleDebugLogging}
          className="mt-0.5"
        />
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
          Verbose (debug) logging
          <span className="block" style={{ color: 'var(--color-muted-foreground)' }}>
            Records detailed backend diagnostics (log-file reads, trigger fires)
            to <code>server.log</code>. Enable only while reproducing an issue —
            it makes the log noisier. Takes effect immediately.
          </span>
        </span>
      </label>

      {/* Workaround for a known Windows Electron/Chromium bug where hardware-
          accelerated transparent overlay windows can render solid black
          instead of transparent (see LIMITATIONS.md §13.2) — most visible on
          the trigger alert overlay, which is shown/hidden on every trigger
          fire. Off by default since it disables GPU-accelerated rendering
          app-wide; only takes effect after a restart. */}
      {hasSettingsBridge && (
        <label className="mt-3 flex cursor-pointer items-start gap-2">
          <input
            type="checkbox"
            checked={hwAccelDisabled}
            onChange={handleToggleHwAccel}
            className="mt-0.5"
          />
          <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
            Disable hardware acceleration
            <span className="block" style={{ color: 'var(--color-muted-foreground)' }}>
              Workaround for a display glitch where overlay alerts briefly
              black out the screen on some Windows systems. Only enable this
              if you're seeing that issue — it turns off GPU-accelerated
              rendering app-wide, which may make the UI feel less smooth.
              Requires a restart.
            </span>
          </span>
        </label>
      )}
      {hwAccelPendingRestart && (
        <div
          className="mt-2 flex items-center gap-2 rounded border p-2 text-xs"
          style={{ borderColor: 'var(--color-primary)', backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
        >
          <span>Restart to apply.</span>
          <button
            type="button"
            onClick={handleHwAccelRestart}
            className="rounded px-2 py-1 text-xs font-medium"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
          >
            Restart now
          </button>
        </div>
      )}
    </section>
  )
}
