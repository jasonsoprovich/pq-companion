import React, { useState } from 'react'
import { Sparkles, MessageCircle, Coffee, Code2 } from 'lucide-react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import type { ChangelogEntry } from '../services/api'

// Mirrors the community-link constants in SettingsPage.tsx (About tab) —
// duplicated rather than imported so this modal, which mounts eagerly at app
// boot, doesn't pull in the lazy-loaded Settings page bundle.
const DISCORD_URL = 'https://discord.gg/Srj4FXcRaz'
const KOFI_URL = 'https://ko-fi.com/jasonsoprovich'
const GITHUB_URL = 'https://github.com/jasonsoprovich/pq-companion'

export interface WhatsNewModalProps {
  entries: ChangelogEntry[]
  onDismiss: (disablePopup: boolean) => void
}

// renderBody turns a changelog entry's raw markdown (### subheadings, "- "
// bullets, plain paragraphs — the only constructs CHANGELOG.md ever uses)
// into lightweight JSX. Not a general markdown renderer; the format is
// narrow enough that pulling in a markdown dependency isn't worth it.
export function renderChangelogBody(body: string): React.ReactElement {
  const lines = body.split('\n')
  const blocks: React.ReactElement[] = []
  let bullets: string[] = []
  const flushBullets = (key: string): void => {
    if (bullets.length === 0) return
    blocks.push(
      <ul key={key} className="mb-2 ml-4 list-disc space-y-1">
        {bullets.map((b, i) => (
          <li key={i} className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
            {b}
          </li>
        ))}
      </ul>
    )
    bullets = []
  }
  lines.forEach((line, i) => {
    const trimmed = line.trim()
    if (trimmed.startsWith('### ')) {
      flushBullets(`ul-${i}`)
      blocks.push(
        <p key={i} className="mb-1 mt-2 text-xs font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted)' }}>
          {trimmed.slice(4)}
        </p>
      )
    } else if (trimmed.startsWith('- ')) {
      bullets.push(trimmed.slice(2))
    } else if (trimmed) {
      flushBullets(`ul-${i}`)
      blocks.push(
        <p key={i} className="mb-2 text-sm leading-relaxed" style={{ color: 'var(--color-foreground)' }}>
          {trimmed}
        </p>
      )
    }
  })
  flushBullets('ul-end')
  return <>{blocks}</>
}

export default function WhatsNewModal({ entries, onDismiss }: WhatsNewModalProps): React.ReactElement {
  const [dontShowAgain, setDontShowAgain] = useState(false)
  useEscapeToClose(() => onDismiss(dontShowAgain))

  return (
    <div
      onClick={() => onDismiss(dontShowAgain)}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="flex flex-col rounded-lg"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 480,
          maxHeight: '80vh',
        }}
      >
        <div className="flex items-center gap-2 px-4 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
          <Sparkles size={16} style={{ color: 'var(--color-primary)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            What&rsquo;s New
          </span>
        </div>

        <div className="overflow-y-auto px-4 py-3" style={{ flex: 1 }}>
          {entries.map((entry) => (
            <div key={entry.version} className="mb-4 last:mb-0">
              <p className="mb-1 text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
                v{entry.version} <span style={{ color: 'var(--color-muted-foreground)', fontWeight: 400 }}>— {entry.date}</span>
              </p>
              {renderChangelogBody(entry.body)}
            </div>
          ))}
        </div>

        <div className="px-4 py-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
          <div className="mb-3 grid grid-cols-3 gap-2">
            <a
              href={DISCORD_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center justify-center gap-1.5 rounded px-2 py-1.5 text-xs font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <MessageCircle size={13} style={{ color: '#5865F2' }} />
              Discord
            </a>
            <a
              href={KOFI_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center justify-center gap-1.5 rounded px-2 py-1.5 text-xs font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <Coffee size={13} style={{ color: '#FF5E5B' }} />
              Ko-fi
            </a>
            <a
              href={GITHUB_URL}
              target="_blank"
              rel="noreferrer noopener"
              className="flex items-center justify-center gap-1.5 rounded px-2 py-1.5 text-xs font-medium transition-colors"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                textDecoration: 'none',
              }}
            >
              <Code2 size={13} style={{ color: 'var(--color-muted-foreground)' }} />
              GitHub
            </a>
          </div>

          <div className="flex items-center justify-between">
            <label className="flex cursor-pointer items-center gap-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              <input
                type="checkbox"
                checked={dontShowAgain}
                onChange={(e) => setDontShowAgain(e.target.checked)}
              />
              Don&rsquo;t show this again
            </label>
            <button
              onClick={() => onDismiss(dontShowAgain)}
              className="px-3 py-1.5 text-sm font-medium rounded"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: 'var(--color-primary-foreground, #fff)',
                border: 'none',
              }}
            >
              Got it
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
