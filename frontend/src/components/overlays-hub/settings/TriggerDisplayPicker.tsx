/**
 * TriggerDisplayPicker — which monitor trigger alert text (and its
 * positioning card) appears on. The trigger layer covers exactly one
 * monitor. Hidden on single-monitor setups, where there's nothing to choose.
 */
import React, { useCallback, useEffect, useState } from 'react'

type DisplayInfo = { id: number; label: string; width: number; height: number; isPrimary: boolean; isCurrent: boolean }

export default function TriggerDisplayPicker(): React.ReactElement | null {
  const [displays, setDisplays] = useState<DisplayInfo[]>([])
  const refresh = useCallback(() => {
    window.electron?.screen?.listDisplays?.()
      .then((list) => setDisplays((list ?? []) as DisplayInfo[]))
      .catch(() => {})
  }, [])
  useEffect(() => { refresh() }, [refresh])

  if (displays.length < 2) return null
  const current = displays.find((d) => d.isCurrent)?.id
  return (
    <div className="flex items-center justify-between gap-3 py-1.5">
      <div>
        <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>Alert monitor</p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          The screen trigger alert text appears on.
        </p>
      </div>
      <select
        value={current ?? ''}
        onChange={(e) => {
          window.electron?.overlay?.setDisplay?.(Number(e.target.value)).then(refresh).catch(() => {})
        }}
        className="shrink-0 rounded px-1.5 py-1 text-xs outline-none"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          border: '1px solid var(--color-border)',
          color: 'var(--color-foreground)',
        }}
      >
        {displays.map((d, i) => (
          <option key={d.id} value={d.id}>
            {d.label || `Display ${i + 1}`} ({d.width}×{d.height}){d.isPrimary ? ' · primary' : ''}
          </option>
        ))}
      </select>
    </div>
  )
}
