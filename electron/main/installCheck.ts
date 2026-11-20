import { execFile } from 'child_process'
import { existsSync, readFileSync, writeFileSync } from 'fs'
import { join, win32 } from 'path'

// Detects a copy of PQ Companion that the NSIS installer doesn't manage — an
// unzipped portable build or a copied folder. The installer records where it
// put the app in the registry and every later install (including the silent
// one the auto-updater runs) reuses that folder, so a properly installed copy
// always updates in place. An unmanaged copy has no such record: when its
// auto-updater runs the installer, it lands in the default folder instead,
// leaving the user with two copies while their shortcut keeps opening the old
// one (seen twice: issue #160 and a 0.20.1 copy that "kept asking to update").
//
// So an unmanaged copy must never self-update, and if a managed install exists
// elsewhere we point the user at it.

// electron-builder derives this from appId (UUID v5 of
// com.jasonsoprovich.pq-companion in its namespace) and writes
// InstallLocation under Software\<guid>. It only changes if appId does.
const APP_GUID = 'bd487ffa-8bf4-539a-a5b6-f77c7ea057b0'
const INSTALL_KEY = `Software\\${APP_GUID}`
const UNINSTALL_KEY = `Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\${APP_GUID}`

// The installer drops this next to the exe; the portable zip doesn't ship it.
const UNINSTALLER_NAME = 'Uninstall PQ Companion.exe'

// Set to silence the startup warning for deliberate side-by-side test copies.
// Self-update stays disabled for unmanaged copies regardless.
export const SKIP_ENV = 'PQ_COMPANION_SKIP_INSTALL_CHECK'

const REG_TIMEOUT_MS = 3000

export interface ManagedInstall {
  exePath: string
  version: string // "" when the uninstall entry carries none
}

function normalizePath(p: string): string {
  return p.replace(/\//g, '\\').replace(/\\+$/, '').toLowerCase()
}

export function isManagedCopy(): boolean {
  return existsSync(join(win32.dirname(process.execPath), UNINSTALLER_NAME))
}

function regValue(key: string, name: string, view?: string): Promise<string> {
  const args = ['query', key, '/v', name]
  if (view) args.push(view)
  return new Promise((resolve) => {
    execFile('reg.exe', args, { timeout: REG_TIMEOUT_MS, windowsHide: true }, (err, stdout) => {
      if (err) {
        resolve('')
        return
      }
      const m = stdout.match(new RegExp(`^\\s*${name}\\s+REG_\\w+\\s+(.+?)\\s*$`, 'm'))
      resolve(m ? m[1] : '')
    })
  })
}

// findManagedInstall returns the installer-managed copy recorded in the
// registry when it lives in a different folder than this one. Per-user
// installs (the default since perMachine: false) are in HKCU; older builds
// offered an all-users install, recorded in HKLM.
export async function findManagedInstall(): Promise<ManagedInstall | null> {
  if (process.platform !== 'win32') return null
  const exeName = win32.basename(process.execPath)
  const ownDir = normalizePath(win32.dirname(process.execPath))
  const hives: [string, string | undefined][] = [
    ['HKCU', undefined],
    ['HKLM', '/reg:64'],
    ['HKLM', '/reg:32'],
  ]
  for (const [hive, view] of hives) {
    const dir = await regValue(`${hive}\\${INSTALL_KEY}`, 'InstallLocation', view)
    if (!dir || normalizePath(dir) === ownDir) continue
    const exePath = win32.join(dir, exeName)
    if (!existsSync(exePath)) continue
    const version = await regValue(`${hive}\\${UNINSTALL_KEY}`, 'DisplayVersion', view)
    return { exePath, version }
  }
  return null
}

// Folders whose warning the user chose to silence, kept in userData so the
// choice survives restarts. Keyed by folder so a different stray copy still
// warns.
function ackFile(userDataDir: string): string {
  return join(userDataDir, 'installCheck.json')
}

export function isWarningSilenced(userDataDir: string): boolean {
  try {
    const data = JSON.parse(readFileSync(ackFile(userDataDir), 'utf8'))
    const own = normalizePath(win32.dirname(process.execPath))
    return Array.isArray(data.silencedFolders) && data.silencedFolders.includes(own)
  } catch {
    return false
  }
}

export function silenceWarning(userDataDir: string): void {
  let folders: string[] = []
  try {
    const data = JSON.parse(readFileSync(ackFile(userDataDir), 'utf8'))
    if (Array.isArray(data.silencedFolders)) folders = data.silencedFolders
  } catch {
    // missing or corrupt — start fresh
  }
  const own = normalizePath(win32.dirname(process.execPath))
  if (!folders.includes(own)) folders.push(own)
  try {
    writeFileSync(ackFile(userDataDir), JSON.stringify({ silencedFolders: folders }, null, 2))
  } catch {
    // best effort; worst case the warning shows again next launch
  }
}

export function unmanagedCopyMessage(
  managed: ManagedInstall,
  ownVersion: string,
): { message: string; detail: string } {
  const managedLabel = managed.version ? `v${managed.version}` : 'the installed copy'
  const message = 'You are running a copy of PQ Companion that the installer doesn\'t manage'
  const detail =
    `This copy (v${ownVersion}):\n${win32.dirname(process.execPath)}\n\n` +
    `Installed copy (${managedLabel}):\n${win32.dirname(managed.exePath)}\n\n` +
    'Updates only go to the installed copy, so this one will never update. ' +
    'Use the installed copy instead (Start menu or desktop shortcut), and ' +
    'repin any taskbar shortcut that points here. If you don\'t need this ' +
    'folder, you can delete it.\n\n' +
    'Your settings, characters and triggers are shared by both copies, so ' +
    'nothing is lost.'
  return { message, detail }
}

const RELEASES_URL = 'https://github.com/jasonsoprovich/pq-companion/releases'

export interface ManualUpdateInfo {
  version: string
  downloadUrl: string // the portable zip for this release
  releaseUrl: string // the GitHub release page (notes + installer)
}

// manualUpdateInfo builds the links shown to an unmanaged copy when an update
// is out. It can't self-update, so it gets a one-click download of the same
// portable zip it was most likely unpacked from (electron-builder.yml
// artifactName: PQ-Companion-Setup-${version}.zip) — the installer isn't an
// option for the Wine/Linux users the zip exists for.
export function manualUpdateInfo(version: string): ManualUpdateInfo {
  const tag = `v${version}`
  return {
    version,
    downloadUrl: `${RELEASES_URL}/download/${tag}/PQ-Companion-Setup-${version}.zip`,
    releaseUrl: `${RELEASES_URL}/tag/${tag}`,
  }
}
