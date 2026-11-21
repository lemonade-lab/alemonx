import { lazy, type ComponentProps } from 'react'
import type Markdown from 'markdown-to-jsx'
import { DeferredPanel } from './DeferredPanel'

const MarkdownContent = lazy(() => import('markdown-to-jsx'))

export function DeferredMarkdown(props: ComponentProps<typeof Markdown>) {
  return (
    <DeferredPanel label="文档">
      <MarkdownContent {...props} />
    </DeferredPanel>
  )
}
