import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/admin.js'), 'utf8')
const markup = readFileSync(resolve(process.cwd(), '../further-ui/admin/index.html'), 'utf8')
const request = vi.fn(), toast = vi.fn(), openDialog = vi.fn(), closeDialog = vi.fn()
type Harness = { bind(): void; reset(): void }
let harness: Harness
const form = () => document.querySelector('[data-redeem-form]') as HTMLFormElement
const field = (name: string) => form().elements.namedItem(name) as HTMLInputElement
const submit = () => form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))

beforeEach(() => {
  const documentSource = new DOMParser().parseFromString(markup, 'text/html')
  document.body.innerHTML = `${documentSource.querySelector('#redeem-dialog')!.outerHTML}${documentSource.querySelector('#generated-dialog')!.outerHTML}<div data-redemption-rows></div><span data-redemption-page></span><button data-redemption-prev></button><button data-redemption-next></button>`
  request.mockReset().mockImplementation((path) => path.endsWith('/generate') ? Promise.resolve({ codes: ['DIRECT-CODE'] }) : Promise.resolve({ items: [], total: 0, total_pages: 0 }))
  toast.mockReset(); openDialog.mockReset(); closeDialog.mockReset()
  Object.assign(window, {
    FurtherUI: {
      request, toast, openDialog, closeDialog,
      number: String, date: String, adminListPreference: () => false,
      escapeHTML: (value: unknown) => String(value),
      showBusy: (button: HTMLButtonElement, value: boolean) => { button.disabled = value },
      setText: (selector: string, value: string) => document.querySelectorAll(selector).forEach((element) => { element.textContent = value }),
    },
    FurtherUIAdminModels: { load: vi.fn() }, FurtherUIAdminSettings: { load: vi.fn() }, FurtherUIAdminOverview: { load: vi.fn() },
    FurtherUIResources: { create: () => ({}) },
  })
  window.eval(runtime.replace(/\}\(\)\);\s*$/, 'window.__redemptionGeneration = {bind: bindRedeemForm, reset: resetRedeemForm}; }());'))
  harness = (window as unknown as { __redemptionGeneration: Harness }).__redemptionGeneration
  harness.bind(); harness.reset()
})

describe('direct redemption generation', () => {
  it('defaults to one code and immediately displays returned codes for copying', async () => {
    expect(field('count').value).toBe('1')
    expect(form().querySelector('[type="submit"]')?.textContent).toBe('立即生成')
    submit()
    await vi.waitFor(() => expect(openDialog).toHaveBeenCalledWith('generated-dialog'))
    expect(request).toHaveBeenCalledWith('/api/admin/redemptions/generate', expect.objectContaining({ method: 'POST', body: expect.objectContaining({ count: 1, redeem_type: 'quota' }) }))
    const sent = request.mock.calls[0][1].body
    expect(sent).not.toHaveProperty('user_group'); expect(sent).not.toHaveProperty('batch_id')
    expect((document.querySelector('[data-generated-codes]') as HTMLTextAreaElement).value).toBe('DIRECT-CODE')
    expect(document.querySelector('[data-generated-redemption-title]')?.textContent).toContain('可直接复制')
  })

  it('prevents duplicate submissions and does not reset a pending issuance', async () => {
    let resolve!: (value: unknown) => void
    const pending = new Promise((done) => { resolve = done })
    request.mockImplementation((path) => path.endsWith('/generate') ? pending : Promise.resolve({ items: [], total: 0, total_pages: 0 }))
    submit(); harness.reset(); submit()
    expect(request).toHaveBeenCalledTimes(1)
    expect(form().dataset.saving).toBe('true')
    resolve({ codes: ['ONE-ISSUANCE'] })
    await vi.waitFor(() => expect(form().dataset.saving).toBe('false'))
    expect((document.querySelector('[data-generated-codes]') as HTMLTextAreaElement).value).toBe('ONE-ISSUANCE')
  })

  it('keeps generated codes available when the records refresh fails', async () => {
    request.mockImplementation((path) => path.endsWith('/generate') ? Promise.resolve({ codes: ['SAVED-CODE'] }) : Promise.reject(new Error('network unavailable')))
    submit()
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith(expect.stringContaining('兑换码已保存'), 'danger'))
    expect((document.querySelector('[data-generated-codes]') as HTMLTextAreaElement).value).toBe('SAVED-CODE')
  })
})
