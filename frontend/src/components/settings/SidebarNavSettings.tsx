import React, { useEffect, useMemo, useState } from 'react'
import { PanelLeft, ChevronUp, ChevronDown, RotateCcw, Eye, EyeOff } from 'lucide-react'
import { getConfig, updateConfig } from '../../services/api'
import type { Config } from '../../types/config'
import { NAV_SECTIONS, visibleNavSections, orderItems, type NavItem, type NavSection } from '../../lib/sidebarNav'

// Per-section working order of route keys (includes hidden items so they can be
// re-enabled and moved).
type SecOrder = Record<string, string[]>

// navFlags maps the Developer-tab flags that gate optional nav tabs to their
// enabled state, so gated tabs (Resist Calculator, Trader Tracker) only show in
// this editor while their flag is on — matching the live sidebar.
function navFlags(c: Config | null): Record<string, boolean> {
  return {
    resist_calc_enabled: Boolean(c?.preferences?.resist_calc_enabled),
    trader_tracker_enabled: Boolean(c?.preferences?.trader_tracker_enabled),
  }
}

export default function SidebarNavSettings(): React.ReactElement {
  const [cfg, setCfg] = useState<Config | null>(null)
  const [secOrder, setSecOrder] = useState<SecOrder>({})
  const [hidden, setHidden] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState(false)

  const itemByKey = useMemo(() => {
    const m = new Map<string, NavItem>()
    NAV_SECTIONS.forEach((s) => s.items.forEach((i) => m.set(i.to, i)))
    return m
  }, [])

  // Sections to display/reorder, with flag-gated tabs filtered to those enabled.
  const sections: NavSection[] = useMemo(() => visibleNavSections(navFlags(cfg)), [cfg])

  function hydrate(c: Config) {
    const order = c.preferences?.sidebar_order ?? []
    const next: SecOrder = {}
    visibleNavSections(navFlags(c)).forEach((s) => {
      next[s.id] = orderItems(s.items, order).map((i) => i.to)
    })
    setSecOrder(next)
    setHidden(new Set(c.preferences?.sidebar_hidden ?? []))
  }

  useEffect(() => {
    getConfig()
      .then((c) => { setCfg(c); hydrate(c) })
      .catch(() => {})
  }, [])

  // Persist the given order/hidden, broadcasting config:updated so the live
  // sidebar reflects the change immediately. Only the currently-visible tabs'
  // order is written; tabs hidden by a disabled flag fall back to their default
  // position (via orderItems) until the flag is re-enabled.
  async function persist(nextOrder: SecOrder, nextHidden: Set<string>) {
    if (!cfg) return
    setSaving(true)
    const flatOrder = sections.flatMap((s) => nextOrder[s.id] ?? [])
    const next: Config = {
      ...cfg,
      preferences: {
        ...cfg.preferences,
        sidebar_order: flatOrder,
        sidebar_hidden: Array.from(nextHidden),
      },
    }
    setCfg(next)
    try {
      const saved = await updateConfig(next)
      setCfg(saved)
    } finally {
      setSaving(false)
    }
  }

  function toggleHidden(key: string) {
    const next = new Set(hidden)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setHidden(next)
    persist(secOrder, next)
  }

  function move(sectionId: string, key: string, dir: -1 | 1) {
    const list = [...(secOrder[sectionId] ?? [])]
    const i = list.indexOf(key)
    const j = i + dir
    if (i < 0 || j < 0 || j >= list.length) return
    ;[list[i], list[j]] = [list[j], list[i]]
    const next = { ...secOrder, [sectionId]: list }
    setSecOrder(next)
    persist(next, hidden)
  }

  function resetDefaults() {
    const next: SecOrder = {}
    sections.forEach((s) => { next[s.id] = s.items.map((i) => i.to) })
    setSecOrder(next)
    setHidden(new Set())
    persist(next, new Set())
  }

  const visibleCount = sections.reduce((n, s) => n + s.items.filter((i) => !hidden.has(i.to)).length, 0)
  const totalCount = sections.reduce((n, s) => n + s.items.length, 0)

  return (
    <div className="mx-auto max-w-xl p-6">
      <div className="mb-6 flex items-center gap-3">
        <PanelLeft size={20} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>Navigation</h1>
      </div>

      <section className="rounded-lg p-4" style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
        <div className="mb-3 flex items-center justify-between gap-2">
          <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
            Show, hide, and reorder the side navigation tabs. Hiding a tab only removes it from the sidebar — the
            page is still reachable. The search, back/forward, character switcher, and settings controls can't be
            hidden.
          </p>
          <button
            onClick={resetDefaults}
            disabled={saving}
            className="flex shrink-0 items-center gap-1.5 rounded px-2 py-1 text-xs font-medium disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
            title="Show all tabs in default order"
          >
            <RotateCcw size={12} /> Reset
          </button>
        </div>

        <p className="mb-3 text-[11px]" style={{ color: 'var(--color-muted)' }}>{visibleCount} of {totalCount} tabs shown</p>

        <div className="flex flex-col gap-4">
          {sections.map((section) => {
            const allowed = new Set(section.items.map((i) => i.to))
            const keys = (secOrder[section.id] ?? section.items.map((i) => i.to)).filter((k) => allowed.has(k))
            return (
              <div key={section.id}>
                <p className="mb-1.5 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
                  {section.label}
                </p>
                <div className="flex flex-col gap-1">
                  {keys.map((key, idx) => {
                    const item = itemByKey.get(key)
                    if (!item) return null
                    const isHidden = hidden.has(key)
                    return (
                      <div
                        key={key}
                        className="flex items-center gap-2 rounded px-2 py-1.5"
                        style={{ backgroundColor: 'var(--color-surface-2)', opacity: isHidden ? 0.55 : 1 }}
                      >
                        <span style={{ color: 'var(--color-muted-foreground)' }}>{item.icon}</span>
                        <span className="flex-1 text-sm" style={{ color: 'var(--color-foreground)' }}>{item.label}</span>
                        <button
                          onClick={() => move(section.id, key, -1)}
                          disabled={idx === 0 || saving}
                          className="rounded p-1 disabled:opacity-30"
                          style={{ color: 'var(--color-muted-foreground)' }}
                          title="Move up"
                        >
                          <ChevronUp size={14} />
                        </button>
                        <button
                          onClick={() => move(section.id, key, 1)}
                          disabled={idx === keys.length - 1 || saving}
                          className="rounded p-1 disabled:opacity-30"
                          style={{ color: 'var(--color-muted-foreground)' }}
                          title="Move down"
                        >
                          <ChevronDown size={14} />
                        </button>
                        <button
                          onClick={() => toggleHidden(key)}
                          disabled={saving}
                          className="rounded p-1"
                          style={{ color: isHidden ? 'var(--color-muted)' : 'var(--color-primary)' }}
                          title={isHidden ? 'Hidden — click to show' : 'Visible — click to hide'}
                        >
                          {isHidden ? <EyeOff size={15} /> : <Eye size={15} />}
                        </button>
                      </div>
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>
      </section>
    </div>
  )
}
