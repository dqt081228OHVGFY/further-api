import { beforeEach, describe, expect, it, vi } from 'vitest'

import { handleConsoleRequest } from '../console-adapter'

const http = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ api: http }))

function ok(data?: unknown) {
  return { data: { success: true, data } }
}

const key = {
  id: 7,
  name: '工作 Key',
  key: 'sk-****masked',
  status: 1,
  remain_quota: 1500,
  used_quota: 400,
  unlimited_quota: false,
  expired_time: 1790812800,
  created_time: 1790726400,
  accessed_time: 1790726401,
  model_limits_enabled: true,
  model_limits: 'model-a,model-b',
  allow_ips: '127.0.0.1',
  group: 'auto',
  auto_groups: ['paid', 'default'],
  cross_group_retry: true,
}

beforeEach(() => {
  Object.values(http).forEach((mock) => mock.mockReset())
})

describe('Further console API compatibility', () => {
  it('reads an unrenderable server expiry without throwing or rewriting its raw value', async () => {
    const original = { ...key, expired_time: Number.MAX_SAFE_INTEGER, created_time: Number.MAX_SAFE_INTEGER, accessed_time: Number.MAX_SAFE_INTEGER }
    http.get.mockResolvedValueOnce(ok({ total: 1, items: [original] }))
    const response = await handleConsoleRequest('/api/user/tokens?page=1&page_size=25')
    expect(response).toMatchObject({ handled: true, data: { items: [{ expired_time: Number.MAX_SAFE_INTEGER, expired_at: null, expired_at_unavailable: true, created_at: null, accessed_at: null }] } })
    expect(original.expired_time).toBe(Number.MAX_SAFE_INTEGER)
    expect(http.get).toHaveBeenCalledOnce()
    expect(http.post).not.toHaveBeenCalled()
    expect(http.put).not.toHaveBeenCalled()
    expect(http.delete).not.toHaveBeenCalled()
  })
  it('loads exactly the selected page for the unified key list without changing historical restrictions', async () => {
    http.get.mockResolvedValueOnce(ok({ total: 10001, items: [key] }))
    const response = await handleConsoleRequest('/api/user/tokens?page=401&page_size=25')
    expect(http.get).toHaveBeenCalledOnce()
    expect(http.get.mock.calls[0][0]).toBe('/api/token/?p=401&size=25&page_size=25')
    expect(response).toMatchObject({ handled: true, data: { page: 401, page_size: 25, total: 10001, items: [{ id: 7, group: 'auto', cross_group_retry: true }] } })
  })
  it('rejects invalid page ranges before requesting key data', async () => {
    for (const query of ['page=0', 'page=-1', 'page=1.5', 'page=NaN', 'page_size=101', 'page_size=0']) {
      await expect(handleConsoleRequest(`/api/user/tokens?${query}`)).rejects.toThrow('分页参数无效')
    }
    expect(http.get).not.toHaveBeenCalled()
  })
  it('leaves server-owned overview, logs, preferences and unsupported security flows to the real backend', async () => {
    for (const path of [
      '/api/user/overview',
      '/api/user/logs?page=2',
      '/api/user/ui-preferences',
      '/api/user/public-benefits/token-loan',
      '/api/user/security/totp/setup',
      '/api/user/password',
      '/api/user/tokens/7/rotate',
    ]) {
      expect(await handleConsoleRequest(path, { method: 'POST' })).toEqual({
        handled: false,
      })
    }
    expect(await handleConsoleRequest('/api/user/overview')).toEqual({
      handled: false,
    })
    expect(http.get).not.toHaveBeenCalled()
    expect(http.post).not.toHaveBeenCalled()
  })

  it('loads every key page while retaining masked secrets and converting limits and timestamps', async () => {
    http.get
      .mockResolvedValueOnce(ok({ total: 2, items: [key] }))
      .mockResolvedValueOnce(
        ok({
          total: 2,
          items: [
            { ...key, id: 8, model_limits_enabled: false, expired_time: -1 },
          ],
        })
      )
    const response = await handleConsoleRequest('/api/user/tokens')
    expect(response).toMatchObject({
      handled: true,
      data: [
        {
          id: 7,
          key: 'sk-****masked',
          model_limits: ['model-a', 'model-b'],
          ip_limits: '127.0.0.1',
          expired_at: '2026-10-01T00:00:00.000Z',
        },
        { id: 8, model_limits: [], expired_at: null },
      ],
    })
    expect(http.get.mock.calls[1][0]).toContain('p=2')
    expect(http.post).not.toHaveBeenCalled()
  })

  it('rejects failed or incomplete listing responses instead of presenting an empty successful list', async () => {
    http.get.mockResolvedValueOnce({
      data: { success: false, message: 'Denied by server' },
    })
    await expect(handleConsoleRequest('/api/user/tokens')).rejects.toThrow()
    http.get.mockResolvedValueOnce(ok({ total: 2, items: [] }))
    await expect(handleConsoleRequest('/api/user/tokens')).rejects.toThrow(
      '分页数据不完整'
    )
  })

  it('creates the key with account quota semantics and reveals only the ID returned by the server', async () => {
    http.post
      .mockResolvedValueOnce(ok({ id: 7 }))
      .mockResolvedValueOnce(ok({ key: 'sk-new-server-key' }))
    http.get.mockResolvedValueOnce(ok(key))
    const response = await handleConsoleRequest('/api/user/tokens', {
      method: 'POST',
      body: {
        name: '工作 Key',
        remain_quota: -1,
        unlimited_quota: false,
        expires_at: '2026-10-01T00:00:00.000Z',
        model_limits: ['model-a', 'model-b'],
        ip_limits: '127.0.0.1',
      },
    })
    expect(http.post).toHaveBeenNthCalledWith(
      1,
      '/api/token/',
      expect.objectContaining({
        name: '工作 Key',
        remain_quota: 0,
        unlimited_quota: true,
        expired_time: 1790812800,
        model_limits: 'model-a,model-b',
        model_limits_enabled: true,
        allow_ips: '127.0.0.1',
      })
    )
    expect(http.post).toHaveBeenNthCalledWith(2, '/api/token/7/key', undefined)
    expect(response).toMatchObject({
      handled: true,
      data: { id: 7, key: 'sk-new-server-key' },
    })
  })

  it('does not create a key or retrieve a secret after a validation or server failure', async () => {
    await expect(
      handleConsoleRequest('/api/user/tokens', {
        method: 'POST',
        body: { name: 'invalid', remain_quota: 1, expires_at: 'invalid-date' },
      })
    ).rejects.toThrow('到期时间无效')
    expect(http.post).not.toHaveBeenCalled()
    http.post.mockRejectedValueOnce(new Error('Permission denied'))
    await expect(
      handleConsoleRequest('/api/user/tokens', {
        method: 'POST',
        body: { name: 'denied', remain_quota: 1 },
      })
    ).rejects.toThrow('Permission denied')
    expect(http.post).toHaveBeenCalledTimes(1)
    expect(http.get).not.toHaveBeenCalled()
  })

  it('preserves gateway grouping and auto-retry settings when the original form edits a key', async () => {
    http.get
      .mockResolvedValueOnce(ok(key))
      .mockResolvedValueOnce(ok({ ...key, name: 'renamed' }))
    http.put.mockResolvedValueOnce(ok())
    await handleConsoleRequest('/api/user/tokens/7', {
      method: 'PUT',
      body: {
        name: 'renamed',
        remain_quota: 250,
        unlimited_quota: false,
        model_limits: [],
        ip_limits: '',
        expires_at: '',
      },
    })
    expect(http.put).toHaveBeenCalledWith(
      '/api/token/',
      expect.objectContaining({
        id: 7,
        name: 'renamed',
        remain_quota: 250,
        expired_time: -1,
        group: 'auto',
        auto_groups: ['paid', 'default'],
        cross_group_retry: true,
      })
    )
  })

  it('changes only status through the dedicated status endpoint and returns the confirmed server row', async () => {
    http.put.mockResolvedValueOnce(ok())
    http.get.mockResolvedValueOnce(ok({ ...key, status: 2 }))
    expect(
      await handleConsoleRequest('/api/user/tokens/7', {
        method: 'PUT',
        body: { status: 2 },
      })
    ).toMatchObject({ data: { status: 2 } })
    expect(http.put).toHaveBeenCalledWith('/api/token/?status_only=true', {
      id: 7,
      status: 2,
    })
  })

  it('propagates failed deletion without a synthetic success', async () => {
    http.delete.mockResolvedValueOnce({
      data: { success: false, message: 'Not permitted' },
    })
    await expect(
      handleConsoleRequest('/api/user/tokens/7', { method: 'DELETE' })
    ).rejects.toThrow()
    expect(http.delete).toHaveBeenCalledWith('/api/token/7/')
  })

  it('uses real pricing ratios, endpoints and access lists while leaving dynamic prices explicitly unknown', async () => {
    http.get.mockImplementation(async (path: string) => {
      if (path === '/api/user/self') return ok({ group: 'paid' })
      if (path === '/api/user/models') return ok(['model-a', 'dynamic-model'])
      return {
        data: {
          success: true,
          group_ratio: { paid: 0.5 },
          supported_endpoint: { openai: { path: '/v1/chat/completions' } },
          data: [
            {
              model_name: 'model-a',
              quota_type: 0,
              model_ratio: 1.5,
              completion_ratio: 4,
              cache_ratio: 0.1,
              supported_endpoint_types: ['openai'],
            },
            {
              model_name: 'dynamic-model',
              quota_type: 0,
              model_ratio: 0,
              billing_mode: 'tiered_expr',
              billing_expr: 'price_expr',
            },
            { model_name: 'locked-model', quota_type: 1, model_price: 0.02 },
          ],
        },
      }
    })
    const result = await handleConsoleRequest('/api/user/models')
    expect(result).toMatchObject({
      handled: true,
      data: {
        group: 'paid',
        group_multiplier: 0.5,
        items: [
          {
            model: 'model-a',
            accessible: true,
            base_prices: {
              input: 3,
              output: 12,
              cache_read: 0.30000000000000004,
              cache_write: null,
            },
            effective_prices: { input: 1.5, output: 6 },
            supported_endpoints: ['/v1/chat/completions'],
          },
          {
            model: 'dynamic-model',
            accessible: true,
            free_model: false,
            price_display: '动态计费',
            base_prices: { input: null, output: null },
          },
          {
            model: 'locked-model',
            accessible: false,
            base_request_price: 0.02,
            effective_request_price: 0.01,
          },
        ],
      },
    })
  })

  it('redeems through the gateway and reports actual successes and failures for a mixed batch', async () => {
    http.post.mockResolvedValueOnce(ok(500000)).mockResolvedValueOnce({
      data: { success: false, message: 'Code already used' },
    })
    const result = await handleConsoleRequest('/api/user/redeem/batch', {
      method: 'POST',
      body: { codes: ['valid-code', 'used-code'] },
    })
    expect(http.post).toHaveBeenNthCalledWith(1, '/api/user/topup', {
      key: 'valid-code',
    })
    expect(result).toMatchObject({
      handled: true,
      data: {
        succeeded: 1,
        failed: 1,
        items: [
          { code: 'valid-code', success: true, added_quota: 500000 },
          { code: 'used-code', success: false },
        ],
      },
    })
  })

  it('updates the username without erasing the display name or fabricating a replacement auth token', async () => {
    http.get
      .mockResolvedValueOnce(ok({ username: 'before', display_name: '姓名' }))
      .mockResolvedValueOnce(ok({ username: 'after', display_name: '姓名' }))
    http.put.mockResolvedValueOnce(ok())
    expect(
      await handleConsoleRequest('/api/user/profile', {
        method: 'PATCH',
        body: { username: 'after' },
      })
    ).toEqual({
      handled: true,
      data: { user: { username: 'after', display_name: '姓名' } },
    })
    expect(http.put).toHaveBeenCalledWith('/api/user/self', {
      username: 'after',
      display_name: '姓名',
    })
  })
})
