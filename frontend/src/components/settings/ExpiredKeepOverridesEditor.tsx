import React, { useState } from 'react'
import { Plus, X } from 'lucide-react'
import SpellSearchPicker from '../SpellSearchPicker'

interface ExpiredKeepOverridesEditorProps {
  value: Record<string, number> | undefined
  onChange: (next: Record<string, number>) => void
}

/**
 * Settings editor for spell_timer.expired_keep_overrides: spells whose
 * expired timers should linger for their own number of minutes instead of
 * the Buff / Detrimental values (0 = drop as soon as it expires).
 */
export default function ExpiredKeepOverridesEditor({
  value,
  onChange,
}: ExpiredKeepOverridesEditorProps): React.ReactElement {
  const [picking, setPicking] = useState(false)
  const entries = Object.entries(value ?? {}).sort(([a], [b]) => a.localeCompare(b))

  const set = (name: string, mins: number): void =>
    onChange({ ...(value ?? {}), [name]: mins })
  const remove = (name: string): void => {
    const next = { ...(value ?? {}) }
    delete next[name]
    onChange(next)
  }

  return (
    <div className="mt-3">
      <p className="mb-1 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Per-spell overrides — give a spell its own time, e.g. drop expired
        Quiescence after 1 minute while other buffs stay for 30. Set <b>0</b> to
        remove that spell as soon as it expires. Add each rank or variant you
        want covered (Ferine / Feral / Primal Avatar are separate spells).
      </p>

      {entries.length > 0 && (
        <div className="mb-2 flex flex-col gap-1">
          {entries.map(([name, mins]) => (
            <div
              key={name}
              className="flex items-center gap-2 rounded border px-2 py-1 text-xs"
              style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
            >
              <span className="flex-1 truncate" style={{ color: 'var(--color-foreground)' }}>{name}</span>
              <input
                type="number"
                min={0}
                step={1}
                value={mins}
                onChange={(e) => set(name, Math.max(0, Math.floor(Number(e.target.value) || 0)))}
                className="w-16 rounded border px-1 py-0.5 text-xs"
                style={{
                  borderColor: 'var(--color-border)',
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                }}
              />
              <span style={{ color: 'var(--color-muted-foreground)' }}>min</span>
              <button
                onClick={() => remove(name)}
                title={`Remove ${name} override`}
                style={{ color: 'var(--color-muted)' }}
              >
                <X size={13} />
              </button>
            </div>
          ))}
        </div>
      )}

      <button
        onClick={() => setPicking(true)}
        className="flex items-center gap-1 rounded border px-2 py-1 text-xs"
        style={{
          borderColor: 'var(--color-border)',
          backgroundColor: 'var(--color-surface-2)',
          color: 'var(--color-muted-foreground)',
        }}
      >
        <Plus size={11} /> Add spell
      </button>

      {picking && (
        <SpellSearchPicker
          onPick={(spell) => {
            set(spell.name, 5)
            setPicking(false)
          }}
          onClose={() => setPicking(false)}
        />
      )}
    </div>
  )
}
