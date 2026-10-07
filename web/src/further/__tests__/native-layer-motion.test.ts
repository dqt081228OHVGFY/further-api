import { afterEach, describe, expect, it, vi } from 'vitest'
import { enterNativeLayer } from '../native-layer-motion'

afterEach(() => document.body.replaceChildren())
describe('native page entry ownership', () => {
  it('does not replay the first native arrival after its shell transition has already completed', async () => {
    const panel = document.createElement('section'); panel.className = 'view active'; panel.dataset.shellMotionOwner = 'true'
    const host = document.createElement('div'); panel.append(host); document.body.append(panel)
    const element = document.createElement('div'); host.attachShadow({ mode: 'open' }).append(element)
    const animate = vi.fn(() => ({ finished: Promise.resolve(), playState: 'finished', cancel: vi.fn() } as unknown as Animation))
    element.animate = animate
    const interrupted = { current: null }
    const stop = enterNativeLayer(element, interrupted)
    await Promise.resolve()
    expect(animate).not.toHaveBeenCalled()
    stop()
    enterNativeLayer(element, interrupted)
    expect(animate).toHaveBeenCalledOnce()
  })
})
