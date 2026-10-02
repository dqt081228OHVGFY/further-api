import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, describe, expect, it, vi } from 'vitest'

const relayforRoot = resolve(import.meta.dirname, '../../../../relayfor-ui')
const html = readFileSync(resolve(relayforRoot, 'console/index.html'), 'utf8')
const script = readFileSync(resolve(relayforRoot, 'assets/console.js'), 'utf8')

function mountKeyActions() {
  document.body.innerHTML = new DOMParser().parseFromString(
    html,
    'text/html'
  ).body.innerHTML
  const key = {
    id: 7,
    name: '应用 Key',
    key: 'sk-masked',
    status: 1,
    unlimited_quota: true,
    used_quota: 0,
    model_limits: ['model-a', 'model-b'],
    ip_limits: '127.0.0.1',
    expired_at: '2026-11-01T08:30:00.000Z',
  }
  const api = {
    preferences: () => ({ reducedMotion: true }),
    request: vi.fn(async (path: string) => {
      if (path === '/api/user/tokens') return [key]
      if (path === '/api/user/tokens/7') return key
      if (path === '/api/user/overview') {
        return { user: { id: 1, username: 'test', quota: 0 }, today: {} }
      }
      throw new Error(`Unexpected request: ${path}`)
    }),
    escapeHTML: (value: unknown) => String(value ?? ''),
    statusLabel: () => '已启用',
    date: (value: unknown) => String(value ?? ''),
    moneyFromQuota: () => '$0.00',
    number: (value: unknown) => String(value ?? 0),
    quotaPerUSD: () => 500000,
    openDialog: vi.fn(),
    closeDialog: vi.fn(),
    toast: vi.fn(),
    showBusy: vi.fn(),
    setText: (selector: string, text: string) => {
      document.querySelectorAll(selector).forEach((node) => {
        node.textContent = text
      })
    },
  }
  // Run the real controller against the production form, exposing only the
  // action entry points so unrelated page boot/network work is not required.
  const controller = new Function(
    'window',
    'document',
    'return ' +
      script.replace(
        /\}\(\)\);\s*$/,
        'return { loadTokens, bindCreateToken, resetTokenForm }; }());'
      )
  )(
    {
      ProxyLan: api,
      ProxyLanConsoleModels: { load: vi.fn() },
      ProxyLanResources: { create: vi.fn() },
    },
    document
  ) as {
    loadTokens: () => Promise<void>
    bindCreateToken: () => void
    resetTokenForm: () => void
  }
  const form = document.querySelector<HTMLFormElement>('[data-create-token]')!
  const input = (name: string) =>
    form.elements.namedItem(name) as HTMLInputElement
  return { api, controller, form, input }
}

afterEach(() => {
  document.body.replaceChildren()
})

describe('RelayFor key actions', () => {
  it('opens and saves an existing key using the current form without removed quota fields', async () => {
    const { api, controller, form, input } = mountKeyActions()
    controller.bindCreateToken()
    await controller.loadTokens()
    document.querySelector<HTMLButtonElement>('[data-edit-token]')!.click()

    expect(api.openDialog).toHaveBeenCalledWith('token-dialog')
    expect(api.toast).not.toHaveBeenCalled()
    expect(input('id').value).toBe('7')
    expect(input('name').value).toBe('应用 Key')
    expect(
      input('model_limits')
        .value.split(',')
        .map((value) => value.trim())
    ).toEqual(['model-a', 'model-b'])
    expect(input('ip_limits').value).toBe('127.0.0.1')
    expect(input('expires_at').value).not.toBe('')

    input('name').value = '更新的 Key'
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() =>
      expect(api.closeDialog).toHaveBeenCalledWith('token-dialog')
    )
    expect(api.request).toHaveBeenCalledWith('/api/user/tokens/7', {
      method: 'PUT',
      body: expect.objectContaining({
        name: '更新的 Key',
        model_limits: ['model-a', 'model-b'],
        ip_limits: '127.0.0.1',
        remain_quota: -1,
        unlimited_quota: true,
      }),
    })
    await vi.waitFor(() =>
      expect(api.showBusy).toHaveBeenLastCalledWith(
        expect.any(HTMLButtonElement),
        false
      )
    )
  })

  it('clears the edit target and restrictions before creating another key', async () => {
    const { api, controller, input } = mountKeyActions()
    await controller.loadTokens()
    document.querySelector<HTMLButtonElement>('[data-edit-token]')!.click()
    controller.resetTokenForm()

    expect(input('id').value).toBe('')
    expect(input('name').value).toBe('')
    expect(input('model_limits').value).toBe('')
    expect(input('ip_limits').value).toBe('')
    expect(input('expires_at').value).toBe('')
    expect(document.querySelector('[data-token-submit]')?.textContent).toBe(
      '创建'
    )
    expect(api.toast).not.toHaveBeenCalled()
  })
})
