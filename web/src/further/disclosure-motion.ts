import { MATERIAL_MOTION } from './material-motion'

export function mountDisclosureMotion(root: Document | ShadowRoot): () => void {
  const transitions = new Map<HTMLDetailsElement, { desired: boolean; animations: Animation[]; restore: () => void }>()
  const activate = (event: Event) => {
    if (event.defaultPrevented || !(event.target instanceof Element)) return
    if ((event as MouseEvent).button !== 0) return
    const summary = event.target.closest('summary')
    const details = summary?.parentElement
    if (!(details instanceof HTMLDetailsElement) || !root.contains(details) || details.matches('.activity-history,[data-disclosure-owner]')) return
    if (event.target.closest('a,button,input,select,textarea,[contenteditable=true]')) return
    if (typeof details.animate !== 'function') return
    event.preventDefault()
    const previous = transitions.get(details)
    const desired = previous ? !previous.desired : !details.open
    const start = details.getBoundingClientRect().height
    const contents = [...details.children].filter((child): child is HTMLElement => child instanceof HTMLElement && child !== summary && !child.hidden && !child.classList.contains('hidden'))
    const snapshots = contents.map(child => {
      const current = previous ? getComputedStyle(child) : null
      return { opacity: current?.opacity || (desired ? '0' : '1'), translate: current?.translate || (desired ? '0 -3px' : '0 0') }
    })
    previous?.animations.forEach(animation => animation.cancel())
    previous?.restore()
    const oldOverflow = details.style.overflow
    const attributes = contents.map(child => ({ child, inert: child.hasAttribute('inert'), hidden: child.getAttribute('aria-hidden') }))
    if (!desired && contents.some(child => child.contains(document.activeElement))) summary?.focus({ preventScroll: true })
    details.open = desired
    const end = details.getBoundingClientRect().height
    details.open = true
    summary?.setAttribute('aria-expanded', String(desired))
    details.style.overflow = 'clip'
    details.dataset.disclosureState = desired ? 'opening' : 'closing'
    details.dataset.disclosureMotionEngine = 'web'
    const options = { duration: desired ? MATERIAL_MOTION.enter : MATERIAL_MOTION.exit, easing: desired ? MATERIAL_MOTION.decelerate : MATERIAL_MOTION.accelerate, fill: 'forwards' as const }
    const animations = [details.animate([{ height: `${start}px` }, { height: `${end}px` }], options)]
    contents.forEach((child, index) => {
      child.toggleAttribute('inert', !desired || attributes[index].inert)
      if (!desired) child.setAttribute('aria-hidden', 'true')
      if (typeof child.animate === 'function') animations.push(child.animate([snapshots[index], { opacity: desired ? 1 : 0, translate: desired ? '0 0' : '0 -3px' }], options))
    })
    const restore = () => {
      details.style.overflow = oldOverflow
      delete details.dataset.disclosureState
      delete details.dataset.disclosureMotionEngine
      for (const { child, inert, hidden } of attributes) {
        child.toggleAttribute('inert', inert)
        if (hidden === null) child.removeAttribute('aria-hidden')
        else child.setAttribute('aria-hidden', hidden)
      }
    }
    const transition = { desired, animations, restore }
    transitions.set(details, transition)
    const finish = () => {
      if (transitions.get(details) !== transition) return
      transitions.delete(details)
      details.open = desired
      restore()
      animations.forEach(animation => animation.cancel())
    }
    void Promise.all(animations.map(animation => animation.finished)).then(finish).catch(() => {})
  }
  root.addEventListener('click', activate)
  return () => {
    root.removeEventListener('click', activate)
    for (const [details, transition] of transitions) {
      details.open = transition.desired
      transition.animations.forEach(animation => animation.cancel())
      transition.restore()
    }
    transitions.clear()
  }
}
