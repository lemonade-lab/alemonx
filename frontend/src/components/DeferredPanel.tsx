import { Suspense, type ReactNode } from 'react'
import { ErrorBoundary } from './ErrorBoundary'

export function DeferredPanel({
  children,
  label
}: {
  children: ReactNode
  label: string
}) {
  return (
    <ErrorBoundary>
      <Suspense
        fallback={
          <p className="p-4 text-sm" role="status">
            正在加载{label}…
          </p>
        }
      >
        {children}
      </Suspense>
    </ErrorBoundary>
  )
}
