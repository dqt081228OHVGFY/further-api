import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PaymentsScreen } from '../feature-workspace/payments'
import { checkoutValue } from '../feature-workspace/checkout-values'
import { useAuthStore } from '@/stores/auth-store'

const requests = vi.hoisted(() => ({ read: vi.fn(), write: vi.fn() }))
vi.mock('../feature-workspace/api', async original => ({
  ...(await original<typeof import('../feature-workspace/api')>()), ...requests,
}))
let info: Record<string, unknown>
beforeEach(() => {
  vi.resetAllMocks()
  info = { pay_methods: [{ provider: 'epay', type: 'alipay', name: '支付宝', currency: 'CNY', min_topup: '1' }] }
  requests.read.mockImplementation(async (path: string) => {
    if (path === '/api/user/topup/info') return info
    if (path === '/api/user/topup/self') return { items: [], total: 0 }
    if (path === '/api/user/ldc/orders') return []
    throw new Error(`Unexpected path ${path}`)
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})

describe('independent gateway payment workspace', () => {
  it('keeps previous orders visible while online checkout is unavailable', async () => {
    info = { pay_methods: [] }
    requests.read.mockImplementation(async (path: string) => {
      if (path === '/api/user/topup/info') return info
      if (path === '/api/user/topup/self') return { items: [{ trade_no: 'paid-order', wallet_units: '20', money: 140, payment_currency: 'CNY', payment_provider: 'epay', status: 'success', create_time: 1780000000 }], total: 1 }
      return []
    })
    const previous=useAuthStore.getState()
    useAuthStore.setState({auth:{...previous.auth,user:{id:1,username:'admin',role:100}}})
    render(<PaymentsScreen />)
    expect(await screen.findByText('暂未开放在线充值。')).toBeVisible()
    expect(await screen.findByText('paid-order')).toBeVisible()
    expect(screen.getByText('已到账')).toBeVisible()
    expect(screen.queryByRole('button', { name: '创建支付订单' })).toBeNull()
    expect(screen.queryByRole('link', { name:'前往支付设置' })).toBeNull()
    expect(requests.write).not.toHaveBeenCalled()
    useAuthStore.setState(previous)
  })
  it('preserves the server signed external POST and shows its frozen amount before payment', async () => {
    requests.write.mockResolvedValue({ trade_no: 'epay-order', url: 'https://merchant.example/submit.php', method: 'POST', params: { sign: 'signed-result', money: '70.00', out_trade_no: 'epay-order' }, wallet_units: '10', money: 70, currency: 'CNY' })
    render(<PaymentsScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '创建支付订单' }))
    const dialog = await screen.findByRole('dialog', { name: '订单已创建' })
    const form = within(dialog).getByRole('button', { name: '前往支付' }).closest('form')!
    expect(form).toHaveAttribute('action', 'https://merchant.example/submit.php')
    expect(form).toHaveAttribute('method', 'POST')
    expect(form.querySelector('[name=sign]')).toHaveValue('signed-result')
    expect(within(dialog).getByText(/实付 70 CNY/)).toBeVisible()
    expect(requests.write).toHaveBeenCalledWith('/api/user/checkout', 'POST', { provider: 'epay', method: 'alipay', amount: '10', return_url: '/wallet' })
  })
  it('uses the selected Creem product and leaves the amount to the server', async () => {
    info = { pay_methods: [{ provider: 'creem', type: 'creem', name: '银行卡', currency: 'EUR', min_topup: '1' }], creem_products: [{ productId: 'small', quota: 20, price: 5, currency: 'EUR' }, { productId: 'large', quota: 100, price: 18, currency: 'EUR' }] }
    requests.write.mockResolvedValue({ trade_no: 'creem-order', url: 'https://checkout.creem.io/example', method: 'GET', wallet_units: '100', money: 18, currency: 'EUR' })
    render(<PaymentsScreen />)
    fireEvent.change(await screen.findByLabelText('充值套餐'), { target: { value: 'large' } })
    fireEvent.click(screen.getByRole('button', { name: '创建支付订单' }))
    expect(await screen.findByRole('link', { name: '前往支付' })).toHaveAttribute('href', 'https://checkout.creem.io/example')
    expect(requests.write).toHaveBeenCalledWith('/api/user/checkout', 'POST', { provider: 'creem', method: 'creem', amount: '100', product_id: 'large', return_url: '/wallet' })
    expect(screen.getByText(/实付 18 EUR/)).toBeVisible()
  })
  it('uses the confirmed LDC quote when creating the order', async () => {
    info = { pay_methods: [{ provider: 'ldc', type: 'ldc', name: 'LINUX DO Credit', currency: 'LDC', min_topup: '1' }] }
    requests.write.mockImplementation(async (path: string) => {
      if (path.endsWith('/quote')) return { kind: 'wallet', wallet_units: '10', credits: '2.50', currency: 'LDC' }
      return { trade_no: 'ldc-order', url: 'https://credit.linux.do/epay/pay/submit.php', method: 'POST', currency: 'LDC', credits: '2.50', params: { sign: 'signed-ldc' } }
    })
    render(<PaymentsScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '核对充值报价' }))
    const dialog = await screen.findByRole('dialog', { name: '确认 LDC 充值' })
    expect(within(dialog).getByText(/实付 2.50 LDC/)).toBeVisible()
    expect(requests.write).toHaveBeenCalledTimes(1)
    fireEvent.click(within(dialog).getByRole('button', { name: '确认创建订单' }))
    expect(await screen.findByRole('dialog', { name: '订单已创建' })).toBeVisible()
    expect(requests.write).toHaveBeenLastCalledWith('/api/user/ldc/pay', 'POST', { amount: '10', expected_credits: '2.50' })
  })
  it('retains the amount and allows retry after checkout fails', async () => {
    requests.write.mockRejectedValueOnce(new Error('支付服务暂时不可用'))
    render(<PaymentsScreen />)
    fireEvent.change(await screen.findByLabelText('充值额度（USD）'), { target: { value: '23' } })
    fireEvent.click(screen.getByRole('button', { name: '创建支付订单' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('支付服务暂时不可用')
    await waitFor(() => expect(screen.getByRole('button', { name: '创建支付订单' })).toBeEnabled())
    expect(screen.getByLabelText('充值额度（USD）')).toHaveValue('23')
    expect(screen.queryByRole('dialog')).toBeNull()
  })
  it('rejects executable URLs and malformed signed checkout forms', () => {
    for (const value of [
      { trade_no: 'x', url: 'javascript:alert(1)', method: 'GET' },
      { trade_no: 'x', url: 'https://user:secret@pay.example', method: 'GET' },
      { trade_no: 'x', url: 'https://pay.example', method: 'POST', params: {} },
    ]) expect(() => checkoutValue(value, 'https://gateway.example')).toThrow()
  })
})
