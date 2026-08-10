// Window drag is only allowed from inert header chrome. This deliberately
// includes semantic popovers so a long press in a dropdown never becomes a
// window drag after the pointer event bubbles to the header.
export function isWindowHeaderInteractiveTarget(target: EventTarget | null) {
  // Button icons are SVGElement instances, not HTMLElement instances. Treat
  // every DOM Element consistently so a click on an icon cannot fall through
  // to the header drag handler.
  if (!(target instanceof Element)) return false
  return Boolean(
    target.closest(
      'button, a, input, select, textarea, [contenteditable="true"], [role="button"], [role="menu"], [role="menuitem"], [role="dialog"], [aria-haspopup], [aria-expanded], [data-window-interactive]'
    )
  )
}
