/**
 * Small form pieces shared by the Custom Overlay editor's tabs, styled to
 * match the hub's settings drawers.
 */
import React from 'react'
import { ArrowDown, ArrowUp, Trash2 } from 'lucide-react'

export const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

export const INPUT = 'rounded px-2 py-1 text-xs outline-none focus:border-[var(--color-primary)]'

export function TextInput({
  value,
  onChange,
  placeholder,
  mono,
  className = '',
  onFocus,
  inputRef,
  ariaLabel,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
  className?: string
  onFocus?: (e: React.FocusEvent<HTMLInputElement>) => void
  inputRef?: React.Ref<HTMLInputElement>
  ariaLabel?: string
}): React.ReactElement {
  return (
    <input
      ref={inputRef}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onFocus={onFocus}
      placeholder={placeholder}
      aria-label={ariaLabel ?? placeholder}
      spellCheck={false}
      className={`${INPUT} min-w-0 ${mono ? 'font-mono' : ''} ${className}`}
      style={inputStyle}
    />
  )
}

export function NumberInput({
  value,
  onChange,
  min,
  max,
  placeholder,
  ariaLabel,
}: {
  value: number | undefined
  onChange: (v: number | undefined) => void
  min?: number
  max?: number
  placeholder?: string
  ariaLabel?: string
}): React.ReactElement {
  return (
    <input
      type="number"
      value={value ?? ''}
      min={min}
      max={max}
      placeholder={placeholder}
      aria-label={ariaLabel ?? placeholder}
      onChange={(e) => onChange(e.target.value === '' ? undefined : Number(e.target.value))}
      className={`${INPUT} w-20`}
      style={inputStyle}
    />
  )
}

export function Select<T extends string>({
  value,
  options,
  onChange,
  ariaLabel,
  className = '',
}: {
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
  ariaLabel?: string
  className?: string
}): React.ReactElement {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value as T)}
      aria-label={ariaLabel}
      className={`${INPUT} ${className}`}
      style={inputStyle}
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </select>
  )
}

export function Label({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <span className="w-24 shrink-0 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
      {children}
    </span>
  )
}

export function Row({ label, children }: { label: string; children: React.ReactNode }): React.ReactElement {
  return (
    <div className="flex items-center gap-2">
      <Label>{label}</Label>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">{children}</div>
    </div>
  )
}

export function IconButton({
  icon: Icon,
  title,
  onClick,
  disabled,
  danger,
}: {
  icon: typeof Trash2
  title: string
  onClick: () => void
  disabled?: boolean
  danger?: boolean
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      aria-label={title}
      className="rounded p-1"
      style={{
        color: danger ? 'var(--color-danger)' : 'var(--color-muted-foreground)',
        opacity: disabled ? 0.3 : 1,
        cursor: disabled ? 'default' : 'pointer',
      }}
    >
      <Icon size={13} />
    </button>
  )
}

// Up / down / delete for an item in an ordered list.
export function ListControls({
  index,
  count,
  onMove,
  onDelete,
  what,
}: {
  index: number
  count: number
  onMove: (dir: -1 | 1) => void
  onDelete: () => void
  what: string
}): React.ReactElement {
  return (
    <span className="flex shrink-0 items-center">
      <IconButton icon={ArrowUp} title={`Move ${what} up`} disabled={index === 0} onClick={() => onMove(-1)} />
      <IconButton icon={ArrowDown} title={`Move ${what} down`} disabled={index === count - 1} onClick={() => onMove(1)} />
      <IconButton icon={Trash2} title={`Delete ${what}`} danger onClick={onDelete} />
    </span>
  )
}

export function move<T>(list: T[], index: number, dir: -1 | 1): T[] {
  const j = index + dir
  if (j < 0 || j >= list.length) return list
  const next = [...list]
  ;[next[index], next[j]] = [next[j], next[index]]
  return next
}

export function Box({ children }: { children: React.ReactNode }): React.ReactElement {
  return (
    <div
      className="flex flex-col gap-2 rounded-lg p-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      {children}
    </div>
  )
}

export function AddButton({
  label,
  onClick,
  title,
}: {
  label: string
  onClick: () => void
  title?: string
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      className="rounded px-2 py-1 text-xs"
      style={{ border: '1px dashed var(--color-border)', color: 'var(--color-muted-foreground)' }}
    >
      + {label}
    </button>
  )
}

export function Hint({ children }: { children: React.ReactNode }): React.ReactElement {
  return <p className="text-[11px] leading-snug" style={{ color: 'var(--color-muted-foreground)' }}>{children}</p>
}
