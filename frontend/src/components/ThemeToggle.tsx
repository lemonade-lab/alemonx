import { Moon, Sun } from 'lucide-react'
import { useSyncExternalStore } from 'react'
import { Button } from './Button'

type Theme = 'light' | 'dark'
const key = 'alemonjs-theme'

function preferredTheme(): Theme {
  const saved = window.localStorage.getItem(key)
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

const themeEvent = 'alx:theme-changed'

function subscribeToTheme(onChange: () => void) {
  window.addEventListener(themeEvent, onChange)
  return () => window.removeEventListener(themeEvent, onChange)
}

function setTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme
  document.documentElement.style.colorScheme = theme
  window.localStorage.setItem(key, theme)
  window.dispatchEvent(new Event(themeEvent))
}

export function ThemeToggle({
  menuItem = false,
  embedded = false
}: {
  menuItem?: boolean
  embedded?: boolean
}) {
  const theme = useSyncExternalStore(subscribeToTheme, preferredTheme)
  const next = theme === 'dark' ? 'light' : 'dark'
  return (
    <Button
      variant={embedded ? 'secondary' : 'icon'}
      className={menuItem ? 'workbench-action-item' : undefined}
      onClick={() => setTheme(next)}
      aria-label={`切换到${next === 'dark' ? '暗色' : '亮色'}主题`}
      title={`切换到${next === 'dark' ? '暗色' : '亮色'}主题`}
    >
      {theme === 'dark' ? (
        <Sun className="size-4" />
      ) : (
        <Moon className="size-4" />
      )}
      {(menuItem || embedded) && (
        <span>{`切换到${next === 'dark' ? '暗色' : '亮色'}主题`}</span>
      )}
    </Button>
  )
}
