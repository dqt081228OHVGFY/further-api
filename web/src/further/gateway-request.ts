import axios from 'axios'

import { login, login2fa, logout } from '@/features/auth/api'
import { isLoginChallenge } from '@/features/auth/secure-verification/api'
import { api } from '@/lib/api'
import { applyAuthBundle, clearAuthentication, isAuthBundle } from '@/lib/auth-session'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { handleAdminRequest } from './admin-adapter'
import { handleConsoleRequest } from './console-adapter'
import { loadWorkspaceBootstrap } from './workspace-bootstrap'

export interface LegacyRequestOptions {
  method?: string
  body?: unknown
  _suppressAuthRedirect?: boolean
  passwordEncryptionEnabled?: boolean
}

export function legacyRole(role: number): number {
  if (role >= 100) return 100
  if (role >= 10) return 50
  return role
}

export async function gatewayRequest(path: string, options: LegacyRequestOptions = {}): Promise<unknown> {
  const normalized = { ...options }
  if (typeof normalized.body === 'string') {
    normalized.body = JSON.parse(normalized.body)
  }
  try {
    if (path === '/api/user/login' || path === '/api/auth/login') {
      const credentials = normalized.body as Record<string, unknown> | null
      if (typeof credentials?.username !== 'string' || typeof credentials.password !== 'string') {
        throw new Error('请输入用户名和密码')
      }
      const response = requireServerSuccess(await login({
        username: credentials.username,
        password: credentials.password,
        passwordEncryptionEnabled: normalized.passwordEncryptionEnabled === true,
      }))
      const payload = response.data
      if (isAuthBundle(payload)) {
        applyAuthBundle(payload)
        return { ...payload, user: { ...payload.user, role: legacyRole(payload.user.role) } }
      }
      if (isLoginChallenge(payload) || isLegacyTwoFAChallenge(payload)) return payload
      throw new Error('登录响应未包含有效会话或验证流程')
    }
    if (path === '/api/user/login/2fa' || path === '/api/user/login/verify') {
      const verification = normalized.body as Record<string, unknown> | null
      if (typeof verification?.flow_token !== 'string' || !verification.flow_token || typeof verification.code !== 'string') {
        throw new Error('登录验证流程无效，请重新登录')
      }
      const response = requireServerSuccess(await login2fa({
        flow_token: verification.flow_token,
        code: verification.code,
      }))
      if (!isAuthBundle(response.data)) throw new Error('验证响应未包含有效会话')
      applyAuthBundle(response.data)
      return { ...response.data, user: { ...response.data.user, role: legacyRole(response.data.user.role) } }
    }
    if (path === '/api/user/self') {
      const response = await api.get('/api/user/self')
      const user = requireServerSuccess(response.data).data
      return { ...user, role: legacyRole(Number(user.role)) }
    }
    if (path === '/api/auth/config') {
      const status = (await loadWorkspaceBootstrap()).status
      const registrationResponse = await api.get('/api/further/registration/config')
      const registration = requireServerSuccess(registrationResponse.data).data
      return {
        ...registration,
        system_name: 'further-api',
        logo: '/further-api-logo.svg?v=handshake-20261004',
        linuxdo_client_id: status.linuxdo_client_id,
        local_registration_enabled: status.register_enabled && status.password_register_enabled !== false && registration.local_registration_enabled !== false,
        linuxdo_login_enabled: Boolean(status.linuxdo_oauth),
        linuxdo_registration_enabled: Boolean(status.linuxdo_oauth) && registration.linuxdo_registration_enabled !== false,
        password_login_encryption_enabled: status.password_login_encryption_enabled === true,
      }
    }
    if (path === '/api/site/announcement') {
      const response = await api.get('/api/notice')
      return { content: requireServerSuccess(response.data).data || '', title: '站点公告', user_docs_visible: true }
    }
    const consoleResult = await handleConsoleRequest(path, normalized)
    if (consoleResult.handled) return consoleResult.data
    const adminResult = await handleAdminRequest(path, normalized)
    if (adminResult.handled) return adminResult.data
    const response = await api.request({ url: path, method: normalized.method || 'GET', data: normalized.body })
    const result = requireServerSuccess(response.data)
    const payload = result && typeof result === 'object' && 'data' in result ? result.data : result
    return payload
  } catch (error) {
    const loginPath = path === '/api/user/login' || path === '/api/auth/login' || path === '/api/user/login/2fa' || path === '/api/user/login/verify'
    if (axios.isAxiosError(error)) {
      const status = error.response?.status
      if (status === 502 || status === 503 || status === 504) {
        error.message = `服务暂时不可用（HTTP ${status}），请稍后重试。`
      } else if (error.code === 'ECONNABORTED' || error.code === 'ETIMEDOUT') {
        error.message = '请求超时，请检查网络连接后重试。'
      } else if (error.code === 'ERR_NETWORK') {
        error.message = '无法连接服务，请检查网络连接后重试。'
      }
      if (status === 401 && !normalized._suppressAuthRedirect && !loginPath) {
        const redirect = location.pathname + location.search + location.hash
        location.assign(`/sign-in?redirect=${encodeURIComponent(redirect)}`)
      }
    }
    throw error
  }
}

function isLegacyTwoFAChallenge(value: unknown): value is { require_2fa: true; flow_token: string } {
  if (!value || typeof value !== 'object') return false
  const challenge = value as Record<string, unknown>
  return challenge.require_2fa === true && typeof challenge.flow_token === 'string' && challenge.flow_token.length > 0
}

export async function gatewayLogout(): Promise<void> {
  requireServerSuccess(await logout())
  clearAuthentication()
}

export function gatewayAuthenticated(): boolean {
  return Boolean(useAuthStore.getState().auth.user)
}

export async function gatewayExportLogs(): Promise<Response> {
  const response = await api.get<Blob>('/api/user/logs/export', { responseType: 'blob' })
  return new Response(response.data, { status: response.status })
}
