import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, describe, expect, it, vi } from 'vitest'

const resourcesSource = readFileSync(resolve(process.cwd(), '../further-ui/assets/resource-loader.js'), 'utf8')
const appSource = readFileSync(resolve(process.cwd(), '../further-ui/assets/app.js'), 'utf8')

type Resources = { ensure: (name:string) => Promise<unknown>; refresh: (names:string[]) => Promise<void> }
const runtime = () => (window as unknown as { FurtherUIResources:{create: (loaders:Record<string,()=>Promise<unknown>>) => Resources} }).FurtherUIResources

afterEach(() => { localStorage.clear(); document.body.replaceChildren(); vi.restoreAllMocks() })

describe('workspace data refresh', () => {
  it('joins a pending load and fetches a fresh snapshot after it settles', async () => {
    window.eval(resourcesSource)
    let complete!: (value:unknown) => void
    const load = vi.fn().mockImplementationOnce(() => new Promise(resolve => { complete = resolve })).mockResolvedValue({ version:2 })
    const resources = runtime().create({ overview:load })
    const initial = resources.ensure('overview')
    const refreshWhileLoading = resources.refresh(['overview'])
    await vi.waitFor(() => expect(load).toHaveBeenCalledOnce())
    complete({ version:1 })
    await Promise.all([initial, refreshWhileLoading])
    await resources.refresh(['overview'])
    expect(load).toHaveBeenCalledTimes(2)
    await expect(resources.ensure('overview')).resolves.toEqual({ version:2 })
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('allows retry after a refresh fails without remembering the failed promise', async () => {
    window.eval(resourcesSource)
    const load = vi.fn().mockResolvedValueOnce('first').mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce('recovered')
    const resources = runtime().create({ overview:load })
    await resources.ensure('overview')
    await expect(resources.refresh(['overview'])).rejects.toThrow('offline')
    await expect(resources.ensure('overview')).resolves.toBe('recovered')
    expect(load).toHaveBeenCalledTimes(3)
  })

  it('keeps the shell and scroll position while the toolbar waits for current data', async () => {
    document.body.dataset.page = 'admin'
    document.body.innerHTML = '<main class="main-content"><section class="view active" data-panel="overview">existing content</section><button data-workspace-refresh>刷新本页</button></main>'
    const main = document.querySelector<HTMLElement>('main')!
    main.scrollTop = 240
    let complete!: () => void
    const pending = new Promise<void>(resolve => { complete = resolve })
    const receive = vi.fn((event:Event) => (event as CustomEvent<{name:string;waitUntil:(promise:Promise<void>)=>void}>).detail.waitUntil(pending))
    window.addEventListener('further-ui:refresh', receive)
    try {
      window.eval(appSource)
      document.dispatchEvent(new Event('further:ready'))
      const button = document.querySelector<HTMLButtonElement>('[data-workspace-refresh]')!
      button.click(); button.click()
      expect(receive).toHaveBeenCalledOnce()
      expect(button.disabled).toBe(true)
      expect(main.scrollTop).toBe(240)
      expect(document.querySelector('[data-panel=overview]')?.textContent).toBe('existing content')
      complete()
      await vi.waitFor(() => expect(button.disabled).toBe(false))
      expect(button.textContent).toBe('刷新本页')
    } finally { window.removeEventListener('further-ui:refresh', receive) }
  })
})
