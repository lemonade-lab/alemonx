import cn from 'classnames'
import { RefreshCw } from 'lucide-react'
import type { ComponentProps } from 'react'
import { Button } from './Button'

type RefreshButtonProps = Omit<
  ComponentProps<typeof Button>,
  'aria-label' | 'children' | 'loading' | 'loadingLabel' | 'size' | 'variant'
> & {
  label: string
  busy?: boolean
}

export function RefreshButton({
  label,
  title = label,
  busy = false,
  disabled,
  className,
  ...props
}: RefreshButtonProps) {
  return (
    <Button
      type="button"
      {...props}
      variant="icon"
      className={cn('shrink-0', className)}
      aria-label={label}
      title={title}
      aria-busy={busy || undefined}
      disabled={disabled || busy}
    >
      <RefreshCw className={cn('size-4', busy && 'animate-spin')} aria-hidden />
    </Button>
  )
}
