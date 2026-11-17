// In-app help: the markdown articles in docs/help/ (shared with the
// website), bundled at build time so help works offline.
//
// Each file starts with a frontmatter block:
//   title, slug, section, order, routes (JSON array), keywords (JSON array)

// The renderer root is frontend/, so the glob is relative to this file.
const files = import.meta.glob('../../../docs/help/*.md', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

export interface HelpArticle {
  slug: string
  title: string
  section: string
  order: number
  routes: string[]
  keywords: string[]
  // Markdown body with the frontmatter and the leading "# Title" removed.
  body: string
  headings: string[]
  // Plain text (markdown stripped) for snippets, and its lower-cased copy
  // for matching. Same length, so indexes line up.
  plain: string
  text: string
}

// Table-of-contents order for sections; unknown sections sort last.
export const HELP_SECTIONS = [
  'Getting started',
  'Live & combat',
  'History',
  'Raids',
  'Character',
  'Progress & goals',
  'Database & tools',
]

function parseValue(raw: string): unknown {
  const v = raw.trim()
  if (v === '') return ''
  try {
    return JSON.parse(v)
  } catch {
    return v
  }
}

// parseArticle splits frontmatter from body. Returns null for a file
// without a slug, so a stray file can't break the help page.
export function parseArticle(src: string): HelpArticle | null {
  const m = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?([\s\S]*)$/.exec(src)
  if (!m) return null
  const meta: Record<string, unknown> = {}
  for (const line of m[1].split(/\r?\n/)) {
    const i = line.indexOf(':')
    if (i <= 0) continue
    meta[line.slice(0, i).trim()] = parseValue(line.slice(i + 1))
  }
  const slug = String(meta.slug ?? '')
  if (!slug) return null
  const body = m[2].replace(/^\s*#\s+[^\n]*\n/, '').trim()
  const headings = [...body.matchAll(/^#{2,4}\s+(.+)$/gm)].map((h) => h[1].trim())
  const plain = body
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/<\/?kbd>/g, '')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/^\s*\|?[\s:|-]+\|[\s:|-]*$/gm, ' ')
    .replace(/[*_`#>|]/g, ' ')
  const flat = `${meta.title ?? ''} ${plain}`.replace(/\s+/g, ' ')
  return {
    slug,
    title: String(meta.title ?? slug),
    section: String(meta.section ?? 'Other'),
    order: Number(meta.order ?? 999),
    routes: Array.isArray(meta.routes) ? meta.routes.map(String) : [],
    keywords: Array.isArray(meta.keywords) ? meta.keywords.map(String) : [],
    body,
    headings,
    plain: flat,
    text: flat.toLowerCase(),
  }
}

function sectionRank(s: string): number {
  const i = HELP_SECTIONS.indexOf(s)
  return i === -1 ? HELP_SECTIONS.length : i
}

export const HELP_ARTICLES: HelpArticle[] = Object.values(files)
  .map(parseArticle)
  .filter((a): a is HelpArticle => a !== null)
  .sort(
    (a, b) =>
      sectionRank(a.section) - sectionRank(b.section) ||
      a.order - b.order ||
      a.title.localeCompare(b.title),
  )

if (HELP_ARTICLES.length === 0) {
  // Surfaces a broken glob in dev; vite.config's check fails the build.
  console.warn('[help] no help articles were bundled from docs/help')
}

export function helpArticle(slug: string): HelpArticle | undefined {
  return HELP_ARTICLES.find((a) => a.slug === slug)
}

// articleForPath picks the article for a page: an exact route match, else
// the article with the longest route that prefixes the path.
export function articleForPath(pathname: string): HelpArticle | undefined {
  let best: HelpArticle | undefined
  let bestLen = 0
  for (const a of HELP_ARTICLES) {
    for (const r of a.routes) {
      if (r === pathname) return a
      if (pathname.startsWith(r + '/') && r.length > bestLen) {
        best = a
        bestLen = r.length
      }
    }
  }
  return best
}

export interface HelpHit {
  article: HelpArticle
  score: number
  // A short excerpt around the first body match, or '' if the match was
  // only in the title or keywords.
  snippet: string
}

function snippetFor(a: HelpArticle, term: string): string {
  const i = a.text.indexOf(term)
  if (i === -1) return ''
  const start = Math.max(0, i - 50)
  const end = Math.min(a.text.length, i + term.length + 70)
  return (start > 0 ? '…' : '') + a.plain.slice(start, end).trim() + (end < a.plain.length ? '…' : '')
}

// searchHelp ranks articles containing every query word. Title matches
// weigh most, then keywords, headings, and body occurrences.
export function searchHelp(query: string, limit = 20): HelpHit[] {
  const terms = query.toLowerCase().split(/\s+/).filter((t) => t.length > 1)
  if (terms.length === 0) return []
  const hits: HelpHit[] = []
  for (const a of HELP_ARTICLES) {
    const title = a.title.toLowerCase()
    const keys = a.keywords.join(' ').toLowerCase()
    const heads = a.headings.join(' ').toLowerCase()
    let score = 0
    let all = true
    for (const t of terms) {
      if (!a.text.includes(t) && !keys.includes(t)) {
        all = false
        break
      }
      if (title.includes(t)) score += title.startsWith(t) ? 14 : 10
      if (keys.includes(t)) score += 6
      if (heads.includes(t)) score += 4
      score += Math.min(5, a.text.split(t).length - 1)
    }
    if (!all) continue
    hits.push({ article: a, score, snippet: snippetFor(a, terms[0]) })
  }
  return hits.sort((x, y) => y.score - x.score).slice(0, limit)
}
