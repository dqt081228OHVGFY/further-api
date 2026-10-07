import { useLayoutEffect, useRef, type ReactNode } from 'react'

import { enterNativeLayer, type InterruptedMotion } from './native-layer-motion'

export function NativeMotionLayer({ element, destination, children }: { element: HTMLElement; destination: string; children: ReactNode }) {
  const interrupted: InterruptedMotion = useRef(null)
  useLayoutEffect(() => enterNativeLayer(element, interrupted), [element, destination])
  return children
}
