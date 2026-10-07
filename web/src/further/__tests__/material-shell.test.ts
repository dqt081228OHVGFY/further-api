import { readFileSync } from 'node:fs'
import path from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NATIVE_FEATURE_ENTRIES } from '../native-feature-paths'
import { workspaceShellTarget } from '../workspace-target'

const app = readFileSync(path.resolve(process.cwd(), '../further-ui/assets/app.js'), 'utf8')
const search = readFileSync(path.resolve(process.cwd(), '../further-ui/assets/material-shell.js'), 'utf8')

function mount() {
  vi.useFakeTimers()
  localStorage.clear()
  document.body.dataset.page = 'admin'
  document.body.dataset.gatewayRole = '100'
  document.body.innerHTML = '<style>.view:not(.active){display:none}.dialog-backdrop:not(.show){display:none}</style><nav class="nav-rail" data-shell><button class="nav-item" data-nav="overview">总览</button><button class="nav-item" data-nav="models">模型</button></nav><main class="main-content"><header class="top-app-bar"><span class="appbar-spacer"></span><span data-page-title></span></header><section class="view active" data-panel="overview"></section><section class="view" data-panel="models"></section><section class="view" data-panel="workspace"></section></main>'
  history.replaceState(null, '', '/admin/#overview')
  Object.defineProperty(window, 'matchMedia', { configurable:true, value:() => ({ matches:false, addEventListener() {}, removeEventListener() {} }) })
  Object.assign(window, { FurtherFeatures:{ entries:[{ path:'/task-plugins', title:'任务插件', role:100 }] }, FurtherGateway:{ workspaceShellTarget } })
  window.eval(app)
  window.eval(search)
  document.dispatchEvent(new Event('further:ready'))
  ;(window as unknown as { FurtherUI:{ mountShell:() => void } }).FurtherUI.mountShell()
  return { launch:document.querySelector<HTMLButtonElement>('.workspace-search')!, dialog:document.querySelector<HTMLElement>('#workspace-jump')!, input:document.querySelector<HTMLInputElement>('.workspace-jump-input')! }
}
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

describe('animated workspace page search', () => {
  it('clears revealed credentials only after the closing animation and keeps a reopened dialog intact', () => {
    mount()
    const dialog = document.createElement('div')
    dialog.id = 'created-token-dialog'; dialog.className = 'dialog-backdrop'
    dialog.innerHTML = '<div class="md-dialog"><input type="password" value="temporary-test-secret"><code data-created-token>sk-test-revealed-key</code></div>'
    document.body.append(dialog)
    const api = (window as unknown as { FurtherUI: { openDialog: (id: string) => void; closeDialog: (id: string) => void } }).FurtherUI
    api.openDialog(dialog.id)
    api.closeDialog(dialog.id)
    expect(dialog.querySelector('[data-created-token]')!.textContent).toBe('sk-test-revealed-key')
    api.openDialog(dialog.id)
    vi.advanceTimersByTime(250)
    expect(dialog.querySelector('[data-created-token]')!.textContent).toBe('sk-test-revealed-key')
    api.closeDialog(dialog.id)
    vi.advanceTimersByTime(230)
    expect(dialog.querySelector<HTMLInputElement>('input')!.value).toBe('')
    expect(dialog.querySelector('[data-created-token]')!.textContent).toBe('')
  })
  it('starts page navigation while the accessible search surface exits', () => {
    const { launch, dialog, input } = mount()
    launch.click()
    expect(launch).toHaveAttribute('aria-expanded', 'true')
    expect(dialog).toHaveClass('show')
    input.value='模型'; input.dispatchEvent(new Event('input', {bubbles:true}))
    const result=dialog.querySelector<HTMLButtonElement>('.workspace-jump-results button')!
    result.click()
    expect(dialog).toHaveClass('closing')
    expect(location.hash).toBe('#models')
    vi.advanceTimersByTime(180)
    expect(location.hash).toBe('#models')
    expect(launch).toHaveAttribute('aria-expanded', 'false')
  })
  it('keeps permitted tools reachable and does not defer a stale choice after search reopens', () => {
    const { launch, dialog, input } = mount()
    launch.click()
    input.value='插件'; input.dispatchEvent(new Event('input', {bubbles:true}))
    dialog.querySelector<HTMLButtonElement>('.workspace-jump-results button')!.click()
    launch.click()
    vi.advanceTimersByTime(240)
    expect(location.hash).toBe('#workspace/%2Ftask-plugins')
    input.value='插件'; input.dispatchEvent(new Event('input', {bubbles:true}))
    dialog.querySelector<HTMLButtonElement>('.workspace-jump-results button')!.click()
    vi.advanceTimersByTime(180)
    expect(location.hash).toBe('#workspace/%2Ftask-plugins')
    expect(document.body.textContent).not.toContain('更多功能')
  })
  it('merges differently named aliases with the existing page while keeping distinct audit and task logs', () => {
    const { launch, dialog, input } = mount()
    document.querySelector('.nav-rail')!.insertAdjacentHTML('beforeend', '<button class="nav-item" data-nav="channels">渠道</button><button class="nav-item" data-nav="logs">日志</button>')
    window.FurtherFeatures.entries = NATIVE_FEATURE_ENTRIES
    launch.click()
    for (const [term, title] of [['渠道', '渠道'], ['日志', '日志']]) {
      input.value = term; input.dispatchEvent(new Event('input', { bubbles: true }))
      const names = [...dialog.querySelectorAll('.workspace-jump-results button')].map(node => node.textContent)
      expect(names.filter(value => value === title)).toHaveLength(1)
      if (term === '日志') {
        expect(names).not.toContain('调用日志')
        expect(names).toContain('绘图日志')
        expect(names).toContain('任务日志')
      }
    }
  })
  it('never offers root-only features to ordinary users and merges account aliases by target', () => {
    const { launch, dialog, input } = mount()
    document.body.dataset.page = 'console'; document.body.dataset.gatewayRole = '1'
    // The role is captured when the search mounts; rebuild for the user shell.
    launch.remove(); dialog.remove()
    document.querySelector('.nav-rail')!.innerHTML = '<button class="nav-item" data-nav="tokens">API Key</button><button class="nav-item" data-nav="profile">账户设置</button>'
    Object.assign(window.FurtherFeatures, { entries: NATIVE_FEATURE_ENTRIES })
    window.eval(search)
    document.dispatchEvent(new Event('further:ready'))
    document.querySelector<HTMLButtonElement>('.workspace-search')!.click()
    const userInput = document.querySelector<HTMLInputElement>('.workspace-jump-input')!
    userInput.value = '账户'; userInput.dispatchEvent(new Event('input', { bubbles: true }))
    expect([...document.querySelectorAll('.workspace-jump-results button')].map(node => node.textContent)).toEqual(['账户设置'])
    userInput.value = '安全'; userInput.dispatchEvent(new Event('input', { bubbles: true }))
    expect([...document.querySelectorAll('.workspace-jump-results button')].map(node => node.textContent)).toEqual(['账户设置'])
    userInput.value = '设置'; userInput.dispatchEvent(new Event('input', { bubbles: true }))
    expect(document.querySelector('.workspace-jump-results')!.textContent).not.toContain('基础认证')
    userInput.value = '插件'; userInput.dispatchEvent(new Event('input', { bubbles: true }))
    expect(document.querySelectorAll('.workspace-jump-results button')).toHaveLength(0)
    expect(input.isConnected).toBe(false)
  })
})
