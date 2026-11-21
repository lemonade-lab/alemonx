import Markdown, { sanitizer } from 'markdown-to-jsx'
import type { ReactNode } from 'react'

export function ExplorerMarkdownPreview({ content }: { content: string }) {
  return (
    <article
      aria-label="Markdown 预览"
      tabIndex={0}
      className="markdown-page explorer-markdown-preview min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain"
    >
      <Markdown
        options={{
          disableParsingRawHTML: true,
          forceBlock: true,
          sanitizer(value, tag) {
            const url = sanitizer(value)
            if (!url) return null
            if (/^https?:\/\//i.test(url)) return url
            if (tag === 'a' && /^(#|mailto:)/i.test(url)) return url
            return null
          },
          overrides: {
            a: {
              component: ({ href, children }: { href?: string; children?: ReactNode }) => (
                href?.startsWith('#')
                  ? <a href={href}>{children}</a>
                  : <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>
              )
            },
            img: {
              props: {
                loading: 'lazy',
                decoding: 'async',
                referrerPolicy: 'no-referrer'
              }
            }
          }
        }}
      >
        {content}
      </Markdown>
    </article>
  )
}
