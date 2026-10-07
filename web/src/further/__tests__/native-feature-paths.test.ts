/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it, vi } from 'vitest'

import {
  NATIVE_FEATURE_ENTRIES,
  normalizeNativeFeaturePath,
  RETIRED_FEATURE_PATHS,
} from '../native-feature-paths'
import {
  activityShellTarget,
  workspaceShellTarget,
} from '../workspace-target'
import { SETTINGS_PAGES, settingsPageFor } from '../feature-workspace/catalog'

describe('Further native feature destinations', () => {
  it('uses the account page for administrator security and keeps the audit entry with logs', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const markup = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/admin/index.html'), 'utf8')
    document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
    document.body.dataset.page = 'admin'; document.body.dataset.gatewayRole = '100'; delete document.body.dataset.furtherInlineMounted
    expect(document.querySelector('[data-settings-page="security"]')).toBeNull()
    expect(document.querySelector('[data-settings-subpage="security"]')).toBeNull()
    expect(document.querySelector('#admin-totp-setup-dialog')).toBeNull()
    expect(document.querySelector('[data-panel="logs"] a[href="/admin/#workspace/%2Fusage-logs%2Faudit"]')).not.toBeNull()
    expect(document.querySelectorAll('a[href="/admin/#workspace/%2Fusage-logs%2Faudit"]')).toHaveLength(1)
    expect(normalizeNativeFeaturePath('/security')).toBe('/profile')
    expect(workspaceShellTarget('/security', 100)).toBe('/console/#profile')
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount: vi.fn() } })
    let ready!: EventListener
    const listener = vi.spyOn(document, 'addEventListener').mockImplementation((name, callback) => {
      if (name === 'further:ready') ready = callback as EventListener
    })
    const savedLocation = { hash: '#settings/security', replace: vi.fn() }
    new Function('location', source)(savedLocation)
    listener.mockRestore()
    ready(new Event('further:ready'))
    expect(savedLocation.replace).toHaveBeenCalledExactlyOnceWith('/console/#profile')
  })

  it('retires advanced pricing editors and omits duplicate registration, balance and shell settings', () => {
    const retired = '/system-settings/billing/group-pricing'
    expect(NATIVE_FEATURE_ENTRIES.some(entry => entry.path === retired)).toBe(false)
    expect(normalizeNativeFeaturePath(retired)).toBe('/models')
    expect(normalizeNativeFeaturePath(`${retired}/?from=saved`)).toBe('/models?from=saved')
    expect(settingsPageFor(retired)).toBeUndefined()
    expect(settingsPageFor('/system-settings/billing/model-pricing')).toBeUndefined()
    const fields = Object.values(SETTINGS_PAGES).flatMap(page => page.fields.map(field => field.key))
    for (const key of ['GroupRatio', 'TopupGroupRatio', 'UserUsableGroups', 'AutoGroups', 'MaxTokenAutoGroups', 'DefaultUseAutoGroup', 'ModelRequestRateLimitGroup', 'RegisterEnabled', 'PasswordRegisterEnabled', 'QuotaForNewUser', 'HeaderNavModules', 'SidebarModulesAdmin', 'DefaultCollapseSidebar', 'ModelPrice', 'billing_setting.billing_mode', 'billing_setting.billing_expr']) expect(fields).not.toContain(key)
  })

  it('migrates saved pricing and obsolete settings hashes without mounting retired controls', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount }, FurtherUI: { toast: vi.fn(), navigatePage: vi.fn() } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="settings">安全与设置</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="settings"><div data-settings-directory></div></section></main>'
    history.replaceState(null, '', '/admin/#native/%2Fsystem-settings%2Fbilling%2Fgroup-pricing')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#models')
    expect(document.querySelector('[data-settings-page="group-pricing"]')).toBeNull()
    expect(document.querySelector('[data-settings-subpage="group-pricing"]')).toBeNull()
    for (const [hash, expected] of [
      ['#models/pricing', '#models'],
      ['#settings/group-pricing', '#models'],
      ['#settings/model-pricing', '#models'],
      ['#workspace/%2Fsystem-settings%2Fbilling%2Fgroup-pricing', '#models'],
      ['#gateway/%2Fsystem-settings%2Fbilling%2Fmodel-pricing', '#models'],
      ['#settings/header-navigation', '#settings'],
      ['#settings/sidebar-modules', '#settings'],
      ['#settings/content-announcements', '#settings/announcements'],
      ['#settings/api-info', '#settings/site-system-info'],
      ['#settings/faq', '#settings/site-system-info'],
      ['#settings/uptime-kuma', '#settings/system-info'],
      ['#settings/update-checker', '#settings/system-info'],
      ['#native/%2Fsystem-settings%2Foperations%2Fupdate-checker', '#settings/system-info'],
      ['#settings/request-policies', '#settings/routing'],
    ]) {
      history.replaceState(null, '', hash)
      window.dispatchEvent(new PopStateEvent('popstate'))
      expect(location.hash).toBe(expected)
    }
    expect(mount.mock.calls.every(call => !RETIRED_FEATURE_PATHS[call[1]?.path])).toBe(true)
    history.replaceState(null, '', '/')
  })

  it('routes every retired settings editor to its current destination and removes its catalog code', () => {
    for (const [path, target] of Object.entries(RETIRED_FEATURE_PATHS)) {
      expect(normalizeNativeFeaturePath(path)).toBe(target)
      expect(NATIVE_FEATURE_ENTRIES.some(entry => entry.path === path)).toBe(false)
      expect(settingsPageFor(path)).toBeUndefined()
    }
    expect(workspaceShellTarget('/system-settings/billing/model-pricing', 100)).toBe('/admin/#models')
    expect(workspaceShellTarget('/system-settings/content/announcements', 100)).toBe('/admin/#settings/announcements')
    expect(workspaceShellTarget('/system-settings/content/uptime-kuma', 100)).toBe('/admin/#settings/system-info')
    expect(workspaceShellTarget('/system-settings/operations/update-checker', 100)).toBe('/admin/#settings/system-info')
    expect(workspaceShellTarget('/system-settings/content/faq', 100)).toBe('/admin/#settings/site-system-info')
    expect(workspaceShellTarget('/system-settings/site/header-navigation', 100)).toBe('/admin/#settings')
    expect(settingsPageFor('/system-settings/auth/basic-auth')?.fields.map(field => field.key)).toContain('PasswordLoginEnabled')
    expect(settingsPageFor('/system-settings/security/ssrf')?.fields.map(field => field.key)).toContain('fetch_setting.enable_ssrf_protection')
  })

  it('opens feature deep links in their section without a catalog and deduplicates history synchronization', async () => {
    document.body.dataset.gatewayRole = '100'
    document.body.dataset.page = 'admin'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"></main>'
    history.replaceState(null, '', '/admin/#workspace/%2Ftask-plugins')
    const mount = vi.fn(async (host: HTMLElement, options: { path: string }) => {
      host.textContent = options.path
    })
    Object.assign(window, {
      FurtherFeatures: { entries: [
        { path: '/task-plugins', title: '插件', group: '工具', role: 100 },
        { path: '/playground', title: '游乐场', group: '工具', role: 100 },
      ], mount },
      FurtherUI: { toast: vi.fn(), navigatePage:vi.fn() },
    })
    window.eval(readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8'))
    document.dispatchEvent(new Event('further:ready'))
    const area = document.querySelector('.content-area')!
    const host = area.querySelector<HTMLElement>('[data-native-feature-path]')!
    expect(host.textContent).toBe('/task-plugins')
    expect(area.querySelector('.workspace-context a')).toHaveAttribute('href','#settings')
    expect(document.querySelector('[data-nav="native"]')).toBeNull()
    expect(document.body.textContent).not.toContain('更多功能')
    expect(mount).toHaveBeenCalledTimes(1)
    history.replaceState(null, '', `/admin/#workspace/${encodeURIComponent('/playground')}`)
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    window.dispatchEvent(new PopStateEvent('popstate'))
    await Promise.resolve()
    expect(host.textContent).toBe('/playground')
    expect(mount).toHaveBeenCalledTimes(2)
    expect(mount).toHaveBeenLastCalledWith(host, { path:'/playground' })
    expect(area.querySelector('.workspace-context a')).toHaveAttribute('href','#overview')
    history.replaceState(null,'','/admin/#native')
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    expect(location.hash).toBe('#overview')
    expect(mount).toHaveBeenCalledTimes(2)
  })
  it('migrates old activity entrances and preserves each festival destination', () => {
    for (const entrance of [
      'activities',
      'activity',
      'benefits',
      'checkin',
      'profile/checkin',
    ]) {
      const path = `/${entrance}/?from=saved-link`
      expect(activityShellTarget(path)).toBe('/console/#activities')
      expect(workspaceShellTarget(path, 1)).toBe('/console/#activities')
    }
    for (const key of [
      'duanwu',
      'qixi',
      'mid_autumn',
      'chongyang',
      'new_year_eve',
      'national_day',
    ]) {
      const path = `/activities/${key}`
      expect(activityShellTarget(path)).toBe(`/console/#activities/${key}`)
      expect(workspaceShellTarget(path, 1)).toBe(`/console/#activities/${key}`)
    }
    expect(workspaceShellTarget('/system-settings/billing/checkin', 100)).toBe(
      '/admin/#activities'
    )
    expect(
      NATIVE_FEATURE_ENTRIES.some((entry) => entry.path.endsWith('/checkin'))
    ).toBe(false)
    for (const path of [
      '/activities/unknown',
      '//activities',
      '/activities/../keys',
      'https://evil.example/activities',
    ]) {
      expect(activityShellTarget(path)).toBeNull()
    }
  })

  it('preserves direct route and section destinations in the matching shell', () => {
    expect(workspaceShellTarget('/subscriptions', 100)).toBe('/admin/#subscriptions/paid')
    expect(NATIVE_FEATURE_ENTRIES.some((entry) => entry.path === '/subscriptions')).toBe(false)
    expect(workspaceShellTarget('/system-settings/billing/payment', 100)).toBe('/admin/#settings/payment')
    expect(workspaceShellTarget('/system-settings/billing/payment?from=saved', 100)).toBe('/admin/#settings/payment')
    expect(workspaceShellTarget('/keys?filter=active', 1)).toBe(
      '/console/#tokens'
    )
    expect(workspaceShellTarget('/models/deployments', 10)).toBe(
      '/admin/#models'
    )
    expect(workspaceShellTarget('/usage-logs/task', 10)).toBe(
      '/admin/#workspace/%2Fusage-logs%2Ftask'
    )
    expect(workspaceShellTarget('/task-plugins', 100)).toBe(
      '/admin/#workspace/%2Ftask-plugins'
    )
    expect(workspaceShellTarget('/system-settings/operations/email', 100)).toBe(
      '/admin/#workspace/%2Fsystem-settings%2Foperations%2Femail'
    )
    expect(workspaceShellTarget('/wallet?show_history=true', 1)).toBe(
      '/console/#wallet'
    )
    expect(workspaceShellTarget('/wallet', 1)).toBe('/console/#wallet')
    expect(workspaceShellTarget('/profile/checkin', 1)).toBe(
      '/console/#activities'
    )
  })

  it('returns retired console analysis routes to usage records while retaining admin analytics', () => {
    for (const section of ['models', 'flow']) {
      expect(workspaceShellTarget(`/dashboard/${section}`, 1)).toBe(
        '/console/#usage'
      )
      expect(
        workspaceShellTarget(`/dashboard/${section}?from=saved`, 1)
      ).toBe('/console/#usage')
      expect(workspaceShellTarget(`/dashboard/${section}`, 10)).toBe(
        section === 'models' ? '/admin/#overview/analysis' : `/admin/#workspace/%2Fdashboard%2F${section}`
      )
    }
  })

  it('keeps every system settings section reachable from the native catalog', () => {
    const groups = [
      'site',
      'auth',
      'security',
      'request-policies',
      'models',
      'billing',
      'content',
      'operations',
    ]
    const paths = new Set(NATIVE_FEATURE_ENTRIES.map((entry) => entry.path))
    for (const group of groups) {
      const sections = [...paths].filter((path) => path.startsWith(`/system-settings/${group}/`))
      expect(sections.length, group).toBeGreaterThan(0)
      for (const section of sections) {
        const dedicated = ['/system-settings/auth/custom-oauth', '/system-settings/billing/payment', '/system-settings/operations/update-checker']
        expect(Boolean(settingsPageFor(section)) || dedicated.includes(section), section).toBe(true)
      }
    }
    for (const entry of NATIVE_FEATURE_ENTRIES) {
      expect(normalizeNativeFeaturePath(entry.path)).toBe(entry.path)
    }
  })

  it('retires model metadata and deployment tools while keeping the main model configuration page', () => {
    for(const path of ['/models/metadata','/models/deployments']) {
      expect(NATIVE_FEATURE_ENTRIES.some(entry=>entry.path===path)).toBe(false)
      expect(normalizeNativeFeaturePath(path)).toBe('/models')
      expect(workspaceShellTarget(path,100)).toBe('/admin/#models')
    }
    const source=readFileSync(resolve(import.meta.dirname,'../../../../further-ui/assets/extensions.js'),'utf8')
    expect(source).not.toContain('模型资料与部署')
  })

  it('rejects external URLs, API paths, scripts, and encoded traversal', () => {
    expect(normalizeNativeFeaturePath(`${location.origin}/keys`)).toBe('/keys')
    for (const path of [
      '/api/user/self',
      '//evil.example/keys',
      'https://evil.example/keys',
      'javascript:alert(1)',
      '/keys/%2e%2e/api',
      '/keys/%252e%252e/api',
      '/keys/..',
    ]) {
      expect(() => normalizeNativeFeaturePath(path), path).toThrow()
    }
  })

  it('migrates saved native checkin links before the activities nav mounts', () => {
    const source = readFileSync(
      resolve(
        import.meta.dirname,
        '../../../../further-ui/assets/extensions.js'
      ),
      'utf8'
    )
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: [], mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML =
      '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"></main>'
    history.replaceState(
      null,
      '',
      '/admin/#native/%2Fsystem-settings%2Fbilling%2Fcheckin'
    )
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#activities')
    expect(mount).not.toHaveBeenCalled()

    const nav = document.createElement('button')
    nav.dataset.nav = 'activities'
    const activated = vi.fn()
    nav.addEventListener('click', activated)
    document.querySelector('.nav-rail')?.append(nav)
    history.replaceState(
      null,
      '',
      '#native/%2Fsystem-settings%2Fbilling%2Fcheckin'
    )
    window.dispatchEvent(
      new CustomEvent('further-ui:page', { detail: { name: 'native' } })
    )
    expect(location.hash).toBe('#activities')
    expect(activated).toHaveBeenCalledOnce()
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('keeps all independent configuration pages in settings after retiring the feature catalog', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="settings"><div data-settings-directory></div></section></main>'
    history.replaceState(null, '', '/admin/#native')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    const headings = [...document.querySelectorAll('.further-feature-group h2')].map((node) => node.textContent)
    expect(headings).toEqual([])
    expect(document.querySelector('[data-settings-page="ssrf"]')).not.toBeNull()
    expect(document.querySelector('[data-settings-page="gemini"]')).not.toBeNull()
    const settingPaths = [...document.querySelectorAll<HTMLElement>('[data-settings-feature-path]')].map((node) => node.dataset.settingsFeaturePath)
    for (const entry of NATIVE_FEATURE_ENTRIES.filter((entry) => entry.path.startsWith('/system-settings/'))) {
      expect(settingPaths.filter((path) => path === entry.path)).toHaveLength(1)
      expect(settingsPageFor(entry.path)).toBeDefined()
    }
    expect(new Set([...document.querySelectorAll<HTMLElement>('[data-settings-page]')].map((node) => node.dataset.settingsPage)).size).toBe(document.querySelectorAll('[data-settings-page]').length)
    expect(document.querySelector('[data-native-feature-path="/system-settings/billing/payment"]')).toBeNull()
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('opens every new settings route lazily and keeps a filtered directory when returning', () => {
    const uiRoot = resolve(import.meta.dirname, '../../../../further-ui')
    const markup = readFileSync(resolve(uiRoot, 'admin/index.html'), 'utf8')
    const source = readFileSync(resolve(uiRoot, 'assets/extensions.js'), 'utf8')
    const settingsRuntime = readFileSync(resolve(uiRoot, 'assets/admin/admin-settings.js'), 'utf8')
    const mount = vi.fn(), request = vi.fn().mockResolvedValue({settings:{},announcements:{}})
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount }, FurtherUI: {
      request, toast:vi.fn(), showBusy() {}, syncQuotaUSD() {}, quotaFromUSD:(value:number)=>value*500000,
      setText:(selector:string,value:string)=>document.querySelectorAll(selector).forEach(node=>{node.textContent=value}),
    } })
    const parsed = new DOMParser().parseFromString(markup, 'text/html')
    document.body.innerHTML = parsed.body.innerHTML
    document.body.dataset.page = 'admin'; document.body.dataset.gatewayRole = '100'; delete document.body.dataset.furtherInlineMounted
    document.querySelector('[data-panel="settings"]')!.classList.add('active')
    history.replaceState(null, '', '/admin/#settings')
    window.eval(source); document.dispatchEvent(new Event('further:ready'))
    window.eval(settingsRuntime)
    ;(window as unknown as {FurtherUIAdminSettings:{bind():void}}).FurtherUIAdminSettings.bind()
    expect(mount).not.toHaveBeenCalled()
    const payment = document.querySelectorAll('[data-settings-page="payment"]')
    expect(payment).toHaveLength(1)
    expect(document.querySelector('[data-settings-feature-path="/system-settings/billing/payment"]')).toBeNull()
    const search = document.querySelector<HTMLInputElement>('input[aria-label="查找安全与设置"]')!
    search.value = 'Gemini'; search.dispatchEvent(new Event('input', {bubbles:true}))
    const gemini = document.querySelector<HTMLButtonElement>('[data-settings-feature-path="/system-settings/models/gemini"]')!
    expect(gemini.hidden).toBe(false)
    expect(document.querySelector<HTMLElement>('[data-settings-feature-path="/system-settings/security/ssrf"]')!.hidden).toBe(true)
    gemini.click()
    expect(location.hash).toBe('#settings/gemini')
    expect(document.querySelector('[data-settings-subpage="gemini"]')).not.toHaveAttribute('hidden')
    expect(mount).toHaveBeenCalledExactlyOnceWith(expect.any(HTMLElement), {path:'/system-settings/models/gemini'})
    search.value = '邮件'; search.dispatchEvent(new Event('input', {bubbles:true}))
    expect(document.querySelector('[data-settings-subpage="gemini"]')).not.toHaveAttribute('hidden')
    expect(mount).toHaveBeenCalledOnce()
    document.querySelector<HTMLButtonElement>('[data-settings-subpage="gemini"] [data-settings-back]')!.click()
    expect(location.hash).toBe('#settings')
    expect(search.value).toBe('邮件')
    expect(document.querySelector('[data-settings-directory]')).not.toHaveAttribute('hidden')
    expect(document.querySelector<HTMLElement>('[data-settings-feature-path="/system-settings/operations/email"]')!.hidden).toBe(false)
    expect(request).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('moves saved settings feature links into their single settings editor', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'admin'; document.body.dataset.gatewayRole = '100'; delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="settings">安全与设置</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="settings"><div data-settings-directory></div></section></main>'
    history.replaceState(null, '', '/admin/#native/%2Fsystem-settings%2Fsecurity%2Fssrf')
    window.eval(source); document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#settings/ssrf')
    expect(mount).toHaveBeenCalledExactlyOnceWith(expect.any(HTMLElement), {path:'/system-settings/security/ssrf'})
    history.replaceState(null, '', '/')
  })

  it('migrates saved payment links into the local settings page without mounting the legacy form', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: [], mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"></main>'
    history.replaceState(null, '', '/admin/#native/%2Fsystem-settings%2Fbilling%2Fpayment')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#settings/payment')
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })
  it('unifies announcement aliases and request retry settings into one editor each', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const markup = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/admin/index.html'), 'utf8')
    document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
    document.body.dataset.page = 'admin'; document.body.dataset.gatewayRole = '100'; delete document.body.dataset.furtherInlineMounted
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount }, FurtherUI: { toast: vi.fn(), navigatePage: vi.fn() } })
    history.replaceState(null, '', '/admin/#workspace/%2Fsystem-settings%2Fsite%2Fnotice')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#settings/announcements')
    expect(document.querySelectorAll('[data-settings-page="announcements"]')).toHaveLength(1)
    expect(document.querySelector('[data-settings-feature-path="/system-settings/site/notice"]')).toBeNull()
    expect(NATIVE_FEATURE_ENTRIES.some(entry => entry.path === '/system-settings/site/notice')).toBe(false)
    expect(document.querySelector('[data-routing-settings]')).toBeNull()
    expect(document.querySelectorAll('[data-settings-feature-path="/system-settings/request-policies/routing"]')).toHaveLength(1)
    expect(document.querySelector('[data-settings-page="routing"]')).toHaveTextContent('会话与重试')
    expect(workspaceShellTarget('/system-settings/site/notice', 100)).toBe('/admin/#settings/announcements')
    expect(workspaceShellTarget('/system-settings/request-policies/routing', 100)).toBe('/admin/#settings/routing')
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })
  it('moves legacy log, analysis and audit hashes into the unique working pages', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const markup = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/admin/index.html'), 'utf8')
    const previousGateway = window.FurtherGateway
    const mount = vi.fn().mockResolvedValue({ navigate: vi.fn(), destroy: vi.fn() })
    Object.assign(window, { FurtherGateway: { workspaceShellTarget }, FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount }, FurtherUI: { toast: vi.fn(), navigatePage: vi.fn() } })
    document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
    document.body.dataset.page = 'admin'; document.body.dataset.gatewayRole = '100'; delete document.body.dataset.furtherInlineMounted
    history.replaceState(null, '', '/admin/#workspace/%2Fusage-logs%2Fcommon')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#logs')
    expect(document.querySelector('[data-panel="logs"] [data-further-inline-tabs]')).toBeNull()
    for (const [hash, canonical] of [
      ['#logs/tasks', '#logs'],
      ['#native/%2Fusage-logs', '#logs'],
      ['#gateway/%2Fusage-logs%2Fcommon', '#logs'],
      ['#workspace/%2Fdashboard%2Fmodels', '#overview/analysis'],
      ['#audit', '#workspace/%2Fusage-logs%2Faudit'],
    ]) {
      history.replaceState(null, '', hash)
      window.dispatchEvent(new PopStateEvent('popstate'))
      expect(location.hash).toBe(canonical)
    }
    expect(document.querySelector('[data-panel="audit"]')).toBeNull()
    expect(document.querySelector('[data-admin-audit-filter]')).toBeNull()
    expect(document.querySelector('a[href="/admin/#workspace/%2Fusage-logs%2Faudit"]')).not.toBeNull()
    expect(mount).toHaveBeenCalledWith(expect.any(HTMLElement), { path: '/usage-logs/audit' })
    expect(mount.mock.calls.some(call => call[1]?.path === '/usage-logs' || call[1]?.path === '/usage-logs/common')).toBe(false)
    Object.assign(window, { FurtherGateway: previousGateway })
    history.replaceState(null, '', '/')
  })

  it('migrates saved subscription links into the unified admin page', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="subscriptions">订阅与福利</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="subscriptions"></section></main>'
    history.replaceState(null, '', '/admin/#native/%2Fsubscriptions')
    // oxlint-disable-next-line no-eval
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#subscriptions/paid')
    expect(mount).not.toHaveBeenCalled()
    expect(document.querySelector('[data-panel="subscriptions"] [data-further-inline-tabs]')).toBeNull()
    history.replaceState(null, '', '/')
  })

  it('removes the console feature directory and migrates saved links to existing pages', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'console'
    document.body.dataset.gatewayRole = '1'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="profile">账户</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="profile"></section></main>'
    history.replaceState(null, '', '/console/#native/%2Fsecurity')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#profile')
    expect(document.querySelector('[data-nav="native"]')).toBeNull()
    expect(document.querySelector('[data-panel="native"]')).toBeNull()
    expect(document.body.textContent).not.toContain('更多功能')
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/console/#native/%2Fwallet')
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(location.hash).toBe('#wallet')
    history.replaceState(null, '', '/console/#native')
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(location.hash).toBe('#overview')
    history.replaceState(null, '', '/')
  })
  it('retires duplicate API Key editors and routes every saved key alias to the primary list', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount }, FurtherUI: { toast: vi.fn(), navigatePage: vi.fn() } })
    document.body.dataset.page = 'console'; document.body.dataset.gatewayRole = '1'; delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="tokens">API Key</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="tokens"><p>API Key 主列表</p></section></main>'
    history.replaceState(null, '', '/console/#tokens/keys')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#tokens')
    expect(document.querySelector('[data-panel="tokens"] [data-further-inline-tabs]')).toBeNull()
    for (const hash of ['#tokens/keys', '#native/%2Fkeys', '#workspace/%2Fkeys', '#gateway/%2Fkeys']) {
      history.replaceState(null, '', hash)
      window.dispatchEvent(new PopStateEvent('popstate'))
      expect(location.hash).toBe('#tokens')
    }
    expect(mount).not.toHaveBeenCalled()
    expect(document.body.textContent).not.toContain('Key 扩展设置')
    expect(document.body.textContent).toContain('API Key 主列表')
    history.replaceState(null, '', '/')
  })
  it('keeps privileged legacy aliases inside the callers role boundary', () => {
    for (const path of ['/channels', '/users', '/redemption-codes', '/models', '/risk-control', '/subscriptions']) {
      expect(workspaceShellTarget(path, 1)).toBe('/console/#overview')
    }
    for (const path of ['/system-info', '/task-plugins', '/system-settings', '/system-settings/billing/payment', '/system-settings/billing/checkin']) {
      expect(workspaceShellTarget(path, 10)).toBe('/admin/#overview')
    }
    expect(workspaceShellTarget('/usage-logs/common', 1)).toBe('/console/#usage')
    expect(workspaceShellTarget('/usage-logs/common', 10)).toBe('/admin/#logs')
    expect(workspaceShellTarget('/usage-logs/audit', 1)).toBe('/console/#overview')
    expect(workspaceShellTarget('/usage-logs/audit', 100)).toBe('/admin/#workspace/%2Fusage-logs%2Faudit')
  })

  it('merges the two user management views without mounting the former account tab', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: [], mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="users"></section></main>'
    history.replaceState(null, '', '/admin/#users/accounts')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))
    expect(location.hash).toBe('#users')
    expect(document.querySelector('[data-panel="users"] [data-further-inline-tabs]')).toBeNull()
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('removes console usage analysis and sends saved links back to usage records', () => {
    const source = readFileSync(
      resolve(import.meta.dirname, '../../../../further-ui/assets/extensions.js'),
      'utf8'
    )
    const mount = vi.fn()
    Object.assign(window, { FurtherFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'console'
    document.body.dataset.gatewayRole = '1'
    delete document.body.dataset.furtherInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="usage">调用记录</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="usage"><p>调用记录列表</p></section></main>'
    history.replaceState(null, '', '/console/#usage/analysis')
    window.eval(source)
    document.dispatchEvent(new Event('further:ready'))

    expect(location.hash).toBe('#usage')
    expect(document.querySelector('[data-panel="usage"] [data-further-inline-tabs]')).toBeNull()
    expect(document.querySelector('[data-panel="usage"] .further-inline-host')).toBeNull()
    expect(document.body.textContent).toContain('调用记录列表')
    expect(document.body.textContent).not.toContain('用量分析')

    const activate = vi.fn()
    document.querySelector('[data-nav="usage"]')?.addEventListener('click', activate)
    for (const hash of [
      '#usage/analysis',
      '#native/%2Fdashboard%2Fmodels',
      '#native/%2Fdashboard%2Fflow',
      '#gateway/%2Fdashboard%2Fmodels',
      '#gateway/%2Fdashboard%2Fflow',
    ]) {
      history.replaceState(null, '', hash)
      window.dispatchEvent(new PopStateEvent('popstate'))
      expect(location.hash).toBe('#usage')
    }
    expect(activate).toHaveBeenCalledTimes(5)
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })
})
