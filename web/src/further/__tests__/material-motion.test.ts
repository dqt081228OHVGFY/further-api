import { afterEach, describe, expect, it, vi } from 'vitest'
import { mountNativeMaterialFeedback } from '../material-motion'

afterEach(() => { delete document.documentElement.dataset.furtherMotion; delete document.documentElement.dataset.reducedMotion; vi.useRealTimers() })
function control() {
  const host = document.createElement('div')
  document.body.append(host)
  const shadow = host.attachShadow({ mode: 'open' })
  const button = document.createElement('button')
  button.type = 'button'; button.textContent = '保存'
  button.getBoundingClientRect = () => ({ width:100,height:40,left:10,top:20,right:110,bottom:60,x:10,y:20,toJSON:()=>({}) })
  shadow.append(button)
  return { host, shadow, button }
}
describe('Material feedback inside native workspaces', () => {
  it('places pointer feedback inside the real control and removes it on feature disposal', () => {
    document.documentElement.dataset.furtherMotion = 'on'
    const { host, shadow, button } = control()
    const dispose = mountNativeMaterialFeedback(shadow)
    button.dispatchEvent(new MouseEvent('pointerdown', { bubbles:true,button:0,clientX:30,clientY:35 }))
    const ripple = button.querySelector('.rfw-material-ripple')!
    expect(ripple).not.toBeNull()
    expect(ripple).toHaveAttribute('aria-hidden', 'true')
    expect(button.textContent).toBe('保存')
    dispose()
    expect(button.querySelector('.rfw-material-ripple')).toBeNull()
    button.dispatchEvent(new MouseEvent('pointerdown', { bubbles:true,button:0 }))
    expect(button.querySelector('.rfw-material-ripple')).toBeNull()
    host.remove()
  })
  it('keeps feedback under legacy motion-off preferences while skipping disabled controls', () => {
    const { host, shadow, button } = control()
    const dispose = mountNativeMaterialFeedback(shadow)
    document.documentElement.dataset.furtherMotion = 'off'
    document.documentElement.dataset.reducedMotion = 'true'
    button.dispatchEvent(new MouseEvent('pointerdown', { bubbles:true,button:0 }))
    expect(button.querySelector('.rfw-material-ripple')).not.toBeNull()
    button.querySelector('.rfw-material-ripple')?.remove()
    button.disabled = true
    button.dispatchEvent(new MouseEvent('pointerdown', { bubbles:true,button:0 }))
    expect(button.querySelector('.rfw-material-ripple')).toBeNull()
    dispose(); host.remove()
  })
})
