import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const root = resolve(process.cwd(), '../relayfor-ui')
const markup = readFileSync(resolve(root, 'admin/index.html'), 'utf8')
const runtime = readFileSync(resolve(root, 'assets/admin/admin-models.js'), 'utf8')
const request = vi.fn()
type RuntimeWindow = Window & {
  ProxyLan: unknown
  ProxyLanAdminModels: { bind(): void; load(): Promise<void>; resetForm(): void; setChannels(rows: unknown[]): void }
}
const app = () => (window as unknown as RuntimeWindow).ProxyLanAdminModels
const field = (name: string) => (document.querySelector('[data-model-form]') as HTMLFormElement).elements.namedItem(name) as HTMLInputElement

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
  history.replaceState(null, '', '#models')
  request.mockReset()
  ;(window as unknown as RuntimeWindow).ProxyLan = {
    request, adminListPreference: () => false, saveAdminListPreference: () => false,
    escapeHTML: (value: unknown) => String(value ?? '').replaceAll(/[&<>"']/g, (symbol) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[symbol]!),
    number: (value: unknown) => String(value ?? 0), toast: vi.fn(), openDialog: vi.fn(), closeDialog: vi.fn(), showBusy: vi.fn(),
    setText(selector: string, value: string) { document.querySelectorAll(selector).forEach((element) => { element.textContent = value }) },
  }
  window.eval(runtime)
  app().bind()
})

describe('native model configuration form', () => {
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

  it('keeps expression pricing read-only and links to the complete supported editor', async () => {
    request.mockResolvedValue([{ id: 1, metadata_id: 1, row_key: '1', model: 'advanced', enabled: true, pricing_editable: false, unsupported_reason: '表达式计费', bindings: [] }])
    await app().load()
    ;(document.querySelector('[data-edit-model="1"]') as HTMLButtonElement).click()
    const form = document.querySelector('[data-model-form]') as HTMLFormElement
    expect((form.querySelector('[type="submit"]') as HTMLButtonElement).disabled).toBe(true)
    expect(form.querySelector('[data-model-pricing-notice] a')?.getAttribute('href')).toBe('/system-settings/billing/model-pricing')
    request.mockClear()
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(request).not.toHaveBeenCalled()
  })
})
