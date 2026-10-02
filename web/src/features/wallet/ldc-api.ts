import { api } from '@/lib/api'
import { DEFAULT_LOGO } from '@/lib/constants'

export interface LdcInfo {
  enabled: boolean
  configured: boolean
  currency: 'LDC'
  credits_per_unit: string
  min_topup: string
  max_topup?: string
  amount_options: string[]
  wallet_unit: string
}

export interface LdcQuote {
  kind: 'topup' | 'subscription'
  wallet_units: string
  quota: number
  credits: string
  credits_per_unit: string
  currency: 'LDC'
  plan_id?: number
  plan_title?: string
}

export interface LdcOrder {
  trade_no: string
  kind: 'topup' | 'subscription'
  status: string
  credits: string
  currency: 'LDC'
  url: string
  method: 'POST'
  params: Record<string, string | number>
}

export interface LdcOrderRecord {
  trade_no: string
  kind: 'topup' | 'subscription'
  status: string
  credits: string
  wallet_units: string
  quota: number
  plan_title?: string
  created_at: number
  completed_at?: number
}

export async function getLdcOrders(): Promise<LdcOrderRecord[]> {
  return unwrap((await api.get('/api/user/ldc/orders')).data)
}

export interface LdcConfig {
  enabled: boolean
  configured: boolean
  client_id: string
  secret_configured: boolean
  private_key_configured: boolean
  public_key: string
  credits_per_unit: string
  min_topup: string
  max_topup?: string
  callback_base_url: string
  notify_url: string
  return_url: string
}

function unwrap<T>(response: { success?: boolean; message?: string; data?: T }): T {
  if (!response.success || response.data == null) throw new Error(response.message || 'LDC 服务暂不可用')
  return response.data
}

export async function getLdcInfo(): Promise<LdcInfo> {
  return unwrap((await api.get('/api/user/ldc/info')).data)
}

export async function quoteLdc(input: { amount: string } | { plan_id: number }): Promise<LdcQuote> {
  return unwrap((await api.post('/api/user/ldc/quote', input)).data)
}

export async function payLdc(input: { amount: string; expected_credits: string } | { plan_id: number; expected_credits: string }): Promise<LdcOrder> {
  const endpoint = 'plan_id' in input ? '/api/subscription/ldc/pay' : '/api/user/ldc/pay'
  return unwrap((await api.post(endpoint, input)).data)
}

export async function getLdcConfig(): Promise<LdcConfig> {
  return unwrap((await api.get('/api/admin/ldc/config')).data)
}

export async function saveLdcConfig(config: Partial<LdcConfig> & { client_secret?: string; private_key?: string }): Promise<LdcConfig> {
  return unwrap((await api.put('/api/admin/ldc/config', config)).data)
}

// Only signed fields returned by our server are sent to LDC. Creating an order
// is not proof of payment; account credit is reconciled by the server callback.
export function submitLdcOrder(order: LdcOrder): void {
  const target = new URL(order.url)
  if (target.origin !== 'https://credit.linux.do' || target.pathname !== '/epay/pay/submit.php' || target.username || target.password || target.search || target.hash || order.method !== 'POST' || !order.params || !order.trade_no || order.currency !== 'LDC') {
    throw new Error('LDC 支付地址无效，请刷新后重试')
  }
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = target.href
  for (const [name, value] of Object.entries(order.params)) {
    const input = document.createElement('input')
    input.type = 'hidden'; input.name = name; input.value = String(value)
    form.append(input)
  }
  form.hidden = true
  document.body.append(form)
  try { HTMLFormElement.prototype.submit.call(form) } finally { form.remove() }
}

export function sitePaymentLogo(configured?: string): string {
  try {
    const url = new URL(configured || DEFAULT_LOGO, window.location.origin)
    return ['http:', 'https:'].includes(url.protocol) ? url.href : DEFAULT_LOGO
  } catch { return DEFAULT_LOGO }
}
