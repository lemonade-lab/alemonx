import cn from 'classnames'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ComponentProps } from 'react'
import type { highlightFileSource } from '../lib/highlightFileSource'

type Props = Pick<ComponentProps<'textarea'>,
  'aria-invalid' | 'aria-label' | 'className' | 'hidden' | 'onBlur' | 'placeholder' | 'readOnly' | 'wrap'
> & {
  file: string
  content: string
  containerClassName?: string
  onChange: (content: string) => void
}

export function SourceTextarea({
  file, content, hidden, className, containerClassName, wrap = 'soft', onChange, ...props
}: Props) {
  const [highlighter, setHighlighter] = useState<typeof highlightFileSource>()
  const [composing, setComposing] = useState(false)
  const editor = useRef<HTMLTextAreaElement>(null)
  const viewport = useRef<HTMLDivElement>(null)
  const paint = useRef<HTMLPreElement>(null)
  const markup = useMemo(
    () => hidden || composing || !content ? null : highlighter?.(file, content) ?? null,
    [file, content, hidden, composing, highlighter]
  )
  const highlighted = markup !== null

  useEffect(() => {
    if (hidden) return
    let active = true
    void import('../lib/highlightFileSource').then(
      module => { if (active) setHighlighter(() => module.highlightFileSource) },
      () => { /* Keep the native text editor if the optional highlighter cannot load. */ }
    )
    return () => { active = false }
  }, [hidden])

  function syncScroll() {
    if (!editor.current || !paint.current) return
    const { scrollLeft, scrollTop } = editor.current
    paint.current.style.translate = `${-scrollLeft}px ${-scrollTop}px`
  }

  useLayoutEffect(() => {
    const input = editor.current
    const frame = viewport.current
    const source = paint.current
    if (!highlighted || !input || !frame || !source) return

    const syncGeometry = () => {
      const style = getComputedStyle(input)
      Object.assign(frame.style, {
        left: `${input.offsetLeft + input.clientLeft}px`,
        top: `${input.offsetTop + input.clientTop}px`,
        width: `${input.clientWidth}px`,
        height: `${input.clientHeight}px`
      })
      Object.assign(source.style, {
        fontFamily: style.fontFamily,
        fontSize: style.fontSize,
        fontWeight: style.fontWeight,
        fontVariantLigatures: style.fontVariantLigatures,
        lineHeight: style.lineHeight,
        letterSpacing: style.letterSpacing,
        padding: style.padding,
        tabSize: style.tabSize,
        whiteSpace: style.whiteSpace,
        overflowWrap: style.overflowWrap,
        wordBreak: style.wordBreak
      })
      syncScroll()
    }

    syncGeometry()
    const observer = new ResizeObserver(syncGeometry)
    observer.observe(input)
    return () => observer.disconnect()
  }, [highlighted, wrap])

  useLayoutEffect(syncScroll, [highlighted, content])

  return (
    <div hidden={hidden} className={cn('relative flex min-h-0 min-w-0', hidden && 'hidden', containerClassName)}>
      <textarea
        {...props}
        ref={editor}
        className={cn('min-w-0 max-w-full', className, highlighted && 'source-textarea-highlighted')}
        spellCheck={false}
        wrap={wrap}
        value={content}
        onChange={event => onChange(event.currentTarget.value)}
        onCompositionStart={() => setComposing(true)}
        onCompositionEnd={() => setComposing(false)}
        onScroll={syncScroll}
      />
      {highlighted && (
        <div ref={viewport} aria-hidden="true" className="pointer-events-none absolute overflow-hidden">
          <pre ref={paint} className="source-textarea-paint m-0" dangerouslySetInnerHTML={{ __html: `${markup}\n` }} />
        </div>
      )}
    </div>
  )
}
