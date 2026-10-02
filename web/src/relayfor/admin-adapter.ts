import { api } from '@/lib/api'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'
import { requireServerSuccess } from '@/lib/server-error-message'

import type { LegacyRequestOptions } from './gateway-request'

type Data = Record<string, unknown>
type Envelope = Data & { success: boolean; data?: unknown }
type Handled = { handled: boolean; data?: unknown }
const channelBalanceReports = new Map<number, Data>()

function object(value: unknown): Data {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('服务器数据格式不正确')
  }
  return value as Data
}
function rows(value: unknown): Data[] {
  if (value === null) return []
  if (!Array.isArray(value)) throw new Error('服务器未返回有效列表')
  return value.map(object)
}
function num(value: unknown, fallback = 0): number {
  return value !== null && value !== undefined && Number.isFinite(Number(value))
    ? Number(value)
    : fallback
}
function int(
  value: unknown,
  minimum = 0,
  maximum = Number.MAX_SAFE_INTEGER
): number {
  const number = Number(value)
  if (!Number.isSafeInteger(number) || number < minimum || number > maximum) {
    throw new Error('数值超出允许范围')
  }
  return number
}
function iso(value: unknown): string | null {
  return num(value) > 0 ? new Date(num(value) * 1000).toISOString() : null
}
function json(value: unknown): Data {
  if (typeof value === 'string') {
    return value.trim() ? object(JSON.parse(value)) : {}
  }
  return value ? object(value) : {}
}
function channelModelNames(channel: Data): string[] {
  return String(channel.models ?? '').split(',').map((name) => name.trim()).filter(Boolean)
}
function unsupported(fields: string): never {
  throw new Error(`原页面字段尚无等价执行规则：${fields}。本次未提交这些设置。`)
}
function assertDefault(body: Data, key: string, expected: unknown): void {
  if (
    key in body &&
    (typeof expected === 'number'
      ? Number(body[key]) !== expected
      : body[key] !== expected)
  ) {
    unsupported(key)
  }
}
async function request(
  path: string,
  method = 'GET',
  body?: unknown
): Promise<Envelope> {
  const response =
    method === 'GET'
      ? await api.get<Envelope>(path)
      : await api.request<Envelope>({ url: path, method, data: body })
  const value = requireServerSuccess(response.data)
  if (value.success !== true) throw new Error('服务器未确认操作成功')
  return value
}
async function get(
  path: string,
  method = 'GET',
  body?: unknown
): Promise<unknown> {
  const value = await request(path, method, body)
  if (!('data' in value)) throw new Error('服务器未返回请求的数据')
  return value.data
}
async function all(
  path: string,
  params = new URLSearchParams()
): Promise<Data[]> {
  const result: Data[] = []
  for (let page = 1; ; page++) {
    const query = new URLSearchParams(params)
    query.set('p', String(page))
    query.set('page_size', '100')
    const payload = object(await get(`${path}?${query}`))
    const total = int(payload.total)
    if (total > 10000) throw new Error('列表超过10000条，请缩小筛选范围')
    const items = rows(payload.items)
    result.push(...items)
    if (result.length >= total) return result
    if (!items.length) throw new Error('分页数据不完整，请刷新后重试')
  }
}
function pagination(query: URLSearchParams): URLSearchParams {
  const result = new URLSearchParams(query)
  result.set('p', String(int(query.get('page') ?? 1, 1, 10000)))
  result.set('page_size', String(int(query.get('page_size') ?? 25, 1, 100)))
  result.delete('page')
  return result
}
function page(
  payload: Data,
  size: number,
  transform: (item: Data) => Data
): Data {
  return {
    ...payload,
    items: rows(payload.items).map(transform),
    total_pages: Math.ceil(int(payload.total) / size),
  }
}
function legacyRole(role: unknown): number {
  if (num(role) >= 100) return 100
  return num(role) >= 10 ? 50 : num(role)
}
function newRole(role: unknown): number {
  const value = int(role)
  if (value === 50) return 10
  if (value === 1 || value === 100) return value
  return unsupported('旧操作员角色，不能转为更高权限管理员')
}

const providerNames: Record<number, string> = {
  0: 'Unknown',
  1: 'OpenAI',
  2: 'Midjourney',
  3: 'Azure',
  4: 'Ollama',
  5: 'Midjourney Plus',
  6: 'OpenAI Max',
  7: 'OhMyGPT',
  8: 'Custom',
  9: 'AILS',
  10: 'AI Proxy',
  11: 'PaLM',
  12: 'API2GPT',
  13: 'AIGC2D',
  14: 'Anthropic',
  15: 'Baidu',
  16: 'Zhipu',
  17: 'Ali',
  18: 'Xunfei',
  19: '360',
  20: 'OpenRouter',
  21: 'AI Proxy Library',
  22: 'FastGPT',
  23: 'Tencent',
  24: 'Gemini',
  25: 'Moonshot',
  26: 'Zhipu v4',
  27: 'Perplexity',
  31: 'LingYiWanWu',
  33: 'AWS',
  34: 'Cohere',
  35: 'MiniMax',
  36: 'SunoAPI',
  37: 'Dify',
  38: 'Jina',
  39: 'Cloudflare',
  40: 'SiliconFlow',
  41: 'Vertex AI',
  42: 'Mistral',
  43: 'DeepSeek',
  44: 'MokaAI',
  45: 'VolcEngine',
  46: 'Baidu V2',
  47: 'Xinference',
  48: 'xAI',
  49: 'Coze',
  50: 'Kling',
  51: 'Jimeng',
  52: 'Vidu',
  53: 'Submodel',
  54: 'Doubao Video',
  55: 'Sora',
  56: 'Replicate',
  57: 'Codex',
  58: 'Advanced Custom',
  59: 'Sub2API',
  60: 'New API',
  61: 'Task Plugin',
  62: 'vLLM',
  63: 'SGLang',
}
function legacyType(type: unknown): number {
  if (num(type) === 14) return 2
  return num(type) === 1 ? 1 : 3
}
function channelFormat(type: number): string {
  if (type === 14) return 'anthropic'
  return type === 57 || type === 59 ? 'openai_responses' : 'openai_chat'
}
function channelView(channel: Data, policy: Data = {}): Data {
  const info = json(channel.channel_info),
    type = int(channel.type)
  return {
    ...channel,
    type: legacyType(type),
    newapi_type: type,
    provider_name: providerNames[type] ?? `New API 类型 ${type}`,
    groups: channel.group,
    key: '',
    key_count: info.is_multi_key ? num(info.multi_key_size) : 1,
    upstream_format: channelFormat(type),
    upstream_formats: [channelFormat(type)],
    quota_total: num(policy.quota_total, -1),
    quota_remaining: num(policy.quota_remaining, -1),
    requests_per_minute: num(policy.requests_per_minute),
    concurrent_requests: num(policy.concurrent_requests),
    quota_policy_available: false,
    capacity_policy_available: false,
    health_unavailable: true,
    last_probe: null,
    created_at: iso(channel.created_time),
  }
}
async function saveChannel(id: number | null, body: Data, snapshot?: Data): Promise<Envelope> {
  const previous = snapshot ?? (id === null ? {} : object(await get(`/api/channel/${id}`)))
  const inputType = int(body.type ?? (id === null ? 1 : legacyType(previous.type)), 1)
  // The three-option legacy form must not turn Gemini/Azure/Midjourney into Anthropic.
  let type =
    ({ 1: 1, 2: 14, 3: 8 } as Record<number, number>)[inputType] ?? inputType
  if (body.newapi_type !== undefined) type = int(body.newapi_type, 1)
  else if (id !== null && inputType === legacyType(previous.type)) {
    type = int(previous.type)
  }
  for (const key of ['quota_total', 'quota_remaining']) {
    assertDefault(body, key, -1)
  }
  for (const key of ['requests_per_minute', 'concurrent_requests']) {
    assertDefault(body, key, 0)
  }
  for (const key of ['data_train', 'remote_compaction']) {
    assertDefault(body, key, false)
  }
  if (
    'upstream_format' in body &&
    body.upstream_format !== channelFormat(type)
  ) {
    unsupported('upstream_format')
  }
  if (
    'upstream_formats' in body &&
    JSON.stringify(body.upstream_formats) !==
      JSON.stringify([channelFormat(type)])
  ) {
    unsupported('upstream_formats')
  }
  const payload: Data = {
    ...previous,
    type,
    group: body.groups ?? previous.group ?? 'default',
    priority: int(body.priority ?? previous.priority ?? 0, -2147483648, 2147483647),
    weight: int(body.weight ?? previous.weight ?? 1),
  }
  delete payload.status
  for (const key of [
    'name', 'base_url', 'models', 'model_mapping', 'tag', 'remark',
    'test_model', 'openai_organization', 'other', 'status_code_mapping',
    'param_override', 'header_override', 'setting', 'settings',
  ]) {
    if (key in body) payload[key] = String(body[key] ?? '')
  }
  if ('auto_ban' in body) payload.auto_ban = int(body.auto_ban, 0, 1)
  const key = String(body.key ?? '').trim()
  if (key) payload.key = key
  else delete payload.key
  if (id === null) {
    if (!key) throw new Error('创建渠道必须提供密钥')
    const mode = String(body.multi_key_mode ?? (key.includes('\n') ? 'multi_to_single' : 'single'))
    if (!['single', 'multi_to_single', 'batch'].includes(mode)) {
      throw new Error('渠道创建模式无效')
    }
    if (mode === 'single' && key.includes('\n')) {
      throw new Error('多个 Key 请选用合并或批量创建模式')
    }
    const multiKeyType = String(body.multi_key_type ?? 'random')
    if (!['random', 'polling'].includes(multiKeyType)) {
      throw new Error('多 Key 策略无效')
    }
    return request('/api/channel/', 'POST', {
      channel: { ...payload, status: 1 },
      mode,
      multi_key_mode: multiKeyType,
      batch_add_set_key_prefix_2_name: body.batch_add_set_key_prefix_2_name === true,
    })
  }
  if (body.multi_key_type) {
    const multiKeyType = String(body.multi_key_type)
    if (!['random', 'polling'].includes(multiKeyType)) {
      throw new Error('多 Key 策略无效')
    }
    payload.multi_key_mode = multiKeyType
  }
  if (key && body.key_mode) {
    const keyMode = String(body.key_mode)
    if (!['append', 'replace'].includes(keyMode)) {
      throw new Error('渠道 Key 编辑模式无效')
    }
    payload.key_mode = keyMode
  }
  const result = await request('/api/channel/', 'PUT', { ...payload, id })
  if (['quota_total', 'quota_remaining', 'requests_per_minute', 'concurrent_requests'].some((field) => field in body)) {
    try {
      await request(`/api/relayfor/channels/${id}/policy`, 'PUT', {
        quota_total: -1, quota_remaining: -1,
        requests_per_minute: 0, concurrent_requests: 0,
      })
    } catch (error) {
      throw new Error(`渠道资料已保存，渠道策略保存失败：${error instanceof Error ? error.message : '服务器错误'}。请刷新确认。`)
    }
  }
  return result
}
async function channelKeys(id: number): Promise<Data[]> {
  const report = channelBalanceReports.get(id)
  return rows(await get(`/api/relayfor/channels/${id}/keys`)).map((key) => ({
    ...key, masked_key: key.masked_key ?? key.key ?? '',
    quota_policy_available: false,
    ...(report ? { balance_checked_at: report.checked_at, balance_check_error: report.status === 'failed' ? report.message : '', upstream_balance_usd: report.balance_usd ?? null } : {}),
  }))
}

async function allChannelKeys(): Promise<Data[]> {
  const channels = await all('/api/channel/')
  const result: Data[] = []
  for (const channel of channels) {
    const keys = await channelKeys(int(channel.id, 1))
    for (const key of keys) {
      result.push({
        ...key, channel_id: channel.id, channel_name: channel.name,
        channel_status: channel.status, balance_scope: 'channel',
        upstream_balance_usd: num(channel.balance_updated_time) > 0 ? channel.balance : key.upstream_balance_usd ?? null,
        balance_checked_at: key.balance_checked_at ?? iso(channel.balance_updated_time),
      })
    }
  }
  return result
}

const balanceProviderTypes = new Set([1, 8, 10, 12, 13, 20, 25, 40, 43, 58])

async function probeChannelBalance(channel: Data, keyID?: number): Promise<Data> {
  const id = int(channel.id, 1)
  const report: Data = { channel_id: id, channel_name: channel.name, key_id: keyID ?? null, scope: 'channel', checked_at: new Date().toISOString(), attempted: false }
  try {
    if (keyID !== undefined) {
      const keys = await channelKeys(id)
      if (!keys.some((key) => num(key.id) === keyID)) throw new Error('该渠道 Key 已不存在，请刷新列表')
    }
    if (json(channel.channel_info).is_multi_key) {
      return { ...report, status: 'unavailable', message: '该渠道使用多个 Key，供应商余额接口不支持逐个 Key 查询。' }
    }
    if (!balanceProviderTypes.has(num(channel.type))) {
      return { ...report, status: 'unavailable', message: '此供应商尚未提供可用的渠道余额查询。' }
    }
    report.attempted = true
    const response = await request(`/api/channel/update_balance/${id}`)
    const balance = num(response.balance, Number.NaN)
    if (!Number.isFinite(balance)) return { ...report, status: 'unavailable', message: '上游返回了余额明细，无法转换为单一美元余额。' }
    return { ...report, status: 'updated', balance_usd: balance, message: '上游渠道余额已查询；独立 Key 配额与启停状态保持不变。' }
  } catch (error) {
    return { ...report, status: 'failed', message: error instanceof Error ? error.message : '渠道余额查询失败' }
  }
}

async function refreshChannelBalances(id?: number, keyID?: number): Promise<Data> {
  const channels = id === undefined ? await all('/api/channel/') : [object(await get(`/api/channel/${id}`))]
  const reports: Data[] = []
  // Sequential probes avoid an unbounded burst against upstream providers.
  for (const channel of channels) {
    const report = await probeChannelBalance(channel, keyID)
    channelBalanceReports.set(int(channel.id, 1), report)
    reports.push(report)
  }
  return {
    scope: 'channel', checked: reports.filter((report) => report.attempted).length,
    updated: reports.filter((report) => report.status === 'updated').length,
    failed: reports.filter((report) => report.status === 'failed').length,
    unavailable: reports.filter((report) => report.status === 'unavailable').length,
    recovered: 0, still_zero: reports.filter((report) => report.status === 'updated' && num(report.balance_usd) <= 0).length,
    reports,
  }
}
async function saveKey(
  channelID: number,
  keyID: number | null,
  method: string,
  body: Data
): Promise<Envelope> {
  if (method !== 'DELETE') {
    for (const field of ['quota_total', 'quota_remaining']) assertDefault(body, field, -1)
    for (const field of ['requests_per_minute', 'concurrent_requests']) assertDefault(body, field, 0)
    if ('weight' in body) body = { ...body, weight: int(body.weight, 1, 100000) }
  }
  const path = `/api/relayfor/channels/${channelID}/keys${
    keyID === null ? '' : `/${keyID}`
  }`
  return request(path, method, method === 'DELETE' ? undefined : body)
}
async function listUsers(query: URLSearchParams): Promise<Data> {
  const params = pagination(query)
  if (query.get('sleeping') === '1') params.set('inactive_days', '14')
  params.delete('sleeping')
  if (params.has('role')) {
    params.set('role', String(newRole(params.get('role'))))
  }
  return page(
    object(await get(`/api/user/search?${params}`)),
    Number(params.get('page_size')),
    (user) => ({
      ...user,
      role: legacyRole(user.role),
      newapi_role: user.role,
      created_at: iso(user.created_at),
    })
  )
}
async function saveUser(id: number, body: Data): Promise<Data> {
  if (body.password || body.password_encrypted || body.totp_secret) {
    unsupported('密码或TOTP变更，必须经过安全验证')
  }
  const current = object(await get(`/api/user/${id}`))
  const role = body.role === undefined ? int(current.role) : newRole(body.role),
    status =
      body.status === undefined ? int(current.status) : int(body.status, 1, 2)
  const quota = body.quota === undefined ? num(current.quota) : int(body.quota)
  if (
    role !== num(current.role) &&
    !([1, 10].includes(role) && [1, 10].includes(num(current.role)))
  ) {
    unsupported('直接分配超级管理员角色')
  }
  const completed: string[] = []
  try {
    if (
      ['username', 'display_name', 'group', 'remark'].some(
        (key) => key in body && body[key] !== current[key]
      )
    ) {
      await request('/api/user/', 'PUT', {
        id,
        username: body.username ?? current.username,
        display_name: body.display_name ?? current.display_name,
        group: body.group ?? current.group,
        remark: body.remark ?? current.remark,
        role: current.role,
      })
      completed.push('用户资料')
    }
    if (quota !== num(current.quota)) {
      await request('/api/user/manage', 'POST', {
        id,
        action: 'add_quota',
        mode: 'override',
        value: quota,
      })
      completed.push('余额')
    }
    if (status !== num(current.status)) {
      await request('/api/user/manage', 'POST', {
        id,
        action: status === 1 ? 'enable' : 'disable',
      })
      completed.push('状态')
    }
    if (role !== num(current.role)) {
      await request('/api/user/manage', 'POST', {
        id,
        action: role === 10 ? 'promote' : 'demote',
      })
      completed.push('角色')
    }
    return { success: true }
  } catch (error) {
    if (!completed.length) throw error
    throw new Error(
      `已更新${completed.join('、')}，后续失败：${error instanceof Error ? error.message : '服务器错误'}。请刷新确认。`
    )
  }
}

function redemptionView(row: Data): Data {
  const items: Data[] = rows(row.items).map((code) => {
    const expired = num(code.expired_time) > 0 && num(code.expired_time) < Date.now() / 1000
    let status = 1
    if (num(code.status) === 3) status = 2
    else if (num(code.status) === 2) status = 3
    else if (expired) status = 4
    return {
      ...code, code: code.key, status, newapi_status: code.status,
      created_at: iso(code.created_time), used_at: iso(code.redeemed_time),
      expires_at: iso(code.expired_time), used_by_user_id: code.used_user_id,
      redeem_type: 'quota', user_group: '', quota_group: '', is_public: false,
    }
  })
  const first = items[0] ?? {}
  return {
    batch_id: row.batch_id,
    name: first.name,
    created_at: first.created_at,
    quota: first.quota,
    user_group: '',
    redeem_type: 'quota',
    count: items.length,
    used_count: items.filter((code) => code.status === 2).length,
    unused_count: items.filter((code) => code.status === 1).length,
    deletable_count: items.filter((code) => code.status !== 2).length,
    disabled_count: items.filter((code) => code.status === 3).length,
    expired_count: items.filter((code) => code.status === 4).length,
    items,
  }
}
async function listRedemptions(query: URLSearchParams): Promise<Data> {
  if (query.get('user_group')) unsupported('兑换码用户组过滤')
  const params = pagination(query)
  params.delete('group_by')
  params.delete('user_group')
  if (params.get('status') === '2') params.set('status', '3')
  else if (params.get('status') === '3') params.set('status', '2')
  else if (params.get('status') === '4') params.set('status', 'expired')
  return page(
    object(await get(`/api/redemption/batches?${params}`)),
    Number(params.get('page_size')),
    redemptionView
  )
}
async function generateRedemptions(body: Data): Promise<Data> {
  if (
    (body.user_group && body.user_group !== 'default') ||
    body.quota_group ||
    body.is_public ||
    num(body.redeem_limit, 1) > 1 ||
    (body.redeem_type && body.redeem_type !== 'quota')
  ) {
    unsupported('用户组/额度组、公共多次或订阅型兑换码')
  }
  let quota = body.quota
  if (body.amount_usd !== undefined) {
    const unit = num(
      object(await get('/api/status')).quota_per_unit,
      Number.NaN
    )
    if (!Number.isFinite(unit) || unit <= 0) {
      throw new Error('服务器额度单位无效')
    }
    quota = Math.round(num(body.amount_usd) * unit)
  }
  const codes = await get('/api/redemption/', 'POST', {
    name: String(body.name ?? ''),
    count: int(body.count, 1, 100),
    quota: int(quota, 1),
    expired_time: int(body.expired_time ?? 0),
  })
  if (
    !Array.isArray(codes) ||
    !codes.every((code) => typeof code === 'string')
  ) {
    throw new Error('服务器未返回生成的兑换码')
  }
  return { codes, count: codes.length, user_group: '' }
}

function pricingView(entry?: Data): Data {
  const effective = entry ? object(entry.effective) : {},
    expression =
      effective['billing_setting.billing_mode'] === 'tiered_expr' ||
      Object.keys(json(effective['billing_setting.plugin_billing_expr']))
        .length > 0
  const perRequest = effective.ModelPrice !== undefined && !expression,
    input = num(effective.ModelRatio, Number.NaN) * 2,
    output = input * num(effective.CompletionRatio, Number.NaN)
  const available =
    !expression &&
    (perRequest
      ? Number.isFinite(num(effective.ModelPrice, Number.NaN))
      : Number.isFinite(input) && Number.isFinite(output))
  return {
    billing_mode: perRequest ? 'request' : 'token',
    input_price: available && !perRequest ? input : null,
    output_price: available && !perRequest ? output : null,
    request_price:
      available && perRequest ? Number(effective.ModelPrice) : null,
    cache_read_price:
      available && !perRequest ? input * num(effective.CacheRatio, 1) : null,
    cache_write_price:
      available && !perRequest
        ? input * num(effective.CreateCacheRatio, 1)
        : null,
    price_multiplier: 1,
    pricing_unavailable: !available,
    pricing_editable: !expression,
    unsupported_reason: available
      ? ''
      : ['当前模型尚未配置可显示的价格', '当前模型使用表达式或插件计费'][
          Number(expression)
        ],
    free_model:
      available &&
      (perRequest
        ? Number(effective.ModelPrice) === 0
        : input === 0 && output === 0),
  }
}
async function listModels(): Promise<Data[]> {
  const canEditPricing = !document.body.dataset.gatewayRole || num(document.body.dataset.gatewayRole) >= 100
  const [models, channels, value, accessValue] = await Promise.all([
    all(
      '/api/models/',
      new URLSearchParams({ include_channel_models: 'true' })
    ),
    all('/api/channel/'),
    canEditPricing ? get('/api/option/model_pricing') : publicPricingSnapshot(),
    get('/api/relayfor/models/access'),
  ])
  const entries = new Map(
    rows(object(value).entries).map((entry) => [
      String(entry.model_name),
      entry,
    ])
  )
  const access = new Map(rows(accessValue).map((entry) => [num(entry.model_id), entry]))
  return models.map((model) => {
    const name = String(model.model_name)
    const bindings = channels
      .filter((channel) => channelModelNames(channel).includes(name))
      .map((channel) => ({
        channel_id: channel.id,
        upstream_model: json(channel.model_mapping)[name] ?? name,
        priority: num(channel.priority),
        weight: num(channel.weight),
        enabled: num(channel.status) === 1,
        upstream_free: false,
        channel_policy_shared: true,
      }))
    const available = bindings.filter((binding) => binding.enabled).length
    return {
      ...model,
      ...pricingView(entries.get(name)),
      ...(!canEditPricing ? { pricing_editable: false, pricing_read_only: true, unsupported_reason: '仅超级管理员可编辑模型计费' } : {}),
      row_key: num(model.id) > 0 ? String(model.id) : `model:${name}`,
      model: name,
      display_name: name,
      enabled: num(model.id) === 0 || num(model.status) === 1,
      catalog_visibility_only: true,
      bindings,
      available_channel_count: available,
      enabled_binding_count: bindings.length,
      metadata_id: model.id,
      advanced_routing_enabled: true,
      availability_status: available ? 'available' : 'unavailable',
      visible_to_users: model.square_state === 'visible' || model.square_state === 'partial',
      prepaid_compatible: true,
      subscription_only: access.get(num(model.id))?.subscription_only === true,
    }
  })
}

async function publicPricingSnapshot(): Promise<Data> {
  const catalog = rows(await get('/api/pricing'))
  return {
    entries: catalog.map((entry) => {
      const effective: Data = { 'billing_setting.billing_mode': entry.billing_mode ?? 'ratio' }
      if (num(entry.quota_type) === 1) effective.ModelPrice = entry.model_price
      else {
        effective.ModelRatio = entry.model_ratio
        effective.CompletionRatio = entry.completion_ratio
        if (entry.cache_ratio != null) effective.CacheRatio = entry.cache_ratio
        if (entry.create_cache_ratio != null) effective.CreateCacheRatio = entry.create_cache_ratio
      }
      if (Array.isArray(entry.billing_plugin_variants) && entry.billing_plugin_variants.length) {
        effective['billing_setting.plugin_billing_expr'] = { configured: true }
      }
      return { model_name: entry.model_name, effective }
    }),
  }
}

async function saveModel(id: number | null, body: Data): Promise<Data> {
  const name = String(body.model ?? '').trim()
  if (!name) throw new Error('模型名称不能为空')
  if (
    body.free_model === true &&
    (num(body.input_price) > 0 ||
      num(body.output_price) > 0 ||
      num(body.request_price) > 0)
  ) {
    unsupported('免费模型标记与非零价格冲突')
  }
  for (const key of ['compaction_price', 'long_context_threshold']) {
    assertDefault(body, key, 0)
  }
  for (const key of [
    'long_context_input_multiplier',
    'long_context_output_multiplier',
    'price_multiplier',
  ]) {
    assertDefault(body, key, 1)
  }
  for (const key of [
    'remote_compaction_enabled',
    'data_train',
    'session_enabled',
    'session_only',
  ]) {
    assertDefault(body, key, false)
  }
  assertDefault(body, 'prepaid_compatible', true)
  assertDefault(body, 'quota_group', '')
  if ('subscription_only' in body && typeof body.subscription_only !== 'boolean') {
    throw new Error('订阅专属设置必须为布尔值')
  }
  if (body.display_name && body.display_name !== name) {
    unsupported('独立模型显示名称')
  }
  const current = id === null ? null : object(await get(`/api/models/${id}`))
  if (current && current.model_name !== name) {
    unsupported('模型及路由同时重命名')
  }
  if (
    (current && Boolean(body.enabled) !== (num(current.status) === 1)) ||
    (!current && body.enabled === false)
  ) {
    unsupported('模型启停，目录隐藏不能替代路由禁用')
  }
  const channels = await all('/api/channel/')
  if (Array.isArray(body.bindings)) {
    const expected = channels.filter((channel) => channelModelNames(channel).includes(name)),
      proposed = rows(body.bindings)
    if (expected.length !== proposed.length) unsupported('模型渠道绑定变更')
    for (const binding of proposed) {
      const channel = expected.find(
        (item) => item.id === num(binding.channel_id)
      )
      if (
        !channel ||
        binding.upstream_free ||
        num(binding.priority) !== num(channel.priority) ||
        num(binding.weight) !== num(channel.weight) ||
        Boolean(binding.enabled) !== (num(channel.status) === 1) ||
        String(binding.upstream_model || name) !==
          String(json(channel.model_mapping)[name] ?? name)
      ) {
        unsupported('模型独立权重、优先级或上游绑定')
      }
    }
  }
  const snapshot = object(
      await get(`/api/option/model_pricing?model=${encodeURIComponent(name)}`)
    ),
    entry = rows(snapshot.entries).find((item) => item.model_name === name)
  if (entry && pricingView(entry).pricing_editable === false) {
    unsupported('表达式或插件计费，请在完整计费配置中编辑')
  }
  // The pricing endpoint replaces the complete model draft. Keep unrelated
  // image/audio rates when editing the token fields shown by this form.
  const pricing: Data = { ...json(entry?.configured), 'billing_setting.billing_mode': 'ratio' }
  for (const key of ['ModelPrice', 'ModelRatio', 'CompletionRatio', 'CacheRatio', 'CreateCacheRatio']) {
    delete pricing[key]
  }
  if (body.billing_mode === 'request') {
    const price = Number(body.request_price)
    if (!Number.isFinite(price) || price < 0) throw new Error('按次价格无效')
    pricing.ModelPrice = price
  } else {
    const input = Number(body.input_price),
      output = Number(body.output_price)
    if (
      ![input, output].every((number) => Number.isFinite(number) && number >= 0)
    ) {
      throw new Error('输入/输出价格无效')
    }
    if (input === 0 && output > 0) unsupported('零输入价且非零输出价的倍率计费')
    pricing.ModelRatio = input / 2
    pricing.CompletionRatio = input === 0 ? 1 : output / input
    for (const [field, target] of [
      ['cache_read_price', 'CacheRatio'],
      ['cache_write_price', 'CreateCacheRatio'],
    ]) {
      const price = Number(body[field])
      if (!Number.isFinite(price) || price < -1) throw new Error('缓存价格无效')
      if (input === 0 && price > 0) unsupported('零输入价且非零缓存价')
      pricing[target] = price < 0 || input === 0 ? 1 : price / input
    }
  }
  let created = false
  let savedID = id
  let pricingSaved = false
  try {
    if (!current) {
      const saved = object(await get('/api/models/', 'POST', {
        model_name: name,
        status: 1,
        name_rule: 0,
      }))
      created = true
      savedID = int(saved.id, 1)
    }
    await request('/api/option/model_pricing', 'PATCH', {
      changes: [
        {
          model_name: name,
          expected_version: entry?.version ?? snapshot.empty_version,
          pricing,
        },
      ],
    })
    pricingSaved = true
    if ('subscription_only' in body) {
      await request(`/api/relayfor/models/${savedID}/access`, 'PUT', {
        model: name, subscription_only: body.subscription_only,
      })
    }
    return { success: true }
  } catch (error) {
    if (pricingSaved) {
      throw new Error(`模型定价已保存，订阅访问规则保存失败：${error instanceof Error ? error.message : '服务器错误'}。请刷新确认。`)
    }
    if (!created) throw error
    throw new Error(
      `模型目录已创建，定价保存失败：${error instanceof Error ? error.message : '服务器错误'}。请刷新后重新保存。`
    )
  }
}

async function routingChannels(): Promise<Data[]> {
  return (await all('/api/channel/')).map((channel) => ({
    id: channel.id,
    name: channel.name,
    status: channel.status,
    group: channel.group,
    models: String(channel.models ?? ''),
    priority: num(channel.priority),
    weight: num(channel.weight),
    model_mapping: json(channel.model_mapping),
  }))
}

async function saveChannelAlias(id: number, body: Data, remove: boolean): Promise<Envelope> {
  const channel = object(await get(`/api/channel/${id}`))
  const mapping: Data = Object.assign(Object.create(null), json(channel.model_mapping))
  const original = String(body.original_alias ?? '').trim()
  const alias = String(body.alias ?? original).trim()
  const target = String(body.target_model ?? '').trim()
  if (!alias || (!remove && !target)) throw new Error('别名和目标模型不能为空')
  if (alias.length > 128 || target.length > 128 || /[,\r\n]/.test(alias)) {
    throw new Error('模型名称最多128个字符，别名不能包含逗号或换行')
  }
  if (original && !Object.hasOwn(mapping, original)) throw new Error('该别名已被修改，请刷新后重试')
  if (remove && !Object.hasOwn(mapping, alias)) throw new Error('该别名已被删除，请刷新后重试')
  if (remove) delete mapping[alias]
  else {
    if (alias !== original && Object.hasOwn(mapping, alias)) throw new Error('该渠道已存在同名别名')
    if (original !== alias) delete mapping[original]
    mapping[alias] = target
    for (const source of Object.keys(mapping)) {
      const visited = new Set<string>()
      let cursor = source
      while (Object.hasOwn(mapping, cursor)) {
        if (mapping[cursor] === cursor) break // New API permits an identity mapping.
        if (visited.has(cursor)) throw new Error('模型映射不能包含循环')
        visited.add(cursor)
        cursor = String(mapping[cursor])
      }
    }
  }
  const models = new Set(String(channel.models ?? '').split(',').map((item) => item.trim()).filter(Boolean))
  if (original && (remove || original !== alias)) models.delete(original)
  if (!remove) models.add(alias)
  return saveChannel(id, {
    newapi_type: channel.type,
    models: [...models].join(','),
    model_mapping: JSON.stringify(mapping),
  }, channel)
}

async function settings(
  path: string,
  method: string,
  body: Data
): Promise<unknown> {
  if (path === '/api/admin/settings/announcements') {
    if (method === 'GET') {
      const notice = await get('/api/notice')
      return {
        announcements: { public: notice, pay: notice },
        shared_announcement: true,
      }
    }
    if ('pay' in body && String(body.public ?? '') !== String(body.pay ?? '')) {
      unsupported('公益/付费站分开展示公告')
    }
    return request('/api/option/', 'PUT', {
      key: 'Notice',
      value: String(body.public ?? ''),
    })
  }
  if (path === '/api/admin/settings/routing') {
    if (method === 'GET') {
      const config = object(await get('/api/option/request_policy'))
      const options = object(config.options)
      const retries = int(options.RetryTimes ?? 0, 0, 99)
      return {
        settings: {
          retry_enabled: retries > 0,
          retry_max_attempts: retries + 1,
          retry_status_codes: String(options.AutomaticRetryStatusCodes ?? ''),
        },
      }
    }
    if (method === 'PUT') {
      for (const key of Object.keys(body)) {
        if (!['retry_enabled', 'retry_max_attempts', 'retry_status_codes'].includes(key)) {
          unsupported(key)
        }
      }
      const attempts = int(body.retry_max_attempts, 1, 100)
      const retryTimes = body.retry_enabled === false ? 0 : attempts - 1
      const parsedCodes = parseHttpStatusCodeRules(body.retry_status_codes)
      if (!parsedCodes.ok) {
        throw new Error('可重试状态码必须在 100 到 599 之间，可使用 500-503 范围')
      }
      return request('/api/option/request_policy', 'PATCH', {
        options: {
          RetryTimes: String(retryTimes),
          AutomaticRetryStatusCodes: parsedCodes.normalized,
        },
      })
    }
    unsupported('路由设置请求方法')
  }
  if (path === '/api/admin/settings/site') {
    if (method === 'GET') {
      const [status, optionValue] = await Promise.all([
        get('/api/status'),
        get('/api/option/'),
      ])
      const options = rows(optionValue)
      const values = new Map(options.map((option) => [String(option.key), String(option.value ?? '')]))
      return {
        settings: {
          system_name: String(object(status).system_name ?? ''),
          logo: String(object(status).logo ?? ''),
          server_address: values.get('ServerAddress') ?? String(object(status).server_address ?? ''),
          linuxdo_redirect_url: String(object(status).linuxdo_redirect_url ?? ''),
          linuxdo_enabled: values.get('LinuxDOOAuthEnabled') === 'true' || Boolean(object(status).linuxdo_oauth),
          linuxdo_client_id: values.get('LinuxDOClientId') ?? String(object(status).linuxdo_client_id ?? ''),
          linuxdo_minimum_trust_level: Number(values.get('LinuxDOMinimumTrustLevel') ?? object(status).linuxdo_minimum_trust_level ?? 0),
          linuxdo_client_secret_configured: values.get('LinuxDOClientSecretConfigured') === 'true',
        },
      }
    }
    if (method === 'PUT') {
      const updates: Array<[string, string]> = []
      const name = String(body.system_name ?? '').trim()
      if ('system_name' in body) {
        if (!name || [...name].length > 64) throw new Error('网站名称必须为 1 到 64 个字符')
        updates.push(['SystemName', name])
      }
      if ('logo' in body) updates.push(['Logo', String(body.logo ?? '').trim()])
      if ('server_address' in body) {
        const address = new URL(String(body.server_address ?? '').trim())
        if (!['http:', 'https:'].includes(address.protocol) || address.username || address.password || address.pathname !== '/' || address.search || address.hash) {
          throw new Error('站点地址须为完整的 HTTP 或 HTTPS Origin')
        }
        updates.push(['ServerAddress', address.origin])
      }
      if (body.linuxdo_enabled === false) updates.unshift(['LinuxDOOAuthEnabled', 'false'])
      if ('linuxdo_client_id' in body) updates.push(['LinuxDOClientId', String(body.linuxdo_client_id ?? '').trim()])
      if ('linuxdo_client_secret' in body && String(body.linuxdo_client_secret ?? '').trim()) updates.push(['LinuxDOClientSecret', String(body.linuxdo_client_secret).trim()])
      if ('linuxdo_minimum_trust_level' in body) updates.push(['LinuxDOMinimumTrustLevel', String(int(body.linuxdo_minimum_trust_level, 0, 4))])
      if (body.linuxdo_enabled === true) updates.push(['LinuxDOOAuthEnabled', 'true'])
      const completed: string[] = []
      try {
        for (const [key, value] of updates) {
          await request('/api/option/', 'PUT', { key, value })
          completed.push(key)
        }
      } catch (error) {
        if (!completed.length) throw error
        throw new Error(`已保存 ${completed.join('、')}，后续保存失败：${error instanceof Error ? error.message : '服务器错误'}。请刷新确认。`)
      }
      return { success: true }
    }
    unsupported('站点设置请求方法')
  }
  return unsupported('原运行参数、紧急停机、IP/并发或安全验证有效期配置')
}

export async function handleAdminRequest(
  path: string,
  options: LegacyRequestOptions = {}
): Promise<Handled> {
  const url = new URL(path, 'http://relayfor.local'),
    pathname = url.pathname,
    method = (options.method ?? 'GET').toUpperCase()
  if (
    !/^\/api\/admin\/(?:channels(?:\/|$)|channel-keys(?:\/|$)|users(?:\/|$)|redemptions(?:\/|$)|models(?:\/|$)|model-health$|routing\/|settings\/(?:announcements|routing|site|operational|emergency-stop)$|security(?:$|\/totp\/|\/proof-lifetime$))/.test(
      pathname
    )
  ) {
    return { handled: false }
  }
  const body = options.body === undefined ? {} : object(options.body)
  const done = (value: unknown): Handled => ({ handled: true, data: value })
  if (pathname === '/api/admin/channel-keys' && method === 'GET') return done(await allChannelKeys())
  if (pathname === '/api/admin/channel-keys/refresh-balances' && method === 'POST') return done(await refreshChannelBalances())
  if (pathname === '/api/admin/channels') {
    if (method === 'GET') {
      const channels = await all('/api/channel/')
      const policies = new Map(rows(await get('/api/relayfor/channels/policies')).map((policy) => [num(policy.channel_id), policy]))
      return done(channels.map((channel) => channelView(channel, policies.get(num(channel.id)))))
    }
    if (method === 'POST') return done(await saveChannel(null, body))
  }
  const channel = pathname.match(/^\/api\/admin\/channels\/(\d+)(?:\/(.*))?$/)
  if (channel) {
    const id = int(channel[1], 1),
      action = channel[2] ?? ''
    if (!action && method === 'PUT') return done(await saveChannel(id, body))
    if (!action && method === 'DELETE') {
      return done(await request(`/api/channel/${id}`, 'DELETE'))
    }
    if (action === 'probe' && method === 'POST') {
      const result = await request(`/api/channel/test/${id}`)
      return done({
        success: true,
        latency_ms: Math.round(num(result.time) * 1000),
      })
    }
    if (action === 'operation' && method === 'POST') {
      if (body.action !== 'drain' && body.action !== 'restore') {
        unsupported('重置故障计数')
      }
      return done(
        await request(`/api/channel/${id}/status`, 'POST', {
          status: body.action === 'restore' ? 1 : 2,
        })
      )
    }
    if (action === 'keys' && method === 'GET') {
      return done(await channelKeys(id))
    }
    if (action === 'keys' && method === 'POST') {
      return done(await saveKey(id, null, method, body))
    }
    const key = action.match(/^keys\/(\d+)$/)
    if (key && ['PUT', 'DELETE'].includes(method)) {
      return done(await saveKey(id, int(key[1], 1), method, body))
    }
    const keyStatus = action.match(/^keys\/(\d+)\/status$/)
    if (keyStatus && method === 'PATCH') return done(await request(`/api/relayfor/channels/${id}/keys/${int(keyStatus[1], 1)}/status`, 'PATCH', { status: int(body.status, 1, 3) }))
    const keyBalance = action.match(/^keys\/(\d+)\/refresh-balance$/)
    if (keyBalance && method === 'POST') return done(await refreshChannelBalances(id, int(keyBalance[1], 1)))
  }
  if (pathname === '/api/admin/users' && method === 'GET') {
    return done(await listUsers(url.searchParams))
  }
  const user = pathname.match(/^\/api\/admin\/users\/(\d+)$/)
  if (user && method === 'PATCH') {
    return done(await saveUser(int(user[1], 1), body))
  }
  if (pathname === '/api/admin/redemptions' && method === 'GET') {
    return done(await listRedemptions(url.searchParams))
  }
  if (pathname === '/api/admin/redemptions/generate' && method === 'POST') {
    return done(await generateRedemptions(body))
  }
  if (pathname === '/api/admin/redemptions/used' && method === 'DELETE') {
    return done(await get('/api/redemption/used', 'DELETE'))
  }
  const redemption = pathname.match(/^\/api\/admin\/redemptions\/(\d+)(\/status)?$/)
  if (redemption && method === 'PUT') {
    const id = int(redemption[1], 1)
    const current = object(await get(`/api/redemption/${id}`))
    if (num(current.status) === 3) throw new Error('已使用兑换码不能重新启用或修改')
    if (redemption[2]) {
      return done(await request('/api/redemption/?status_only=true', 'PUT', { id, status: body.enabled === true ? 1 : 2 }))
    }
    let quota = current.quota
    if (body.amount_usd !== undefined) {
      const unit = num(object(await get('/api/status')).quota_per_unit, Number.NaN)
      if (!Number.isFinite(unit) || unit <= 0) throw new Error('服务器额度单位无效')
      quota = Math.round(num(body.amount_usd) * unit)
    }
    return done(await request('/api/redemption/', 'PUT', {
      id, name: String(body.name ?? current.name), quota: int(quota, 1),
      expired_time: int(body.expired_time ?? current.expired_time ?? 0),
    }))
  }
  if (redemption && method === 'DELETE') {
    return done(
      await request(`/api/redemption/${int(redemption[1], 1)}`, 'DELETE')
    )
  }
  if (pathname === '/api/admin/redemptions' && method === 'DELETE') {
    const batchID = url.searchParams.get('batch_id')
    if (!batchID) throw new Error('兑换码批次标识不能为空')
    return done(await get(`/api/redemption/batches/${encodeURIComponent(batchID)}/unused`, 'DELETE'))
  }
  if (pathname === '/api/admin/models' && method === 'GET') {
    return done(await listModels())
  }
  if (pathname === '/api/admin/models' && method === 'POST') {
    return done(await saveModel(null, body))
  }
  if (pathname === '/api/admin/model-health' && method === 'GET') return { handled: false }
  if (pathname === '/api/admin/routing/channels' && method === 'GET') return done(await routingChannels())
  const routeChannel = pathname.match(/^\/api\/admin\/routing\/channels\/(\d+)(\/aliases)?$/)
  if (routeChannel) {
    const id = int(routeChannel[1], 1)
    if (routeChannel[2] && (method === 'PUT' || method === 'DELETE')) {
      return done(await saveChannelAlias(id, body, method === 'DELETE'))
    }
    if (!routeChannel[2] && method === 'PUT') {
      const priority = int(body.priority, -2147483648, 2147483647)
      const weight = int(body.weight, 0, 10000)
      return done(await saveChannel(id, { priority, weight }))
    }
  }
  const model = pathname.match(/^\/api\/admin\/models\/(\d+)(\/status)?$/)
  if (model) {
    const id = int(model[1], 1)
    if (model[2]) unsupported('模型启停；目录隐藏不能替代路由禁用')
    if (method === 'PUT') return done(await saveModel(id, body))
    if (method === 'DELETE') {
      return done(
        await request(
          `/api/models/${id}?remove_from_channels=true&remove_pricing=false`,
          'DELETE'
        )
      )
    }
  }
  if (
    [
      '/api/admin/settings/announcements',
      '/api/admin/settings/routing',
      '/api/admin/settings/site',
      '/api/admin/settings/operational',
      '/api/admin/settings/emergency-stop',
    ].includes(pathname)
  ) {
    return done(await settings(pathname, method, body))
  }
  if (pathname === '/api/admin/security' && method === 'GET') {
    const status = object(await get('/api/user/2fa/status'))
    return done({ ...status, totp_enabled: status.enabled, pending: false })
  }
  if (
    pathname.startsWith('/api/admin/security/totp/') ||
    pathname === '/api/admin/security/proof-lifetime'
  ) {
    unsupported('TOTP必须完成New API的一次性安全验证')
  }
  // Statistics, benefits, subscription, registration, prepaid, risk and audit
  // endpoints already have server-owned contracts and must pass through.
  return { handled: false }
}
