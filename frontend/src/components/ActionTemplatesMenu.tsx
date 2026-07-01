import React, { useEffect, useState } from 'react'
import { BookmarkPlus, LayoutTemplate, Star, Trash2, X, Check } from 'lucide-react'
import {
  listActionTemplates,
  createActionTemplate,
  updateActionTemplate,
  deleteActionTemplate,
} from '../services/api'
import type { Action, ActionTemplate } from '../types/trigger'

interface ActionTemplatesMenuProps {
  /** The editor's current actions — used by "Save current as template". */
  currentActions: Action[]
  /** Replace the editor's actions with a (cloned) template's actions. */
  onApply: (actions: Action[]) => void
}

/** Deep-clone actions so applying a template never shares references. */
function cloneActions(actions: Action[]): Action[] {
  return JSON.parse(JSON.stringify(actions)) as Action[]
}

/** Short human summary of a template's action types, e.g. "overlay + sound". */
function actionsSummary(actions: Action[]): string {
  if (actions.length === 0) return 'no actions'
  const label: Record<string, string> = {
    overlay_text: 'overlay',
    play_sound: 'sound',
    text_to_speech: 'TTS',
    clipboard: 'clipboard',
  }
  return actions.map((a) => label[a.type] ?? a.type).join(' + ')
}

/**
 * The "Templates" button next to "Add" in the trigger editor's Actions
 * section: save the current actions under a name, apply a saved template
 * (replacing the current actions), star one template as the default that
 * prefills new triggers, and delete templates.
 */
export default function ActionTemplatesMenu({
  currentActions,
  onApply,
}: ActionTemplatesMenuProps): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [templates, setTemplates] = useState<ActionTemplate[]>([])
  const [saving, setSaving] = useState(false)
  const [newName, setNewName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const refresh = () => {
    listActionTemplates()
      .then(setTemplates)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    if (open) {
      setError(null)
      refresh()
    }
  }, [open])

  const handleSaveCurrent = () => {
    const name = newName.trim()
    if (!name || currentActions.length === 0) return
    setSaving(true)
    setError(null)
    createActionTemplate(name, cloneActions(currentActions), false)
      .then(() => {
        setNewName('')
        refresh()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  const handleSetDefault = (t: ActionTemplate) => {
    updateActionTemplate({ ...t, is_default: !t.is_default })
      .then(refresh)
      .catch((err: Error) => setError(err.message))
  }

  const handleDelete = (t: ActionTemplate) => {
    deleteActionTemplate(t.id)
      .then(refresh)
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded"
        style={{
          backgroundColor: 'var(--color-surface-2)',
          color: 'var(--color-muted-foreground)',
          border: '1px solid var(--color-border)',
        }}
        title="Save or apply reusable action sets"
      >
        <LayoutTemplate size={10} /> Templates
      </button>
      {open && (
        <>
          <div onClick={() => setOpen(false)} style={{ position: 'fixed', inset: 0, zIndex: 40 }} />
          <div
            className="absolute right-0 top-full mt-1 rounded shadow-lg p-2 space-y-2"
            style={{
              backgroundColor: 'var(--color-surface)',
              border: '1px solid var(--color-border)',
              zIndex: 50,
              width: 300,
            }}
          >
            <div className="flex items-center justify-between">
              <p className="text-[11px] font-semibold" style={{ color: 'var(--color-foreground)' }}>
                Action templates
              </p>
              <button type="button" onClick={() => setOpen(false)} aria-label="Close">
                <X size={12} style={{ color: 'var(--color-muted)' }} />
              </button>
            </div>

            {templates.length === 0 && (
              <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                No templates yet. Set up the actions you like below, then save
                them here to reuse on other triggers.
              </p>
            )}
            {templates.map((t) => (
              <div
                key={t.id}
                className="flex items-center gap-1.5 rounded px-1.5 py-1"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <button
                  type="button"
                  onClick={() => handleSetDefault(t)}
                  title={
                    t.is_default
                      ? 'Default — prefills new triggers. Click to unset.'
                      : 'Make this the default for new triggers'
                  }
                >
                  <Star
                    size={12}
                    fill={t.is_default ? 'var(--color-warning, #f59e0b)' : 'none'}
                    style={{ color: t.is_default ? 'var(--color-warning, #f59e0b)' : 'var(--color-muted)' }}
                  />
                </button>
                <div className="flex-1 min-w-0">
                  <p className="text-[11px] truncate" style={{ color: 'var(--color-foreground)' }}>
                    {t.name}
                  </p>
                  <p className="text-[10px] truncate" style={{ color: 'var(--color-muted)' }}>
                    {actionsSummary(t.actions)}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    onApply(cloneActions(t.actions))
                    setOpen(false)
                  }}
                  className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded shrink-0"
                  style={{
                    backgroundColor: 'var(--color-primary)',
                    color: 'var(--color-background)',
                  }}
                  title="Replace this trigger's actions with the template"
                >
                  <Check size={10} /> Apply
                </button>
                <button
                  type="button"
                  onClick={() => handleDelete(t)}
                  className="shrink-0"
                  title="Delete template"
                >
                  <Trash2 size={11} style={{ color: 'var(--color-danger)' }} />
                </button>
              </div>
            ))}

            <div className="flex items-center gap-1.5 pt-1" style={{ borderTop: '1px solid var(--color-border)' }}>
              <input
                type="text"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    handleSaveCurrent()
                  }
                }}
                placeholder="Save current actions as…"
                className="flex-1 min-w-0 text-[11px] px-1.5 py-1 rounded"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              />
              <button
                type="button"
                onClick={handleSaveCurrent}
                disabled={saving || !newName.trim() || currentActions.length === 0}
                className="flex items-center gap-1 text-[10px] px-1.5 py-1 rounded shrink-0"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                  opacity: saving || !newName.trim() || currentActions.length === 0 ? 0.5 : 1,
                }}
                title={
                  currentActions.length === 0
                    ? 'Add at least one action first'
                    : 'Save the actions currently in the editor as a template'
                }
              >
                <BookmarkPlus size={10} /> Save
              </button>
            </div>
            {error && (
              <p className="text-[10px]" style={{ color: 'var(--color-danger)' }}>
                {error}
              </p>
            )}
          </div>
        </>
      )}
    </div>
  )
}
