import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'

const source = readFileSync(resolve(process.cwd(), '../further-ui/assets/resource-loader.js'), 'utf8')
type Manager = { load: (names: string[]) => Promise<void>; refresh: (names: string[]) => Promise<void> }
type Activate = ((name: string, force?: boolean) => Promise<void>) & { dispose: () => void }
type Runtime = { create: (loaders: Record<string, () => Promise<unknown>>) => Manager; mount: (manager: Manager, select: (name: string) => string[]) => Activate }
let dispose: (() => void) | undefined
const runtime = () => (window as unknown as { FurtherUIResources: Runtime }).FurtherUIResources
function enter(name: string) {
  document.querySelectorAll<HTMLElement>('[data-panel]').forEach(panel => panel.classList.toggle('active', panel.dataset.panel === name))
  window.dispatchEvent(new CustomEvent('further-ui:page', { detail: { name } }))
}
beforeEach(() => {
  document.body.innerHTML = '<main class="main-content" data-shell><section class="view active" data-panel="overview"><p>已有概览</p></section><section class="view" data-panel="channels"><p>已有渠道</p></section><section class="view" data-panel="models"><p>已有模型</p></section></main>'
  Object.defineProperty(window, 'CSS', { configurable: true, value: { escape: (value: string) => value } })
  window.eval(source)
})
afterEach(() => { dispose?.(); dispose = undefined; document.body.replaceChildren(); vi.restoreAllMocks() })

describe('one page data owner during navigation', () => {
  it('preserves cached content and navigable controls while refreshing exactly once on reentry', async () => {
    let finish!: () => void
    const overview = vi.fn().mockResolvedValueOnce(undefined).mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    const channels = vi.fn().mockResolvedValue(undefined)
    const activate = runtime().mount(runtime().create({ overview, channels }), name => [name]); dispose = activate.dispose
    await vi.waitFor(() => expect(document.querySelector('[data-panel=overview]')).not.toHaveAttribute('aria-busy'))
    enter('channels'); await vi.waitFor(() => expect(channels).toHaveBeenCalledOnce())
    enter('overview'); enter('overview')
    await vi.waitFor(() => expect(overview).toHaveBeenCalledTimes(2))
    const panel = document.querySelector<HTMLElement>('[data-panel=overview]')!
    expect(panel.inert).not.toBe(true)
    expect(panel.firstElementChild?.textContent).toBe('已有概览')
    expect(panel.querySelector('[data-page-load-status]')).toBeNull()
    expect(document.querySelectorAll('[data-page-load-status]')).toHaveLength(1)
    finish(); await vi.waitFor(() => expect(panel).not.toHaveAttribute('aria-busy'))
  })

  it('skips intermediate route queries during rapid navigation and mounts each owner once', async () => {
    const overview = vi.fn().mockResolvedValue(undefined), channels = vi.fn().mockResolvedValue(undefined), models = vi.fn().mockResolvedValue(undefined)
    const manager = runtime().create({ overview, channels, models })
    const activate = runtime().mount(manager, name => [name]); dispose = activate.dispose
    expect(runtime().mount(manager, name => [name])).toBe(activate)
    enter('channels'); enter('models')
    await vi.waitFor(() => expect(models).toHaveBeenCalledOnce())
    expect(overview).not.toHaveBeenCalled()
    expect(channels).not.toHaveBeenCalled()
    enter('models')
    expect(models).toHaveBeenCalledOnce()
  })

  it('retains cached content after a failed refresh and offers an explicit retry', async () => {
    const overview = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined)
    const channels = vi.fn().mockResolvedValue(undefined)
    const activate = runtime().mount(runtime().create({ overview, channels }), name => [name]); dispose = activate.dispose
    await vi.waitFor(() => expect(document.querySelector('[data-panel=overview]')).not.toHaveAttribute('aria-busy'))
    enter('channels'); await vi.waitFor(() => expect(channels).toHaveBeenCalledOnce())
    enter('overview')
    await vi.waitFor(() => expect(document.querySelector('[data-page-load-status]')).toHaveTextContent('更新失败，已保留当前内容：offline'))
    expect(document.querySelector('[data-panel=overview]')?.textContent).toBe('已有概览')
    document.querySelector<HTMLButtonElement>('[data-page-load-status] button')!.click()
    await vi.waitFor(() => expect(overview).toHaveBeenCalledTimes(3))
  })
})
