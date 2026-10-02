import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../relayfor-ui/assets/app.js'), 'utf8')
const request = vi.fn()
const logout = vi.fn()
type RuntimeAPI = {
  moneyFromQuota(value: number, digits?: number): string
  quotaFromUSD(value: number): number
  usdFromQuota(value: number): string
  bindQuotaUSDFields(root: ParentNode): void
}
const app = () => (window as unknown as { ProxyLan: RuntimeAPI }).ProxyLan

beforeAll(() => { window.eval(runtime) })
beforeEach(() => {
  vi.useFakeTimers()
  document.body.dataset.page = 'public'
  document.body.innerHTML = '<div data-public-guest><a href="/auth/">登录</a><a data-public-register>注册</a></div><div data-public-user class="hidden"><span data-public-username></span><a data-public-destination><span data-public-destination-label></span></a><button data-public-logout>退出</button></div>'
  request.mockReset().mockResolvedValue({ username: 'verified-user', role: 1 })
  logout.mockReset().mockResolvedValue(undefined)
  Object.assign(window, {
    RelayForQuotaPerUnit: 200000,
    RelayForGateway: { request, authenticated: () => true, logout },
    ProxyLanSite: {
      loadAuthConfig: async () => ({ system_name: 'Gateway', local_registration_enabled: true }),
      loadAnnouncement: async () => undefined, applyContext() {},
    },
  })
})
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

describe('authoritative quota and public authentication state', () => {
  it('uses the current server unit for display and money entry rather than a frozen or hardcoded divisor', () => {
    expect(app().moneyFromQuota(300000)).toBe('$1.50')
    expect(app().quotaFromUSD(1.5)).toBe(300000)
    Object.assign(window, { RelayForQuotaPerUnit: 400000 })
    expect(app().moneyFromQuota(300000)).toBe('$0.75')
    expect(app().usdFromQuota(400000)).toBe('1')
    document.body.innerHTML += '<form><input name="quota" value="400000"><input data-quota-usd-for="quota"></form>'
    app().bindQuotaUSDFields(document)
    const usd = document.querySelector<HTMLInputElement>('[data-quota-usd-for]')!
    expect(usd.value).toBe('1')
    usd.value = '1.25'
    usd.dispatchEvent(new Event('input', { bubbles: true }))
    expect(document.querySelector<HTMLInputElement>('[name="quota"]')!.value).toBe('500000')
  })

  it('does not invent a conversion when the authoritative unit is unavailable', () => {
    Object.assign(window, { RelayForQuotaPerUnit: 0 })
    expect(() => app().moneyFromQuota(100)).toThrow('额度换算尚未加载')
    expect(() => app().quotaFromUSD(1)).toThrow('额度换算尚未加载')
  })

  it('shows a retry state after a session network failure and only displays a verified user after retry', async () => {
    request.mockRejectedValueOnce(new Error('network unavailable'))
    document.dispatchEvent(new Event('relayfor:ready'))
    await vi.waitFor(() => expect(document.querySelector('[data-public-session-status]')).not.toBeNull())
    expect(document.querySelector('[data-public-user]')!.classList.contains('hidden')).toBe(true)
    expect(document.querySelector('[data-public-guest]')!.classList.contains('hidden')).toBe(true)
    expect(logout).not.toHaveBeenCalled()
    ;(document.querySelector('[data-public-session-status] button') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(document.querySelector('[data-public-username]')!.textContent).toBe('verified-user'))
    expect(document.querySelector('[data-public-session-status]')).toBeNull()
    expect(document.querySelector('[data-public-user]')!.classList.contains('hidden')).toBe(false)
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('clears a rejected session and returns to guest actions without synthesizing a logged-in identity', async () => {
    request.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { response: { status: 401 } }))
    document.dispatchEvent(new Event('relayfor:ready'))
    await vi.waitFor(() => expect(logout).toHaveBeenCalledOnce())
    expect(document.querySelector('[data-public-guest]')!.classList.contains('hidden')).toBe(false)
    expect(document.querySelector('[data-public-user]')!.classList.contains('hidden')).toBe(true)
    expect(document.querySelector('[data-public-session-status]')).toBeNull()
  })
})
