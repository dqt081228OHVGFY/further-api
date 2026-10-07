import { asRow, type Row } from './api'

export type Checkout = {
  url: string
  method: 'GET' | 'POST'
  tradeNo: string
  params: Record<string, string>
  walletUnits: string
  money: string
  currency: string
}

export function checkoutValue(value: unknown, origin: string, ldc = false): Checkout {
  const row = asRow(value)
  const url = new URL(String(row.url || ''), origin)
  if (!row.url || !row.trade_no || url.username || url.password ||
      (url.protocol !== 'https:' && !(url.protocol === 'http:' && url.origin === origin)) ||
      !['GET', 'POST'].includes(String(row.method))) throw new Error('支付服务返回的订单地址无效，请重试。')
  if (ldc && (url.origin !== 'https://credit.linux.do' || url.pathname !== '/epay/pay/submit.php' ||
      url.search || url.hash || row.method !== 'POST' || row.currency !== 'LDC')) throw new Error('LDC 支付订单无效，请重试。')
  const params = asRow(row.params)
  if (row.method === 'POST' && (!Object.keys(params).length || Object.values(params).some(item => typeof item !== 'string'))) {
    throw new Error('支付表单无效，请重新创建订单。')
  }
  return {
    url: url.href, method: row.method as 'GET' | 'POST', tradeNo: String(row.trade_no),
    params: params as Record<string, string>, walletUnits: String(row.wallet_units || ''),
    money: String(row.money || row.credits || ''), currency: String(row.currency || ''),
  }
}

export function paymentHistory(external: Row[], ldc: Row[]): Row[] {
  const orders = new Map<string, Row>()
  for (const row of external) { orders.set(String(row.trade_no), {
    ...row, provider: row.payment_provider || row.payment_method,
    created: row.create_time, units: row.wallet_units || row.amount,
    cost: row.money, currency: row.payment_currency || '',
  }) }
  for (const row of ldc) { orders.set(String(row.trade_no), {
    ...row, provider: 'ldc', created: row.created_at, units: row.wallet_units,
    cost: row.credits, currency: 'LDC',
  }) }
  return [...orders.values()].sort((left, right) => Number(right.created) - Number(left.created))
}
