import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, expect, it, vi } from 'vitest'

const root = resolve(process.cwd(), '../further-ui')
const markup = readFileSync(resolve(root, 'admin/index.html'), 'utf8')
const runtime = readFileSync(resolve(root, 'assets/admin/admin-settings.js'), 'utf8')
const config = { enabled: true, configured: true, client_id: 'merchant', secret_configured: true, credits_per_unit: '10', min_topup: '1', callback_base_url: 'https://relay.example', public_key: 'merchant-public', notify_url: 'https://relay.example/api/ldc/notify', return_url: 'https://relay.example/api/ldc/return' }
const request = vi.fn()
const toast = vi.fn()
type Runtime = Window & { FurtherUIAdminSettings: { bind(): void; load(): Promise<void> } }
const runtimeApi = () => (window as unknown as Runtime).FurtherUIAdminSettings
const form = () => document.querySelector('[data-payment-settings]') as HTMLFormElement
const field = (name: string) => form().elements.namedItem(name) as HTMLInputElement

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
  request.mockReset(); toast.mockReset()
  request.mockImplementation(async (path: string) => path === '/api/admin/ldc/config' ? config : { settings: {}, announcements: {} })
  Object.assign(window, { FurtherUI: { request, toast, syncQuotaUSD() {}, quotaFromUSD(value: number) { return value * 500000 }, showBusy() {}, setText(selector: string, value: string) { document.querySelectorAll(selector).forEach(element => { element.textContent = value }) } } })
  window.eval(runtime)
  runtimeApi().bind()
})

it('opens the native payment form and reads merchant configuration without an agreement', async () => {
  ;(document.querySelector('[data-settings-page="payment"]') as HTMLButtonElement).click()
  await runtimeApi().load()
  expect(document.querySelector('[data-settings-subpage="payment"]')).not.toHaveAttribute('hidden')
  expect(field('client_id').value).toBe('merchant')
  expect(field('client_secret').value).toBe('')
  expect(field('public_key').value).toBe('merchant-public')
  expect(document.querySelector('[data-payment-notify]')).toHaveTextContent(config.notify_url)
  expect(form().textContent).not.toMatch(/合规|同意|New API/)
})

it('saves native payment configuration while preserving empty secrets', async () => {
  await runtimeApi().load()
  field('credits_per_unit').value = '12'
  form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('支付配置已保存'))
  expect(request).toHaveBeenCalledWith('/api/admin/ldc/config', { method: 'PUT', body: { enabled: true, client_id: 'merchant', credits_per_unit: '12', min_topup: '1', callback_base_url: 'https://relay.example' } })
  expect(request.mock.calls.some(call => call[0].includes('payment_compliance'))).toBe(false)
})

it('rejects invalid amounts and non-HTTPS callbacks before saving', async () => {
  await runtimeApi().load()
  field('min_topup').value = '0'
  form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  expect(toast).toHaveBeenCalledWith(expect.stringContaining('大于 0'), 'danger')
  field('min_topup').value = '1'
  field('callback_base_url').value = 'http://relay.example'
  form().dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  expect(toast).toHaveBeenCalledWith(expect.stringContaining('HTTPS'), 'danger')
  expect(request.mock.calls.some(call => call[1]?.method === 'PUT')).toBe(false)
})

it('keeps saving unavailable after a failed initial load and allows retry', async () => {
  request.mockImplementation(async (path: string) => { if (path === '/api/admin/ldc/config') throw new Error('暂时不可用'); return { settings: {}, announcements: {} } })
  await runtimeApi().load()
  expect(document.querySelector('[data-payment-fields]')).toBeDisabled()
  expect(document.querySelector('[data-payment-status]')).toHaveTextContent('读取失败')
  request.mockResolvedValue(config)
  ;(document.querySelector('[data-payment-reload]') as HTMLButtonElement).click()
  await vi.waitFor(() => expect(document.querySelector('[data-payment-fields]')).not.toBeDisabled())
})
