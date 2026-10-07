import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/admin.js'), 'utf8')
const html = readFileSync(resolve(process.cwd(), '../further-ui/admin/index.html'), 'utf8')
const request = vi.fn()
const toast = vi.fn()
const openDialog = vi.fn()
const closeDialog = vi.fn()
const confirm = vi.fn()
let loaders: Record<string, () => Promise<void>>
type Harness = { role: (role: number) => void; bind: () => void; capabilities: () => void }
let harness: Harness
const user = { id: 7, username: 'alice', display_name: 'Alice', remark: 'test note', quota: 100, group: 'default', role: 1, status: 1 }
const page = (items = [user], total = 101) => ({ items, total, total_pages: Math.ceil(total / 50) })

beforeEach(() => {
  const parsed = new DOMParser().parseFromString(html, 'text/html')
  document.body.innerHTML = parsed.querySelector('[data-panel="users"]')!.outerHTML + parsed.querySelector('#user-dialog')!.outerHTML
  request.mockReset().mockResolvedValue(page())
  toast.mockReset()
  openDialog.mockReset()
  closeDialog.mockReset()
  confirm.mockReset().mockResolvedValue(true)
  Object.assign(window, {
    FurtherUI: {
      request, toast, openDialog, closeDialog, confirm,
      adminListPreference: () => false, number: String, moneyFromQuota: String, date: () => '',
      escapeHTML: (value: unknown) => String(value).replaceAll('"', '&quot;'), statusLabel: () => '启用', syncQuotaUSD: () => {},
    },
    FurtherUIAdminOverview: { load: vi.fn() }, FurtherUIAdminSettings: { load: vi.fn() }, FurtherUIAdminModels: { load: vi.fn() },
    FurtherUIResources: { create(value: typeof loaders) { loaders = value; return {} } },
  })
  // Expose the existing event binders only in the test evaluation, without running unrelated page bootstraps.
  window.eval(runtime.replace(/\}\(\)\);\s*$/, `window.__adminUsersTest = {
    role: function(role) { currentAdminRole = role; },
    bind: function() { bindUserForm(); bindUserFilters(); }, capabilities: applyAdminCapabilities
  }; }());`))
  harness = (window as unknown as { __adminUsersTest: Harness }).__adminUsersTest
  harness.role(100)
  harness.bind()
})

afterEach(() => { document.body.innerHTML = '' })
const click = (selector: string) => (document.querySelector(selector) as HTMLButtonElement).click()
const form = () => document.querySelector('[data-user-form]') as HTMLFormElement
const submit = (element: HTMLFormElement) => element.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))

describe('native user management buttons', () => {
  it('lets administrators edit regular users, and keeps peer and root actions protected', async () => {
    harness.role(50)
    request.mockResolvedValue(page([user, { ...user, id: 8, role: 50 }, { ...user, id: 9, role: 100 }]))
    await loaders.users()
    harness.capabilities()
    expect(document.querySelector('[data-edit-user="7"]')!.classList.contains('hidden')).toBe(false)
    expect(document.querySelector('[data-edit-user="8"]')!.classList.contains('hidden')).toBe(true)
    expect(document.querySelector('[data-user-actions="9"]')!.classList.contains('hidden')).toBe(true)
    click('[data-edit-user="8"]')
    expect(openDialog).not.toHaveBeenCalled()
    click('[data-edit-user="7"]')
    expect(openDialog).toHaveBeenCalledWith('user-dialog')
    expect((form().elements.namedItem('role') as HTMLSelectElement).disabled).toBe(true)
    expect((form().elements.namedItem('display_name') as HTMLInputElement).value).toBe('Alice')
    submit(form())
    await vi.waitFor(() => expect(closeDialog).toHaveBeenCalledWith('user-dialog'))
    const body = request.mock.calls.find(([path, options]) => path === '/api/admin/users/7' && options?.method === 'PATCH')![1].body
    expect(body).toMatchObject({ display_name: 'Alice', remark: 'test note', quota: 100, status: 1 })
    expect(body).not.toHaveProperty('role')
  })

  it('never offers disabling the root account or sends its locked status and role on save', async () => {
    request.mockResolvedValue(page([{ ...user, role: 100 }]))
    await loaders.users()
    expect(document.querySelector('[data-toggle-user="7"]')!.classList.contains('hidden')).toBe(true)
    click('[data-toggle-user="7"]')
    expect(confirm).not.toHaveBeenCalled()
    click('[data-edit-user="7"]')
    submit(form())
    await vi.waitFor(() => expect(closeDialog).toHaveBeenCalled())
    const body = request.mock.calls.find(([, options]) => options?.method === 'PATCH')![1].body
    expect(body).not.toHaveProperty('role')
    expect(body).not.toHaveProperty('status')
  })

  it('disables a regular account once and refreshes the visible list', async () => {
    await loaders.users()
    click('[data-toggle-user="7"]')
    click('[data-toggle-user="7"]')
    await vi.waitFor(() => expect(request.mock.calls.filter(([, options]) => options?.method === 'PATCH')).toHaveLength(1))
    expect(request).toHaveBeenCalledWith('/api/admin/users/7', { method: 'PATCH', body: { status: 2 } })
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(3))
  })

  it('submits search criteria and restores page state when paging fails', async () => {
    await loaders.users()
    const filter = document.querySelector('[data-admin-user-filter]') as HTMLFormElement
    ;(filter.elements.namedItem('keyword') as HTMLInputElement).value = 'alice'
    submit(filter)
    await vi.waitFor(() => expect(request.mock.lastCall?.[0]).toContain('keyword=alice'))
    request.mockRejectedValueOnce(new Error('offline'))
    click('[data-admin-user-next]')
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('offline', 'danger'))
    click('[data-admin-user-next]')
    await vi.waitFor(() => expect(request.mock.lastCall?.[0]).toContain('page=2&'))
    expect(request.mock.lastCall?.[0]).toContain('keyword=alice')
    await vi.waitFor(() => expect(document.querySelector('[data-admin-user-page]')!.textContent).toContain('第 2 页'))
  })

  it('ignores an older list response after a newer search has completed', async () => {
    let resolveOld!: (value: ReturnType<typeof page>) => void
    request.mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve }))
    const old = loaders.users()
    request.mockResolvedValueOnce(page([{ ...user, username: 'new-result' }]))
    await loaders.users()
    resolveOld(page([{ ...user, username: 'stale-result' }]))
    await old
    expect(document.querySelector('[data-admin-user-rows]')!.textContent).toContain('new-result')
    expect(document.querySelector('[data-admin-user-rows]')!.textContent).not.toContain('stale-result')
  })
})
