// Per-position badge colors for the CH Chain overlay's numeric indicator
// (1, 2, 3, 4…). Requested on Discord: the badge used a single flat blue for
// every position, which is hard to pick out against the moving countdown
// bars, especially once several rows share a near-identical color as their
// bar nears "landing" green. A distinct, repeating hue per chain position
// lets a raid leader track "the 3 spot" by color at a glance.
//
// Chosen to stay clear of the bar's own state palette (blue = counting down,
// green = landing soon, red = possible miss) so the badge color never gets
// confused for chain health.
const POSITION_BADGE_COLORS = [
  'rgba(139, 92, 246, 0.7)', // violet
  'rgba(236, 72, 153, 0.7)', // pink
  'rgba(245, 158, 11, 0.7)', // amber
  'rgba(20, 184, 166, 0.7)', // teal
  'rgba(99, 102, 241, 0.7)', // indigo
  'rgba(217, 70, 239, 0.7)', // fuchsia
  'rgba(249, 115, 22, 0.7)', // orange
  'rgba(6, 182, 212, 0.7)', // cyan
]

// chainPositionColor returns the badge background for a 1-based chain
// position, cycling through the palette for chains longer than its length.
export function chainPositionColor(position: number): string {
  if (!Number.isFinite(position) || position < 1) return POSITION_BADGE_COLORS[0]
  return POSITION_BADGE_COLORS[(position - 1) % POSITION_BADGE_COLORS.length]
}
