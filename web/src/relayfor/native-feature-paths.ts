/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

// The in-shell React router only receives application page paths. Keep section
// names dynamic: each existing route owns validation and permission checks.
const featureRoots = new Set([
  'channels',
  'chat',
  'chat2link',
  'dashboard',
  'errors',
  'keys',
  'models',
  'playground',
  'profile',
  'redemption-codes',
  'risk-control',
  'security',
  'subscriptions',
  'system-info',
  'system-settings',
  'task-plugins',
  'usage-logs',
  'users',
  'wallet',
])

export type NativeFeatureEntry = {
  title: string
  path: string
  group: string
  /** Minimum New API role; the route itself remains the authority. */
  role?: number
}

const settingsSections = {
  site: [
    ['system-info', '站点信息'],
    ['notice', '系统公告'],
    ['header-navigation', '顶部导航'],
    ['sidebar-modules', '侧栏模块'],
  ],
  auth: [
    ['basic-auth', '基础认证'],
    ['oauth', 'OAuth 登录'],
    ['passkey', '通行密钥'],
    ['bot-protection', '机器人防护'],
    ['custom-oauth', '自定义 OAuth'],
  ],
  security: [
    ['rate-limit', '速率限制'],
    ['ssrf', '上游地址防护'],
    ['token-limits', '令牌限制'],
  ],
  'request-policies': [
    ['filtering', '请求检测'],
    ['routing', '会话与重试'],
    ['health', '渠道健康'],
  ],
  models: [
    ['global', '全局模型'],
    ['gemini', 'Gemini 协议'],
    ['claude', 'Claude 协议'],
    ['grok', 'Grok 协议'],
    ['model-deployment', '模型部署'],
  ],
  billing: [
    ['quota', '额度设置'],
    ['currency', '货币与显示'],
    ['model-pricing', '模型计费'],
    ['group-pricing', '分组倍率'],
    ['payment', '支付网关'],
  ],
  content: [
    ['dashboard', '数据看板'],
    ['announcements', '公告内容'],
    ['api-info', 'API 地址'],
    ['faq', '常见问题'],
    ['uptime-kuma', 'Uptime Kuma'],
    ['chat', '聊天预设'],
    ['drawing', '绘图配置'],
  ],
  operations: [
    ['behavior', '系统行为'],
    ['alerts', '监控与告警'],
    ['email', 'SMTP 邮件'],
    ['worker', 'Worker 代理'],
    ['logs', '日志维护'],
    ['performance', '性能'],
    ['update-checker', '系统维护'],
  ],
} as const

const settingsGroupTitles: Record<keyof typeof settingsSections, string> = {
  site: '系统设置 · 站点',
  auth: '系统设置 · 认证',
  security: '系统设置 · 安全',
  'request-policies': '系统设置 · 请求策略',
  models: '系统设置 · 模型',
  billing: '系统设置 · 计费',
  content: '系统设置 · 内容',
  operations: '系统设置 · 运维',
}

const settingsEntries = Object.entries(settingsSections).flatMap(
  ([group, sections]) =>
    sections.map(([section, title]) => ({
      title,
      path: `/system-settings/${group}/${section}`,
      group: settingsGroupTitles[group as keyof typeof settingsSections],
      role: 100,
    }))
)

/** All stable authenticated feature destinations used by the RelayFor shell. */
export const NATIVE_FEATURE_ENTRIES: readonly NativeFeatureEntry[] = [
  { title: '模型调用分析', path: '/dashboard/models', group: '分析' },
  { title: '流量分析', path: '/dashboard/flow', group: '分析' },
  { title: '用户分析', path: '/dashboard/users', group: '分析', role: 10 },
  { title: 'API Key 扩展设置', path: '/keys', group: '账户' },
  { title: '更多账户设置', path: '/profile', group: '账户' },
  { title: '账户安全', path: '/security', group: '账户' },
  { title: '充值与支付', path: '/wallet', group: '账户' },
  { title: '请求游乐场', path: '/playground', group: '开发' },
  { title: '聊天直达', path: '/chat2link', group: '开发' },
  { title: '调用日志', path: '/usage-logs/common', group: '日志' },
  { title: '绘图日志', path: '/usage-logs/drawing', group: '日志' },
  { title: '任务日志', path: '/usage-logs/task', group: '日志' },
  { title: '敏感操作审计', path: '/usage-logs/audit', group: '日志', role: 10 },
  { title: '渠道管理', path: '/channels', group: '管理', role: 10 },
  { title: '模型资料', path: '/models/metadata', group: '管理', role: 10 },
  { title: '模型厂商', path: '/models/vendors', group: '管理', role: 10 },
  { title: '模型部署', path: '/models/deployments', group: '管理', role: 10 },
  { title: '用户管理', path: '/users', group: '管理', role: 10 },
  { title: '订阅管理', path: '/subscriptions', group: '管理', role: 10 },
  { title: '兑换码管理', path: '/redemption-codes', group: '管理', role: 10 },
  { title: '风险控制', path: '/risk-control', group: '管理', role: 10 },
  { title: '任务插件', path: '/task-plugins', group: '管理', role: 100 },
  { title: '运行监控', path: '/system-info', group: '管理', role: 100 },
  ...settingsEntries,
]

function decodedPath(path: string): string {
  let decoded = path
  for (let depth = 0; depth < 4; depth += 1) {
    let next: string
    try {
      next = decodeURIComponent(decoded)
    } catch {
      throw new Error('Invalid feature path encoding')
    }
    if (next === decoded) break
    decoded = next
  }
  return decoded
}

function hasUnsafeCharacter(value: string): boolean {
  return [...value].some((character) => {
    const code = character.charCodeAt(0)
    return code < 32 || code === 127 || character === '\\'
  })
}

export function normalizeNativeFeaturePath(raw: string): string {
  if (typeof raw !== 'string' || !raw || raw !== raw.trim()) {
    throw new Error('Invalid feature path')
  }
  if (hasUnsafeCharacter(raw) || raw.includes('#')) {
    throw new Error('Invalid feature path')
  }
  if (!raw.startsWith('/') && !/^https?:\/\//i.test(raw)) {
    throw new Error('Feature path must be an application URL')
  }
  if (raw.startsWith('//')) {
    throw new Error('External feature URL is not allowed')
  }

  const origin =
    typeof location === 'undefined'
      ? 'https://relayfor.invalid'
      : location.origin
  let url: URL
  try {
    url = new URL(raw, origin)
  } catch {
    throw new Error('Invalid feature URL')
  }
  if (url.origin !== origin || url.username || url.password) {
    throw new Error('External feature URL is not allowed')
  }

  const rawPath = raw.startsWith('/')
    ? raw.split('?')[0]
    : new URL(raw).pathname
  const check = decodedPath(rawPath)
  if (
    !check.startsWith('/') ||
    check.startsWith('//') ||
    hasUnsafeCharacter(check) ||
    check.split('/').some((part) => part === '.' || part === '..')
  ) {
    throw new Error('Invalid feature path')
  }
  const pathname = url.pathname.replace(/\/+$/, '') || '/'
  const root = pathname.split('/')[1]
  if (!featureRoots.has(root) || /^\/api(?:\/|$)/i.test(check)) {
    throw new Error('Unsupported feature path')
  }
  return pathname + url.search
}
