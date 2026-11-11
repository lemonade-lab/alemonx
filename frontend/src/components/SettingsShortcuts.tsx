import { ChevronRight } from 'lucide-react'
import { useDependencySourcesQuery } from '../store/workspaceApi'
import { Button } from './Button'
import { settingsSections, type SettingsSection } from './settingsSections'

export function SettingsShortcuts({
  onSelect
}: {
  onSelect: (section: SettingsSection) => void
}) {
  const { data } = useDependencySourcesQuery()
  return settingsSections
    .filter(item => item.id !== 'dependency-sources' || data?.supported)
    .map(({ id, label, icon: Icon }) => (
      <Button
        key={id}
        variant="secondary"
        className="workbench-action-item"
        onClick={() => onSelect(id)}
      >
        <Icon className="size-4" aria-hidden="true" />
        {label}
        <ChevronRight className="ml-auto size-3.5" aria-hidden="true" />
      </Button>
    ))
}
