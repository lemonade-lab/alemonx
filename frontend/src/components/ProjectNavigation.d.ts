import type { ReactNode } from 'react'

export type ProjectNavigationItem = {
  id: string
  label: string
  icon: ReactNode
  onSelect: () => void
  loading?: boolean
  children?: Array<{
    id: string
    label: string
    active: boolean
    onSelect: () => void
  }>
}

export type ProjectNavigationProps = {
  items: ProjectNavigationItem[]
  activeID: string | null
  children?: ReactNode
}
