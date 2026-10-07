import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const root = resolve(process.cwd(), '../further-ui')
const markup = readFileSync(resolve(root, 'admin/index.html'), 'utf8')
const runtime = readFileSync(resolve(root, 'assets/admin/model-imports.js'), 'utf8')
const request = vi.fn()
const toast = vi.fn()
const refresh = vi.fn()
const openChannelModels = vi.fn()
const endpoint = '/api/further/models/pricing/models-dev'
type Source = { provider_id: string; provider_name?: string; model_id: string; input_price: number; output_price: number; cache_read_price?: number; recommended?: boolean; effective_price?: Record<string, number> }
type PreviewItem = { model: string; status: string; channel_count: number; reason?: string; current: Record<string, number | null>; candidates: Source[] }
type RuntimeWindow = Window & { FurtherUIModelImports: { bind(): void; setChannels(rows: unknown[]): void; openPricing(options?: { models: string[] }): void; setModels(rows: unknown[]): void; afterChannelSave(models: string, refreshFailed?: boolean): void } }
const app = () => (window as unknown as RuntimeWindow).FurtherUIModelImports
const button = (selector: string) => document.querySelector(selector) as HTMLButtonElement
function item(model: string, status = 'matched', candidates?: Source[]): PreviewItem {
  return { model, status, channel_count: 1, current: { input_price: 1, output_price: 3, cache_read_price: 0.5, cache_write_price: 1 }, candidates: candidates ?? [{ provider_id: 'official', provider_name: '官方', model_id: model, input_price: 2, output_price: 8, recommended: true }] }
}
const response = (items: PreviewItem[]) => ({ snapshot_id: 'preview-snapshot', generated_at: 1780800000, expires_at: 1780800600, items })

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
  document.body.dataset.gatewayRole = '100'
  request.mockReset(); toast.mockReset(); refresh.mockReset().mockResolvedValue(undefined); openChannelModels.mockReset()
  Object.assign(window, {
    FurtherUI: {
      request, toast,
      escapeHTML: (value: unknown) => String(value ?? '').replaceAll(/[&<>"']/g, symbol => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[symbol]!),
      openDialog: (id: string) => document.querySelector(`#${id}`)?.classList.add('show'),
      closeDialog: (id: string) => { const dialog = document.querySelector(`#${id}`) as HTMLElement | null; if (dialog?.dataset.dialogBusy !== 'true') dialog?.classList.remove('show') },
      showBusy: (target: HTMLButtonElement, busy: boolean) => { target.disabled = busy },
    },
    FurtherUIAdminModels: { refresh },
    FurtherUIAdminChannelModels: { open: openChannelModels, setChannels: vi.fn() },
  })
  window.eval(runtime)
})

describe('channel-linked models.dev price import', () => {
  it('waits for an explicit price read and forwards the upstream entry without loading channels again', () => {
    app().bind(); app().bind()
    app().setChannels([{ id: 1, name: '渠道 A' }])
    expect(document.querySelectorAll('[data-model-price-import]')).toHaveLength(1)
    expect(document.querySelector('[data-model-channel-count]')?.textContent).toBe('1 个渠道')
    button('[data-model-pull-upstream]').click()
    expect(openChannelModels).toHaveBeenCalledWith({})
    expect(request).not.toHaveBeenCalled()
  })

  it('previews all states and only saves eligible selected source IDs, then reloads persisted model pricing', async () => {
    request.mockResolvedValueOnce(response([
      item('matched'), item('missing', 'unmatched', []), item('request', 'protected', []), item('unbound', 'unbound', []),
    ])).mockResolvedValueOnce({ updated_count: 1, skipped_count: 0 })
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    expect(request).toHaveBeenCalledTimes(1)
    expect(document.querySelector('[data-price-model="matched"]')?.textContent).toContain('$0.5')
    for (const model of ['missing', 'request', 'unbound']) expect((document.querySelector(`[data-price-select="${model}"]`) as HTMLInputElement).disabled).toBe(true)
    button('[data-price-import-apply]').click()
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledOnce())
    expect(request).toHaveBeenLastCalledWith(endpoint, { method: 'POST', body: { snapshot_id: 'preview-snapshot', selections: [{ model: 'matched', provider_id: 'official', model_id: 'matched' }] } })
    expect(document.querySelector('#model-price-import-dialog')?.classList.contains('show')).toBe(false)
  })

  it('uses the recommended official candidate and requires a source choice for ambiguous matches', async () => {
    const sources = [
      { provider_id: 'mirror', model_id: 'model', input_price: 1, output_price: 4 },
      { provider_id: 'official', model_id: 'model', input_price: 2, output_price: 8, recommended: true },
    ]
    request.mockResolvedValueOnce(response([item('recommended', 'matched', sources), item('ambiguous', 'ambiguous', sources.map(source => ({ ...source, recommended: false })))])).mockResolvedValueOnce({ updated_count: 2 })
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(document.querySelector('[data-price-source="recommended"]')).not.toBeNull())
    expect((document.querySelector('[data-price-source="recommended"]') as HTMLSelectElement).value).toBe('1')
    expect((document.querySelector('[data-price-select="ambiguous"]') as HTMLInputElement).disabled).toBe(true)
    const select = document.querySelector('[data-price-source="ambiguous"]') as HTMLSelectElement
    select.value = '0'; select.dispatchEvent(new Event('change', { bubbles: true }))
    expect(document.activeElement).toBe(document.querySelector('[data-price-source="ambiguous"]'))
    expect((document.querySelector('[data-price-select="ambiguous"]') as HTMLInputElement).checked).toBe(true)
    button('[data-price-import-apply]').click()
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(2))
    expect(request.mock.calls[1][1].body.selections).toEqual([
      { model: 'recommended', provider_id: 'official', model_id: 'model' },
      { model: 'ambiguous', provider_id: 'mirror', model_id: 'model' },
    ])
  })

  it('shows a website error and supports retry after a failed external read', async () => {
    request.mockRejectedValueOnce(new Error('数据源暂时不可用')).mockResolvedValueOnce(response([item('model')]))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(document.querySelector('[data-price-import-body] [role="alert"]')?.textContent).toContain('数据源暂时不可用'))
    expect(button('[data-price-import-apply]').disabled).toBe(true)
    button('[data-price-import-retry]').click()
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('prevents duplicate submissions and dismissal while a batch is being saved', async () => {
    let resolveApply!: (value: unknown) => void
    request.mockResolvedValueOnce(response([item('model')])).mockImplementationOnce(() => new Promise(resolve => { resolveApply = resolve }))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    button('[data-price-import-apply]').click(); button('[data-price-import-apply]').click(); button('[data-price-import-close]').click()
    expect(request).toHaveBeenCalledTimes(2)
    expect(document.querySelector('#model-price-import-dialog')?.classList.contains('show')).toBe(true)
    expect((document.querySelector('#model-price-import-dialog') as HTMLElement)?.dataset.dialogBusy).toBe('true')
    resolveApply({ updated_count: 1 })
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledOnce())
  })

  it('discards a conflicting snapshot so the old batch cannot be resubmitted', async () => {
    request.mockResolvedValueOnce(response([item('model')])).mockRejectedValueOnce(Object.assign(new Error('配置已更新'), { response: { status: 409 } })).mockResolvedValueOnce(response([item('model')]))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    button('[data-price-import-apply]').click()
    await vi.waitFor(() => expect(document.querySelector('[data-price-import-save-error]')?.textContent).toContain('请重新读取'))
    expect(button('[data-price-import-apply]').disabled).toBe(true)
    expect(document.querySelector('[data-price-select]')).toBeNull()
    button('[data-price-import-retry]').click()
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    expect((document.querySelector('[data-price-import-save-error]') as HTMLElement).hidden).toBe(true)
  })

  it('reports saved prices distinctly if the subsequent catalog read fails', async () => {
    request.mockResolvedValueOnce(response([item('model')])).mockResolvedValueOnce({ updated_count: 1 })
    refresh.mockRejectedValueOnce(new Error('列表不可用'))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    button('[data-price-import-apply]').click()
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('价格已保存，模型列表刷新失败：列表不可用', 'danger'))
    expect(document.querySelector('[data-price-import-save-error]')?.textContent).toBe('')
    expect(button('[data-price-import-apply]').disabled).toBe(true)
  })

  it('restricts price imports to root while keeping channel discovery available to administrators', () => {
    document.body.dataset.gatewayRole = '10'
    app().bind()
    expect(button('[data-model-price-import]').disabled).toBe(true)
    expect(button('[data-model-pull-upstream]').disabled).toBe(false)
    app().openPricing()
    expect(request).not.toHaveBeenCalled()
    expect(document.querySelector('.model-channel-bridge')?.textContent).toContain('仅超级管理员')
  })
  it('defaults to missing prices and permits existing-price updates only after switching the mode', async () => {
    request.mockResolvedValueOnce(response([item('configured', 'configured', [])])).mockResolvedValueOnce(response([item('configured')]))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(document.querySelector('[data-price-model="configured"]')).not.toBeNull())
    expect(request).toHaveBeenCalledWith(`${endpoint}?missing_only=true`)
    expect(button('[data-price-import-apply]').disabled).toBe(true)
    const onlyMissing = document.querySelector('[data-price-missing-only]') as HTMLInputElement
    onlyMissing.checked = false; onlyMissing.dispatchEvent(new Event('change', { bubbles: true }))
    await vi.waitFor(() => expect(button('[data-price-import-apply]').disabled).toBe(false))
    expect(request).toHaveBeenLastCalledWith(endpoint)
  })

  it('checks only the just-saved channel models when their pricing is missing', async () => {
    request.mockResolvedValueOnce(response([item('new'), item('other')]))
    app().bind()
    app().setModels([{ model: 'new', pricing_unavailable: true }, { model: 'priced', pricing_unavailable: false }, { model: 'other', pricing_unavailable: true }])
    expect(document.querySelector('[data-model-pricing-count]')?.textContent).toBe('2 个模型待定价')
    app().afterChannelSave('new,priced')
    await vi.waitFor(() => expect(document.querySelector('[data-price-model="new"]')).not.toBeNull())
    expect(document.querySelector('[data-price-model="other"]')).toBeNull()
    expect(document.querySelector('[data-channel-price-setup]')).not.toBeNull()
  })

  it('fills a new model draft with reference prices without persisting or marking it free', async () => {
    request.mockResolvedValueOnce(response([item('draft', 'reference')]))
    app().bind()
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    field('model').value = 'draft'
    button('[data-model-fill-pricing]').click()
    await vi.waitFor(() => expect(field('input_price').value).toBe('2'))
    expect(field('output_price').value).toBe('8')
    expect(field('free_model').checked).toBe(false)
    expect(request).toHaveBeenCalledExactlyOnceWith(`${endpoint}?model=draft`)
  })

  it('reads reference prices for an already-priced model and preserves unspecified cache prices', async () => {
    request.mockResolvedValueOnce(response([item('draft')]))
    app().bind()
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    field('model').value = 'draft'; field('input_price').value = '9'; field('output_price').value = '18'
    field('cache_read_price').value = '1.25'; field('cache_write_price').value = '6'
    field('free_model').checked = true
    button('[data-model-fill-pricing]').click()
    await vi.waitFor(() => expect(field('input_price').value).toBe('2'))
    expect(request).toHaveBeenCalledExactlyOnceWith(`${endpoint}?model=draft`)
    expect(field('cache_read_price').value).toBe('1.25')
    expect(field('cache_write_price').value).toBe('6')
    expect(field('free_model').checked).toBe(false)
  })

  it('previews the actual default cache price for a newly-priced model', async () => {
    const missing = item('missing-cache')
    missing.current = { input_price: null, output_price: null, cache_read_price: null, cache_write_price: null }
    missing.candidates[0].effective_price = { input_price: 2, output_price: 8, cache_read_price: 2, cache_write_price: 2.5 }
    request.mockResolvedValueOnce(response([missing]))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(document.querySelector('[data-price-model="missing-cache"] .model-import-price-grid')?.textContent).toContain('$2.5（默认倍率）'))
  })

  it('keeps changed free-model intent and restored permissions while a reference is loading', async () => {
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    app().bind()
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    field('model').value = 'draft'
    button('[data-model-fill-pricing]').click()
    field('free_model').checked = true
    form.dataset.pricingEditable = 'false'
    finish(response([item('draft', 'reference')]))
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('表单已修改，本次参考价格未填入，请重新读取', 'danger'))
    expect(field('input_price').value).toBe('0')
    expect(field('free_model').checked).toBe(true)
    expect(button('[data-model-fill-pricing]').disabled).toBe(true)
  })

  it('requires selecting zero-price sources explicitly', async () => {
    request.mockResolvedValueOnce(response([item('free-source', 'matched', [{ provider_id: 'official', model_id: 'free-source', input_price: 0, output_price: 0, recommended: true }])]))
    app().bind(); app().openPricing()
    await vi.waitFor(() => expect(document.querySelector('[data-price-select="free-source"]')).not.toBeNull())
    const checkbox = document.querySelector('[data-price-select="free-source"]') as HTMLInputElement
    expect(checkbox.checked).toBe(false)
    expect(document.querySelector('[data-price-model="free-source"] .model-import-price-grid')?.textContent).toContain('$0')
    expect(checkbox.disabled).toBe(false)
    expect(button('[data-price-import-apply]').disabled).toBe(true)
    checkbox.checked = true; checkbox.dispatchEvent(new Event('change', { bubbles: true }))
    expect(button('[data-price-import-apply]').disabled).toBe(false)
  })

  it('does not replace user edits made while a reference price is loading', async () => {
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    app().bind()
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    field('model').value = 'draft'
    button('[data-model-fill-pricing]').click()
    field('input_price').value = '9'
    finish(response([item('draft', 'reference')]))
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('表单已修改，本次参考价格未填入，请重新读取', 'danger'))
    expect(field('input_price').value).toBe('9')
  })

  it('keeps channel-save success explicit when the extra price lookup fails', async () => {
    request.mockRejectedValueOnce(new Error('价格源暂时不可用'))
    app().bind()
    app().afterChannelSave('new', true)
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('渠道已保存，价格读取失败：价格源暂时不可用', 'danger'))
    expect(document.querySelector('[data-price-import-body]')?.textContent).toContain('价格读取失败')
    expect(button('[data-price-import-apply]').disabled).toBe(true)
  })

})
