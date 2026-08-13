import type { ReactNode } from 'react'
import { BotWorkspace } from './BotWorkspace'
import {
  RobotPanelHeader,
  type RobotPanelHeaderProps
} from './RobotPanelHeader'

type Props = Omit<RobotPanelHeaderProps, 'className'> & {
  children: ReactNode
  /** Controls the page canvas width and layout. */
  className?: string
  /** Adds a page-specific class to the shared header when necessary. */
  headerClassName?: string
}

/**
 * Standard robot workbench page: the shared canvas and its matching header.
 * Feature panels supply only content plus their visible page metadata.
 */
export function RobotPanel({
  children,
  className,
  headerClassName,
  ...header
}: Props) {
  return (
    <BotWorkspace
      className={[
        'max-w-190 [container-name:robot-panel] [container-type:inline-size]',
        className
      ]
        .filter(Boolean)
        .join(' ')}
      header={<RobotPanelHeader {...header} className={headerClassName} />}
    >
      {children}
    </BotWorkspace>
  )
}
