// Window drag is only allowed from inert header chrome. Interactive controls
// (buttons, inputs, menus) never start a drag, and neither does anything inside
// a popover the header owns — a long press there must stay a text selection,
// not a window drag.
//
// A popover counts as the header's own only when it is a *descendant* of the
// header chrome: the app's popovers (topbar-popover etc.) are absolutely
// positioned in-DOM children of the header, so their pointer events bubble into
// the header's drag handler and need blocking. The window shell is itself a
// role="dialog" too, but it is an *ancestor* of the header, so it must not be
// treated as an interactive target — otherwise the header could never be
// dragged at all.
export function isWindowHeaderInteractiveTarget(target: EventTarget | null) {
  // Button icons are SVGElement instances, not HTMLElement instances. Treat
  // every DOM Element consistently so a click on an icon cannot fall through
  // to the header drag handler.
  if (!(target instanceof Element)) return false
  if (
    target.closest(
      'button, a, input, select, textarea, [contenteditable="true"], [role="button"], [role="menu"], [role="menuitem"], [aria-haspopup], [aria-expanded], [data-window-interactive]'
    )
  )
    return true
  const dialog = target.closest('[role="dialog"]')
  if (!dialog) return false
  const chrome = target.closest('.floating-window-header, .topbar')
  return chrome !== null && chrome.contains(dialog)
}
