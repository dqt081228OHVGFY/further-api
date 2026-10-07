import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const source = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/admin-user-actions.js'), 'utf8')
const request = vi.fn()
const confirm = vi.fn()
const toast = vi.fn()
const closeDialog = vi.fn()
let role = 100
let targetRole = 1
const catalog = {
  resources: [{ resource: 'channel', label_key: 'Channel Management', actions: [
    { action: 'read', label_key: 'Read channels', description_key: 'View channel lists and details without secrets.' },
    { action: 'write', label_key: 'Edit channel routing', description_key: '' },
  ] }],
  roles: [{ key: 'admin', grants: { channel: { read: true, write: true } } }],
}
let listeners: [string, EventListenerOrEventListenerObject][] = []
const click = (selector: string) => (document.querySelector(selector) as HTMLButtonElement).click()

beforeEach(() => {
  role = 100; targetRole = 1
  document.body.innerHTML = '<button data-user-actions="7">管理</button><button data-create-user>创建</button>'
  request.mockReset().mockImplementation(async (path: string, options?: { method?: string }) => {
    if (options?.method) return {}
    if (path === '/api/user/self') return { id: 1, role }
    if (path === '/api/user/7') return { id: 7, username: 'alice', display_name: 'Alice', role: targetRole, email: 'alice@example.com', group: 'team', remark: 'Keep this note', admin_permissions: { channel: { write: false } } }
    if (path === '/api/authz/catalog') return catalog
    if (path.endsWith('/oauth/bindings')) return [{ provider_id: 3, provider_name: '企业登录', provider_user_id: 'u7' }]
    if (path.endsWith('/plans')) return [{ plan: { id: 2, title: '月度套餐', enabled: true } }]
    if (path.endsWith('/subscriptions')) return [{ subscription: { id: 9, plan_id: 2, status: 'active', end_time: 1000, amount_total: 100, amount_used: 20 } }]
    throw new Error(`Unexpected path: ${  path}`)
  })
  confirm.mockReset().mockResolvedValue(true); toast.mockReset(); closeDialog.mockReset()
  Object.assign(window, { FurtherUI: { request, confirm, toast, closeDialog, openDialog: vi.fn(), showBusy: (button: HTMLButtonElement, busy: boolean) => { button.disabled = busy }, date: String, moneyFromQuota: String, escapeHTML: (value: string) => value.replaceAll('<', '&lt;').replaceAll('"', '&quot;') } })
  const add = document.addEventListener.bind(document)
  vi.spyOn(document, 'addEventListener').mockImplementation((type, listener, options) => {
    listeners.push([type, listener]); add(type, listener, options)
  })
  window.eval(source)
  ;(window as unknown as { FurtherUIAdminUserActions: { bind: () => void } }).FurtherUIAdminUserActions.bind()
})
afterEach(() => {
  listeners.forEach(([type, listener]) => document.removeEventListener(type, listener))
  listeners = []; vi.restoreAllMocks(); document.body.innerHTML = ''
})

describe('merged native user actions', () => {
  it('creates users through the real API and clears the password only after success', async () => {
    click('[data-create-user]')
    await vi.waitFor(() => expect(document.querySelector('[data-create-user-form]')).not.toBeNull())
    const form = document.querySelector('[data-create-user-form]') as HTMLFormElement
    ;(form.elements.namedItem('username') as HTMLInputElement).value = 'alice'
    ;(form.elements.namedItem('password') as HTMLInputElement).value = 'password123'
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(closeDialog).toHaveBeenCalledWith('create-user-dialog'))
    expect(request).toHaveBeenCalledWith('/api/user/', { method: 'POST', body: { username: 'alice', password: 'password123', display_name: '', role: 1 } })
    expect((form.elements.namedItem('password') as HTMLInputElement).value).toBe('')
  })

  it('shows bindings and subscriptions together, confirms cancellation and reports failed mutations', async () => {
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-action="oauth"]')).not.toBeNull())
    expect(document.querySelector('[data-grant-user-subscription]')).not.toBeNull()
    confirm.mockResolvedValueOnce(false)
    click('[data-account-action="delete-user"]')
    await vi.waitFor(() => expect(confirm).toHaveBeenCalled())
    expect(request.mock.calls.some(([, options]) => options?.method === 'DELETE')).toBe(false)
    await new Promise(resolve => setTimeout(resolve, 0))
    request.mockRejectedValueOnce(new Error('权限不足'))
    click('[data-account-action="passkey"]')
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('权限不足', 'danger'))
    expect(toast).not.toHaveBeenCalledWith('操作成功')
    expect(request).toHaveBeenCalledWith('/api/user/7/reset_passkey', { method: 'DELETE' })
  })

  it('disables modifications for peer administrators and closes the dynamically created dialog', async () => {
    role = 50; targetRole = 10
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-action="passkey"]')).not.toBeNull())
    expect((document.querySelector('[data-account-action="passkey"]') as HTMLButtonElement).disabled).toBe(true)
    expect((document.querySelector('[data-account-action="delete-user"]') as HTMLButtonElement).disabled).toBe(true)
    click('[data-account-action="delete-user"]')
    expect(confirm).not.toHaveBeenCalled()
    click('#user-actions-dialog [data-dialog-close]')
    expect(closeDialog).toHaveBeenCalledWith('user-actions-dialog')
  })

  it.each(['passkey', 'twofa'])('allows root to reset a root account %s while preventing root deletion', async (action) => {
    targetRole = 100
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-permissions]')?.getAttribute('data-loaded')).toBe('true'))
    expect((document.querySelector(`[data-account-action="${action}"]`) as HTMLButtonElement).disabled).toBe(false)
    expect((document.querySelector('[data-account-action="delete-user"]') as HTMLButtonElement).disabled).toBe(true)
    click(`[data-account-action="${action}"]`)
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith(action === 'passkey' ? '/api/user/7/reset_passkey' : '/api/user/7/2fa', { method: 'DELETE' }))
  })

  it.each(['😀'.repeat(7), 'a'.repeat(129)])('rejects invalid password lengths using Unicode characters', async (password) => {
    click('[data-create-user]')
    await vi.waitFor(() => expect(document.querySelector('[data-create-user-form]')).not.toBeNull())
    const form = document.querySelector('[data-create-user-form]') as HTMLFormElement
    ;(form.elements.namedItem('username') as HTMLInputElement).value = 'alice'
    ;(form.elements.namedItem('password') as HTMLInputElement).value = password
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(toast).toHaveBeenCalledWith('密码须包含 8–128 个字符', 'danger')
    expect(request.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  })

  it('accepts 128 supplementary characters and preserves password whitespace', async () => {
    click('[data-create-user]')
    await vi.waitFor(() => expect(document.querySelector('[data-create-user-form]')).not.toBeNull())
    const form = document.querySelector('[data-create-user-form]') as HTMLFormElement
    const password = ` ${  '😀'.repeat(126)  } `
    const input = form.elements.namedItem('password') as HTMLInputElement
    expect(input.hasAttribute('maxlength')).toBe(false)
    ;(form.elements.namedItem('username') as HTMLInputElement).value = 'alice'
    input.value = password
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/user/', { method: 'POST', body: expect.objectContaining({ password }) }))
  })

  it('resets passwords without overwriting profile fields and clears the password after success', async () => {
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-reset-user-password]')).not.toBeNull())
    const form = document.querySelector('[data-reset-user-password]') as HTMLFormElement
    const input = form.elements.namedItem('password') as HTMLInputElement
    input.value = 'new-password'
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/user/', { method: 'PUT', body: { id: 7, username: 'alice', display_name: 'Alice', group: 'team', remark: 'Keep this note', password: 'new-password' } }))
    await vi.waitFor(() => expect(input.value).toBe(''))
  })

  it('retains a new password on failure so the user can retry', async () => {
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-action="oauth"]')).not.toBeNull())
    const form = document.querySelector('[data-reset-user-password]') as HTMLFormElement
    const input = form.elements.namedItem('password') as HTMLInputElement
    input.value = 'new-password'
    const implementation = request.getMockImplementation()!
    request.mockImplementation((path, options) => options?.method === 'PUT' ? Promise.reject(new Error('保存失败')) : implementation(path, options))
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('保存失败', 'danger'))
    expect(input.value).toBe('new-password')
    expect((form.querySelector('[type="submit"]') as HTMLButtonElement).disabled).toBe(false)
    expect(toast).not.toHaveBeenCalledWith('操作成功')
  })

  it('never exposes the administrator permission editor to a regular administrator', async () => {
    role = 50
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-action="oauth"]')).not.toBeNull())
    expect(document.querySelector('[data-update-admin-permissions]')).toBeNull()
    expect(request).not.toHaveBeenCalledWith('/api/authz/catalog')
    click('[data-create-user]')
    await vi.waitFor(() => expect(document.querySelector('[data-create-user-form]')).not.toBeNull())
    expect(document.querySelector('[data-create-permissions]')).toBeNull()
    expect(document.querySelector('[data-create-user-form] option[value="10"]')).toBeNull()
  })

  it('loads the backend permission matrix with defaults, preserves overrides, and saves checked values', async () => {
    targetRole = 10
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-permissions]')?.getAttribute('data-loaded')).toBe('true'))
    expect(document.querySelector('[data-account-permissions]')?.textContent).toContain('渠道管理')
    const read = document.querySelector('[data-permission-action="read"]') as HTMLInputElement
    const write = document.querySelector('[data-permission-action="write"]') as HTMLInputElement
    expect(read.checked).toBe(true)
    expect(write.checked).toBe(false)
    read.checked = false; write.checked = true
    const form = document.querySelector('[data-update-admin-permissions]') as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/user/', { method: 'PUT', body: expect.objectContaining({ admin_permissions: { channel: { read: false, write: true } }, group: 'team', remark: 'Keep this note' }) }))
  })

  it('includes the selected permission matrix when creating an administrator', async () => {
    click('[data-create-user]')
    await vi.waitFor(() => expect(document.querySelector('[data-create-user-form]')).not.toBeNull())
    const form = document.querySelector('[data-create-user-form]') as HTMLFormElement
    ;(form.elements.namedItem('username') as HTMLInputElement).value = 'admin2'
    ;(form.elements.namedItem('password') as HTMLInputElement).value = 'password123'
    const select = form.elements.namedItem('role') as HTMLSelectElement
    select.value = '10'; select.dispatchEvent(new Event('change', { bubbles: true }))
    await vi.waitFor(() => expect(document.querySelector('[data-create-permissions]')?.getAttribute('data-loaded')).toBe('true'))
    ;(document.querySelector('[data-permission-action="write"]') as HTMLInputElement).checked = false
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/user/', { method: 'POST', body: expect.objectContaining({ role: 10, admin_permissions: { channel: { read: true, write: false } } }) }))
  })

  it('keeps other actions available when the permission catalog fails and supports retry', async () => {
    targetRole = 10
    const implementation = request.getMockImplementation()!
    request.mockImplementation((path, options) => path === '/api/authz/catalog' ? Promise.reject(new Error('目录暂不可用')) : implementation(path, options))
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-permissions]')?.textContent).toContain('目录暂不可用'))
    expect(document.querySelector('[data-reset-user-password]')).not.toBeNull()
    expect((document.querySelector('[data-update-admin-permissions] [type="submit"]') as HTMLButtonElement).disabled).toBe(true)
    request.mockImplementation(implementation)
    click('[data-account-action="reload-permissions"]')
    await vi.waitFor(() => expect(document.querySelector('[data-account-permissions]')?.getAttribute('data-loaded')).toBe('true'))
  })

  it('grants and resets subscription use through the real subscription endpoints', async () => {
    click('[data-user-actions]')
    await vi.waitFor(() => expect(document.querySelector('[data-reset-user-subscriptions]')).not.toBeNull())
    const grant = document.querySelector('[data-grant-user-subscription]') as HTMLFormElement
    grant.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/subscription/admin/users/7/subscriptions', { method: 'POST', body: { plan_id: 2 } }))
    await new Promise(resolve => setTimeout(resolve, 0))
    const reset = document.querySelector('[data-reset-user-subscriptions]') as HTMLFormElement
    ;(reset.elements.namedItem('advance_reset_time') as HTMLInputElement).checked = true
    reset.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/subscription/admin/users/7/subscriptions/reset', { method: 'POST', body: { plan_id: 2, advance_reset_time: true } }))
  })
})
