import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/channel-model-picker.js'), 'utf8')
const markup = readFileSync(resolve(process.cwd(), '../further-ui/admin/index.html'), 'utf8')
const request = vi.fn(), refresh = vi.fn(), toast = vi.fn(), refreshChannels = vi.fn()
type Picker = { open(options?: {channelID?: number}): Promise<void>; setChannels(rows: unknown[]): void; mountForm(form: HTMLFormElement): void; resetForm(form: HTMLFormElement): void; setRefreshHandler(handler: () => Promise<void>): void }
const app = () => (window as unknown as { FurtherUIAdminChannelModels: Picker }).FurtherUIAdminChannelModels
const root = () => document.querySelector('#channel-model-picker-dialog')!
const click = (selector: string, owner: ParentNode = root()) => (owner.querySelector(selector) as HTMLButtonElement).click()
const choices = () => [...root().querySelectorAll<HTMLInputElement>('[data-upstream-model]')]
const deferred = () => { let resolve!: (value: unknown) => void; const promise = new Promise((done) => { resolve = done }); return { promise, resolve } }

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').querySelector('#channel-dialog')!.outerHTML
  document.body.dataset.gatewayRole = '100'
  request.mockReset(); refresh.mockReset().mockResolvedValue(undefined); toast.mockReset(); refreshChannels.mockReset().mockResolvedValue(undefined)
  Object.assign(window, {
    FurtherUI: {
      request, toast,
      escapeHTML: (value: unknown) => String(value ?? '').replaceAll(/[&<>"']/g, (symbol) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[symbol]!),
      openDialog: (id: string) => document.querySelector(`#${id}`)?.classList.add('show'),
      closeDialog: (id: string) => document.querySelector(`#${id}`)?.classList.remove('show'),
      showBusy: (button: HTMLButtonElement, value: boolean) => { button.disabled = value },
    },
    FurtherUIAdminModels: { refresh },
  })
  window.eval(runtime)
  app().setChannels([{ id: 9, name: '上游渠道', status: 1, models: 'old-model,alias' }, { id: 10, name: '第二渠道', models: '' }])
  app().setRefreshHandler(refreshChannels)
})

describe('channel upstream model picker', () => {
  it('uses the saved channel ID and only imports the selected new models, then refreshes the model configuration', async () => {
    request.mockImplementation((path) => path.endsWith('/import') ? Promise.resolve({ models: 'old-model,alias,new-model', added_models: ['new-model'] }) : Promise.resolve(['old-model', 'new-model', 'other-model', 'new-model']))
    await app().open({ channelID: 9 })
    await vi.waitFor(() => expect(choices()).toHaveLength(3))
    expect(request).toHaveBeenCalledWith('/api/channel/fetch_models/9')
    expect(choices().find((item) => item.value === 'old-model')?.disabled).toBe(true)
    const other = choices().find((item) => item.value === 'other-model')!; other.checked = false; other.dispatchEvent(new Event('change', { bubbles: true }))
    click('[data-upstream-add]')
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(1))
    expect(request).toHaveBeenCalledWith('/api/channel/9/models/import', { method: 'POST', body: { models: ['new-model'] } })
    expect(refreshChannels).toHaveBeenCalledTimes(1)
    expect(choices().find((item) => item.value === 'new-model')?.disabled).toBe(true)
  })

  it('blocks duplicate imports until the first save completes', async () => {
    const saved = deferred()
    request.mockImplementation((path) => path.endsWith('/import') ? saved.promise : Promise.resolve(['new']))
    await app().open({ channelID: 9 }); await vi.waitFor(() => expect(choices()).toHaveLength(1))
    click('[data-upstream-add]'); click('[data-upstream-all]')
    expect(request.mock.calls.filter(([path]) => path.endsWith('/import'))).toHaveLength(1)
    saved.resolve({ models: 'old-model,alias,new', added_models: ['new'] })
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledTimes(1))
  })

  it('keeps the form draft additive and never requests or exposes a saved credential', async () => {
    const form = document.querySelector('[data-channel-form]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    field('id').value = '9'; field('models').value = 'old-model,alias'; field('key').value = ''
    app().mountForm(form)
    request.mockResolvedValue(['old-model', 'new-model'])
    const owner = form.querySelector('[data-upstream-draft]')!
    click('[data-upstream-fetch]', owner)
    await vi.waitFor(() => expect(owner.querySelectorAll('[data-upstream-model]')).toHaveLength(2))
    expect(request).toHaveBeenCalledWith('/api/channel/fetch_models', expect.objectContaining({ method: 'POST', body: expect.objectContaining({ channel_id: 9, type: 1, key: '' }) }))
    click('[data-upstream-all]', owner)
    expect(field('models').value).toBe('old-model,alias,new-model')
    expect(request).toHaveBeenCalledTimes(1)
    expect(owner.textContent).toContain('保存渠道后立即生效')
  })

  it('discards stale discovery responses after the channel changes and permits retry', async () => {
    const slow = deferred()
    request.mockImplementation((path) => path.endsWith('/9') ? slow.promise : Promise.resolve(['second-model']))
    await app().open({ channelID: 9 })
    const select = root().querySelector('[data-upstream-channel]') as HTMLSelectElement
    select.value = '10'; select.dispatchEvent(new Event('change', { bubbles: true }))
    click('[data-upstream-fetch]'); await vi.waitFor(() => expect(choices()[0]?.value).toBe('second-model'))
    slow.resolve(['stale-model']); await slow.promise; await Promise.resolve()
    expect(choices().map((item) => item.value)).toEqual(['second-model'])
  })

  it('shows provider failures with a working retry and blocks ordinary users', async () => {
    request.mockRejectedValueOnce(new Error('上游返回 HTTP 502')).mockResolvedValueOnce(['retry-model'])
    await app().open({ channelID: 9 }); await vi.waitFor(() => expect(root().textContent).toContain('可点击'))
    click('[data-upstream-fetch]'); await vi.waitFor(() => expect(choices()[0]?.value).toBe('retry-model'))
    document.body.dataset.gatewayRole = '1'; request.mockClear()
    await app().open({ channelID: 9 }); expect(request).not.toHaveBeenCalled()
    expect(toast).toHaveBeenCalledWith('当前账号没有渠道配置权限', 'danger')
  })

  it('renders a bounded large directory while one click imports all model IDs', async () => {
    const models = Array.from({ length: 1000 }, (_, index) => `upstream-${index}`)
    request.mockImplementation((path) => path.endsWith('/import') ? Promise.resolve({ models: models.join(','), added_models: models }) : Promise.resolve(models))
    await app().open({ channelID: 9 }); await vi.waitFor(() => expect(choices()).toHaveLength(200))
    expect(root().textContent).toContain('上游 1000 个')
    click('[data-upstream-all]'); await vi.waitFor(() => expect(refresh).toHaveBeenCalled())
    expect(request).toHaveBeenCalledWith('/api/channel/9/models/import', { method: 'POST', body: { models } })
  })
  it('opens unified pricing for the selected channel models', async () => {
    const openPricing = vi.fn()
    Object.assign(window, { FurtherUIModelImports: { openPricing } })
    request.mockResolvedValue([])
    await app().open({ channelID: 9 })
    click('[data-upstream-pricing]')
    expect(openPricing).toHaveBeenCalledWith({ models: ['old-model', 'alias'] })
    expect(root().classList.contains('show')).toBe(false)
  })

  it('reads and fills all upstream models into the channel draft in one action without saving', async () => {
    const form = document.querySelector('[data-channel-form]') as HTMLFormElement
    ;(form.elements.namedItem('key') as HTMLInputElement).value = 'example-key'
    ;(form.elements.namedItem('models') as HTMLInputElement).value = 'existing'
    app().mountForm(form)
    request.mockResolvedValue(['existing', 'new-model'])
    click('[data-upstream-fill-draft]', form)
    await vi.waitFor(() => expect((form.elements.namedItem('models') as HTMLInputElement).value).toBe('existing,new-model'))
    expect(request).toHaveBeenCalledTimes(1)
    expect(request.mock.calls[0][0]).toBe('/api/channel/fetch_models')
    expect(toast).toHaveBeenCalledWith('已加入渠道草稿，点击“保存”生效')
  })

})
