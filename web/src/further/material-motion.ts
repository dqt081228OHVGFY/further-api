export const MATERIAL_MOTION = {
  enter: 220,
  exit: 140,
  standard: 'cubic-bezier(.2,.8,.2,1)',
  decelerate: 'cubic-bezier(.2,.8,.2,1)',
  accelerate: 'cubic-bezier(.4,0,1,1)',
} as const

// ShadowRoot events keep their actual target, so the same Material feedback
// reaches native workspaces without replacing the existing page animations.
export function mountNativeMaterialFeedback(root: ShadowRoot): () => void {
  const ripples = new Map<HTMLElement, ReturnType<typeof setTimeout>>()
  const create = (target: EventTarget | null, x?: number, y?: number) => {
    if (!(target instanceof Element)) return
    const control = target.closest<HTMLElement>('button, a.rfw-primary, a.rfw-secondary')
    if (!control || !root.contains(control) || control.matches(':disabled, [aria-disabled="true"]')) return
    const bounds = control.getBoundingClientRect()
    if (!bounds.width || !bounds.height) return
    const horizontal = x === undefined ? bounds.width / 2 : Math.max(0, Math.min(x - bounds.left, bounds.width))
    const vertical = y === undefined ? bounds.height / 2 : Math.max(0, Math.min(y - bounds.top, bounds.height))
    const size = Math.hypot(Math.max(horizontal, bounds.width - horizontal), Math.max(vertical, bounds.height - vertical)) * 2
    const ripple = document.createElement('span')
    ripple.className = 'rfw-material-ripple'
    ripple.setAttribute('aria-hidden', 'true')
    Object.assign(ripple.style, { width: `${size}px`, height: `${size}px`, left: `${horizontal - size / 2}px`, top: `${vertical - size / 2}px` })
    control.classList.add('rfw-material-control')
    control.append(ripple)
    const remove = () => { clearTimeout(ripples.get(ripple)); ripples.delete(ripple); ripple.remove() }
    ripple.addEventListener('animationend', remove, { once: true })
    ripples.set(ripple, setTimeout(remove, 700))
  }
  const pointer = (event: Event) => {
    const point = event as PointerEvent
    if (point.button === 0 && point.isPrimary !== false) create(point.target, point.clientX, point.clientY)
  }
  const keyboard = (event: Event) => {
    const click = event as MouseEvent
    if (click.isTrusted && click.detail === 0) create(click.target)
  }
  root.addEventListener('pointerdown', pointer)
  root.addEventListener('click', keyboard)
  return () => {
    root.removeEventListener('pointerdown', pointer)
    root.removeEventListener('click', keyboard)
    for (const [ripple, timer] of ripples) { clearTimeout(timer); ripple.remove() }
    ripples.clear()
  }
}
