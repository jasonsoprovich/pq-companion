/**
 * overlayTextStyle — single source of truth for how trigger overlay_text
 * alerts are styled. Resolution order for every field:
 *
 *   per-action override  →  global default (Settings → Preferences)  →  built-in
 *
 * Built-ins reproduce the pre-customization look exactly (white text, glow
 * derived from the text color, system-ui font, 20px), so triggers and
 * configs that predate the style fields render unchanged.
 *
 * Used by the overlay renderer (TriggerOverlayWindowPage), the trigger
 * action editor, and the Settings live preview — keep them in sync by
 * changing only this file.
 */

/** The per-action style fields of an overlay_text Action (all optional). */
export interface OverlayTextStyleOverride {
  color?: string
  glow_color?: string
  font_family?: string
  font_size?: number
}

/** The global-default style fields of Preferences (all optional). */
export interface OverlayTextStyleDefaults {
  default_overlay_text_color?: string
  default_overlay_glow_color?: string
  default_overlay_font_family?: string
  default_overlay_font_size?: number
}

export interface ResolvedOverlayTextStyle {
  color: string
  /** 6-digit hex; the renderer applies its own glow alpha. */
  glowColor: string
  /** Empty string = no override (inherit the window's system-ui stack). */
  fontFamily: string
  fontSize: number
}

export const BUILTIN_TEXT_COLOR = '#ffffff'
export const BUILTIN_FONT_SIZE = 20

/**
 * Fonts that ship with every stock Windows 10/11 install. Offering only
 * these (rather than a free-text font field) guarantees the overlay never
 * references a font the user doesn't have. Most also exist on macOS, which
 * keeps dev parity reasonable.
 */
export const WINDOWS_SAFE_FONTS: readonly string[] = [
  'Arial',
  'Arial Black',
  'Bahnschrift',
  'Calibri',
  'Cambria',
  'Candara',
  'Comic Sans MS',
  'Consolas',
  'Constantia',
  'Corbel',
  'Courier New',
  'Franklin Gothic Medium',
  'Gabriola',
  'Georgia',
  'Impact',
  'Lucida Console',
  'Segoe UI',
  'Tahoma',
  'Times New Roman',
  'Trebuchet MS',
  'Verdana',
]

export function resolveOverlayTextStyle(
  action: OverlayTextStyleOverride | null | undefined,
  defaults: OverlayTextStyleDefaults | null | undefined,
): ResolvedOverlayTextStyle {
  const color = action?.color || defaults?.default_overlay_text_color || BUILTIN_TEXT_COLOR
  // Glow falls back to the text color itself — that reproduces the original
  // hard-coded `${color}aa` halo for un-customized alerts.
  const glowColor = action?.glow_color || defaults?.default_overlay_glow_color || color
  const fontFamily = action?.font_family || defaults?.default_overlay_font_family || ''
  const actionSize = action?.font_size && action.font_size > 0 ? action.font_size : 0
  const defaultSize =
    defaults?.default_overlay_font_size && defaults.default_overlay_font_size > 0
      ? defaults.default_overlay_font_size
      : 0
  const fontSize = actionSize || defaultSize || BUILTIN_FONT_SIZE
  return { color, glowColor, fontFamily, fontSize }
}

/**
 * The overlay alert text-shadow: a colored halo plus two dark shadows for
 * contrast against any game background. `glowColor` must be a 6-digit hex
 * (color inputs only produce those); the `aa` suffix is the halo alpha.
 */
export function overlayTextShadow(glowColor: string): string {
  return `0 0 8px ${glowColor}aa, 0 0 3px rgba(0,0,0,0.95), 0 1px 2px rgba(0,0,0,0.95)`
}

/** CSS font-family value with a safe fallback stack; undefined = inherit. */
export function overlayFontFamilyCSS(fontFamily: string): string | undefined {
  return fontFamily ? `'${fontFamily}', system-ui, sans-serif` : undefined
}
