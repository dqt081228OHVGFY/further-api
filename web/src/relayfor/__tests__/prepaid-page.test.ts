import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { screen, waitFor } from '@testing-library/dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const root = resolve(import.meta.dirname, '../../../../relayfor-ui')
const html = readFileSync(resolve(root, 'prepaid/index.html'), 'utf8')
const script = readFileSync(resolve(root, 'assets/prepaid.js'), 'utf8')
const request = vi.fn()
const authenticated = vi.fn()

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(
    html,
    'text/html'
  ).body.innerHTML
  sessionStorage.clear()
  localStorage.clear()
  request.mockReset()
  authenticated.mockReset().mockReturnValue(true)
  Object.assign(window, {
    RelayForGateway: { authenticated },
    ProxyLan: {
      request,
      showBusy: (button: HTMLButtonElement, busy: boolean) => {
        button.disabled = busy
      },
      moneyFromQuota: (quota: number) => String(quota),
      date: (value: unknown) => String(value ?? '—'),
      escapeHTML: (value: unknown) => {
        const element = document.createElement('span')
        element.textContent = String(value)
        return element.innerHTML
      },
      toast: vi.fn(),
    },
  })
})

describe('original RelayFor prepaid page with authenticated gateway requests', () => {
  it('redeems into the current account and shows the returned key without storing it', async () => {
    request.mockResolvedValue({
      added_quota: 500000,
      user_group: 'default',
      api_key: 'server-issued-key',
      expires_at: '2026-10-24T00:00:00Z',
      merged: false,
    })
    window.eval(script)
    const input = screen.getByRole<HTMLInputElement>('textbox')
    input.value = 'rfp_card-owned-by-user'
    screen.getByRole('button', { name: '登录并兑换额度卡' }).click()
    await waitFor(() =>
      expect(document.querySelector('[data-prepaid-key]')?.textContent).toBe(
        'server-issued-key'
      )
    )
    expect(request).toHaveBeenCalledWith('/api/prepaid/redeem', {
      method: 'POST',
      body: { code: 'rfp_card-owned-by-user' },
      _suppressAuthRedirect: true,
    })
    expect(document.querySelector('[data-prepaid-result]')).not.toHaveClass(
      'hidden'
    )
    expect(sessionStorage.length).toBe(0)
    expect(localStorage.length).toBe(0)
  })

  it('resumes the pending card once after login and removes the stored code before submitting', async () => {
    sessionStorage.setItem(
      'relayfor:prepaid:pending',
      JSON.stringify({
        action: 'redeem',
        code: 'pending-code',
        createdAt: Date.now(),
      })
    )
    request.mockImplementation(async () => {
      expect(sessionStorage.getItem('relayfor:prepaid:pending')).toBeNull()
      return {
        added_quota: 20,
        user_group: 'default',
        expires_at: '2026-10-24',
        merged: true,
      }
    })
    window.eval(script)
    await waitFor(() =>
      expect(
        document.querySelector('[data-prepaid-result-title]')?.textContent
      ).toBe('额度卡已合并')
    )
    window.eval(script)
    expect(request).toHaveBeenCalledTimes(1)
    expect(request.mock.calls[0][1].body).toEqual({ code: 'pending-code' })
    expect(window.location.href).not.toContain('pending-code')
  })

  it('keeps a failed redemption on the original form and never reveals a synthetic key', async () => {
    request.mockRejectedValue(new Error('额度卡已被兑换'))
    window.eval(script)
    screen.getByRole<HTMLInputElement>('textbox').value = 'used-card'
    screen.getByRole('button', { name: '登录并兑换额度卡' }).click()
    await waitFor(() =>
      expect(document.querySelector('[data-prepaid-error]')?.textContent).toBe(
        '额度卡已被兑换'
      )
    )
    expect(document.querySelector('[data-prepaid-result]')).toHaveClass(
      'hidden'
    )
    expect(document.querySelector('[data-prepaid-key]')?.textContent).toBe('')
    expect(document.querySelector('button[type=submit]')).not.toBeDisabled()
  })

  it('does not replay an expired pending code', () => {
    sessionStorage.setItem(
      'relayfor:prepaid:pending',
      JSON.stringify({
        action: 'redeem',
        code: 'stale-code',
        createdAt: Date.now() - 31 * 60 * 1000,
      })
    )
    window.eval(script)
    expect(request).not.toHaveBeenCalled()
    expect(sessionStorage.getItem('relayfor:prepaid:pending')).toBeNull()
  })

  it('loads only the authenticated balance endpoint and displays masked key and funding preference', async () => {
    request.mockResolvedValue({
      total_remaining: 75,
      expires_at: '2026-10-24',
      key_hint: 'abcd******wxyz',
      key_status: 1,
      billing_preference: 'wallet_only',
      buckets: [
        {
          name: '<unsafe>',
          quota_total: 100,
          quota_remaining: 75,
          expires_at: '2026-10-24',
        },
      ],
    })
    window.eval(script)
    screen.getByRole('button', { name: '登录查询额度卡余额' }).click()
    await waitFor(() =>
      expect(
        document.querySelector('[data-prepaid-balance-total]')?.textContent
      ).toBe('75')
    )
    expect(request).toHaveBeenCalledWith('/api/prepaid/balance', {
      _suppressAuthRedirect: true,
    })
    expect(
      document.querySelector('[data-prepaid-balance-key]')?.textContent
    ).toBe('abcd******wxyz')
    expect(
      document.querySelector('[data-prepaid-billing-hint]')?.textContent
    ).toContain('优先使用限时额度')
    expect(document.querySelector('unsafe')).toBeNull()
  })
})

