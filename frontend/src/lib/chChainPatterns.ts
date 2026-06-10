/**
 * Default CH-chain call patterns — mirrors the constants in
 * backend/internal/config/config.go (chChainPatternPrefix/Suffix and the
 * three Default* variants). Keep them in sync.
 *
 * The settings page uses these to auto-swap the primary pattern when the
 * secondary (ramp/split) chain is toggled: enabling it moves a still-default
 * primary from the catch-all to the numeric-only variant and fills an empty
 * secondary with the letters-only variant; disabling reverts a still-default
 * numeric primary back to the catch-all. Hand-customized patterns are never
 * touched.
 */

const PREFIX =
  `^(?P<caster>(You|[A-Z][a-z]{3,14})) (?:tells? (?:the (?:raid|group|guild)|your (party|raid|guild)|[A-Za-z]+(?:-[A-Za-z]+)+:\\d)|says out of character|shouts|auctions?),\\s+'[^a-zA-Z0-9]*\\b(?P<chainnum>`
const SUFFIX =
  `)[^a-zA-Z0-9]*\\b(?:CH|COMPLETE HEALING)\\b(?:[^a-zA-Z0-9]*(?:on|to)[^a-zA-Z0-9]*)?[^a-zA-Z0-9]*(?P<target>[A-Z][a-z]{3,14})\\b(.*)$`

// Single-chain catch-all: numeric (001) and letter (AAA) markers both feed
// the one main chain.
export const CH_CHAIN_DEFAULT_PATTERN = `${PREFIX}\\d{3,4}|[A-Za-z]{3,4}${SUFFIX}`

// Numeric-only markers (001 002 …) — the primary chain when split.
export const CH_CHAIN_NUMERIC_PATTERN = `${PREFIX}\\d{3,4}${SUFFIX}`

// Letter-only markers (AAA BBB …) — the secondary ramp/split chain.
export const CH_CHAIN_SECONDARY_PATTERN = `${PREFIX}[A-Za-z]{3,4}${SUFFIX}`
