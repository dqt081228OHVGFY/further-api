import { api } from '@/lib/api'
import {
  getServerErrorMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

type RecordData = Record<string, unknown>
type RequestOptions = { method?: string; body?: unknown }
type Handled = { handled: boolean; data?: unknown }
type Envelope = {
  success: boolean
  message?: string
  data?: unknown
} & RecordData

function record(value: unknown): RecordData {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('服务器返回的数据格式不正确')
  }
  return value as RecordData
}

function rows(value: unknown): RecordData[] {
  if (value === null) return [] // A successful Go nil slice represents no rows.
  if (!Array.isArray(value)) throw new Error('服务器未返回有效列表')
  return value.map(record)
}

function number(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

function isoTime(value: unknown): string | null {
  const seconds = number(value)
  return seconds > 0 ? new Date(seconds * 1000).toISOString() : null
}

async function getEnvelope(path: string): Promise<Envelope> {
  const response = requireServerSuccess((await api.get<Envelope>(path)).data)
  if (response.success !== true) throw new Error('服务器未确认请求成功')
  return response
}

async function getData(path: string): Promise<unknown> {
  const response = await getEnvelope(path)
  if (!('data' in response)) throw new Error('服务器未返回请求的数据')
  return response.data
}

async function mutate(
  method: string,
  path: string,
  body?: unknown
): Promise<Envelope> {
  let response: Envelope
  if (method === 'DELETE') response = (await api.delete<Envelope>(path)).data
  else if (method === 'PUT') {
    response = (await api.put<Envelope>(path, body)).data
  } else response = (await api.post<Envelope>(path, body)).data
  const result = requireServerSuccess(response)
  if (result.success !== true) throw new Error('服务器未确认操作成功')
  return result
}

// RelayFor displays every API key in one list. Follow server pagination so
// keys after the first page are not silently omitted.
async function allPages(
  path: string,
  params = new URLSearchParams(),
  limit = 10000
): Promise<RecordData[]> {
  const result: RecordData[] = []
  for (let page = 1; ; page++) {
    const query = new URLSearchParams(params)
    query.set('p', String(page))
    query.set('size', '100')
    query.set('page_size', '100')
    const payload = record(await getData(`${path}?${query}`))
    if (
      typeof payload.total !== 'number' ||
      !Number.isInteger(payload.total) ||
      payload.total < 0
    ) {
      throw new Error('服务器未返回有效分页总数')
    }
    const total = payload.total
    if (total > limit) {
      throw new Error('API Key 超过 10000 个，请通过分页密钥管理页面查看')
    }
    const items = rows(payload.items)
    result.push(...items)
    if (result.length >= total) return result
    if (!items.length) throw new Error('分页数据不完整，请刷新后重试')
  }
}

function tokenForRelayFor(token: RecordData): RecordData {
  const modelLimits =
    typeof token.model_limits === 'string'
      ? token.model_limits
          .split(',')
          .map((value) => value.trim())
          .filter(Boolean)
      : []
  return {
    ...token,
    model_limits: token.model_limits_enabled ? modelLimits : [],
    ip_limits: token.allow_ips ?? '',
    expired_at: isoTime(token.expired_time),
    created_at: isoTime(token.created_time),
    accessed_at: isoTime(token.accessed_time),
  }
}

function tokenForNewAPI(
  body: RecordData,
  existing: RecordData = {}
): RecordData {
  const modelLimits = Array.isArray(body.model_limits)
    ? body.model_limits
        .filter((value): value is string => typeof value === 'string')
        .join(',')
    : String(body.model_limits ?? '')
  const expiration = body.expires_at
    ? Date.parse(String(body.expires_at))
    : -1000
  if (!Number.isFinite(expiration)) throw new Error('API Key 到期时间无效')
  const quota = Number(body.remain_quota)
  if (!Number.isFinite(quota)) throw new Error('API Key 额度无效')
  // RelayFor's -1 follows the account balance; New API calls this unlimited_quota.
  const unlimited = body.unlimited_quota === true || quota === -1
  return {
    name: body.name,
    remain_quota: unlimited ? 0 : quota,
    unlimited_quota: unlimited,
    expired_time: Math.floor(expiration / 1000),
    model_limits_enabled: modelLimits.length > 0,
    model_limits: modelLimits,
    allow_ips: body.ip_limits ?? '',
    group: existing.group ?? '',
    auto_groups: existing.auto_groups ?? null,
    cross_group_retry: existing.cross_group_retry ?? false,
  }
}

async function legacyModels(): Promise<RecordData> {
  const [pricing, allowedValue, selfValue] = await Promise.all([
    getEnvelope('/api/pricing'),
    getData('/api/user/models'),
    getData('/api/user/self'),
  ])
  if (!Array.isArray(allowedValue)) throw new Error('服务器未返回可用模型')
  const allowed = new Set(allowedValue)
  const self = record(selfValue)
  const group = String(self.group || 'default')
  const groupRatios = record(pricing.group_ratio)
  const multiplier =
    typeof groupRatios[group] === 'number'
      ? (groupRatios[group] as number)
      : null
  const endpointMap = pricing.supported_endpoint
    ? record(pricing.supported_endpoint)
    : {}
  const items = rows(pricing.data).map((model) => {
    const dynamic =
      model.billing_mode === 'tiered_expr' || Boolean(model.billing_expr)
    const requestBilled = model.quota_type === 1
    let input: number | null = null
    if (!dynamic && typeof model.model_ratio === 'number') {
      input = model.model_ratio * 2
    }
    const base = {
      input,
      output:
        input !== null && typeof model.completion_ratio === 'number'
          ? input * model.completion_ratio
          : null,
      cache_read:
        input !== null && typeof model.cache_ratio === 'number'
          ? input * model.cache_ratio
          : null,
      cache_write:
        input !== null && typeof model.create_cache_ratio === 'number'
          ? input * model.create_cache_ratio
          : null,
    }
    const effective = Object.fromEntries(
      Object.entries(base).map(([key, value]) => [
        key,
        value !== null && multiplier !== null ? value * multiplier : null,
      ])
    )
    const price =
      dynamic || typeof model.model_price !== 'number'
        ? null
        : model.model_price
    return {
      ...model,
      model: model.model_name,
      display_name: model.model_name,
      accessible: allowed.has(model.model_name),
      access_reason: allowed.has(model.model_name)
        ? undefined
        : '当前用户组没有此模型权限',
      billing_mode: requestBilled ? 'request' : 'token',
      price_display: dynamic ? '动态计费' : undefined,
      dynamic_pricing: dynamic,
      base_prices: base,
      effective_prices: effective,
      base_request_price: price,
      effective_request_price:
        price !== null && multiplier !== null ? price * multiplier : null,
      price_multiplier: 1,
      effective_multiplier: multiplier,
      free_model: !dynamic && (requestBilled ? price === 0 : input === 0),
      supported_endpoints: Array.isArray(model.supported_endpoint_types)
        ? model.supported_endpoint_types.map((type) => {
            const endpoint = endpointMap[String(type)]
            let value: unknown = type
            if (typeof endpoint === 'string') value = endpoint
            else if (endpoint && typeof endpoint === 'object') {
              value = record(endpoint).path
            }
            return String(value ?? type).replaceAll(
              '{model}',
              String(model.model_name)
            )
          })
        : [],
    }
  })
  return { group, group_multiplier: multiplier, items }
}

/** Translate only confirmed contracts; remaining RelayFor routes reach the real compatibility backend. */
export async function handleConsoleRequest(
  path: string,
  options: RequestOptions = {}
): Promise<Handled> {
  if (!path.startsWith('/api/user/')) return { handled: false }
  const url = new URL(path, 'http://relayfor.local')
  const endpoint = url.pathname.replace(/\/$/, '')
  const method = (options.method || 'GET').toUpperCase()
  const body = options.body === undefined ? {} : record(options.body)

  if (method === 'GET' && endpoint === '/api/user/models') {
    return { handled: true, data: await legacyModels() }
  }
  if (method === 'GET' && endpoint === '/api/user/tokens') {
    return {
      handled: true,
      data: (await allPages('/api/token/')).map(tokenForRelayFor),
    }
  }
  if (method === 'POST' && endpoint === '/api/user/tokens') {
    const payload = tokenForNewAPI(body)
    const created = await mutate('POST', '/api/token/', payload)
    const createdToken = record(created.data)
    const id = number(createdToken.id)
    if (id <= 0) {
      throw new Error(
        'API Key 已创建，但服务器未返回 ID，请刷新密钥列表查看，勿重复创建'
      )
    }
    const token = record(await getData(`/api/token/${id}`))
    const revealed = record(await getDataForKey(number(token.id)))
    return {
      handled: true,
      data: { ...tokenForRelayFor(token), key: revealed.key },
    }
  }
  const tokenMatch = endpoint.match(/^\/api\/user\/tokens\/(\d+)$/)
  if (tokenMatch) {
    const id = Number(tokenMatch[1])
    if (method === 'GET') {
      return {
        handled: true,
        data: tokenForRelayFor(record(await getData(`/api/token/${id}`))),
      }
    }
    if (method === 'DELETE') {
      return {
        handled: true,
        data: await mutate('DELETE', `/api/token/${id}/`),
      }
    }
    if (method === 'PUT') {
      if ('status' in body && Object.keys(body).length === 1) {
        await mutate('PUT', '/api/token/?status_only=true', {
          id,
          status: body.status,
        })
      } else {
        const current = record(await getData(`/api/token/${id}`))
        await mutate('PUT', '/api/token/', {
          id,
          ...tokenForNewAPI(body, current),
        })
      }
      return {
        handled: true,
        data: tokenForRelayFor(record(await getData(`/api/token/${id}`))),
      }
    }
  }
  if (method === 'POST' && endpoint === '/api/user/redeem') {
    const response = await mutate('POST', '/api/user/topup', { key: body.code })
    if (typeof response.data !== 'number') {
      throw new Error('兑换已提交，但服务器未返回充值额度，请刷新账户核实')
    }
    return {
      handled: true,
      data: { redeem_type: 'quota', added_quota: response.data },
    }
  }
  if (method === 'POST' && endpoint === '/api/user/redeem/batch') {
    if (
      !Array.isArray(body.codes) ||
      body.codes.some((code) => typeof code !== 'string')
    ) {
      throw new Error('兑换码列表格式无效')
    }
    if (body.codes.length < 1 || body.codes.length > 100) {
      throw new Error('单次兑换码数量须为 1 至 100 个')
    }
    const items = []
    for (const code of body.codes) {
      try {
        const response = await handleConsoleRequest('/api/user/redeem', {
          method: 'POST',
          body: { code },
        })
        items.push({ code, success: true, ...record(response.data) })
      } catch (error) {
        items.push({
          code,
          success: false,
          error: getServerErrorMessage(error),
        })
      }
    }
    const succeeded = items.filter((item) => item.success).length
    const failed = items.length - succeeded
    return {
      handled: true,
      data: {
        items,
        succeeded,
        failed,
        message: `已完成：${succeeded} 个成功，${failed} 个失败`,
      },
    }
  }
  if (method === 'PATCH' && endpoint === '/api/user/profile') {
    const current = record(await getData('/api/user/self'))
    await mutate('PUT', '/api/user/self', {
      username: body.username,
      display_name: current.display_name,
    })
    return { handled: true, data: { user: await getData('/api/user/self') } }
  }
  if (method === 'GET' && endpoint === '/api/user/security/totp') {
    const status = record(await getData('/api/user/2fa/status'))
    return {
      handled: true,
      data: {
        ...status,
        totp_enabled: status.enabled,
        login_required: status.enabled,
        key_required: false,
        abandon_required: false,
      },
    }
  }
  // Preferences, per-action TOTP policies, password changes, token rotation,
  // timed quotas and welfare require genuine backend/proof-flow equivalents.
  return { handled: false }
}

async function getDataForKey(id: number): Promise<unknown> {
  const response = await mutate('POST', `/api/token/${id}/key`)
  const data = record(response.data)
  if (typeof data.key !== 'string' || !data.key) {
    throw new Error('服务器未返回可用 API Key')
  }
  return data
}
