export type SettingValues = Record<string, string>
export type SettingPreset = {
  id: string
  label: string
  description: string
  values: SettingValues
  unavailable?: string
}

const preset = (id: string, label: string, description: string, values: SettingValues): SettingPreset => ({ id, label, description, values })

function siteURL(values: SettingValues, origin: string): URL | undefined {
  try {
    const url = new URL(values.ServerAddress?.trim() || origin)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) return
    return url
  } catch { return }
}

function mergeDefault(raw: string | undefined, value: unknown): string {
  const previous: unknown = raw?.trim() ? JSON.parse(raw) : {}
  if (!previous || typeof previous !== 'object' || Array.isArray(previous)) throw new Error('请先将现有模型规则改为 JSON 对象。')
  return JSON.stringify({ ...previous, default: value }, null, 2)
}

/** Presets only fill visible, editable fields. Persistence remains the form's normal save operation. */
export function settingPresetsFor(pathname: string, values: SettingValues, origin: string): SettingPreset[] {
  const section = pathname.replace(/^\/system-settings\//, '')
  switch (section) {
    case 'site/system-info': {
      const url = siteURL(values, origin)
      return [{ ...preset('site-address', '使用站点任务地址', '把已配置的站点地址和部署路径用作任务回调地址；文档和条款保留原内容。', url ? { TaskPublicAddress: url.origin + url.pathname.replace(/\/+$/, '') } : {}), unavailable: url ? undefined : '请先在品牌与登录中填写有效的站点地址。' }]
    }
    case 'auth/basic-auth': {
      const email = preset('verified-email', '邮箱验证注册', '启用密码登录、注册邮件验证及邮箱别名限制；现有域名白名单保持不变。', { PasswordLoginEnabled: 'true', EmailVerificationEnabled: 'true', EmailAliasRestrictionEnabled: 'true' })
      if (!values.SMTPServer || !values.SMTPAccount || !(values.SMTPToken || values.SMTPTokenConfigured === 'true')) email.unavailable = '先在系统邮件中保存 SMTP 服务器、账号和授权码，再启用邮件验证。'
      return [email]
    }
    case 'auth/passkey': {
      const url = siteURL(values, origin)
      const valid = url && url.protocol === 'https:' && !url.hostname.includes(':') && !/^\d+(?:\.\d+){3}$/.test(url.hostname)
      const currentDomain = values['passkey.rp_id']?.trim()
      const domains = new Set((values['passkey.legacy_rp_ids'] || '').split(/[\s,]+/).filter(Boolean))
      if (currentDomain && currentDomain !== url?.hostname) domains.add(currentDomain)
      return [{ ...preset('passkey-domain', '按站点域名配置', '启用设备身份验证，自动填写 HTTPS 来源；旧服务域名会保留，保存时仍需检查域名变更。', valid ? {
        'passkey.enabled': 'true', 'passkey.rp_id': url.hostname,
        'passkey.origins': url.origin, 'passkey.legacy_rp_ids': [...domains].join(','),
        'passkey.user_verification': 'required',
      } : {}), unavailable: valid ? undefined : '请先在品牌与登录中配置 HTTPS 站点域名；IP 地址不能作为通行密钥服务域名。' }]
    }
    case 'security/rate-limit':
      return [
        preset('shared', '小规模共享', '每位用户每分钟最多 60 次请求、45 次成功调用。', { ModelRequestRateLimitEnabled: 'true', ModelRequestRateLimitDurationMinutes: '1', ModelRequestRateLimitCount: '60', ModelRequestRateLimitSuccessCount: '45' }),
        preset('high-volume', '高并发接入', '每位用户每分钟最多 600 次请求、450 次成功调用。', { ModelRequestRateLimitEnabled: 'true', ModelRequestRateLimitDurationMinutes: '1', ModelRequestRateLimitCount: '600', ModelRequestRateLimitSuccessCount: '450' }),
      ]
    case 'security/ssrf':
      return [preset('public-https', '公网访问 · 443 端口', '开启地址防护和域名解析后的 IP 检查，禁止访问私有 IP，仅开放 443 端口；现有域名和 IP 列表保留。', { 'fetch_setting.enable_ssrf_protection': 'true', 'fetch_setting.allow_private_ip': 'false', 'fetch_setting.apply_ip_filter_for_domain': 'true', 'fetch_setting.allowed_ports': '["443"]' })]
    case 'security/token-limits':
      return [preset('personal-keys', '个人接入 · 10 个 Key', '每个账户最多创建 10 个 API Key。', { 'token_setting.max_user_tokens': '10' }), preset('team-keys', '团队分发 · 50 个 Key', '每个账户最多创建 50 个 API Key。', { 'token_setting.max_user_tokens': '50' })]
    case 'request-policies/filtering':
      return [{ ...preset('prompt-filter', '检测请求与提示词', '使用已填写的敏感词同时检测请求内容和提示词。', { CheckSensitiveEnabled: 'true', CheckSensitiveOnPromptEnabled: 'true' }), unavailable: values.SensitiveWords?.trim() ? undefined : '先填写需要检测的敏感词，再应用检测预设。' }]
    case 'request-policies/routing':
      return [
        preset('resilient', '稳健重试', '上游限流或临时故障最多重试 2 次；同一会话优先沿用原渠道 5 分钟。', { RetryTimes: '2', AutomaticRetryStatusCodes: '429,500,502,503,504', 'channel_affinity_setting.enabled': 'true', 'channel_affinity_setting.default_ttl_seconds': '300', 'channel_affinity_setting.max_entries': '100000' }),
        preset('low-latency', '低延迟请求', '仅对网关临时故障重试 1 次；同一会话优先沿用原渠道 3 分钟。', { RetryTimes: '1', AutomaticRetryStatusCodes: '502,503,504', 'channel_affinity_setting.enabled': 'true', 'channel_affinity_setting.default_ttl_seconds': '180', 'channel_affinity_setting.max_entries': '100000' }),
      ]
    case 'request-policies/health':
      return [preset('automatic-recovery', '定时检测与恢复', '每 10 分钟测试渠道；鉴权失败自动停用，恢复健康后自动启用。响应超过 5 秒视作异常。', { AutomaticDisableChannelEnabled: 'true', AutomaticEnableChannelEnabled: 'true', ChannelDisableThreshold: '5', AutomaticDisableStatusCodes: '401,403', 'monitor_setting.auto_test_channel_enabled': 'true', 'monitor_setting.auto_test_channel_minutes': '10' })]
    case 'models/global':
      return [preset('stream-heartbeat', '流式连接保活', '每 15 秒发送流式心跳，减少代理等待上游期间断开连接；模型转换规则保留。', { 'general_setting.ping_interval_enabled': 'true', 'general_setting.ping_interval_seconds': '15' })]
    case 'models/gemini':
      return [preset('gemini-thinking', 'Gemini 思考适配', '启用思考模型适配，使用 60% 的输出预算；模型版本和安全策略保留。', { 'gemini.thinking_adapter_enabled': 'true', 'gemini.thinking_adapter_budget_tokens_percentage': '0.6' })]
    case 'models/claude': {
      const item = preset('claude-thinking', 'Claude 思考适配', '默认输出上限 8192，思考预算为 80%；单个模型的例外值和自定义请求头保留。', { 'claude.thinking_adapter_enabled': 'true', 'claude.thinking_adapter_budget_tokens_percentage': '0.8' })
      try { item.values['claude.default_max_tokens'] = mergeDefault(values['claude.default_max_tokens'], 8192) }
      catch { item.unavailable = '请先修正默认最大令牌的 JSON 对象，再应用预设。' }
      return [item]
    }
    case 'billing/quota':
      return [preset('preconsume', '标准预扣策略', '按预计输入费用的 1 倍预留额度，关闭钱包免预扣；邀请奖励和充值地址保留。', { 'quota_setting.trust_quota_usd': '0', 'quota_setting.pre_consume_multiplier': '1' })]
    case 'billing/currency':
      return [preset('usd-display', '美元额度显示', '使用美元展示额度并显示 Token 用量；额度单位和业务汇率保持不变。', { DisplayInCurrencyEnabled: 'true', DisplayTokenStatEnabled: 'true', 'general_setting.quota_display_type': 'USD' })]
    case 'content/dashboard':
      return [preset('hourly-report', '小时统计', '启用数据导出，每 5 分钟汇总，默认查看小时数据。', { DataExportEnabled: 'true', DataExportInterval: '5', DataExportDefaultTime: 'hour' })]
    case 'content/drawing':
      return [preset('drawing-service', '绘图任务接入', '启用绘图、任务通知和账户过滤；图片转发与模式清理保留原设置。', { DrawingEnabled: 'true', MjNotifyEnabled: 'true', MjAccountFilterEnabled: 'true' })]
    case 'operations/behavior':
      return [preset('public-service', '正式对外服务', '关闭演示模式和个人使用模式。', { DemoSiteEnabled: 'false', SelfUseModeEnabled: 'false' }), preset('personal-service', '个人使用', '关闭演示模式，启用个人使用模式。', { DemoSiteEnabled: 'false', SelfUseModeEnabled: 'true' })]
    case 'operations/alerts':
      return [
        preset('diagnostics', '近期排障', '每分钟采集并按分钟聚合指标，只保留最近 7 天；余额提醒阈值保留。', { 'perf_metrics_setting.enabled': 'true', 'perf_metrics_setting.flush_interval': '1', 'perf_metrics_setting.bucket_time': 'minute', 'perf_metrics_setting.retention_days': '7' }),
        preset('trends', '月度趋势', '每 5 分钟采集、按小时聚合，只保留最近 30 天；余额提醒阈值保留。', { 'perf_metrics_setting.enabled': 'true', 'perf_metrics_setting.flush_interval': '5', 'perf_metrics_setting.bucket_time': 'hour', 'perf_metrics_setting.retention_days': '30' }),
      ]
    case 'operations/email': {
      const sender: SettingValues = !values.SMTPFrom?.trim() && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(values.SMTPAccount || '') ? { SMTPFrom: values.SMTPAccount } : {}
      const transport = (port: string, ssl: boolean, host?: string): SettingValues => ({ ...sender, SMTPPort: port, SMTPSSLEnabled: String(ssl), SMTPStartTLSEnabled: String(!ssl), ...(host ? { SMTPServer: host } : {}) })
      return [
        preset('smtp-ssl', 'SSL · 465', '使用 465 端口和 SSL 加密；邮件服务器、账号和授权码保留。', transport('465', true)),
        preset('smtp-starttls', 'STARTTLS · 587', '使用 587 端口和 STARTTLS 加密；邮件服务器、账号和授权码保留。', transport('587', false)),
        preset('smtp-qq', 'QQ 邮箱', '填写 smtp.qq.com、465 端口和 SSL；账号与授权码需自行填写。', transport('465', true, 'smtp.qq.com')),
        preset('smtp-gmail', 'Gmail', '填写 smtp.gmail.com、587 端口和 STARTTLS；账号与应用密码需自行填写。', transport('587', false, 'smtp.gmail.com')),
      ]
    }
    case 'operations/performance': {
      const monitor = { 'performance_setting.monitor_enabled': 'true', 'performance_setting.monitor_cpu_threshold': '90', 'performance_setting.monitor_memory_threshold': '90', 'performance_setting.monitor_disk_threshold': '95' }
      return [
        preset('monitor-only', '资源监控', '启用资源监控：CPU 和内存 90%、磁盘 95% 告警；缓存策略保留。', monitor),
        preset('low-memory', '减少大请求内存占用', '超过 10 MB 的请求体使用磁盘缓存，上限 1 GB，并启用资源监控；缓存目录保留。', { ...monitor, 'performance_setting.disk_cache_enabled': 'true', 'performance_setting.disk_cache_threshold_mb': '10', 'performance_setting.disk_cache_max_size_mb': '1024' }),
      ]
    }
    default: return []
  }
}
