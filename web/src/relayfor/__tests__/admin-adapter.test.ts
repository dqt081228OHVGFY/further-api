import { beforeEach, describe, expect, it, vi } from 'vitest'

import { handleAdminRequest } from '../admin-adapter'

const http = vi.hoisted(() => ({ get: vi.fn(), request: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: http }))
const ok = (data?: unknown) => ({ data: { success: true, data } })
beforeEach(() => {
  http.get.mockReset()
  http.request.mockReset()
  delete document.body.dataset.gatewayRole
})

describe('original RelayFor administrator API adapter', () => {
  it('loads and saves the real retry policy without losing status code ranges', async () => {
    http.get.mockResolvedValueOnce(ok({
      options: {
        RetryTimes: '2',
        AutomaticRetryStatusCodes: '429,500-503',
      },
    }))
    expect(await handleAdminRequest('/api/admin/settings/routing')).toMatchObject({
      handled: true,
      data: {
        settings: {
          retry_enabled: true,
          retry_max_attempts: 3,
          retry_status_codes: '429,500-503',
        },
      },
    })
    expect(http.get).toHaveBeenCalledWith('/api/option/request_policy')

    http.request.mockResolvedValueOnce(ok({ options: {} }))
    await handleAdminRequest('/api/admin/settings/routing', {
      method: 'PUT',
      body: {
        retry_enabled: true,
        retry_max_attempts: 3,
        retry_status_codes: '429,500-503',
      },
    })
    expect(http.request).toHaveBeenCalledWith({
      url: '/api/option/request_policy',
      method: 'PATCH',
      data: {
        options: {
          RetryTimes: '2',
          AutomaticRetryStatusCodes: '429,500-503',
        },
      },
    })
  })

  it('accepts the server default retry ranges when saving unchanged settings', async () => {
    http.request.mockResolvedValueOnce(ok({ options: {} }))
    await handleAdminRequest('/api/admin/settings/routing', {
      method: 'PUT',
      body: {
        retry_enabled: false,
        retry_max_attempts: 1,
        retry_status_codes: '100-199,300-399,401-407,409-499,500-503,505-523,525-599',
      },
    })
    expect(http.request.mock.calls[0][0].data.options).toMatchObject({
      RetryTimes: '0',
      AutomaticRetryStatusCodes: '100-199,300-399,401-407,409-503,505-523,525-599',
    })
  })

  it('saves one shared announcement from the original public notice form', async () => {
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/settings/announcements', {
      method: 'PUT',
      body: { public: '维护公告' },
    })
    expect(http.request).toHaveBeenCalledWith({
      url: '/api/option/',
      method: 'PUT',
      data: { key: 'Notice', value: '维护公告' },
    })
  })

  it('leaves server-owned contracts untouched', async () => {
    for (const path of [
      '/api/admin/overview',
      '/api/admin/logs?page=2',
      '/api/admin/registration-codes',
      '/api/admin/prepaid-cards',
      '/api/admin/subscription-plans',
      '/api/admin/subscription-benefits',
      '/api/admin/settings/token-loan',
      '/api/risk/users',
      '/api/admin/security/audit',
    ]) {
      expect(await handleAdminRequest(path, { body: [1, 2] })).toEqual({
        handled: false,
      })
    }
    expect(http.get).not.toHaveBeenCalled()
  })

  it('loads all channels and distinguishes Anthropic from New API Midjourney', async () => {
    http.get
      .mockResolvedValueOnce(
        ok({
          total: 2,
          items: [
            { id: 1, type: 14, status: 1, group: 'team', channel_info: {} },
          ],
        })
      )
      .mockResolvedValueOnce(
        ok({
          total: 2,
          items: [
            {
              id: 2,
              type: 2,
              status: 3,
              channel_info: { is_multi_key: true, multi_key_size: 4 },
            },
          ],
        })
      )
      .mockResolvedValueOnce(ok([]))
    expect(await handleAdminRequest('/api/admin/channels', {})).toMatchObject({
      handled: true,
      data: [
        {
          id: 1,
          type: 2,
          newapi_type: 14,
          provider_name: 'Anthropic',
          groups: 'team',
        },
        {
          id: 2,
          type: 3,
          newapi_type: 2,
          provider_name: 'Midjourney',
          key_count: 4,
          status: 3,
        },
      ],
    })
    expect(http.get.mock.calls[1][0]).toContain('p=2')
  })

  it('preserves Gemini channel provider and advanced configuration when editing the original form', async () => {
    http.get.mockResolvedValueOnce(
      ok({
        id: 7,
        type: 24,
        status: 1,
        name: 'old',
        models: 'gemini',
        group: 'default',
        setting: '{"proxy":"http://proxy"}',
        priority: 0,
        weight: 1,
        channel_info: {},
      })
    )
    http.request.mockResolvedValue(ok())
    await handleAdminRequest('/api/admin/channels/7', {
      method: 'PUT',
      body: {
        type: 3,
        name: 'new',
        key: '',
        groups: 'default',
        priority: 0,
        weight: 1,
        upstream_format: 'openai_chat',
        upstream_formats: ['openai_chat'],
        quota_total: -1,
        quota_remaining: -1,
      },
    })
    const sent = http.request.mock.calls[0][0]
    expect(sent).toMatchObject({
      url: '/api/channel/',
      method: 'PUT',
      data: {
        id: 7,
        type: 24,
        name: 'new',
        setting: '{"proxy":"http://proxy"}',
      },
    })
    expect(sent.data).not.toHaveProperty('status')
    expect(sent.data).not.toHaveProperty('key')
  })

  it('creates a multi-key channel with the original form advanced fields', async () => {
    http.request.mockResolvedValueOnce(ok({ id: 9 }))
    await handleAdminRequest('/api/admin/channels', {
      method: 'POST',
      body: {
        type: 3,
        newapi_type: 24,
        name: 'Gemini pool',
        key: 'first-key\nsecond-key',
        models: 'gemini-2.5-pro',
        groups: 'default',
        priority: 0,
        weight: 1,
        multi_key_mode: 'multi_to_single',
        multi_key_type: 'polling',
        batch_add_set_key_prefix_2_name: false,
        auto_ban: 0,
        tag: 'primary',
        param_override: '{"temperature":0.2}',
      },
    })
    expect(http.request).toHaveBeenCalledWith({
      url: '/api/channel/',
      method: 'POST',
      data: expect.objectContaining({
        mode: 'multi_to_single',
        multi_key_mode: 'polling',
        channel: expect.objectContaining({
          type: 24,
          auto_ban: 0,
          tag: 'primary',
          param_override: '{"temperature":0.2}',
        }),
      }),
    })
  })

  it('honours an explicit provider change within the same legacy category', async () => {
    http.get.mockResolvedValueOnce(ok({
      id: 7,
      type: 24,
      status: 1,
      group: 'default',
      priority: 0,
      weight: 1,
    }))
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/channels/7', {
      method: 'PUT',
      body: {
        type: 3,
        newapi_type: 20,
        key: '',
        name: 'OpenRouter',
        priority: 0,
        weight: 1,
      },
    })
    expect(http.request.mock.calls[0][0]).toMatchObject({
      url: '/api/channel/',
      method: 'PUT',
      data: { id: 7, type: 20, name: 'OpenRouter' },
    })
    expect(http.request.mock.calls[0][0].data).not.toHaveProperty('key')
  })

  it('refuses unsupported channel limits before any mutation', async () => {
    await expect(
      handleAdminRequest('/api/admin/channels', {
        method: 'POST',
        body: { type: 1, key: 'secret', quota_total: 10 },
      })
    ).rejects.toThrow('quota_total')
    expect(http.request).not.toHaveBeenCalled()
  })

  it('uses stable key IDs and rejects resource policies that are not enforced', async () => {
    http.get.mockResolvedValueOnce(
      ok([{ id: 83, current_index: 0, masked_key: 'sk-***', weight: 1 }])
    )
    expect(
      await handleAdminRequest('/api/admin/channels/7/keys', {})
    ).toMatchObject({ data: [{ id: 83, current_index: 0 }] })
    http.request.mockResolvedValueOnce(ok())
    await expect(handleAdminRequest('/api/admin/channels/7/keys/83', {
      method: 'PUT',
      body: { weight: 5, status: 2, quota_remaining: 120 },
    })).rejects.toThrow('quota_remaining')
    expect(http.request).not.toHaveBeenCalled()
    await handleAdminRequest('/api/admin/channels/7/keys/83', {
      method: 'PUT', body: { weight: 1, status: 2, label: 'backup' },
    })
    expect(http.request).toHaveBeenCalledWith({
      url: '/api/relayfor/channels/7/keys/83',
      method: 'PUT',
      data: { weight: 1, status: 2, label: 'backup' },
    })
  })

  it('converts seconds from a real probe and never reports a failed probe as successful', async () => {
    http.get.mockResolvedValueOnce({ data: { success: true, time: 0.125 } })
    expect(
      await handleAdminRequest('/api/admin/channels/7/probe', {
        method: 'POST',
      })
    ).toMatchObject({ data: { success: true, latency_ms: 125 } })
    http.get.mockResolvedValueOnce({
      data: { success: false, message: 'upstream failed', time: 0.1 },
    })
    await expect(
      handleAdminRequest('/api/admin/channels/7/probe', { method: 'POST' })
    ).rejects.toThrow()
  })

  it('maps roles only in UI and never invents privileges', async () => {
    http.get.mockResolvedValueOnce(
      ok({
        total: 1,
        items: [{ id: 7, role: 10, status: 1, created_at: 1790812800 }],
      })
    )
    expect(
      await handleAdminRequest(
        '/api/admin/users?page=2&page_size=25&role=50',
        {}
      )
    ).toMatchObject({
      data: { total_pages: 1, items: [{ role: 50, newapi_role: 10 }] },
    })
    expect(http.get.mock.calls[0][0]).toContain('role=10')
    expect(http.get.mock.calls[0][0]).toContain('p=2')
    await expect(
      handleAdminRequest('/api/admin/users?role=25', {})
    ).rejects.toThrow('角色')
  })

  it('passes sleeping-user filters to real activity queries without filtering only one page', async () => {
    http.get.mockResolvedValueOnce(ok({ total: 3, items: [{ id: 3, role: 1, status: 1 }] }))
    expect(await handleAdminRequest('/api/admin/users?page=2&page_size=1&sleeping=1&role=1')).toMatchObject({ data: { total: 3, total_pages: 3, items: [{ id: 3 }] } })
    expect(http.get.mock.calls[0][0]).toContain('inactive_days=14')
    expect(http.get.mock.calls[0][0]).not.toContain('sleeping=')
    expect(http.get.mock.calls[0][0]).toContain('p=2')
  })

  it('queries the native supported channel balance endpoint without adjusting Key quotas or status', async () => {
    http.get.mockResolvedValueOnce(ok({ id: 7, name: 'DeepSeek', type: 43, channel_info: {} }))
      .mockResolvedValueOnce(ok([{ id: 83, key: 'sk-***', quota_remaining: 0 }]))
      .mockResolvedValueOnce({ data: { success: true, balance: 4.25 } })
    expect(await handleAdminRequest('/api/admin/channels/7/keys/83/refresh-balance', { method: 'POST' })).toMatchObject({ data: { scope: 'channel', checked: 1, updated: 1, recovered: 0, reports: [{ channel_id: 7, key_id: 83, scope: 'channel', balance_usd: 4.25, status: 'updated' }] } })
    expect(http.get).toHaveBeenCalledWith('/api/channel/update_balance/7')
    expect(http.request).not.toHaveBeenCalled()
  })

  it('returns separate batch balance results for unsupported, multi-key and failed channels', async () => {
    http.get.mockImplementation(async (url: string) => {
      if (url.startsWith('/api/channel/?')) {
        return ok({ total: 4, items: [
          { id: 7, name: 'supported', type: 43 }, { id: 8, name: 'unsupported', type: 24 },
          { id: 9, name: 'multi', type: 1, channel_info: { is_multi_key: true } }, { id: 10, name: 'failed', type: 1 },
        ] })
      }
      if (url === '/api/channel/update_balance/7') return { data: { success: true, balance: 6 } }
      if (url === '/api/channel/update_balance/10') throw new Error('provider unavailable')
      throw new Error('unexpected request')
    })
    expect(await handleAdminRequest('/api/admin/channel-keys/refresh-balances', { method: 'POST' })).toMatchObject({ data: { checked: 2, updated: 1, unavailable: 2, failed: 1, recovered: 0, reports: [{ status: 'updated' }, { status: 'unavailable' }, { status: 'unavailable' }, { status: 'failed', message: 'provider unavailable' }] } })
    expect(http.get).not.toHaveBeenCalledWith('/api/channel/update_balance/8')
    expect(http.get).not.toHaveBeenCalledWith('/api/channel/update_balance/9')
    expect(http.request).not.toHaveBeenCalled()
  })

  it('passes enforced per-key selection weights to the real metadata mutation', async () => {
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/channels/7/keys/83', { method: 'PUT', body: { weight: 6, status: 1, label: 'weighted' } })
    expect(http.request).toHaveBeenCalledWith({ url: '/api/relayfor/channels/7/keys/83', method: 'PUT', data: { weight: 6, status: 1, label: 'weighted' } })
  })

  it('adjusts quota through the atomic New API quota endpoint', async () => {
    http.get.mockResolvedValueOnce(
      ok({
        id: 7,
        username: 'user',
        display_name: 'User',
        role: 1,
        status: 1,
        quota: 100,
        group: 'default',
      })
    )
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/users/7', {
      method: 'PATCH',
      body: { quota: 200, status: 1, role: 1 },
    })
    expect(http.request).toHaveBeenCalledExactlyOnceWith({
      url: '/api/user/manage',
      method: 'POST',
      data: { id: 7, action: 'add_quota', mode: 'override', value: 200 },
    })
  })

  it('saves merged profile fields without overwriting the role or other profile fields', async () => {
    http.get.mockResolvedValueOnce(ok({ id: 7, username: 'alice', display_name: 'Alice', role: 1, status: 1, quota: 100, group: 'default', remark: 'old' }))
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/users/7', { method: 'PATCH', body: { display_name: 'Alice Chen', remark: '' } })
    expect(http.request).toHaveBeenCalledExactlyOnceWith({ url: '/api/user/', method: 'PUT', data: { id: 7, username: 'alice', display_name: 'Alice Chen', role: 1, group: 'default', remark: '' } })
  })

  it('distinguishes used codes and does not guess a shared batch identity', async () => {
    http.get.mockResolvedValueOnce(
      ok({
        total: 2,
        items: [
          {
            batch_id: 'newapi-1',
            items: [{
            id: 1,
            name: 'same',
            created_time: 1790812800,
            status: 3,
            key: 'used',
            quota: 10,
            }],
          },
          {
            batch_id: 'newapi-2',
            items: [{
            id: 2,
            name: 'same',
            created_time: 1790812800,
            status: 1,
            key: 'unused',
            quota: 10,
            }],
          },
        ],
      })
    )
    const result = await handleAdminRequest(
      '/api/admin/redemptions?page=1&page_size=25&status=2',
      {}
    )
    expect(result).toMatchObject({
      data: {
        items: [
          {
            batch_id: 'newapi-1',
            used_count: 1,
            items: [{ code: 'used', status: 2 }],
          },
          { batch_id: 'newapi-2', unused_count: 1 },
        ],
      },
    })
    expect(http.get.mock.calls[0][0]).toContain('status=3')
    expect(http.get.mock.calls[0][0]).toContain('/api/redemption/batches?')
  })

  it('saves real channel routing without changing provider, credentials, or unrelated settings', async () => {
    http.get.mockResolvedValueOnce(ok({ id: 7, type: 24, name: 'Gemini', group: 'default', models: 'gemini', priority: 0, weight: 0, model_mapping: '', setting: '{"proxy":"http://proxy"}', status: 1 }))
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/routing/channels/7', { method: 'PUT', body: { priority: -2, weight: 0 } })
    expect(http.request).toHaveBeenCalledExactlyOnceWith({
      url: '/api/channel/', method: 'PUT', data: expect.objectContaining({ id: 7, type: 24, priority: -2, weight: 0, models: 'gemini', setting: '{"proxy":"http://proxy"}' }),
    })
    expect(http.request.mock.calls[0][0].data).not.toHaveProperty('key')
    expect(http.request.mock.calls[0][0].data).not.toHaveProperty('status')
  })

  it('makes a new alias routable on the selected channel and preserves existing mappings', async () => {
    http.get.mockResolvedValue(ok({ id: 7, type: 1, models: 'upstream', group: 'default', weight: 3, model_mapping: '{"keep":"upstream"}' }))
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/routing/channels/7/aliases', { method: 'PUT', body: { alias: 'public', target_model: 'upstream' } })
    expect(http.request.mock.calls[0][0].data).toMatchObject({ models: 'upstream,public', weight: 3 })
    expect(JSON.parse(http.request.mock.calls[0][0].data.model_mapping)).toEqual({ keep: 'upstream', public: 'upstream' })
  })

  it('rejects cyclic aliases before saving channel configuration', async () => {
    http.get.mockResolvedValueOnce(ok({ id: 7, type: 1, models: 'upstream', model_mapping: '{"first":"second"}' }))
    await expect(handleAdminRequest('/api/admin/routing/channels/7/aliases', { method: 'PUT', body: { alias: 'second', target_model: 'first' } })).rejects.toThrow('循环')
    expect(http.request).not.toHaveBeenCalled()
  })

  it('preserves identity mappings supported by the relay and reads one channel snapshot', async () => {
    http.get.mockResolvedValueOnce(ok({ id: 7, type: 1, models: 'upstream', model_mapping: '{"upstream":"upstream"}' }))
    http.request.mockResolvedValueOnce(ok())
    await handleAdminRequest('/api/admin/routing/channels/7/aliases', { method: 'PUT', body: { alias: 'public', target_model: 'upstream' } })
    expect(http.get).toHaveBeenCalledExactlyOnceWith('/api/channel/7')
    expect(JSON.parse(http.request.mock.calls[0][0].data.model_mapping)).toEqual({ upstream: 'upstream', public: 'upstream' })
  })

  it('rejects comma aliases and missing deletes before reporting success', async () => {
    http.get.mockResolvedValue(ok({ id: 7, type: 1, models: 'upstream', model_mapping: '' }))
    await expect(handleAdminRequest('/api/admin/routing/channels/7/aliases', { method: 'PUT', body: { alias: 'one,two', target_model: 'upstream' } })).rejects.toThrow('逗号')
    await expect(handleAdminRequest('/api/admin/routing/channels/7/aliases', { method: 'DELETE', body: { alias: 'missing' } })).rejects.toThrow('已被删除')
    expect(http.request).not.toHaveBeenCalled()
  })

  it('saves Linux DO credentials before enabling OAuth and leaves an empty secret unchanged', async () => {
    http.request.mockResolvedValue(ok())
    await handleAdminRequest('/api/admin/settings/site', { method: 'PUT', body: { system_name: 'Gateway', linuxdo_client_id: 'client', linuxdo_client_secret: 'new-secret', linuxdo_minimum_trust_level: 2, linuxdo_enabled: true } })
    expect(http.request.mock.calls.map(([request]) => request.data.key)).toEqual(['SystemName', 'LinuxDOClientId', 'LinuxDOClientSecret', 'LinuxDOMinimumTrustLevel', 'LinuxDOOAuthEnabled'])
    http.request.mockClear()
    await handleAdminRequest('/api/admin/settings/site', { method: 'PUT', body: { system_name: 'Gateway', linuxdo_client_secret: '' } })
    expect(http.request).toHaveBeenCalledExactlyOnceWith({ url: '/api/option/', method: 'PUT', data: { key: 'SystemName', value: 'Gateway' } })
  })

  it('generates a general quota batch and refuses to reactivate a used code', async () => {
    http.get.mockResolvedValueOnce(ok({ quota_per_unit: 500000 }))
    http.request.mockResolvedValueOnce(ok(['code-a', 'code-b']))
    await handleAdminRequest('/api/admin/redemptions/generate', { method: 'POST', body: { name: 'batch', count: 2, amount_usd: 1, expired_time: 1800000000 } })
    expect(http.request).toHaveBeenCalledExactlyOnceWith({ url: '/api/redemption/', method: 'POST', data: { name: 'batch', count: 2, quota: 500000, expired_time: 1800000000 } })
    http.request.mockClear()
    http.get.mockResolvedValueOnce(ok({ id: 2, status: 3 }))
    await expect(handleAdminRequest('/api/admin/redemptions/2/status', { method: 'PUT', body: { enabled: true } })).rejects.toThrow('已使用')
    expect(http.request).not.toHaveBeenCalled()
  })

  it('shows unknown or expression pricing explicitly without inventing a free model', async () => {
    http.get.mockImplementation(async (url: string) => {
      if (url.startsWith('/api/relayfor/models/')) {
        return ok([{ model_id: 1, subscription_only: true }])
      }
      if (url.startsWith('/api/models/')) {
        return ok({
          total: 3,
          items: [
            { id: 1, model_name: 'known', status: 1 },
            { id: 2, model_name: 'advanced', status: 1 },
            { id: 0, model_name: 'channel-only', status: 0 },
          ],
        })
      }
      if (url.startsWith('/api/channel/')) {
        return ok({
          total: 1,
          items: [
            {
              id: 7,
              models: 'known, channel-only',
              status: 1,
              priority: 3,
              weight: 8,
              model_mapping: '{"known":"upstream"}',
            },
          ],
        })
      }
      return ok({
        entries: [
          {
            model_name: 'known',
            effective: { ModelRatio: 2, CompletionRatio: 3, CacheRatio: 0.25 },
          },
          {
            model_name: 'advanced',
            effective: { 'billing_setting.billing_mode': 'tiered_expr' },
          },
        ],
      })
    })
    expect(await handleAdminRequest('/api/admin/models', {})).toMatchObject({
      data: [
        {
          model: 'known',
          subscription_only: true,
          input_price: 4,
          output_price: 12,
          cache_read_price: 1,
          free_model: false,
          bindings: [
            {
              channel_id: 7,
              upstream_model: 'upstream',
              priority: 3,
              weight: 8,
            },
          ],
        },
        {
          model: 'advanced',
          subscription_only: false,
          input_price: null,
          pricing_unavailable: true,
          free_model: false,
        },
        {
          model: 'channel-only', row_key: 'model:channel-only', metadata_id: 0,
          enabled: true, pricing_editable: true, advanced_routing_enabled: true,
          available_channel_count: 1,
        },
      ],
    })
  })

  it('uses pricing versions and correct USD-per-million conversion', async () => {
    http.get
      .mockResolvedValueOnce(ok({ id: 1, model_name: 'model-a', status: 1 }))
      .mockResolvedValueOnce(ok({ total: 0, items: [] }))
      .mockResolvedValueOnce(
        ok({
          entries: [
            {
              model_name: 'model-a',
              version: 'v1',
              effective: { ModelRatio: 1, CompletionRatio: 2 },
            },
          ],
          empty_version: 'empty',
        })
      )
    http.request.mockResolvedValue(ok())
    await handleAdminRequest('/api/admin/models/1', {
      method: 'PUT',
      body: {
        model: 'model-a',
        enabled: true,
        input_price: 4,
        output_price: 12,
        cache_read_price: -1,
        cache_write_price: 8,
        bindings: [],
        subscription_only: true,
      },
    })
    expect(http.request).toHaveBeenCalledWith({
      url: '/api/option/model_pricing',
      method: 'PATCH',
      data: {
        changes: [
          {
            model_name: 'model-a',
            expected_version: 'v1',
            pricing: {
              'billing_setting.billing_mode': 'ratio',
              ModelRatio: 2,
              CompletionRatio: 3,
              CacheRatio: 1,
              CreateCacheRatio: 2,
            },
          },
        ],
      },
    })
    expect(http.request).toHaveBeenCalledWith({
      url: '/api/relayfor/models/1/access', method: 'PUT',
      data: { model: 'model-a', subscription_only: true },
    })
  })

  it('creates a first price for an unconfigured model and preserves independent image/audio rates', async () => {
    http.get
      .mockResolvedValueOnce(ok({ id: 1, model_name: 'custom', status: 1 }))
      .mockResolvedValueOnce(ok({ total: 0, items: [] }))
      .mockResolvedValueOnce(ok({ entries: [{ model_name: 'custom', version: 'v1', configured: { ImageRatio: 4, AudioRatio: 2, AudioCompletionRatio: 3 }, effective: {} }], empty_version: 'empty' }))
    http.request.mockResolvedValue(ok())
    await handleAdminRequest('/api/admin/models/1', { method: 'PUT', body: { model: 'custom', enabled: true, input_price: 4, output_price: 12, cache_read_price: 1, cache_write_price: 8 } })
    expect(http.request.mock.calls[0][0].data.changes[0].pricing).toMatchObject({ ImageRatio: 4, AudioRatio: 2, AudioCompletionRatio: 3, ModelRatio: 2, CompletionRatio: 3 })
  })

  it('loads a read-only model catalog for ordinary administrators without a root-only pricing request', async () => {
    document.body.dataset.gatewayRole = '10'
    http.get.mockImplementation(async (url: string) => {
      if (url.startsWith('/api/models/')) return ok({ total: 1, items: [{ id: 1, model_name: 'public', status: 1 }] })
      if (url.startsWith('/api/channel/')) return ok({ total: 0, items: [] })
      if (url === '/api/relayfor/models/access') return ok([])
      if (url === '/api/pricing') return ok([{ model_name: 'public', quota_type: 0, model_ratio: 2, completion_ratio: 3, cache_ratio: 0.25 }])
      throw new Error('unexpected request')
    })
    expect(await handleAdminRequest('/api/admin/models')).toMatchObject({ data: [{ model: 'public', input_price: 4, output_price: 12, cache_read_price: 1, pricing_editable: false, pricing_read_only: true }] })
    expect(http.get.mock.calls.some(([url]) => url.startsWith('/api/option/'))).toBe(false)
  })

  it('does not bypass proof or translate catalog hiding into gateway disabling', async () => {
    await expect(
      handleAdminRequest('/api/admin/security/totp/disable', {
        method: 'POST',
        body: { code: '123456' },
      })
    ).rejects.toThrow('安全验证')
    await expect(
      handleAdminRequest('/api/admin/models/1/status', {
        method: 'PATCH',
        body: { enabled: false },
      })
    ).rejects.toThrow('路由禁用')
    expect(http.request).not.toHaveBeenCalled()
  })
})
