/**
 * HelpMarkdown — renders a docs/help article body.
 *
 * Link schemes:
 *   help:<slug>[#anchor]  another help article (calls onOpenArticle)
 *   app:/<path>           a page in the app (router navigation)
 *   http(s)://…           opened in the system browser
 *
 * Also renders <kbd>…</kbd> (kept as raw HTML in the shared source so the
 * website shows it too) and ```flow fences ("A | B | C") as a step diagram.
 */
import React, { useMemo } from 'react'
import Markdown, { defaultUrlTransform, type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { useNavigate } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'

// Marks inline code that came from <kbd>; a private-use character can't
// appear in real article text.
const KBD = ''

export function headingId(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
}

function textOf(node: React.ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textOf).join('')
  if (React.isValidElement<{ children?: React.ReactNode }>(node)) return textOf(node.props.children)
  return ''
}

function FlowDiagram({ src }: { src: string }): React.ReactElement {
  const steps = src.split('|').map((s) => s.trim()).filter(Boolean)
  return (
    <div className="my-4 flex flex-wrap items-center gap-2">
      {steps.map((s, i) => (
        <React.Fragment key={i}>
          <span
            className="rounded px-3 py-1.5 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
          >
            {s}
          </span>
          {i < steps.length - 1 && (
            <ChevronRight size={14} style={{ color: 'var(--color-primary)' }} />
          )}
        </React.Fragment>
      ))}
    </div>
  )
}

const kbdStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-primary)',
}

export default function HelpMarkdown({
  body,
  onOpenArticle,
}: {
  body: string
  onOpenArticle: (slug: string, anchor?: string) => void
}): React.ReactElement {
  const navigate = useNavigate()
  const source = useMemo(
    () => body.replace(/<kbd>([^<]*)<\/kbd>/g, (_, k: string) => `\`${KBD}${k}\``),
    [body],
  )

  const components = useMemo<Components>(() => {
    const heading =
      (Tag: 'h2' | 'h3' | 'h4', cls: string) =>
      ({ children }: { children?: React.ReactNode }) => (
        <Tag id={headingId(textOf(children))} className={cls} style={{ color: 'var(--color-foreground)' }}>
          {children}
        </Tag>
      )
    return {
      h1: heading('h2', 'mt-6 mb-2 text-lg font-semibold'),
      h2: heading('h2', 'mt-6 mb-2 text-lg font-semibold'),
      h3: heading('h3', 'mt-5 mb-1.5 text-base font-semibold'),
      h4: heading('h4', 'mt-4 mb-1 text-sm font-semibold'),
      p: ({ children }) => <p className="my-2.5 leading-relaxed">{children}</p>,
      ul: ({ children }) => <ul className="my-2.5 list-disc space-y-1 pl-5">{children}</ul>,
      ol: ({ children }) => <ol className="my-2.5 list-decimal space-y-1 pl-5">{children}</ol>,
      strong: ({ children }) => (
        <strong className="font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {children}
        </strong>
      ),
      blockquote: ({ children }) => (
        <blockquote
          className="my-3 rounded border-l-2 px-3 py-1"
          style={{ borderColor: 'var(--color-primary)', backgroundColor: 'var(--color-surface-2)' }}
        >
          {children}
        </blockquote>
      ),
      table: ({ children }) => (
        <div className="my-3 overflow-x-auto">
          <table className="w-full border-collapse text-sm">{children}</table>
        </div>
      ),
      th: ({ children }) => (
        <th
          className="border-b px-2 py-1.5 text-left font-semibold"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-foreground)' }}
        >
          {children}
        </th>
      ),
      td: ({ children }) => (
        <td className="border-b px-2 py-1.5" style={{ borderColor: 'var(--color-border)' }}>
          {children}
        </td>
      ),
      pre: ({ children }) => <>{children}</>,
      code: ({ className, children }) => {
        const text = textOf(children)
        if (className === 'language-flow') return <FlowDiagram src={text} />
        if (text.startsWith(KBD)) {
          return (
            <kbd className="rounded px-1.5 py-0.5 text-xs" style={kbdStyle}>
              {text.slice(1)}
            </kbd>
          )
        }
        if (className) {
          return (
            <pre
              className="my-3 overflow-x-auto rounded p-3 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
            >
              <code>{text}</code>
            </pre>
          )
        }
        return (
          <code
            className="rounded px-1 py-0.5 text-[0.85em]"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-primary)' }}
          >
            {children}
          </code>
        )
      },
      a: ({ href = '', children }) => {
        const linkStyle = { color: 'var(--color-primary)', cursor: 'pointer' }
        if (href.startsWith('help:')) {
          const [slug, anchor] = href.slice(5).split('#')
          return (
            <a
              href="#"
              style={linkStyle}
              onClick={(e) => {
                e.preventDefault()
                onOpenArticle(slug, anchor)
              }}
            >
              {children}
            </a>
          )
        }
        if (href.startsWith('app:')) {
          return (
            <a
              href="#"
              style={linkStyle}
              onClick={(e) => {
                e.preventDefault()
                navigate(href.slice(4))
              }}
            >
              {children}
            </a>
          )
        }
        return (
          <a href={href} target="_blank" rel="noreferrer" style={linkStyle}>
            {children}
          </a>
        )
      },
    }
  }, [navigate, onOpenArticle])

  return (
    <div className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
      <Markdown
        remarkPlugins={[remarkGfm]}
        components={components}
        urlTransform={(url, key) =>
          key === 'href' && /^(help|app):/.test(url) ? url : defaultUrlTransform(url)
        }
      >
        {source}
      </Markdown>
    </div>
  )
}
