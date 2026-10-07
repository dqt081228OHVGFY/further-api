import { NATIVE_FEATURE_ENTRIES } from '../native-feature-paths'

export type Field = {
  key: string
  label: string
  kind?: 'toggle' | 'number' | 'multiline' | 'secret'
  hint?: string
  format?: 'json' | 'url'
  min?: number
  options?: { value: string; label: string }[]
}

export type SettingsPage = {
  title: string
  description: string
  fields: Field[]
  related?: { label: string; href: string }[]
}

const fieldOptions: Record<string, Field['options']> = {
  'passkey.user_verification': [
    { value: 'required', label: '每次都验证设备身份（推荐）' },
    { value: 'preferred', label: '优先验证设备身份' },
    { value: 'discouraged', label: '由设备自行决定' },
  ],
  'fetch_setting.domain_filter_mode': [
    { value: 'false', label: '阻止列表内的域名' },
    { value: 'true', label: '仅允许列表内的域名' },
  ],
  'fetch_setting.ip_filter_mode': [
    { value: 'false', label: '阻止列表内的 IP 地址' },
    { value: 'true', label: '仅允许列表内的 IP 地址' },
  ],
}

const structuredKeys = new Set([
  'gemini.safety_settings',
  'gemini.version_settings',
  'claude.model_headers_settings',
  'claude.default_max_tokens',
  'global.thinking_model_blacklist',
  'fetch_setting.domain_list',
  'fetch_setting.ip_list',
  'fetch_setting.allowed_ports',
  'global.chat_completions_to_responses_policy',
  'PayMethods', 'CreemProducts', 'WaffoPayMethods',
])
const urlKeys = new Set([
  'ServerAddress',
  'TaskPublicAddress',
  'general_setting.docs_link',
  'TopUpLink',
  'PayAddress', 'WaffoNotifyUrl', 'WaffoReturnUrl', 'WaffoPancakeReturnURL',
  'WorkerUrl',
  'oidc.well_known',
])
const fieldHints: Record<string, string> = {
  Price: '每 1 USD 账户额度的人民币实付价格。',
  PayMethods: '支付方式数组，例如 [{"name":"支付宝","type":"alipay"}]。',
  StripeUnitPrice: '与 Stripe Price 的单价一致；每份对应 1 USD 账户额度。',
  StripeWebhookSecret: 'Stripe 付款通知签名密钥；设置后才能核验到账通知。',
  CreemProducts: '固定产品数组，例如 [{"productId":"prod_x","price":10,"quota":10,"currency":"USD"}]。金额和到账额度由服务器冻结。',
  CreemWebhookSecret: 'Creem 付款通知签名密钥。',
  WaffoPayMethods: '填写 Waffo 商户支持的支付方式数组。',
  RetryTimes: '首个上游请求失败后，允许切换渠道重试的次数。',
  AutomaticRetryStatusCodes: '使用英文逗号分隔 HTTP 状态码，例如 429,502,503。',
  AutomaticDisableStatusCodes: '命中这些状态码时，渠道可被自动暂停。',
  SensitiveWords: '每行填写一个需要检测的词。',
  'passkey.rp_id': '填写站点域名，不带 https:// 和路径。',
  'passkey.legacy_rp_ids': '保留旧网站的通行密钥域名；每行一项，也可使用英文逗号分隔。移除前会检查受影响的凭据。',
  'passkey.origins': '填写允许使用通行密钥的完整站点来源。',
  'passkey.user_verification': '可选 required、preferred 或 discouraged。',
  'channel_affinity_setting.default_ttl_seconds':
    '同一会话优先沿用原渠道的时长。',
  'monitor_setting.auto_test_channel_minutes': '定期检测的时间间隔；使用分钟。',
  ChannelDisableThreshold: '测试响应超过此秒数时，可自动停用渠道。',
  'claude.default_max_tokens': '填写模型名称到令牌上限的 JSON 对象，例如 {"default":8192}；default 为默认上限。',
  'claude.thinking_adapter_budget_tokens_percentage': '填写 0 到 1 的比例，例如 0.8 表示 80%。',
  'gemini.thinking_adapter_budget_tokens_percentage': '填写 0 到 1 的比例，例如 0.6 表示 60%。',
  'general_setting.ping_interval_seconds': '等待上游的流式响应时，向客户端发送心跳的间隔。',
  'perf_metrics_setting.flush_interval': '指标写入间隔，单位为分钟。',
  'general_setting.docs_link': '用户在管理页面打开的接入文档地址。',
  'perf_metrics_setting.retention_days': '超过保留天数的历史指标会被清理。',
  SMTPToken: '填写邮件服务提供的授权码或应用密码。',
  SMTPFrom: '系统邮件向收件人显示的发件邮箱。',
  PasswordLoginEnabled: '允许已有账户使用密码登录。',
  EmailVerificationEnabled: '新用户注册时需要完成邮箱验证。',
  EmailDomainRestrictionEnabled: '仅接受域名白名单内的注册邮箱。',
  EmailAliasRestrictionEnabled: '防止通过同一邮箱的不同别名重复注册。',
  EmailDomainWhitelist: '每行填写一个邮箱域名，例如 example.com。',
  ServerAddress: 'further-api 对外提供服务的完整地址。',
  TaskPublicAddress: '任务回调时外部服务可以访问的站点地址。',
  Notice: '向所有用户展示的站点公告。',
  'fetch_setting.allow_private_ip': '允许上游访问内网地址；请按部署环境选择。',
  'fetch_setting.domain_list': '填写域名的 JSON 数组，例如 ["example.com"]，按上方过滤模式生效。',
  'fetch_setting.ip_list': '填写 IP 或网段的 JSON 数组，例如 ["203.0.113.0/24"]，按上方过滤模式生效。',
  'fetch_setting.allowed_ports': '填写端口或范围的 JSON 数组，例如 ["443","8000-9000"]。',
  'token_setting.max_user_tokens': '每个账户最多可创建的 API Key 数量。',
  'ModelRequestRateLimitDurationMinutes': '请求计数会在这个时间窗口内累计。',
  'ModelRequestRateLimitCount': '同一用户在一个窗口内允许发送的总请求数。',
  'ModelRequestRateLimitSuccessCount': '同一用户在一个窗口内允许成功的请求数。',
}
const field = (key: string, label: string, kind?: Field['kind']): Field => {
  let format: Field['format']
  if (structuredKeys.has(key)) format = 'json'
  else if (urlKeys.has(key)) format = 'url'
  return { key, label, kind, hint: fieldHints[key], format, min: kind === 'number' ? 0 : undefined, options: fieldOptions[key] }
}

export const SETTINGS_PAGES: Record<string, SettingsPage> = {
  'site/system-info': {
    title: '任务回调与条款',
    description: '任务地址、接入文档和服务条款',
    fields: [
      field('TaskPublicAddress', '任务公开地址'),
      field('general_setting.docs_link', '文档地址'),
      field('legal.user_agreement', '用户协议', 'multiline'),
      field('legal.privacy_policy', '隐私政策', 'multiline'),
    ],
    related: [{ label: '管理站点地址与 LinuxDO 登录', href: '#settings/site' }],
  },
  'auth/basic-auth': {
    title: '登录与邮箱验证',
    description: '配置密码登录和注册邮箱验证规则',
    fields: [
      field('PasswordLoginEnabled', '允许密码登录', 'toggle'),
      field('EmailVerificationEnabled', '注册邮件验证', 'toggle'),
      field('EmailDomainRestrictionEnabled', '限制邮箱域名', 'toggle'),
      field('EmailAliasRestrictionEnabled', '限制邮箱别名', 'toggle'),
      field('EmailDomainWhitelist', '邮箱域名白名单', 'multiline'),
    ],
    related: [{ label: '管理注册与新用户余额', href: '#settings/registration' }],
  },
  'auth/oauth': {
    title: '第三方登录',
    description: '为用户启用可信的外部身份服务',
    fields: [
      field('GitHubOAuthEnabled', 'GitHub 登录', 'toggle'),
      field('GitHubClientId', 'GitHub 应用标识'),
      field('GitHubClientSecret', 'GitHub 应用密钥', 'secret'),
      field('discord.enabled', 'Discord 登录', 'toggle'),
      field('discord.client_id', 'Discord 应用标识'),
      field('discord.client_secret', 'Discord 应用密钥', 'secret'),
      field('oidc.enabled', 'OIDC 登录', 'toggle'),
      field('oidc.client_id', 'OIDC 应用标识'),
      field('oidc.client_secret', 'OIDC 应用密钥', 'secret'),
      field('oidc.well_known', 'OIDC 发现地址'),
    ],
    related: [{ label: '管理 LinuxDO 登录', href: '#settings/site' }],
  },
  'auth/passkey': {
    title: '通行密钥',
    description: '免密登录的服务域名及验证策略',
    fields: [
      field('passkey.enabled', '启用通行密钥', 'toggle'),
      field('passkey.rp_id', '通行密钥服务域名'),
      field('passkey.legacy_rp_ids', '保留的旧服务域名', 'multiline'),
      field('passkey.origins', '允许的来源', 'multiline'),
      field('passkey.user_verification', '用户验证策略'),
    ],
  },
  'auth/bot-protection': {
    title: '机器人防护',
    description: '登录及注册时的人机校验',
    fields: [
      field('TurnstileCheckEnabled', '启用 Turnstile', 'toggle'),
      field('TurnstileSiteKey', '站点密钥'),
      field('TurnstileSecretKey', '服务密钥', 'secret'),
    ],
  },
  'auth/custom-oauth': {
    title: '自定义身份服务',
    description: '接入自己的组织或第三方登录服务',
    fields: [],
  },
  'security/rate-limit': {
    title: '速率限制',
    description: '限制模型请求频率',
    fields: [
      field('ModelRequestRateLimitEnabled', '启用模型限流', 'toggle'),
      field('ModelRequestRateLimitCount', '窗口请求数', 'number'),
      field('ModelRequestRateLimitSuccessCount', '成功请求数', 'number'),
      field(
        'ModelRequestRateLimitDurationMinutes',
        '窗口时长（分钟）',
        'number'
      ),
    ],
  },
  'security/ssrf': {
    title: '上游地址防护',
    description: '约束服务端请求的目标地址',
    fields: [
      field('fetch_setting.enable_ssrf_protection', '启用 SSRF 防护', 'toggle'),
      field('fetch_setting.allow_private_ip', '允许私有 IP', 'toggle'),
      field('fetch_setting.apply_ip_filter_for_domain', '检查域名解析后的 IP', 'toggle'),
      field('fetch_setting.domain_filter_mode', '域名过滤模式'),
      field('fetch_setting.domain_list', '域名列表', 'multiline'),
      field('fetch_setting.ip_filter_mode', 'IP 过滤模式'),
      field('fetch_setting.ip_list', 'IP 列表', 'multiline'),
      field('fetch_setting.allowed_ports', '允许的端口', 'multiline'),
    ],
  },
  'security/token-limits': {
    title: '令牌限制',
    description: '设置单用户令牌数量上限',
    fields: [field('token_setting.max_user_tokens', '最大令牌数', 'number')],
  },
  'request-policies/filtering': {
    title: '请求检测',
    description: '过滤敏感内容',
    fields: [
      field('CheckSensitiveEnabled', '启用敏感词检测', 'toggle'),
      field('CheckSensitiveOnPromptEnabled', '检测提示词', 'toggle'),
      field('SensitiveWords', '敏感词', 'multiline'),
    ],
  },
  'request-policies/routing': {
    title: '会话与重试',
    description: '请求重试和会话绑定',
    fields: [
      field('RetryTimes', '重试次数', 'number'),
      field('AutomaticRetryStatusCodes', '重试状态码'),
      field('channel_affinity_setting.enabled', '启用会话绑定', 'toggle'),
      field(
        'channel_affinity_setting.default_ttl_seconds',
        '会话有效期（秒）',
        'number'
      ),
      field('channel_affinity_setting.max_entries', '最多保留会话数', 'number'),
    ],
  },
  'request-policies/health': {
    title: '渠道健康',
    description: '故障检测及自动恢复',
    fields: [
      field('AutomaticDisableChannelEnabled', '自动停用故障渠道', 'toggle'),
      field('AutomaticEnableChannelEnabled', '自动恢复渠道', 'toggle'),
      field('ChannelDisableThreshold', '响应超时阈值（秒）', 'number'),
      field('AutomaticDisableStatusCodes', '停用状态码'),
      field(
        'monitor_setting.auto_test_channel_enabled',
        '自动测试渠道',
        'toggle'
      ),
      field(
        'monitor_setting.auto_test_channel_minutes',
        '测试间隔（分钟）',
        'number'
      ),
    ],
  },
  'models/global': {
    title: '全局模型',
    description: '模型协议转换和连通性',
    fields: [
      field('global.pass_through_request_enabled', '透传请求', 'toggle'),
      field('global.thinking_model_blacklist', '思考模型黑名单', 'multiline'),
      field(
        'global.chat_completions_to_responses_policy',
        '协议转换策略',
        'multiline'
      ),
      field('general_setting.ping_interval_enabled', '启用流式心跳', 'toggle'),
      field(
        'general_setting.ping_interval_seconds',
        '心跳间隔（秒）',
        'number'
      ),
    ],
  },
  'models/gemini': {
    title: 'Gemini 协议',
    description: 'Gemini 协议参数',
    fields: [
      field('gemini.safety_settings', '安全策略', 'multiline'),
      field('gemini.version_settings', '版本映射', 'multiline'),
      field('gemini.thinking_adapter_enabled', '思考适配', 'toggle'),
      field(
        'gemini.thinking_adapter_budget_tokens_percentage',
        '思考预算百分比',
        'number'
      ),
    ],
  },
  'models/claude': {
    title: 'Claude 协议',
    description: 'Claude 协议参数',
    fields: [
      field('claude.model_headers_settings', '模型请求头', 'multiline'),
      field('claude.default_max_tokens', '默认最大令牌映射', 'multiline'),
      field('claude.thinking_adapter_enabled', '思考适配', 'toggle'),
      field(
        'claude.thinking_adapter_budget_tokens_percentage',
        '思考预算百分比',
        'number'
      ),
    ],
  },
  'models/grok': {
    title: 'Grok 协议',
    description: 'Grok 协议参数',
    fields: [
      field('grok.violation_deduction_enabled', '启用违规扣减', 'toggle'),
      field('grok.violation_deduction_amount', '扣减额度', 'number'),
    ],
  },
  'models/model-deployment': {
    title: '模型部署',
    description: '部署服务连接',
    fields: [
      field('model_deployment.ionet.enabled', '启用部署', 'toggle'),
      field('model_deployment.ionet.api_key', '部署服务密钥', 'secret'),
    ],
  },
  'billing/quota': {
    title: '邀请与额度策略',
    description: '邀请奖励、充值入口和预扣规则',
    fields: [
      field('QuotaForInviter', '邀请人奖励', 'number'),
      field('QuotaForInvitee', '被邀请人奖励', 'number'),
      field('TopUpLink', '充值地址'),
      field('quota_setting.trust_quota_usd', '信任额度（USD）', 'number'),
      field('quota_setting.pre_consume_multiplier', '预扣倍率', 'number'),
    ],
    related: [{ label: '管理新用户初始余额', href: '#settings/registration' }],
  },
  'billing/currency': {
    title: '货币与显示',
    description: '额度换算及前台显示',
    fields: [
      field('QuotaPerUnit', '单位额度', 'number'),
      field('USDExchangeRate', '美元汇率', 'number'),
      field('DisplayInCurrencyEnabled', '显示货币', 'toggle'),
      field('DisplayTokenStatEnabled', '显示令牌用量', 'toggle'),
      field('general_setting.quota_display_type', '额度显示方式'),
      field('general_setting.custom_currency_symbol', '自定义货币符号'),
      field(
        'general_setting.custom_currency_exchange_rate',
        '自定义汇率',
        'number'
      ),
    ],
  },
  'billing/payment': {
    title: '支付网关',
    description: '支付渠道及收款配置',
    fields: [],
  },
  'billing/payment-channels': {
    title: '外部支付渠道',
    description: '配置收款服务、固定价格和付款通知。完整配置的渠道才会出现在用户充值页。',
    fields: [
      field('PayAddress', '在线支付 · 收款服务地址'),
      field('EpayId', '在线支付 · 商户 ID'),
      field('EpayKey', '在线支付 · 签名密钥', 'secret'),
      field('Price', '在线支付 · 人民币单价', 'number'),
      field('MinTopUp', '在线支付 · 最低充值额度', 'number'),
      field('PayMethods', '在线支付 · 支付方式', 'multiline'),
      field('StripeApiSecret', 'Stripe · API 密钥', 'secret'),
      field('StripeWebhookSecret', 'Stripe · 通知签名密钥', 'secret'),
      field('StripePriceId', 'Stripe · 固定价格 ID'),
      field('StripeUnitPrice', 'Stripe · 美元单价', 'number'),
      field('StripeMinTopUp', 'Stripe · 最低充值额度', 'number'),
      field('StripePromotionCodesEnabled', 'Stripe · 接受商户优惠码', 'toggle'),
      field('CreemApiKey', 'Creem · API 密钥', 'secret'),
      field('CreemWebhookSecret', 'Creem · 通知签名密钥', 'secret'),
      field('CreemProducts', 'Creem · 固定充值产品', 'multiline'),
      field('CreemTestMode', 'Creem · 使用测试环境', 'toggle'),
      field('WaffoEnabled', 'Waffo · 开放支付', 'toggle'),
      field('WaffoSandbox', 'Waffo · 使用测试环境', 'toggle'),
      field('WaffoMerchantId', 'Waffo · 商户 ID'),
      field('WaffoApiKey', 'Waffo · API 密钥', 'secret'),
      field('WaffoPrivateKey', 'Waffo · 签名私钥', 'secret'),
      field('WaffoPublicCert', 'Waffo · 平台证书', 'multiline'),
      field('WaffoSandboxApiKey', 'Waffo · 测试 API 密钥', 'secret'),
      field('WaffoSandboxPrivateKey', 'Waffo · 测试签名私钥', 'secret'),
      field('WaffoSandboxPublicCert', 'Waffo · 测试平台证书', 'multiline'),
      field('WaffoNotifyUrl', 'Waffo · 付款通知地址'),
      field('WaffoReturnUrl', 'Waffo · 付款返回地址'),
      field('WaffoCurrency', 'Waffo · 收款币种'),
      field('WaffoUnitPrice', 'Waffo · 每单位额度单价', 'number'),
      field('WaffoMinTopUp', 'Waffo · 最低充值额度', 'number'),
      field('WaffoPayMethods', 'Waffo · 支付方式', 'multiline'),
      field('WaffoPancakeMerchantID', 'Pancake · 商户 ID'),
      field('WaffoPancakePrivateKey', 'Pancake · 签名私钥', 'secret'),
      field('WaffoPancakeStoreID', 'Pancake · 商店 ID'),
      field('WaffoPancakeProductID', 'Pancake · 充值产品 ID'),
      field('WaffoPancakeReturnURL', 'Pancake · 付款返回地址'),
      field('WaffoPancakeUnitPrice', 'Pancake · 每单位额度单价', 'number'),
      field('WaffoPancakeMinTopUp', 'Pancake · 最低充值额度', 'number'),
    ],
  },
  'content/dashboard': {
    title: '数据看板',
    description: '看板数据导出',
    fields: [
      field('DataExportEnabled', '允许导出', 'toggle'),
      field('DataExportInterval', '导出间隔', 'number'),
      field('DataExportDefaultTime', '默认时间范围'),
    ],
  },
  'content/drawing': {
    title: '绘图配置',
    description: '绘图网关功能',
    fields: [
      field('DrawingEnabled', '启用绘图', 'toggle'),
      field('MjNotifyEnabled', '任务通知', 'toggle'),
      field('MjAccountFilterEnabled', '账户过滤', 'toggle'),
      field('MjForwardUrlEnabled', '转发图片地址', 'toggle'),
      field('MjModeClearEnabled', '清理模式', 'toggle'),
    ],
  },
  'operations/behavior': {
    title: '系统行为',
    description: '管理页面基础行为',
    fields: [
      field('DemoSiteEnabled', '演示模式', 'toggle'),
      field('SelfUseModeEnabled', '个人使用模式', 'toggle'),
    ],
  },
  'operations/alerts': {
    title: '监控与告警',
    description: '额度预警和性能指标',
    fields: [
      field('QuotaRemindThreshold', '额度提醒阈值', 'number'),
      field('perf_metrics_setting.enabled', '性能指标', 'toggle'),
      field('perf_metrics_setting.flush_interval', '采集间隔（分钟）', 'number'),
      field('perf_metrics_setting.bucket_time', '聚合粒度'),
      field('perf_metrics_setting.retention_days', '保留天数', 'number'),
    ],
  },
  'operations/email': {
    title: '系统邮件',
    description: '发送验证码、账户通知和服务提醒',
    fields: [
      field('SMTPServer', 'SMTP 服务器'),
      field('SMTPPort', '端口', 'number'),
      field('SMTPAccount', '账号'),
      field('SMTPFrom', '发件地址'),
      field('SMTPToken', '授权密钥', 'secret'),
      field('SMTPSSLEnabled', 'SSL', 'toggle'),
      field('SMTPStartTLSEnabled', 'STARTTLS', 'toggle'),
    ],
  },
  'operations/worker': {
    title: 'Worker 代理',
    description: '图片代理及外部请求',
    fields: [
      field('WorkerUrl', 'Worker 地址'),
      field('WorkerValidKey', '校验密钥', 'secret'),
      field('WorkerAllowHttpImageRequestEnabled', '允许 HTTP 图片', 'toggle'),
    ],
  },
  'operations/logs': {
    title: '日志维护',
    description: '日志记录策略',
    fields: [field('LogConsumeEnabled', '记录消耗日志', 'toggle')],
  },
  'operations/performance': {
    title: '性能',
    description: '缓存及系统指标',
    fields: [
      field('performance_setting.disk_cache_enabled', '磁盘缓存', 'toggle'),
      field(
        'performance_setting.disk_cache_threshold_mb',
        '缓存阈值（MB）',
        'number'
      ),
      field(
        'performance_setting.disk_cache_max_size_mb',
        '缓存容量（MB）',
        'number'
      ),
      field('performance_setting.disk_cache_path', '缓存路径'),
      field('performance_setting.monitor_enabled', '启用资源监控', 'toggle'),
      field(
        'performance_setting.monitor_cpu_threshold',
        'CPU 告警阈值',
        'number'
      ),
      field(
        'performance_setting.monitor_memory_threshold',
        '内存告警阈值',
        'number'
      ),
      field('performance_setting.monitor_disk_threshold', '磁盘告警阈值', 'number'),
    ],
  },
}

export const ENTRY_BY_PATH = new Map(
  NATIVE_FEATURE_ENTRIES.map((entry) => [entry.path, entry])
)

export function settingsPageFor(pathname: string): SettingsPage | undefined {
  const match = pathname.match(/^\/system-settings\/([^/]+)\/([^/]+)$/)
  return match ? SETTINGS_PAGES[`${match[1]}/${match[2]}`] : undefined
}
