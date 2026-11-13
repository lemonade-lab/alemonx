// Baseline compatibility layer for older embedded browsers/WebViews. Import it
// before React and application modules so standard APIs are available during
// initial render. Web Crypto remains intentionally outside core-js; callers
// that need it keep their own secure fallback.
import 'core-js/stable'
import './styles.css'
import { restoreDesktopPreferences } from './lib/desktopPreferences'

void restoreDesktopPreferences().then(() => import('./renderApp'))
