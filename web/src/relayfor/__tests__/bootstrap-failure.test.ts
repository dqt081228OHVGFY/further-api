import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.fn()

vi.mock('@/i18n/config', () => ({}))
vi.mock('@/lib/api', () => ({ api: { get } }))
vi.mock('@/lib/auth-session', () => ({ bootstrapAuthentication: vi.fn() }))
vi.mock('@/stores/auth-store', () => ({ useAuthStore: { getState: () => ({ auth: { user: null } }) } }))
vi.mock('../native-features', () => ({ installNativeFeatureRuntime: vi.fn() }))
vi.mock('../gateway-request', () => ({ gatewayAuthenticated: vi.fn(), gatewayExportLogs: vi.fn(), gatewayLogout: vi.fn(), gatewayRequest: vi.fn() }))

beforeEach(() => {
  vi.resetModules()
  document.head.innerHTML = '<link rel="icon" href="/relayfor-logo.svg">'
  document.body.dataset.page = 'auth'
  document.documentElement.dataset.relayforBoot = 'pending'
  document.body.innerHTML = '<span class="rail-logo"><img src="/relayfor-logo.svg" alt=""></span><form id="auth-form"><input name="password" type="password"></form>'
  get.mockReset()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

describe('original frontend initialization guard', () => {
  it('blocks native form navigation before and after a failed initialization and offers reload', async () => {
    get.mockRejectedValue(new Error('Request failed with status code 502'))
    await import('../bootstrap')
    const form = document.querySelector('form')!
    const beforeFailure = new Event('submit', { bubbles: true, cancelable: true })
    form.dispatchEvent(beforeFailure)
    expect(beforeFailure.defaultPrevented).toBe(true)
    await window.RelayForGatewayReady
    expect(document.documentElement.dataset.relayforBoot).toBeUndefined()
    const afterFailure = new Event('submit', { bubbles: true, cancelable: true })
    form.dispatchEvent(afterFailure)
    expect(afterFailure.defaultPrevented).toBe(true)
    expect(document.body.dataset.gatewayReady).toBe('false')
    expect(document.querySelector('[role="alert"]')).toHaveTextContent('页面初始化失败')
    expect(document.querySelector('[role="alert"] button')).toHaveTextContent('重新载入')
    expect(document.querySelector('.rail-logo img')).toHaveAttribute('src', '/relayfor-logo.svg')
  })

  it('lets JavaScript auth handlers receive submit events after the bridge is ready', async () => {
    get.mockImplementation(async (url: string) => ({ data: { success: true, data: url === '/api/setup' ? { status: true } : { system_name: 'Gateway', logo: '', quota_per_unit: 500000 } } }))
    await import('../bootstrap')
    await window.RelayForGatewayReady
    expect(document.body.dataset.gatewayReady).toBe('true')
    expect(document.documentElement.dataset.relayforBoot).toBeUndefined()
    const form = document.querySelector('form')!
    const handler = vi.fn()
    form.addEventListener('submit', handler)
    const event = new Event('submit', { bubbles: true, cancelable: true })
    form.dispatchEvent(event)
    expect(handler).toHaveBeenCalledOnce()
    expect(event.defaultPrevented).toBe(true)
  })
})
