import cn from 'classnames'
import type { ReactNode } from 'react'

type Props = {
  className?: string
  header: ReactNode
  footer?: ReactNode
  children: ReactNode
  bodyToolbar?: ReactNode
  scrollBody?: boolean
  bodyKey?: string
}

/** A robot page always has sibling header and article regions. */
export function BotWorkspace({
  className = '',
  header,
  footer,
  children,
  bodyToolbar,
  scrollBody = false,
  bodyKey
}: Props) {
  const hasFooter = Boolean(footer)
  const splitBody = Boolean(bodyToolbar) || hasFooter
  return (
    <section
      data-workbench-page
      data-scroll-body={scrollBody || undefined}
      className={cn(
        'grid w-full min-w-0 items-start gap-3',
        hasFooter
          ? 'h-full min-h-0 grid-rows-[auto_minmax(0,1fr)_auto]'
          : scrollBody
            ? 'h-full min-h-0 grid-rows-[auto_minmax(0,1fr)]'
            : 'content-start',
        className
      )}
    >
      {header}
      <article
        className={cn(
          'grid min-w-0 gap-3',
          (scrollBody || hasFooter) && 'h-full min-h-0',
          splitBody && 'grid-rows-[auto_minmax(0,1fr)]',
          scrollBody && !splitBody && 'grid-rows-[minmax(0,1fr)]'
        )}
      >
        {scrollBody ? (
          <>
            {bodyToolbar}
            <div
              key={bodyKey}
              data-workbench-body
              tabIndex={0}
              className="grid min-h-0 min-w-0 content-start gap-3 overflow-auto overscroll-contain"
            >
              {children}
            </div>
          </>
        ) : children}
      </article>
      {footer && <div>{footer}</div>}
    </section>
  )
}
