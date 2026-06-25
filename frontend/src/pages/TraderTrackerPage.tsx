import React, { useCallback, useEffect, useState } from 'react'
import { Store, RefreshCw, Coins, AlertTriangle, Info, TrendingUp, CheckCircle2 } from 'lucide-react'
import { ItemIcon } from '../components/Icon'
import {
  getTraderCharacters,
  getTraderListings,
  getTraderSessions,
  getTraderSnapshots,
  captureTraderSnapshot,
} from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import type {
  TraderCharacter,
  TraderListing,
  TraderSession,
  TraderSnapshotInfo,
} from '../types/trader'

// formatCoin renders a copper amount as p/g/s/c, dropping zero denominations.
// EQ coin is base-10: 1p = 10g = 100s = 1000c.
function formatCoin(copper: number): string {
  if (!copper) return '0p'
  const neg = copper < 0
  let c = Math.abs(copper)
  const p = Math.floor(c / 1000)
  c %= 1000
  const g = Math.floor(c / 100)
  c %= 100
  const s = Math.floor(c / 10)
  c %= 10
  const parts: string[] = []
  if (p) parts.push(`${p.toLocaleString()}p`)
  if (g) parts.push(`${g}g`)
  if (s) parts.push(`${s}s`)
  if (c) parts.push(`${c}c`)
  if (parts.length === 0) parts.push('0p')
  return (neg ? '-' : '') + parts.join(' ')
}

function formatWhen(value: string | number): string {
  const d = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

type Tab = 'sales' | 'listings' | 'snapshots'

export default function TraderTrackerPage(): React.ReactElement {
  const [chars, setChars] = useState<TraderCharacter[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('sales')

  const [sessions, setSessions] = useState<TraderSession[]>([])
  const [listings, setListings] = useState<TraderListing[]>([])
  const [snapshots, setSnapshots] = useState<TraderSnapshotInfo[]>([])

  const [loading, setLoading] = useState(false)
  const [capturing, setCapturing] = useState(false)
  const [captureMsg, setCaptureMsg] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const loadCharacters = useCallback(() => {
    getTraderCharacters()
      .then((cs) => {
        setChars(cs)
        setSelected((prev) => prev ?? (cs[0]?.name ?? null))
      })
      .catch((err: Error) => setError(err.message))
  }, [])

  useEffect(() => {
    loadCharacters()
  }, [loadCharacters])

  const loadDetail = useCallback((character: string) => {
    setLoading(true)
    Promise.all([
      getTraderSessions(character),
      getTraderListings(character),
      getTraderSnapshots(character),
    ])
      .then(([s, l, snaps]) => {
        setSessions(s)
        setListings(l)
        setSnapshots(snaps)
        setError(null)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (selected) loadDetail(selected)
  }, [selected, loadDetail])

  // A new auto-captured snapshot refreshes the current character's view.
  useWebSocket((msg) => {
    if (msg.type !== 'trader:snapshot') return
    const snap = msg.data as { character?: string }
    if (snap?.character && selected && snap.character.toLowerCase() === selected.toLowerCase()) {
      loadDetail(selected)
    }
    loadCharacters()
  })

  const onCapture = (): void => {
    if (!selected || capturing) return
    setCapturing(true)
    setCaptureMsg(null)
    captureTraderSnapshot(selected)
      .then((res) => {
        if (res.captured) {
          setCaptureMsg('Snapshot captured.')
          loadDetail(selected)
          loadCharacters()
        } else {
          setCaptureMsg(res.reason ?? 'No change since the last snapshot.')
        }
      })
      .catch((err: Error) => setCaptureMsg(err.message))
      .finally(() => setCapturing(false))
  }

  const current = chars.find((c) => c.name === selected)

  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-5 p-6">
      <header className="flex items-center gap-3">
        <Store size={22} style={{ color: 'var(--color-primary)' }} />
        <div>
          <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Bazaar Trader Tracker
          </h1>
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Best-guess accounting of what your parked trader sold, from inventory
            snapshots.
          </p>
        </div>
      </header>

      <ProtocolCallout />

      {error && (
        <div
          className="rounded-lg p-3 text-sm"
          style={{ backgroundColor: 'var(--color-surface)', border: '1px solid #f87171', color: '#f87171' }}
        >
          {error}
        </div>
      )}

      {chars.length === 0 ? (
        <EmptyTraders />
      ) : (
        <>
          {/* Character picker */}
          <div className="flex flex-wrap gap-2">
            {chars.map((c) => (
              <button
                key={c.name}
                type="button"
                onClick={() => setSelected(c.name)}
                className="rounded-full px-3 py-1.5 text-sm transition-colors"
                style={{
                  backgroundColor:
                    c.name === selected ? 'var(--color-primary)' : 'var(--color-surface-2)',
                  color:
                    c.name === selected
                      ? 'var(--color-background)'
                      : 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {c.name}
                <span className="ml-2 opacity-70">{c.snapshot_count} snap</span>
              </button>
            ))}
          </div>

          {current && (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap gap-4 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                <span>{current.for_sale_count} items priced for sale</span>
                <span>{current.snapshot_count} snapshots</span>
                {current.last_captured ? (
                  <span>Last capture: {formatWhen(current.last_captured)}</span>
                ) : (
                  <span>Never captured</span>
                )}
              </div>
              <div className="flex items-center gap-3">
                {captureMsg && (
                  <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    {captureMsg}
                  </span>
                )}
                <button
                  type="button"
                  onClick={onCapture}
                  disabled={capturing}
                  className="flex items-center gap-2 rounded px-3 py-1.5 text-xs font-medium transition-colors"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    color: 'var(--color-foreground)',
                    border: '1px solid var(--color-border)',
                    cursor: capturing ? 'default' : 'pointer',
                    opacity: capturing ? 0.6 : 1,
                  }}
                >
                  <RefreshCw size={13} className={capturing ? 'animate-spin' : ''} />
                  Capture now
                </button>
              </div>
            </div>
          )}

          {/* Tabs */}
          <div className="flex gap-1 border-b" style={{ borderColor: 'var(--color-border)' }}>
            {([
              ['sales', 'Sales'],
              ['listings', 'Listings'],
              ['snapshots', 'Snapshots'],
            ] as [Tab, string][]).map(([id, label]) => (
              <button
                key={id}
                type="button"
                onClick={() => setTab(id)}
                className="px-3 py-2 text-sm transition-colors"
                style={{
                  color: tab === id ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                  borderBottom:
                    tab === id ? '2px solid var(--color-primary)' : '2px solid transparent',
                  marginBottom: '-1px',
                }}
              >
                {label}
              </button>
            ))}
          </div>

          {loading ? (
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Loading…
            </p>
          ) : tab === 'sales' ? (
            <SalesTab sessions={sessions} snapshotCount={snapshots.length} />
          ) : tab === 'listings' ? (
            <ListingsTab listings={listings} />
          ) : (
            <SnapshotsTab snapshots={snapshots} />
          )}
        </>
      )}
    </div>
  )
}

function ProtocolCallout(): React.ReactElement {
  return (
    <div
      className="rounded-lg p-4 text-sm"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-muted-foreground)',
      }}
    >
      <div className="mb-2 flex items-center gap-2" style={{ color: 'var(--color-foreground)' }}>
        <Info size={15} style={{ color: 'var(--color-primary)' }} />
        <span className="font-medium">For accurate results, follow this routine</span>
      </div>
      <ol className="ml-5 list-decimal space-y-1">
        <li>
          Run <code>/output inventory</code> on your trader <strong>before</strong>{' '}
          entering trader mode.
        </li>
        <li>Park with <code>/trader</code> and let items sell.</li>
        <li>
          Log the trader back in and run <code>/output inventory</code>{' '}
          <strong>again</strong>.
        </li>
      </ol>
      <p className="mt-2">
        Entering trader mode boots the client and skips the normal export-on-camp,
        so the two manual exports are the only way to capture before/after state.
        Items that left a Trader&rsquo;s Satchel between the two are inferred as
        sales &mdash; this is a best guess, not a real sales log.
      </p>
    </div>
  )
}

function EmptyTraders(): React.ReactElement {
  return (
    <div
      className="rounded-lg p-6 text-center text-sm"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        color: 'var(--color-muted-foreground)',
      }}
    >
      No trader characters found. A character shows up here once it has a{' '}
      <code>BZR_&lt;Name&gt;.ini</code> price file in your EQ directory &mdash; that
      file is created the first time you set a price with <code>/trader</code>.
    </div>
  )
}

function SalesTab({
  sessions,
  snapshotCount,
}: {
  sessions: TraderSession[]
  snapshotCount: number
}): React.ReactElement {
  if (snapshotCount < 2) {
    return (
      <div
        className="rounded-lg p-6 text-center text-sm"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-muted-foreground)',
        }}
      >
        Need at least two snapshots to infer sales. Capture one before and one
        after a trading session (see the routine above).
      </div>
    )
  }
  if (sessions.length === 0) {
    return (
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        No satchel changes detected between snapshots yet.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      {sessions.map((s, i) => (
        <SessionCard key={`${s.to_time}-${i}`} session={s} />
      ))}
    </div>
  )
}

function SessionCard({ session }: { session: TraderSession }): React.ReactElement {
  return (
    <div
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          {formatWhen(session.from_time)} → {formatWhen(session.to_time)}
        </span>
        <div className="flex items-center gap-3">
          <span
            className="flex items-center gap-1.5 text-sm font-semibold"
            style={{ color: 'var(--color-foreground)' }}
          >
            <TrendingUp size={14} style={{ color: 'var(--color-primary)' }} />
            {formatCoin(session.estimated_revenue)} est.
          </span>
          {session.estimated_revenue > 0 &&
            (session.reconciles ? (
              <span className="flex items-center gap-1 text-xs" style={{ color: '#4ade80' }}>
                <CheckCircle2 size={12} /> coin matches
              </span>
            ) : (
              <span className="flex items-center gap-1 text-xs" style={{ color: '#fbbf24' }}>
                <AlertTriangle size={12} /> coin off
              </span>
            ))}
        </div>
      </div>

      {session.sold.length > 0 && (
        <table className="w-full text-sm">
          <thead>
            <tr style={{ color: 'var(--color-muted)' }} className="text-left text-xs uppercase">
              <th className="pb-1 font-medium">Item</th>
              <th className="pb-1 text-right font-medium">Qty</th>
              <th className="pb-1 text-right font-medium">Unit</th>
              <th className="pb-1 text-right font-medium">Total</th>
            </tr>
          </thead>
          <tbody>
            {session.sold.map((it) => (
              <tr key={it.item_id} style={{ color: 'var(--color-foreground)' }}>
                <td className="py-0.5">
                  <span className="flex items-center gap-2">
                    <ItemIcon id={it.icon ?? 0} name={it.name} size={18} />
                    {it.name}
                    {!it.listed && (
                      <span
                        className="rounded px-1 text-[10px]"
                        style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
                        title="No price listed in the BZR file"
                      >
                        unpriced
                      </span>
                    )}
                  </span>
                </td>
                <td className="py-0.5 text-right">{it.qty}</td>
                <td className="py-0.5 text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                  {it.listed ? formatCoin(it.unit_price) : '—'}
                </td>
                <td className="py-0.5 text-right">{it.listed ? formatCoin(it.line_total) : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div className="mt-3 flex flex-wrap gap-4 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        <span className="flex items-center gap-1">
          <Coins size={12} /> On-person: {formatCoin(session.on_person_delta)}
        </span>
        {session.restocked.length > 0 && (
          <span>Restocked: {session.restocked.map((r) => `${r.name} ×${r.qty}`).join(', ')}</span>
        )}
      </div>

      {session.caveats.length > 0 && (
        <details className="mt-2">
          <summary className="cursor-pointer text-xs" style={{ color: 'var(--color-muted)' }}>
            Caveats
          </summary>
          <ul className="ml-4 mt-1 list-disc space-y-0.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {session.caveats.map((c, i) => (
              <li key={i}>{c}</li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}

function ListingsTab({ listings }: { listings: TraderListing[] }): React.ReactElement {
  if (listings.length === 0) {
    return (
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        No BZR price file found for this character.
      </p>
    )
  }
  return (
    <div
      className="overflow-hidden rounded-lg"
      style={{ border: '1px solid var(--color-border)' }}
    >
      <table className="w-full text-sm">
        <thead>
          <tr
            style={{ backgroundColor: 'var(--color-surface)', color: 'var(--color-muted)' }}
            className="text-left text-xs uppercase"
          >
            <th className="px-3 py-2 font-medium">Item</th>
            <th className="px-3 py-2 text-right font-medium">Price</th>
            <th className="px-3 py-2 text-right font-medium">In satchel</th>
          </tr>
        </thead>
        <tbody>
          {listings.map((l) => (
            <tr
              key={l.name}
              style={{
                color: 'var(--color-foreground)',
                borderTop: '1px solid var(--color-border)',
                opacity: l.price > 0 ? 1 : 0.5,
              }}
            >
              <td className="px-3 py-1.5">
                <span className="flex items-center gap-2">
                  {l.item_id ? <ItemIcon id={l.icon ?? 0} name={l.name} size={18} /> : null}
                  {l.name}
                </span>
              </td>
              <td className="px-3 py-1.5 text-right">
                {l.price > 0 ? formatCoin(l.price) : 'not for sale'}
              </td>
              <td className="px-3 py-1.5 text-right">
                {l.in_satchel > 0 ? (
                  <span style={{ color: 'var(--color-primary)' }}>{l.in_satchel}</span>
                ) : (
                  <span style={{ color: 'var(--color-muted)' }}>—</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function SnapshotsTab({ snapshots }: { snapshots: TraderSnapshotInfo[] }): React.ReactElement {
  if (snapshots.length === 0) {
    return (
      <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        No snapshots captured yet.
      </p>
    )
  }
  // Newest first for display.
  const rows = [...snapshots].reverse()
  return (
    <div className="overflow-hidden rounded-lg" style={{ border: '1px solid var(--color-border)' }}>
      <table className="w-full text-sm">
        <thead>
          <tr
            style={{ backgroundColor: 'var(--color-surface)', color: 'var(--color-muted)' }}
            className="text-left text-xs uppercase"
          >
            <th className="px-3 py-2 font-medium">Captured</th>
            <th className="px-3 py-2 text-right font-medium">Satchel items</th>
            <th className="px-3 py-2 text-right font-medium">Total qty</th>
            <th className="px-3 py-2 text-right font-medium">On-person</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((s, i) => (
            <tr
              key={`${s.taken_at}-${i}`}
              style={{ color: 'var(--color-foreground)', borderTop: '1px solid var(--color-border)' }}
            >
              <td className="px-3 py-1.5">{formatWhen(s.taken_at)}</td>
              <td className="px-3 py-1.5 text-right">{s.item_count}</td>
              <td className="px-3 py-1.5 text-right">{s.total_qty}</td>
              <td className="px-3 py-1.5 text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                {formatCoin(s.on_person)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
