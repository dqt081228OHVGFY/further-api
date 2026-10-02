import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { payLdc, quoteLdc, sitePaymentLogo, submitLdcOrder, type LdcOrder } from './ldc-api'

vi.mock('@/lib/api', () => ({ api: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))
afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks(); document.body.replaceChildren() })

const order: LdcOrder = { trade_no: 'LDC-100', kind: 'subscription', status: 'pending', credits: '2.75', currency: 'LDC', url: 'https://credit.linux.do/epay/pay/submit.php', method: 'POST', params: { client_id: 'client', sign: 'signed&value', money: '2.75' } }

describe('LDC payment boundary', () => {
  it('requests the server quote and submits the exact confirmed decimal credits', async () => {
    vi.mocked(api.post).mockResolvedValue({ data: { success: true, data: order } })
    await quoteLdc({ amount: '0.125' })
    expect(api.post).toHaveBeenCalledWith('/api/user/ldc/quote', { amount: '0.125' })
    await payLdc({ plan_id: 7, expected_credits: '2.7500' })
    expect(api.post).toHaveBeenCalledWith('/api/subscription/ldc/pay', { plan_id: 7, expected_credits: '2.7500' })
  })
  it('rejects a failed API result without treating it as payment success', async () => {
    vi.mocked(api.post).mockResolvedValue({ data: { success: false, message: '报价已变化' } })
    await expect(payLdc({ amount: '1', expected_credits: '2' })).rejects.toThrow('报价已变化')
  })
  it('posts signed values without interpreting markup and removes the temporary form', () => {
    let submitted: HTMLFormElement | undefined
    vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(function (this: HTMLFormElement) { submitted = this; expect(this.action).toBe(order.url); expect(this.method).toBe('post'); expect(new FormData(this).get('sign')).toBe('signed&value') })
    submitLdcOrder(order)
    expect(submitted).toBeDefined()
    expect(document.querySelector('form')).toBeNull()
  })
  it('refuses payment links outside the LDC HTTPS host', () => {
    const submit = vi.spyOn(HTMLFormElement.prototype, 'submit')
    expect(() => submitLdcOrder({ ...order, url: 'https://credit.linux.do.attacker.test/pay' })).toThrow('支付地址无效')
    expect(() => submitLdcOrder({ ...order, url: 'javascript:alert(1)' })).toThrow('支付地址无效')
    expect(submit).not.toHaveBeenCalled()
  })
  it('uses the configured site logo with the project brand as its default', () => {
    expect(sitePaymentLogo('/assets/site-brand.png')).toBe(new URL('/assets/site-brand.png', location.origin).href)
    expect(sitePaymentLogo('https://external.test/logo.png')).toBe('https://external.test/logo.png')
    expect(sitePaymentLogo()).toBe(new URL('/relayfor-logo.svg', location.origin).href)
  })
})
