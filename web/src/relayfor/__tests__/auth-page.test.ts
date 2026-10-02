import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const relayforRoot = resolve(process.cwd(), '../relayfor-ui')
const authPage = readFileSync(resolve(relayforRoot, 'auth/index.html'), 'utf8')
const authRuntime = readFileSync(resolve(relayforRoot, 'assets/app.js'), 'utf8')
const siteRuntime = readFileSync(resolve(relayforRoot, 'assets/site.js'), 'utf8')
const buildScript = readFileSync(
  resolve(process.cwd(), 'scripts/build-relayfor.mjs'),
  'utf8'
)

describe('original RelayFor authentication surface', () => {
  it('keeps the original auth shell and form as the source document', () => {
    expect(authPage).toContain('<body data-page="auth"')
    expect(authPage).toContain('class="auth-shell"')
    expect(authPage).toContain('id="auth-form"')
    expect(authPage).toContain('id="auth-switch"')
    expect(authPage).toContain('/relayfor-assets/app.css')
  })

  it('supports sign-up aliases, local New API auth endpoints, and safe redirects', () => {
    expect(authRuntime).toContain("document.body.dataset.authMode === 'register'")
    expect(authRuntime).toContain("'/api/user/login'")
    expect(authRuntime).toContain("'/api/user/register'")
    expect(authRuntime).toContain("value.startsWith('//')")
  })

  it('publishes auth/index.html with the same runtime bridge as the restored pages', () => {
    expect(buildScript).toContain("'auth/index.html'")
    expect(buildScript).toContain('runtimeScripts')
  })
})

describe('authentication provider and password controls', () => {
  const request = vi.fn()
  let config: Record<string, unknown>

  beforeAll(() => {
    window.eval(authRuntime)
    window.eval(siteRuntime)
  })

  beforeEach(() => {
    vi.useFakeTimers()
    const parsed = new DOMParser().parseFromString(authPage, 'text/html')
    document.body.innerHTML = parsed.body.innerHTML
    document.body.dataset.page = 'auth'
    document.title = ''
    delete document.body.dataset.authMode
    localStorage.clear()
    history.replaceState(null, '', '/auth/')
    config = { system_name: 'My AI Gateway', logo: '/custom-logo.svg', local_registration_enabled: true, linuxdo_login_enabled: false, linuxdo_registration_enabled: false, registration_code_required: false }
    request.mockReset()
    request.mockImplementation(async (path: string) => {
      if (path === '/api/auth/config') return config
      if (path === '/api/site/announcement') return { content: '' }
      if (path === '/api/oauth/state') throw new Error('Connect 服务暂时不可用')
      return {}
    })
    Object.assign(window, { RelayForGateway: { request, authenticated: () => false } })
  })

  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

  async function initialize() {
    document.dispatchEvent(new Event('relayfor:ready'))
    await vi.waitFor(() => { expect(document.title).toMatch(/(?:登录|注册) · My AI Gateway/) })
  }

  it('keeps an unconfigured Linux DO entry visible, disabled, and explained', async () => {
    await initialize()
    const button = document.querySelector('#linuxdo-login') as HTMLButtonElement
    expect(button).not.toHaveClass('hidden')
    expect(button).toBeDisabled()
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(document.querySelector('#linuxdo-login-hint')).not.toHaveClass('hidden')
    button.click()
    expect(request).not.toHaveBeenCalledWith('/api/oauth/state', expect.anything())
  })

  it.each(['login', 'register'])('blocks %s and native submission when the configuration read fails', async (mode) => {
    history.replaceState(null, '', `/auth/?mode=${mode}`)
    request.mockImplementation(async (path: string) => {
      if (path === '/api/auth/config') throw new Error('服务暂时不可用（HTTP 502）')
      if (path === '/api/site/announcement') return { content: '' }
      throw new Error('Credentials must not be sent while configuration is unavailable')
    })
    document.dispatchEvent(new Event('relayfor:ready'))
    await vi.waitFor(() => { expect(document.querySelector('#auth-oauth-message')).toHaveTextContent('登录配置加载失败') })

    expect(document.querySelector('#auth-submit')).toBeDisabled()
    const provider = document.querySelector('#linuxdo-login') as HTMLButtonElement
    expect(provider).toBeDisabled()
    expect(provider).toHaveAttribute('aria-disabled', 'true')
    expect(document.querySelector('[data-auth-config-retry]')).toHaveAccessibleName('重新加载')
    expect(document.querySelector('[data-auth-config-retry]')).toHaveAttribute('type', 'button')
    ;(document.querySelector('#auth-username') as HTMLInputElement).value = 'private-user'
    ;(document.querySelector('#auth-password') as HTMLInputElement).value = 'private-password'
    const submission = new Event('submit', { bubbles: true, cancelable: true })
    expect(document.querySelector('#auth-form')?.dispatchEvent(submission)).toBe(false)
    provider.click()
    await Promise.resolve()

    expect(submission.defaultPrevented).toBe(true)
    expect(request.mock.calls.map(([path]) => path)).toEqual(['/api/auth/config', '/api/site/announcement'])
    expect(location.search).toBe(`?mode=${mode}`)
  })

  it('blocks submission while the configuration read is pending and enables login after it succeeds', async () => {
    let resolveConfig!: (value: Record<string, unknown>) => void
    const pendingConfig = new Promise<Record<string, unknown>>((resolve) => { resolveConfig = resolve })
    request.mockImplementation(async (path: string) => path === '/api/auth/config' ? pendingConfig : { content: '' })
    document.dispatchEvent(new Event('relayfor:ready'))
    expect(document.querySelector('#auth-submit')).toBeDisabled()
    expect(document.querySelector('#linuxdo-login')).toBeDisabled()
    const submission = new Event('submit', { bubbles: true, cancelable: true })
    expect(document.querySelector('#auth-form')?.dispatchEvent(submission)).toBe(false)
    expect(request.mock.calls.map(([path]) => path)).toEqual(['/api/auth/config', '/api/site/announcement'])

    resolveConfig(config)
    await vi.waitFor(() => { expect(document.querySelector('#auth-submit')).toBeEnabled() })
    expect(document.title).toBe('登录 · My AI Gateway')
  })

  it('uses the enabled provider endpoint and presents its error without losing the login form', async () => {
    config.linuxdo_login_enabled = true
    await initialize()
    const button = document.querySelector('#linuxdo-login') as HTMLButtonElement
    expect(button).toBeEnabled()
    button.click()
    await vi.waitFor(() => { expect(document.querySelector('#auth-oauth-message')).toHaveTextContent('Connect 服务暂时不可用') })
    expect(request).toHaveBeenCalledWith('/api/oauth/state', { method: 'POST', body: { provider: 'linuxdo', intent: 'login', registration_code: '' } })
    expect(button).toBeEnabled()
    expect(document.querySelector('#auth-form')).toBeInTheDocument()
  })

  it('hides the account creation link when the server closes every registration method', async () => {
    config.local_registration_enabled = false
    await initialize()
    expect(document.querySelector('#auth-switch')).toHaveClass('hidden')
    expect(document.querySelector('[data-auth-switch-hint]')).toHaveClass('hidden')
    expect(document.querySelector('#auth-submit')).not.toHaveClass('hidden')
  })

  it('toggles the password visibility and accessible pressed state without changing its value', async () => {
    await initialize()
    const input = document.querySelector('#auth-password') as HTMLInputElement
    const button = document.querySelector('#auth-password-toggle') as HTMLButtonElement
    input.value = 'my-private-password'
    button.click()
    expect(input.type).toBe('text')
    expect(button).toHaveAttribute('aria-pressed', 'true')
    expect(button).toHaveAccessibleName('隐藏密码')
    button.click()
    expect(input.type).toBe('password')
    expect(input.value).toBe('my-private-password')
    expect(button).toHaveAttribute('aria-pressed', 'false')
  })

  it('uses new-password completion and preserves a local redirect when switching from registration', async () => {
    history.replaceState(null, '', '/auth/?mode=register&redirect=%2Fconsole%2F%23tokens')
    await initialize()
    expect(document.querySelector('#auth-password')).toHaveAttribute('autocomplete', 'new-password')
    expect(document.querySelector('#auth-switch')).toHaveAttribute('href', '/auth/?mode=login&redirect=%2Fconsole%2F%23tokens')
    expect(document.querySelector('.auth-brand strong')).toHaveTextContent('My AI Gateway')
    expect(document.querySelector('.rail-logo img')).toHaveAttribute('src', '/custom-logo.svg')
  })
})
