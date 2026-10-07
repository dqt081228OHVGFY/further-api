import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/admin.js'), 'utf8')
const markup = readFileSync(resolve(process.cwd(), '../further-ui/admin/index.html'), 'utf8')
const request = vi.fn(), toast = vi.fn(), closeDialog = vi.fn(), refreshModels = vi.fn(), overview = vi.fn(), afterChannelSave = vi.fn()
const form = () => document.querySelector('[data-channel-form]') as HTMLFormElement

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').querySelector('#channel-dialog')!.outerHTML
  form().insertAdjacentHTML('beforeend', '<input type="checkbox" data-channel-setup-pricing checked>')
  request.mockReset().mockImplementation((_path, options) => options?.method === 'POST' ? Promise.resolve({}) : Promise.resolve([]))
  toast.mockReset(); closeDialog.mockReset(); refreshModels.mockReset().mockResolvedValue([]); overview.mockReset().mockResolvedValue({}); afterChannelSave.mockReset()
  Object.assign(window, {
    FurtherUI: { request, toast, closeDialog, adminListPreference: () => false, showBusy: (button: HTMLButtonElement, busy: boolean) => { button.disabled = busy } },
    FurtherUIAdminModels: { load: refreshModels, refresh: refreshModels, setChannels: vi.fn() },
    FurtherUIAdminOverview: { load: overview }, FurtherUIAdminSettings: { load: vi.fn() },
    FurtherUIResources: { create: () => ({}) }, FurtherUIAdminChannelModels: undefined,
    FurtherUIModelImports: { afterChannelSave },
  })
  window.eval(runtime.replace(/\}\(\)\);\s*$/, 'window.__bindChannelSave = bindChannelForm; }());'))
  ;(window as unknown as { __bindChannelSave(): void }).__bindChannelSave()
  ;(form().elements.namedItem('models') as HTMLInputElement).value = 'new-model'
})

describe('channel save outcome', () => {
  it('preserves successful saves and continues the pricing check after a refresh failure', async () => {
    request.mockImplementation((_path, options) => options?.method === 'POST' ? Promise.resolve({}) : Promise.reject(new Error('目录不可用')))
    overview.mockRejectedValueOnce(new Error('概览不可用'))
    form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(afterChannelSave).toHaveBeenCalledWith('new-model', false))
    expect(closeDialog).toHaveBeenCalledWith('channel-dialog')
    expect(toast).toHaveBeenCalledWith('渠道已创建')
    expect(toast).toHaveBeenCalledWith('渠道已保存，部分页面数据刷新失败，请点击刷新重试。', 'danger')
    expect(toast.mock.calls.some(([message]) => String(message).includes('渠道保存失败'))).toBe(false)
  })

  it('keeps the draft open and skips price lookup if the channel save fails', async () => {
    request.mockRejectedValueOnce(new Error('无权保存'))
    form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('渠道保存失败：无权保存', 'danger'))
    expect(closeDialog).not.toHaveBeenCalled()
    expect(afterChannelSave).not.toHaveBeenCalled()
    expect(refreshModels).not.toHaveBeenCalled()
  })
  it('fills only blank connection fields from server defaults and never edits keys', async () => {
    const field = (name: string) => form().elements.namedItem(name) as HTMLInputElement
    request.mockResolvedValueOnce({ 43: 'https://api.deepseek.com' })
    field('provider_type').value = '43'; field('name').value = '自定义渠道'; field('key').value = 'test-secret'; field('priority').value = '8'; field('weight').value = ''
    ;(document.querySelector('[data-channel-defaults]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(field('base_url').value).toBe('https://api.deepseek.com'))
    expect(field('name').value).toBe('自定义渠道'); expect(field('key').value).toBe('test-secret'); expect(field('priority').value).toBe('8'); expect(field('weight').value).toBe('1')
    expect(field('test_model').value).toBe('new-model')
    expect(request).toHaveBeenCalledExactlyOnceWith('/api/channel/default_base_urls')
  })

  it('does not apply defaults fetched for a supplier that changed during loading', async () => {
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const field = (name: string) => form().elements.namedItem(name) as HTMLInputElement
    field('provider_type').value = '1'
    ;(document.querySelector('[data-channel-defaults]') as HTMLButtonElement).click()
    field('provider_type').value = '14'
    finish({ 1: 'https://api.openai.com' })
    await vi.waitFor(() => expect(document.querySelector('[data-channel-preset-status]')?.textContent).toContain('已切换'))
    expect(field('base_url').value).toBe('')
  })

  it('normalizes model IDs and configures a visible multi-key rotation draft without submitting', () => {
    const field = (name: string) => form().elements.namedItem(name) as HTMLInputElement
    field('models').value = ' gpt-a，gpt-b,gpt-a '
    field('key').value = 'example-key'
    ;(document.querySelector('[data-channel-normalize-models]') as HTMLButtonElement).click()
    ;(document.querySelector('[data-channel-key-preset]') as HTMLButtonElement).click()
    expect(field('models').value).toBe('gpt-a,gpt-b')
    expect(field('test_model').value).toBe('gpt-a')
    expect(field('multi_key_mode').value).toBe('multi_to_single')
    expect(field('multi_key_type').value).toBe('polling')
    expect(field('key').value).toBe('example-key')
    expect(request).not.toHaveBeenCalled()
  })

})
