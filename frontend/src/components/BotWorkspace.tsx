import type { ReactNode } from 'react'

type Props = {
  className?: string
  header: ReactNode
  children: ReactNode
}

/** A robot page always has sibling header and article regions. */
export function BotWorkspace({ className = '', header, children }: Props) {
  return (
    <section
      className={`grid w-full min-w-0 content-start items-start gap-1 ${className}`.trim()}
    >
      {header}
      <article className="grid min-w-0 gap-4 px-4">{children}</article>
    </section>
  )
}
