import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, describe, expect, it, vi } from 'vitest'

const furtherRoot = resolve(import.meta.dirname, '../../../../further-ui')
const html = readFileSync(resolve(furtherRoot, 'console/index.html'), 'utf8')
const script = readFileSync(resolve(furtherRoot, 'assets/console.js'), 'utf8')
const shell = readFileSync(resolve(furtherRoot, 'assets/app.js'), 'utf8')

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
    request: vi.fn(async (path: string): Promise<unknown> => {
      if (path.startsWith('/api/user/tokens?')) return { items: [key], total: 1 }
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
    `return ${
      script.replace(
        /\}\(\)\);\s*$/,
        'return { loadTokens, bindCreateToken, resetTokenForm, bindTokenPagination }; }());'
      )}`
  )(
    {
      requestAnimationFrame: window.requestAnimationFrame.bind(window),
      cancelAnimationFrame: window.cancelAnimationFrame.bind(window),
      FurtherUI: api,
      FurtherUIConsoleModels: { load: vi.fn() },
      FurtherUIResources: { create: vi.fn() },
    },
    document
  ) as {
    loadTokens: () => Promise<void>
    bindCreateToken: () => void
    resetTokenForm: () => void
    bindTokenPagination: () => void
  }
  const form = document.querySelector<HTMLFormElement>('[data-create-token]')!
  const input = (name: string) =>
    form.elements.namedItem(name) as HTMLInputElement
  return { api, controller, form, input }
}

afterEach(() => {
  document.body.replaceChildren()
})

describe('Further key actions', () => {
  it('fills a local access draft without creating a key or replacing existing restrictions', () => {
    const { api, controller, input } = mountKeyActions()
    controller.bindCreateToken()
    document.querySelector<HTMLButtonElement>('[data-token-preset="local"]')!.click()
    expect(input('name').value).toBe('本机测试')
    expect(input('ip_limits').value).toBe('127.0.0.1,::1')
    expect(new Date(input('expires_at').value).getTime() - Date.now()).toBeGreaterThan(6 * 86400000)
    expect(api.request).not.toHaveBeenCalled()
    input('name').value = '已有名称'
    input('ip_limits').value = '192.0.2.1'
    input('model_limits').value = 'restricted-model'
    const expiry = input('expires_at').value
    document.querySelector<HTMLButtonElement>('[data-token-preset="application"]')!.click()
    expect(input('name').value).toBe('已有名称')
    expect(input('ip_limits').value).toBe('192.0.2.1')
    expect(input('model_limits').value).toBe('restricted-model')
    expect(input('expires_at').value).toBe(expiry)
    expect(api.request).not.toHaveBeenCalled()
  })
  it('explains an unrenderable expiry and preserves the record by preventing an ambiguous edit', async () => {
    const { api, controller } = mountKeyActions()
    api.request.mockResolvedValueOnce({ items: [{ id: 7, name: '超大到期时间', status: 1, expired_time: Number.MAX_SAFE_INTEGER, expired_at: null, expired_at_unavailable: true }], total: 1 })
    await controller.loadTokens()
    expect(document.querySelector('[data-token-rows]')).toHaveTextContent('到期时间无法显示')
    expect(document.querySelector('[data-token-rows]')).not.toHaveTextContent('永久')
    const edit = document.querySelector<HTMLButtonElement>('[data-edit-token]')!
    expect(edit).toBeDisabled()
    edit.click()
    expect(api.openDialog).not.toHaveBeenCalled()
    expect(document.querySelector('[data-toggle-token]')).toBeEnabled()
    expect(document.querySelector('[data-rotate-token]')).toBeEnabled()
    expect(document.querySelector('[data-delete-token]')).toBeEnabled()
    expect(api.request).toHaveBeenCalledOnce()
  })
  it('keeps key pagination in the primary page and corrects a removed last page without mounting another editor', async () => {
    const { api, controller } = mountKeyActions()
    api.request.mockImplementation(async (path: string) => {
      const page = Number(new URL(path, location.origin).searchParams.get('page'))
      return { items: [{ id: page, name: `Key page ${page}`, status: 1 }], total: page === 3 ? 26 : 51 }
    })
    controller.bindTokenPagination()
    controller.bindTokenPagination()
    await controller.loadTokens()
    const previous = document.querySelector<HTMLButtonElement>('[data-token-page-prev]')!
    const next = document.querySelector<HTMLButtonElement>('[data-token-page-next]')!
    expect(previous.disabled).toBe(true)
    expect(next.disabled).toBe(false)
    next.click()
    await vi.waitFor(() => expect(document.querySelector('[data-token-rows]')!.textContent).toContain('Key page 2'))
    expect(api.request.mock.calls.filter(call => call[0].includes('page=2'))).toHaveLength(1)
    next.click()
    await vi.waitFor(() => expect(api.request.mock.calls.filter(call => call[0].includes('page=2'))).toHaveLength(2))
    expect(api.request).toHaveBeenLastCalledWith('/api/user/tokens?page=2&page_size=25')
    expect(document.querySelector('[data-token-rows]')!.textContent).toContain('Key page 2')
    expect(document.querySelector('[data-panel="tokens"] [data-further-inline-tabs]')).toBeNull()
    expect(api.toast).not.toHaveBeenCalled()
  })
  it('retains the previous key page and usable controls when a page request fails', async () => {
    const { api, controller } = mountKeyActions()
    api.request.mockResolvedValueOnce({ items: [{ id: 1, name: '当前 Key' }], total: 51 }).mockRejectedValueOnce(new Error('暂时无法读取'))
    controller.bindTokenPagination()
    await controller.loadTokens()
    const previous = document.querySelector<HTMLButtonElement>('[data-token-page-prev]')!
    const next = document.querySelector<HTMLButtonElement>('[data-token-page-next]')!
    next.click()
    await vi.waitFor(() => expect(api.toast).toHaveBeenCalledWith('暂时无法读取', 'danger'))
    expect(document.querySelector('[data-token-rows]')!.textContent).toContain('当前 Key')
    expect(previous.disabled).toBe(true)
    expect(next.disabled).toBe(false)
    expect(document.querySelector('[data-token-page-summary]')!.textContent).toContain('第 1')
  })
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

  it('opens and reverses model selection motion without hiding the reopened list or losing Escape focus', async () => {
    const { api, controller } = mountKeyActions()
    window.eval(shell)
    const surfaces = (window as unknown as { FurtherUI: { transitionDropdown: (element: HTMLElement, visible: boolean, apply: (shown: boolean) => void) => void } }).FurtherUI
    Object.assign(api, { transitionDropdown: surfaces.transitionDropdown })
    api.request.mockResolvedValue(['model-a', 'model-b'])
    controller.bindCreateToken()
    controller.resetTokenForm()
    const trigger = document.querySelector<HTMLButtonElement>('[data-model-picker-trigger]')!
    const menu = document.querySelector<HTMLElement>('[data-model-picker-menu]')!
    const search = document.querySelector<HTMLInputElement>('[data-model-picker-search]')!
    const effects: Array<() => void> = []
    menu.animate = vi.fn(() => ({ finished: new Promise<void>(resolve => effects.push(resolve)), cancel: vi.fn() } as unknown as Animation))
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    await vi.waitFor(() => expect(document.activeElement).toBe(search))
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(document.activeElement).toBe(trigger)
    expect(menu).toHaveAttribute('inert')
    trigger.click()
    effects[1](); await Promise.resolve()
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
    expect(menu.hidden).toBe(false)
    expect(menu).not.toHaveAttribute('inert')
    effects[2](); await Promise.resolve()
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
