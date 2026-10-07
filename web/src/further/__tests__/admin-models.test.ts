import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const root = resolve(process.cwd(), '../further-ui')
const markup = readFileSync(resolve(root, 'admin/index.html'), 'utf8')
const runtime = readFileSync(resolve(root, 'assets/admin/admin-models.js'), 'utf8')
const request = vi.fn()
type RuntimeWindow = Window & {
  FurtherUI: unknown
  FurtherUIAdminModels: { bind(): void; load(): Promise<void>; refresh(): Promise<void>; resetForm(): void; setChannels(rows: unknown[]): void }
}
const app = () => (window as unknown as RuntimeWindow).FurtherUIAdminModels
const field = (name: string) => (document.querySelector('[data-model-form]') as HTMLFormElement).elements.namedItem(name) as HTMLInputElement

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
  document.body.dataset.gatewayRole = '100'
  history.replaceState(null, '', '#models')
  request.mockReset()
  delete (window as unknown as { FurtherUIAdminRouting?: unknown }).FurtherUIAdminRouting
  ;(window as unknown as RuntimeWindow).FurtherUI = {
    request, adminListPreference: () => false, saveAdminListPreference: () => false,
    escapeHTML: (value: unknown) => String(value ?? '').replaceAll(/[&<>"']/g, (symbol) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[symbol]!),
    number: (value: unknown) => String(value ?? 0), toast: vi.fn(), openDialog: vi.fn(), closeDialog: vi.fn(), showBusy: vi.fn(),
    setText(selector: string, value: string) { document.querySelectorAll(selector).forEach((element) => { element.textContent = value }) },
  }
  window.eval(runtime)
  app().bind()
})

describe('native model configuration form', () => {
  it('shares simultaneous catalog reads and shows channel names and upstream aliases without fetching channels twice', async () => {
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const first = app().load()
    const second = app().load()
    expect(request).toHaveBeenCalledTimes(1)
    finish([{ id: 0, row_key: 'model:public', model: 'public', advanced_routing_enabled: true, bindings: [{ channel_id: 4, upstream_model: 'upstream' }] }])
    await Promise.all([first, second])
    app().setChannels([{ id: 4, name: '主渠道' }])
    await vi.waitFor(() => expect(document.querySelector('.model-channel-bindings')?.textContent).toContain('主渠道 → upstream'))
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('waits for an older catalog read before refreshing after a channel mutation', async () => {
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockResolvedValueOnce([{ id: 1, model: 'fresh', bindings: [] }])
    const first = app().load()
    const afterMutation = app().refresh()
    expect(request).toHaveBeenCalledTimes(1)
    finish([{ id: 1, model: 'stale', bindings: [] }])
    await Promise.all([first, afterMutation])
    expect(request).toHaveBeenCalledTimes(2)
    expect(document.querySelector('[data-model-rows]')?.textContent).toContain('fresh')
    expect(document.querySelector('[data-model-rows]')?.textContent).not.toContain('stale')
  })

  it('does not offer unsupported legacy roles or unimplemented model controls', () => {
    expect(document.querySelector('[data-admin-user-filter] select[name="role"] option[value="25"]')).toBeNull()
    for (const name of ['data_train', 'session_only', 'compaction_price', 'long_context_threshold']) {
      const input = field(name)
      expect(input.disabled).toBe(true)
      expect(input.closest('.hidden')).not.toBeNull()
    }
  })
  it('edits the selected channel-only model and creates its metadata without submitting model ID zero', async () => {
    request.mockResolvedValue([
      { id: 0, metadata_id: 0, row_key: 'model:first', model: 'first', enabled: true, advanced_routing_enabled: true, bindings: [] },
      { id: 0, metadata_id: 0, row_key: 'model:second', model: 'second', enabled: true, advanced_routing_enabled: true, bindings: [] },
    ])
    await app().load()
    ;(document.querySelector('[data-edit-model="model:second"]') as HTMLButtonElement).click()
    expect(field('model').value).toBe('second')
    expect(field('id').value).toBe('')
    field('input_price').value = '4'
    field('output_price').value = '12'
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/admin/models', expect.objectContaining({ method: 'POST', body: expect.objectContaining({ model: 'second', display_name: 'second', price_multiplier: 1, input_price: 4, output_price: 12 }) })))
    const mutation = request.mock.calls.find(([path, options]) => path === '/api/admin/models' && options?.method === 'POST')![1]
    expect(mutation.body).not.toHaveProperty('bindings')
    expect(document.querySelector('[data-delete-model="0"]')).toBeNull()
  })

  it('keeps expression pricing read-only without exposing the removed editor', async () => {
    request.mockResolvedValue([{ id: 1, metadata_id: 1, row_key: '1', model: 'advanced', enabled: true, pricing_editable: false, unsupported_reason: '表达式计费', pricing_details: { effective: { 'billing_setting.billing_expr': 'p * 2 + c * 5 <unsafe>' } }, bindings: [] }])
    await app().load()
    ;(document.querySelector('[data-edit-model="1"]') as HTMLButtonElement).click()
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    expect((form.querySelector('[type="submit"]') as HTMLButtonElement).disabled).toBe(true)
    expect(form.querySelector('[data-model-pricing-notice]')?.textContent).toContain('表达式计费')
    expect(form.querySelector('[data-model-pricing-notice] a')).toBeNull()
    expect(form.querySelector('[data-model-pricing-readonly] pre')?.textContent).toContain('p * 2 + c * 5 <unsafe>')
    expect(form.querySelector('[data-model-pricing-readonly] unsafe')).toBeNull()
    request.mockClear()
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(request).not.toHaveBeenCalled()
  })
  it('redirects obsolete pricing links into the unified model catalog', async () => {
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { mount } })
    request.mockResolvedValue([])
    document.querySelector('[data-panel="models"]')!.classList.add('active')
    history.replaceState(null, '', '#models/pricing')
    app().bind()
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/admin/models'))
    expect(mount).not.toHaveBeenCalled()
    expect(location.hash).toBe('#models')
    expect(document.querySelector('[data-admin-model-page="catalog"]')).not.toHaveClass('hidden')
  })

  it('migrates old routing links and opens routing in the model editor', async () => {
    const editModel = vi.fn(), setModels = vi.fn(), resetModel = vi.fn()
    Object.assign(window, { FurtherUIAdminRouting: { editModel, setModels, resetModel, hasUnsaved: () => false } })
    request.mockResolvedValue([{ id: 1, model: 'unified', bindings: [] }])
    history.replaceState(null, '', '#models/routing')
    await app().load()
    expect(location.hash).toBe('#models')
    expect(document.querySelector('[data-admin-model-tab="routing"]')).toBeNull()
    expect(document.querySelector('#model-alias-dialog')).toBeNull()
    ;(document.querySelector('[data-bind-model-channel="1"]') as HTMLButtonElement).click()
    expect(editModel).toHaveBeenCalledWith('unified')
    expect(field('model').value).toBe('unified')
    expect(document.querySelector('[data-model-routing-editor]')).not.toBeNull()
  })

  it('keeps unsaved route changes visible before saving model pricing', () => {
    Object.assign(window, { FurtherUIAdminRouting: { resetModel: vi.fn(), hasUnsaved: () => true } })
    app().resetForm(); field('model').value = 'pending'; field('input_price').value = '2'; field('output_price').value = '4'
    ;(document.querySelector('[data-model-form]') as HTMLFormElement).dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(request).not.toHaveBeenCalled()
  })
  it('requires explicit free-model intent before saving a zero-price draft', async () => {
    app().resetForm()
    field('model').value = 'unpriced-model'
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(request).not.toHaveBeenCalled()
    field('free_model').checked = true
    request.mockResolvedValue([])
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/admin/models', expect.objectContaining({ method: 'POST', body: expect.objectContaining({ free_model: true }) })))
  })

  it('shows enforced output ratios and keeps the submitted price aligned with the input', async () => {
    request.mockResolvedValue([{ id: 1, model: 'locked', input_price: 2, output_price: 6, completion_ratio_info: { locked: true, ratio: 3 }, bindings: [] }])
    await app().load()
    ;(document.querySelector('[data-edit-model="1"]') as HTMLButtonElement).click()
    expect(field('output_price').readOnly).toBe(true)
    field('input_price').value = '4'
    field('input_price').dispatchEvent(new Event('input', { bubbles: true }))
    expect(field('output_price').value).toBe('12')
    expect(document.querySelector('[data-model-output-ratio-hint]')?.textContent).toContain('固定为 3')
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/admin/models/1', expect.objectContaining({ body: expect.objectContaining({ input_price: 4, output_price: 12 }) })))
    app().resetForm()
    expect(field('output_price').readOnly).toBe(false)
  })
})
