import { MATERIAL_MOTION } from './material-motion'

export type InterruptedMotion = { current: { opacity: string; transform: string } | null }

export function enterNativeLayer(element: HTMLElement, interrupted: InterruptedMotion): () => void {
  let active = true
  const shadow = element.getRootNode()
  const host = shadow instanceof ShadowRoot ? shadow.host : null
  const parent = host?.closest<HTMLElement>('.view')
  const parentAnimations = parent?.getAnimations?.().filter(animation => animation.playState === 'running') || []
  const initialLayer = !element.dataset.nativeEntryMounted
  element.dataset.nativeEntryMounted = 'true'
  const finish = () => { if (active) delete element.dataset.nativeMotionRunning }
  if (parentAnimations.length || initialLayer && parent?.dataset.shellMotionOwner === 'true') {
    void Promise.allSettled(parentAnimations.map(animation => animation.finished)).then(finish).catch(finish)
    return () => { active = false; delete element.dataset.nativeMotionRunning }
  }
  if (typeof element.animate !== 'function') { finish(); return () => {} }
  element.dataset.nativeMotionRunning = 'true'
  const animation = element.animate([
    interrupted.current || { opacity: 0, transform: 'none' },
    { opacity: 1, transform: 'none' },
  ], { duration: MATERIAL_MOTION.enter, easing: MATERIAL_MOTION.decelerate, fill: 'none' })
  interrupted.current = null
  void animation.finished?.then(finish).catch(finish)
  return () => {
    active = false
    if (animation.playState === 'running') {
      const current = getComputedStyle(element)
      interrupted.current = { opacity: current.opacity, transform: current.transform }
    }
    animation.cancel()
    delete element.dataset.nativeMotionRunning
  }
}
