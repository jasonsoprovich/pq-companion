/**
 * Search box matching for lists filtered in the browser, with the same rules
 * as the backend's searches (backend/internal/likequery): a case-insensitive
 * substring match where * is any run of characters (spaces next to it
 * included, so "Spirit * Wolf" finds "Spirit Wolf") and ? is one character.
 * Everything else, regex characters included, matches literally.
 */

const STAR_SPACES = /\s*\*\s*/g

export function hasWildcard(query: string): boolean {
  return /[*?]/.test(query)
}

// The regex source for a lowercased query with wildcards.
function wildcardSource(q: string): string {
  return Array.from(q, (ch) =>
    ch === '*' ? '.*' : ch === '?' ? '.' : ch.replace(/[.+^${}()|[\]\\]/g, '\\$&'),
  ).join('')
}

type Matcher = (text: string | null | undefined) => boolean

// Filters call textMatcher(q)(text) once per row; the cache keeps that from
// rebuilding the regex each time.
const cache = new Map<string, Matcher>()
const CACHE_MAX = 50

// textMatcher(query) returns a test for one piece of text. An empty or
// whitespace-only query matches everything.
export function textMatcher(query: string): Matcher {
  let m = cache.get(query)
  if (!m) {
    if (cache.size >= CACHE_MAX) cache.clear()
    m = buildMatcher(query)
    cache.set(query, m)
  }
  return m
}

// matchesSearch is textMatcher for a single test.
export function matchesSearch(text: string | null | undefined, query: string): boolean {
  return textMatcher(query)(text)
}

function buildMatcher(query: string): Matcher {
  const q = query.trim().replace(STAR_SPACES, '*').toLowerCase()
  if (!q) return () => true
  if (!hasWildcard(q)) return (text) => !!text && text.toLowerCase().includes(q)
  const source = wildcardSource(q)
  const re = new RegExp(source, 'is')
  return (text) => !!text && re.test(text)
}

// Shown as a search box's title so the wildcards can be discovered.
export const WILDCARD_HINT = 'Use * for any text and ? for one letter, e.g. Elemental * Mold'

// searchRanges lists the [start, end) spans of text a query matches, for
// highlighting. A * span runs as far as it can within the text.
export function searchRanges(text: string, query: string): Array<[number, number]> {
  const q = query.trim().replace(STAR_SPACES, '*').toLowerCase()
  const out: Array<[number, number]> = []
  if (!q) return out
  if (!hasWildcard(q)) {
    const lower = text.toLowerCase()
    for (let i = lower.indexOf(q); i !== -1; i = lower.indexOf(q, i + q.length)) out.push([i, i + q.length])
    return out
  }
  const source = wildcardSource(q)
  for (const m of text.matchAll(new RegExp(source, 'gis'))) {
    if (m[0].length > 0) out.push([m.index ?? 0, (m.index ?? 0) + m[0].length])
  }
  return out
}
