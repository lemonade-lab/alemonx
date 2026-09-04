/**
 * Viewport breakpoints describe application navigation and window chrome.
 * Component internals should prefer their named CSS container query instead
 * so a narrow desktop window receives the same compact layout as a phone.
 */
export const PHONE_VIEWPORT_QUERY = '(max-width: 700px)'
export const COMPACT_COMPONENT_QUERY = '(max-width: 759px)'
export const WORKBENCH_NAVIGATION_QUERY = '(max-width: 940px)'
export const TABLET_WINDOW_QUERY = '(max-width: 1024px)'
