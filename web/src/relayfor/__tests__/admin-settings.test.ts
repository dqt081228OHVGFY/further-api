import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const root = resolve(process.cwd(), '../relayfor-ui')
const markup = readFileSync(resolve(root, 'admin/index.html'), 'utf8')
const runtime = readFileSync(resolve(root, 'assets/admin/admin-settings.js'), 'utf8')
const request = vi.fn()
const toast = vi.fn()
const applyContext = vi.fn()
type RuntimeWindow = Window & { ProxyLan: unknown; ProxyLanAdminSettings: { bind(): void; load(): Promise<void> } }

beforeEach(() => {
  const parsed = new DOMParser().parseFromString(markup, 'text/html')
  document.body.innerHTML = parsed.body.innerHTML
  document.querySelector('[data-panel="settings"]')?.classList.add('active')
  request.mockReset()
  toast.mockReset()
  applyContext.mockReset()
  request.mockImplementation(async (path: string) => {
    if (path === '/api/admin/settings/site') return { settings: { system_name: 'Configured Gateway', logo: '/logo.png', server_address: 'https://gateway.example', linuxdo_client_id: 'client', linuxdo_minimum_trust_level: 2, linuxdo_client_secret_configured: true, linuxdo_enabled: true, linuxdo_redirect_url: 'https://gateway.example/api/oauth/linuxdo' } }
    if (path === '/api/status') return { system_name: 'Renamed Gateway', logo: '/new-logo.svg' }
    return { settings: {}, announcements: {} }
  })
  ;(window as unknown as RuntimeWindow).ProxyLan = {
    request, syncQuotaUSD() {}, quotaFromUSD(value: number) { return value * 500000 }, showBusy() {}, toast,
    setText(selector: string, value: string) { document.querySelectorAll(selector).forEach((element) => { element.textContent = value }) },
  }
  Object.assign(window, { ProxyLanSite: { applyContext } })
  window.eval(runtime)
})

describe('native site and Linux DO settings', () => {
  it('opens one complete form, loads persisted configuration, and keeps the secret blank', async () => {
    const api = (window as unknown as RuntimeWindow).ProxyLanAdminSettings
    api.bind()
    ;(document.querySelector('[data-settings-page="site"]') as HTMLButtonElement).click()
    expect((document.querySelector('[data-settings-subpage="site"]') as HTMLElement).hidden).toBe(false)
    await api.load()
    const form = document.querySelector('[data-site-settings]') as HTMLFormElement
    const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
    expect(field('system_name').value).toBe('Configured Gateway')
    expect(field('linuxdo_client_id').value).toBe('client')
    expect(field('linuxdo_enabled').checked).toBe(true)
    expect(field('linuxdo_client_secret').value).toBe('')
    expect(document.querySelector('[data-linuxdo-callback-url]')).toHaveTextContent('https://gateway.example/api/oauth/linuxdo')
    expect(document.querySelectorAll('form[data-site-settings]')).toHaveLength(1)
    expect(document.querySelector('[data-site-preview-name]')).toHaveTextContent('Configured Gateway')
    expect(document.querySelector('[data-site-setup-state]')).toHaveTextContent('已启用')
  })

  it('opens the direct site navigation entry and highlights it separately from the settings directory', () => {
    const api = (window as unknown as RuntimeWindow).ProxyLanAdminSettings
    api.bind()
    ;(document.querySelector('.nav-item[data-open-settings="site"]') as HTMLButtonElement).click()
    expect(document.querySelector('[data-settings-subpage="site"]')).not.toHaveAttribute('hidden')
    expect(document.querySelector('.nav-item[data-open-settings="site"]')).toHaveClass('active')
    expect(document.querySelector('.nav-item[data-nav="settings"]:not([data-open-settings])')).not.toHaveClass('active')
    expect(location.hash).toBe('#settings/site')
  })

  it('saves the visible settings with a blank secret and immediately applies the persisted branding', async () => {
    const api = (window as unknown as RuntimeWindow).ProxyLanAdminSettings
    api.bind()
    await api.load()
    const form = document.querySelector('[data-site-settings]') as HTMLFormElement
    ;(form.elements.namedItem('system_name') as HTMLInputElement).value = 'Renamed Gateway'
    ;(form.elements.namedItem('logo') as HTMLInputElement).value = '/new-logo.svg'
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await vi.waitFor(() => { expect(applyContext).toHaveBeenCalledWith({ system_name: 'Renamed Gateway', logo: '/new-logo.svg' }) })
    expect(request).toHaveBeenCalledWith('/api/admin/settings/site', expect.objectContaining({ method: 'PUT', body: expect.objectContaining({ system_name: 'Renamed Gateway', logo: '/new-logo.svg', linuxdo_client_secret: '' }) }))
    expect(document.title).toBe('管理后台 · Renamed Gateway')
  })

  it('blocks enabling Linux DO without an application ID before issuing any save request', async () => {
    const api = (window as unknown as RuntimeWindow).ProxyLanAdminSettings
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
    const api = (window as unknown as RuntimeWindow).ProxyLanAdminSettings
    api.bind()
    await api.load()
    ;(document.querySelector('[data-linuxdo-copy-callback]') as HTMLButtonElement).click()
    await vi.waitFor(() => { expect(writeText).toHaveBeenCalledWith('https://gateway.example/api/oauth/linuxdo') })
  })
})
