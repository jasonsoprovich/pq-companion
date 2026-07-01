/**
 * DecimalInput — numeric input for fractional seconds fields that accepts
 * either "." or "," as the decimal separator.
 *
 * A plain <input type="number"> silently rejects the "," keystroke in most
 * locales, so users whose keyboards/habits use a comma decimal separator
 * (much of Europe, South America, ...) couldn't type fractional values at
 * all. This renders a text input (inputMode="decimal" keeps the numeric
 * soft keyboard) and normalizes a comma to a period before parsing.
 *
 * Controlled like the number inputs it replaces: `value` is the parent's
 * numeric state, `onValue` fires with the parsed + clamped number on every
 * keystroke. The raw text lives in local state while focused so partial
 * entries like "1," survive re-renders; on blur the display snaps to the
 * canonical stored value.
 */
import React, { useEffect, useRef, useState } from 'react'

type DecimalInputProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  'value' | 'onChange' | 'type' | 'min' | 'max'
> & {
  value: number
  onValue: (v: number) => void
  min?: number
  max?: number
  /** Committed when the field is empty or unparseable (clamped). Default 0. */
  fallback?: number
}

/** parseFloat that also accepts a comma as the decimal separator. */
export function parseDecimal(raw: string): number {
  return parseFloat(raw.trim().replace(',', '.'))
}

export default function DecimalInput({
  value,
  onValue,
  min,
  max,
  fallback = 0,
  ...rest
}: DecimalInputProps) {
  const [text, setText] = useState(String(value))
  const focused = useRef(false)

  // Reflect external changes (loading a different trigger, reset buttons)
  // while the field isn't being edited.
  useEffect(() => {
    if (!focused.current) setText(String(value))
  }, [value])

  const clamp = (n: number) => {
    if (min !== undefined && n < min) n = min
    if (max !== undefined && n > max) n = max
    return n
  }

  return (
    <input
      {...rest}
      type="text"
      inputMode="decimal"
      value={text}
      onFocus={() => {
        focused.current = true
      }}
      onChange={(e) => {
        setText(e.target.value)
        const n = parseDecimal(e.target.value)
        onValue(clamp(Number.isFinite(n) ? n : fallback))
      }}
      onBlur={() => {
        focused.current = false
        setText(String(value))
      }}
    />
  )
}
