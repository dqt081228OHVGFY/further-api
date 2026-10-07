import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const root = resolve(process.cwd(), '../further-ui')
const markup = readFileSync(resolve(root, 'admin/index.html'), 'utf8')
const runtime = readFileSync(resolve(root, 'assets/admin/admin-settings.js'), 'utf8')
const shell = readFileSync(resolve(root, 'assets/app.js'), 'utf8')
const request = vi.fn()
const toast = vi.fn()
const applyContext = vi.fn()
const confirm = vi.fn()
type RuntimeWindow = Window & { FurtherUI: unknown; FurtherUIAdminSettings: { bind(): void; load(): Promise<void> } }

beforeEach(() => {
  const parsed = new DOMParser().parseFromString(markup, 'text/html')
  document.body.innerHTML = parsed.body.innerHTML
  document.querySelector('[data-panel="settings"]')?.classList.add('active')
  request.mockReset()
  toast.mockReset()
  applyContext.mockReset()
  confirm.mockReset().mockResolvedValue(true)
  request.mockImplementation(async (path: string) => {
    if (path === '/api/admin/settings/site') return { settings: { system_name: 'Configured Gateway', logo: '/logo.png', server_address: 'https://gateway.example', linuxdo_client_id: 'client', linuxdo_minimum_trust_level: 2, linuxdo_client_secret_configured: true, linuxdo_enabled: true, linuxdo_redirect_url: 'https://gateway.example/api/oauth/linuxdo' } }
    if (path === '/api/status') return { system_name: 'Renamed Gateway', logo: '/new-logo.svg' }
    return { settings: {}, announcements: {} }
  })
  ;(window as unknown as RuntimeWindow).FurtherUI = {
    request, syncQuotaUSD() {}, quotaFromUSD(value: number) { return value * 500000 }, showBusy() {}, toast, confirm,
    setText(selector: string, value: string) { document.querySelectorAll(selector).forEach((element) => { element.textContent = value }) },
  }
  Object.assign(window, { FurtherUISite: { applyContext } })
  window.eval(runtime)
})

describe('native site and Linux DO settings', () => {
  it('transitions between the settings directory and a form while preserving a rapid return', async () => {
    const controls = (window as unknown as RuntimeWindow).FurtherUI
    const settings = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    window.eval(shell)
    Object.assign(controls as object, { transitionVisibility: (window as unknown as { FurtherUI: { transitionVisibility: unknown } }).FurtherUI.transitionVisibility })
    document.documentElement.dataset.shellReady = 'true'
    history.replaceState(null, '', '/admin/#settings')
    settings.bind()
    const directory = document.querySelector<HTMLElement>('[data-settings-directory]')!
    const form = document.querySelector<HTMLElement>('[data-settings-subpage="site"]')!
    const effects: Array<() => void> = []
    const animate = vi.fn(() => ({ finished: new Promise<void>(resolve => effects.push(resolve)), cancel: vi.fn() } as unknown as Animation))
    directory.animate = animate; form.animate = animate
    document.querySelector<HTMLButtonElement>('[data-settings-page="site"]')!.click()
    expect(directory).toHaveClass('rf-leaving')
    expect(form).toHaveClass('active', 'rf-entering')
    form.querySelector<HTMLButtonElement>('[data-settings-back]')!.click()
    document.querySelector<HTMLButtonElement>('[data-settings-page="site"]')!.click()
    effects.slice(0, 4).forEach(resolve => resolve()); await Promise.resolve()
    expect(form.hidden).toBe(false)
    expect(form).toHaveClass('active', 'rf-entering')
    effects.slice(4).forEach(resolve => resolve()); await Promise.resolve()
    expect(form).not.toHaveClass('rf-entering')
    expect(directory.hidden).toBe(true)
    history.replaceState(null, '', '/')
  })

  it('fills and undoes registration admission without changing balances or saving automatically', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    const form = document.querySelector<HTMLFormElement>('[data-registration-settings]')!
    const local = form.elements.namedItem('local_registration_enabled') as HTMLInputElement
    const quota = form.elements.namedItem('public_initial_quota') as HTMLInputElement
    quota.value = '1250000'; local.checked = false
    request.mockClear()
    form.querySelector<HTMLButtonElement>('[data-settings-preset="registration-invite"]')!.click()
    expect(local.checked).toBe(true)
    expect((form.elements.namedItem('registration_code_required') as HTMLInputElement).checked).toBe(true)
    expect(quota.value).toBe('1250000')
    expect(request).not.toHaveBeenCalled()
    form.querySelector<HTMLButtonElement>('[data-settings-preset="undo"]')!.click()
    expect(local.checked).toBe(false)
    expect(quota.value).toBe('1250000')
  })

  it('fills the site address locally and retains configured login credentials', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    request.mockClear()
    const form = document.querySelector<HTMLFormElement>('[data-site-settings]')!
    form.querySelector<HTMLButtonElement>('[data-settings-preset="site-origin"]')!.click()
    expect((form.elements.namedItem('server_address') as HTMLInputElement).value).toBe(location.origin)
    expect((form.elements.namedItem('linuxdo_client_id') as HTMLInputElement).value).toBe('client')
    expect((form.elements.namedItem('linuxdo_client_secret') as HTMLInputElement).value).toBe('')
    expect(request).not.toHaveBeenCalled()
  })

  it('requires editing maintenance template placeholders before publishing the notice', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    request.mockClear()
    const form = document.querySelector<HTMLFormElement>('[data-site-announcements]')!
    form.querySelector<HTMLButtonElement>('[data-settings-preset="notice-maintenance"]')!.click()
    expect((form.elements.namedItem('public') as HTMLTextAreaElement).value).toContain('【开始时间】')
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(request).not.toHaveBeenCalled()
    expect(toast).toHaveBeenCalledWith('请先补全公告模板中的维护时间和影响范围', 'danger')
  })

  it('saves registration without a second LinuxDO login switch or overwriting the provider setting', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    const form = document.querySelector<HTMLFormElement>('[data-registration-settings]')!
    expect(form.querySelector('[name="linuxdo_login_enabled"]')).toBeNull()
    ;(form.elements.namedItem('local_registration_enabled') as HTMLInputElement).checked = true
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('/api/admin/settings/registration', expect.objectContaining({ method: 'PATCH' })))
    const saved = request.mock.calls.find(([path, options]) => path === '/api/admin/settings/registration' && options?.method === 'PATCH')![1].body
    expect(saved.local_registration_enabled).toBe(true)
    expect(saved).not.toHaveProperty('linuxdo_login_enabled')
    expect(document.querySelectorAll('[name="linuxdo_enabled"]')).toHaveLength(1)
  })

  it('clears the saved secret only after website confirmation and verifies the persisted result by reading it back', async () => {
    const settings = { linuxdo_enabled: false, linuxdo_client_secret_configured: true, linuxdo_client_id: 'existing-client', server_address: 'https://gateway.example' }
    request.mockImplementation(async (path, options) => {
      if (path === '/api/admin/settings/site') return { settings: { ...settings } }
      if (path === '/api/option/' && options?.method === 'PUT') settings.linuxdo_client_secret_configured = false
      return { settings: {}, announcements: {} }
    })
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    const clear = document.querySelector<HTMLButtonElement>('[data-linuxdo-clear-secret]')!
    expect(clear).toBeEnabled()
    clear.click()
    await vi.waitFor(() => expect(toast).toHaveBeenCalledWith('LinuxDO 应用密钥已清除'))
    expect(confirm).toHaveBeenCalledExactlyOnceWith(expect.any(String), { title: '清除 LinuxDO 应用密钥', confirmText: '清除密钥' })
    expect(request).toHaveBeenCalledWith('/api/option/', { method: 'PUT', body: { key: 'LinuxDOClientSecret', value: '' } })
    expect(request.mock.calls.filter(([path]) => path === '/api/admin/settings/site')).toHaveLength(2)
    expect(settings.linuxdo_client_id).toBe('existing-client')
    expect(settings.server_address).toBe('https://gateway.example')
    expect(clear).toBeDisabled()
    expect(document.querySelector('[data-linuxdo-secret-status]')).toHaveTextContent('尚未配置密钥')
  })
  it('keeps the existing secret and blocks a concurrent normal save while confirmation is pending or canceled', async () => {
    request.mockImplementation(async (path) => path === '/api/admin/settings/site' ? { settings: { linuxdo_enabled: false, linuxdo_client_secret_configured: true } } : { settings: {}, announcements: {} })
    let finish!: (result: boolean) => void
    confirm.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    const clear = document.querySelector<HTMLButtonElement>('[data-linuxdo-clear-secret]')!
    clear.click()
    const form = document.querySelector<HTMLFormElement>('[data-site-settings]')!
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(request.mock.calls.some(call => call[1]?.method === 'PUT')).toBe(false)
    expect(clear).toBeDisabled()
    finish(false)
    await vi.waitFor(() => expect(clear).toBeEnabled())
    expect(form.dataset.secretConfigured).toBe('true')
    expect(request.mock.calls.some(call => call[1]?.method === 'PUT')).toBe(false)
  })
  it('requires the currently persisted provider to be disabled before allowing secret removal', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind(); await api.load()
    const clear = document.querySelector<HTMLButtonElement>('[data-linuxdo-clear-secret]')!
    expect(clear).toBeDisabled()
    ;(document.querySelector('[name="linuxdo_enabled"]') as HTMLInputElement).checked = false
    clear.click()
    expect(confirm).not.toHaveBeenCalled()
    expect(request.mock.calls.some(call => call[1]?.method === 'PUT')).toBe(false)
    expect(document.querySelector('[data-linuxdo-clear-secret-hint]')).toHaveTextContent('停用 LinuxDO 登录并保存')
  })
  it('opens one complete form, loads persisted configuration, and keeps the secret blank', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind()
    ;(document.querySelector('[data-settings-page="site"]') as HTMLButtonElement).click()
    expect((document.querySelector('[data-settings-subpage="site"]') as HTMLElement).hidden).toBe(false)
    await api.load()
    const form = document.querySelector('[data-site-settings]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    expect(field('system_name').value).toBe('further-api')
    expect(field('linuxdo_client_id').value).toBe('client')
    expect(field('linuxdo_enabled').checked).toBe(true)
    expect(field('linuxdo_client_secret').value).toBe('')
    expect(document.querySelector('[data-linuxdo-callback-url]')).toHaveTextContent('https://gateway.example/api/oauth/linuxdo')
    expect(document.querySelectorAll('form[data-site-settings]')).toHaveLength(1)
    expect(document.querySelector('[data-site-preview-name]')).toHaveTextContent('further-api')
    expect(document.querySelector('[data-site-setup-state]')).toHaveTextContent('已启用')
  })

  it('opens site settings from the overview shortcut while keeping one settings navigation entry', () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind()
    ;(document.querySelector('.workspace-welcome [data-open-settings="site"]') as HTMLButtonElement).click()
    expect(document.querySelector('[data-settings-subpage="site"]')).not.toHaveAttribute('hidden')
    expect(document.querySelectorAll('.nav-rail .nav-item[data-nav="settings"]')).toHaveLength(1)
    expect(document.querySelector('.nav-item[data-nav="settings"]')).toHaveClass('active')
    expect(document.querySelector('[data-settings-overview-heading]')).toHaveAttribute('hidden')
    expect(location.hash).toBe('#settings/site')
  })

  it('keeps the gateway brand fixed when form values are changed programmatically', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind()
    await api.load()
    const form = document.querySelector('[data-site-settings]') as HTMLFormElement
    ;(form.elements.namedItem('system_name') as HTMLInputElement).value = 'Renamed Gateway'
    ;(form.elements.namedItem('logo') as HTMLInputElement).value = '/new-logo.svg'
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => { expect(applyContext).toHaveBeenCalledWith({ system_name: 'further-api', logo: '/further-api-logo.svg?v=handshake-20261004' }) })
    expect(request).toHaveBeenCalledWith('/api/admin/settings/site', expect.objectContaining({ method: 'PUT', body: expect.objectContaining({ system_name: 'further-api', logo: '/further-api-logo.svg?v=handshake-20261004', linuxdo_client_secret: '' }) }))
    expect(document.title).toBe('管理页面 · further-api')
    expect(request).not.toHaveBeenCalledWith('/api/status')
  })

  it('blocks enabling Linux DO without an application ID before issuing any save request', async () => {
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind()
    await api.load()
    const form = document.querySelector('[data-site-settings]') as HTMLFormElement
    const clientID = form.elements.namedItem('linuxdo_client_id') as HTMLInputElement
    clientID.value = ''
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    expect(toast).toHaveBeenCalledWith(expect.stringContaining('Client ID'), 'danger')
    expect(clientID).toHaveFocus()
    expect(request.mock.calls.some((call) => call[1]?.method === 'PUT')).toBe(false)
  })

  it('copies the persisted Linux DO callback address from the configuration form', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const api = (window as unknown as RuntimeWindow).FurtherUIAdminSettings
    api.bind()
    await api.load()
    ;(document.querySelector('[data-linuxdo-copy-callback]') as HTMLButtonElement).click()
    await vi.waitFor(() => { expect(writeText).toHaveBeenCalledWith('https://gateway.example/api/oauth/linuxdo') })
  })
})
