import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AxiosError } from 'axios'

import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { gatewayRequest } from '../gateway-request'

const http = vi.hoisted(() => ({ request: vi.fn(), get: vi.fn(), post: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: http }))

beforeEach(() => {
  Object.values(http).forEach((mock) => mock.mockReset())
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

describe('Further login session compatibility', () => {
  it('returns a role usable by the admin landing redirect while keeping the server role in the auth session', async () => {
    const bundle: AuthBundle = {
      access_token: 'login-access-token', token_type: 'Bearer', access_expires_at: 1790818800,
      user: { id: 7, username: 'workspace-admin', role: 10 },
      session: { sid: 'login-session', current: true, login_method: 'password', ip: '127.0.0.1', user_agent: 'test-browser', created_at: 1790812800, last_active_at: 1790812800, expires_at: 1790899200 },
    }
    http.post.mockResolvedValue({ data: { success: true, data: bundle } })

    const result = await gatewayRequest('/api/user/login', { method: 'POST', body: { username: 'workspace-admin', password: 'example-password' } })

    expect(result).toMatchObject({ user: { id: 7, role: 50 } })
    expect(useAuthStore.getState().auth.user?.role).toBe(10)
    expect(useAuthStore.getState().auth.accessToken).toBe('login-access-token')
  })
})

describe('Further service failure handling', () => {
  it.each([502, 503, 504])('explains HTTP %s without retrying a mutation or altering its error details', async (status) => {
    const error = new AxiosError(`Request failed with status code ${status}`, 'ERR_BAD_RESPONSE')
    error.response = { status, data: '<html>Bad Gateway</html>', statusText: 'Service unavailable', headers: {}, config: { headers: {} } } as typeof error.response
    http.request.mockRejectedValue(error)

    await expect(gatewayRequest('/api/example', { method: 'POST', body: { value: 'edited' } })).rejects.toMatchObject({
      message: `服务暂时不可用（HTTP ${status}），请稍后重试。`,
      response: { status },
      code: 'ERR_BAD_RESPONSE',
    })
    expect(http.request).toHaveBeenCalledTimes(1)
    expect(useAuthStore.getState().auth.user).toBeNull()
  })

  it.each([
    ['ERR_NETWORK', '无法连接服务，请检查网络连接后重试。'],
    ['ECONNABORTED', '请求超时，请检查网络连接后重试。'],
    ['ETIMEDOUT', '请求超时，请检查网络连接后重试。'],
  ])('explains %s while preserving the rejected request', async (code, message) => {
    const error = new AxiosError('Network Error', code)
    http.get.mockRejectedValue(error)

    await expect(gatewayRequest('/api/auth/config')).rejects.toMatchObject({ code, message })
    expect(http.get).toHaveBeenCalledTimes(1)
  })

  it('lets a later explicit retry read live configuration after an outage', async () => {
    const error = new AxiosError('Network Error', 'ERR_NETWORK')
    http.get.mockRejectedValueOnce(error)
    await expect(gatewayRequest('/api/auth/config')).rejects.toThrow('无法连接服务')
    http.get.mockResolvedValueOnce({ data: { success: true, data: { schema_version: 1, setup: { status: true }, groups: {}, status: { system_name: 'further-api', register_enabled: true } } } })
    http.get.mockResolvedValueOnce({ data: { success: true, data: { local_registration_enabled: true } } })

    await expect(gatewayRequest('/api/auth/config')).resolves.toMatchObject({ system_name: 'further-api', local_registration_enabled: true })
    expect(http.get).toHaveBeenCalledTimes(3)
  })

  it('does not create a session or replay credentials when login fails', async () => {
    const error = new AxiosError('Network Error', 'ERR_NETWORK')
    http.post.mockRejectedValue(error)

    await expect(gatewayRequest('/api/user/login', { method: 'POST', body: { username: 'workspace-admin', password: 'example-password' } })).rejects.toThrow('无法连接服务')
    expect(http.post).toHaveBeenCalledTimes(1)
    expect(http.post).toHaveBeenCalledWith('/api/user/login?turnstile=', { username: 'workspace-admin', password: 'example-password' }, { skipAuthRefresh: true })
    expect(useAuthStore.getState().auth.accessToken).toBeNull()
    expect(useAuthStore.getState().auth.user).toBeNull()
  })

  it('keeps business failures and cancellation messages intact', async () => {
    http.request.mockResolvedValueOnce({ data: { success: false, message: '额度不足' } })
    await expect(gatewayRequest('/api/example')).rejects.toThrow('额度不足')
    const cancelled = new AxiosError('用户已取消', 'ERR_CANCELED')
    http.request.mockRejectedValueOnce(cancelled)
    await expect(gatewayRequest('/api/example')).rejects.toBe(cancelled)
    expect(cancelled.message).toBe('用户已取消')
  })
})
