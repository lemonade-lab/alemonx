import cn from 'classnames'
import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { highlightFileSource } from './highlightFileSource'

export function ExplorerSourceInput({ file, content, hidden, readOnly, onChange }: {
  file: string
  content: string
  hidden: boolean
  readOnly?: boolean
  onChange: (content: string) => void
}) {
  const [composing, setComposing] = useState(false)
  const input = useRef<HTMLTextAreaElement>(null)
  const paint = useRef<HTMLPreElement>(null)
  const markup = useMemo(
    () => hidden || composing ? null : highlightFileSource(file, content),
    [file, content, hidden, composing]
  )
  const highlighted = markup !== null

  function syncScroll() {
    if (!input.current || !paint.current) return
    const { scrollLeft, scrollTop } = input.current
    paint.current.style.translate = `${-scrollLeft}px ${-scrollTop}px`
  }

  useLayoutEffect(syncScroll, [highlighted, content])

  return (
    <div hidden={hidden} className="relative flex min-h-0 min-w-0 flex-1 overflow-hidden bg-(--theme-surface-panel)">
      {highlighted && (
        <pre
          ref={paint}
          aria-hidden="true"
          className="explorer-source-paint pointer-events-none absolute inset-0 m-0 whitespace-pre p-4"
          dangerouslySetInnerHTML={{ __html: `${markup}\n` }}
        />
      )}
      <textarea
        ref={input}
        aria-label={`编辑 ${file}`}
        className={cn(
          'explorer-file-editor relative min-h-0 min-w-0 flex-1 resize-none overflow-auto bg-transparent p-4',
          highlighted && 'explorer-file-editor-highlighted'
        )}
        spellCheck={false}
        wrap="off"
        readOnly={readOnly}
        value={content}
        onChange={event => onChange(event.currentTarget.value)}
        onCompositionStart={() => setComposing(true)}
        onCompositionEnd={() => setComposing(false)}
        onScroll={syncScroll}
      />
    </div>
  )
}
