import type { ReactNode } from 'react'

type Props = {
  className?: string
  header: ReactNode
  footer?: ReactNode
  children: ReactNode
}

/** A robot page always has sibling header and article regions. */
export function BotWorkspace({ className = '', header, footer, children }: Props) {
  return (
    <section
      data-workbench-page
      className={`grid w-full min-w-0 items-start gap-1 ${footer ? 'h-full min-h-0 grid-rows-[auto_minmax(0,1fr)_auto]' : 'content-start'} ${className}`.trim()}
    >
      {header}
      <article className={`grid min-w-0 gap-4 px-4 ${footer ? 'h-full min-h-0 grid-rows-[auto_minmax(0,1fr)]' : ''}`}>
        {children}
      </article>
      {footer && <div className="px-4 pt-2 pb-4">{footer}</div>}
    </section>
  )
}
