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

import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  NATIVE_FEATURE_ENTRIES,
  normalizeNativeFeaturePath,
} from '../native-feature-paths'
import {
  activityShellTarget,
  originalShellTarget,
} from '../original-shell-target'

describe('RelayFor native feature destinations', () => {
  beforeEach(() => {
    window.eval(readFileSync(resolve(import.meta.dirname, '../../../../relayfor-ui/assets/admin/admin-feature-directory.js'), 'utf8'))
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
      expect(originalShellTarget(path, 1)).toBe('/console/#activities')
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
      expect(originalShellTarget(path, 1)).toBe(`/console/#activities/${key}`)
    }
    expect(originalShellTarget('/system-settings/billing/checkin', 100)).toBe(
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
    expect(originalShellTarget('/system-settings/billing/payment', 100)).toBe('/admin/#settings/payment')
    expect(originalShellTarget('/system-settings/billing/payment?from=saved', 100)).toBe('/admin/#settings/payment')
    expect(originalShellTarget('/keys?filter=active', 1)).toBe(
      '/console/#tokens'
    )
    expect(originalShellTarget('/models/deployments', 10)).toBe(
      '/admin/#native/%2Fmodels%2Fdeployments'
    )
    expect(originalShellTarget('/usage-logs/task', 10)).toBe(
      '/admin/#native/%2Fusage-logs%2Ftask'
    )
    expect(originalShellTarget('/task-plugins', 100)).toBe(
      '/admin/#native/%2Ftask-plugins'
    )
    expect(originalShellTarget('/system-settings/operations/email', 100)).toBe(
      '/admin/#native/%2Fsystem-settings%2Foperations%2Femail'
    )
    expect(originalShellTarget('/wallet?show_history=true', 1)).toBe(
      '/console/#vip/payment'
    )
    expect(originalShellTarget('/wallet', 1)).toBe('/console/#vip/payment')
    expect(originalShellTarget('/profile/checkin', 1)).toBe(
      '/console/#activities'
    )
  })

  it('returns retired console analysis routes to usage records while retaining admin analytics', () => {
    for (const section of ['models', 'flow']) {
      expect(originalShellTarget(`/dashboard/${section}`, 1)).toBe(
        '/console/#usage'
      )
      expect(
        originalShellTarget(`/dashboard/${section}?from=saved`, 1)
      ).toBe('/console/#usage')
      expect(originalShellTarget(`/dashboard/${section}`, 10)).toBe(
        `/admin/#native/%2Fdashboard%2F${section}`
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
      const file = resolve(
        import.meta.dirname,
        `../../features/system-settings/${group}/section-registry.tsx`
      )
      const source = readFileSync(file, 'utf8')
      const sections = [...source.matchAll(/\bid: '([^']+)'/g)].map(
        (match) => match[1]
      )
      expect(sections.length, group).toBeGreaterThan(0)
      for (const section of sections) {
        expect(paths.has(`/system-settings/${group}/${section}`)).toBe(true)
      }
    }
    for (const entry of NATIVE_FEATURE_ENTRIES) {
      expect(normalizeNativeFeaturePath(entry.path)).toBe(entry.path)
    }
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
        '../../../../relayfor-ui/assets/extensions.js'
      ),
      'utf8'
    )
    const mount = vi.fn()
    Object.assign(window, { RelayForFeatures: { entries: [], mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.relayforInlineMounted
    document.body.innerHTML =
      '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"></main>'
    history.replaceState(
      null,
      '',
      '/admin/#native/%2Fsystem-settings%2Fbilling%2Fcheckin'
    )
    window.eval(source)
    document.dispatchEvent(new Event('relayfor:ready'))
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
      new CustomEvent('proxylan:page', { detail: { name: 'native' } })
    )
    expect(location.hash).toBe('#activities')
    expect(activated).toHaveBeenCalledOnce()
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('removes the requested admin directory groups and retired security entrances', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../relayfor-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { RelayForFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.relayforInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="settings"><div data-settings-directory></div></section></main>'
    history.replaceState(null, '', '/admin/#native')
    window.eval(source)
    document.dispatchEvent(new Event('relayfor:ready'))
    const headings = [...document.querySelectorAll('.relayfor-feature-group h2')].map((node) => node.textContent)
    expect(headings).toEqual(['分析', '系统设置 · 认证', '系统设置 · 安全', '系统设置 · 请求策略'])
    expect(document.querySelector('[data-settings-page="ssrf"]')).toBeNull()
    expect(document.querySelector('[data-settings-page="gemini"]')).toBeNull()
    expect(document.querySelector('[data-native-feature-path="/system-settings/billing/payment"]')).toBeNull()
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('migrates saved payment links into the local settings page without mounting the legacy form', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../relayfor-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { RelayForFeatures: { entries: [], mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.relayforInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"></main>'
    history.replaceState(null, '', '/admin/#native/%2Fsystem-settings%2Fbilling%2Fpayment')
    window.eval(source)
    document.dispatchEvent(new Event('relayfor:ready'))
    expect(location.hash).toBe('#settings/payment')
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('removes the console feature directory and migrates saved links to existing pages', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../relayfor-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { RelayForFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'console'
    document.body.dataset.gatewayRole = '1'
    delete document.body.dataset.relayforInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="profile">账户</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="profile"></section></main>'
    history.replaceState(null, '', '/console/#native/%2Fsecurity')
    window.eval(source)
    document.dispatchEvent(new Event('relayfor:ready'))
    expect(location.hash).toBe('#profile')
    expect(document.querySelector('[data-nav="native"]')).toBeNull()
    expect(document.querySelector('[data-panel="native"]')).toBeNull()
    expect(document.body.textContent).not.toContain('更多功能')
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/console/#native')
    window.dispatchEvent(new PopStateEvent('popstate'))
    expect(location.hash).toBe('#overview')
    history.replaceState(null, '', '/')
  })

  it('merges the two user management views without mounting the former account tab', () => {
    const source = readFileSync(resolve(import.meta.dirname, '../../../../relayfor-ui/assets/extensions.js'), 'utf8')
    const mount = vi.fn()
    Object.assign(window, { RelayForFeatures: { entries: [], mount } })
    document.body.dataset.page = 'admin'
    document.body.dataset.gatewayRole = '100'
    delete document.body.dataset.relayforInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="users"></section></main>'
    history.replaceState(null, '', '/admin/#users/accounts')
    window.eval(source)
    document.dispatchEvent(new Event('relayfor:ready'))
    expect(location.hash).toBe('#users')
    expect(document.querySelector('[data-panel="users"] [data-relayfor-inline-tabs]')).toBeNull()
    expect(mount).not.toHaveBeenCalled()
    history.replaceState(null, '', '/')
  })

  it('removes console usage analysis and sends saved links back to usage records', () => {
    const source = readFileSync(
      resolve(import.meta.dirname, '../../../../relayfor-ui/assets/extensions.js'),
      'utf8'
    )
    const mount = vi.fn()
    Object.assign(window, { RelayForFeatures: { entries: NATIVE_FEATURE_ENTRIES, mount } })
    document.body.dataset.page = 'console'
    document.body.dataset.gatewayRole = '1'
    delete document.body.dataset.relayforInlineMounted
    document.body.innerHTML = '<nav class="nav-rail"><button data-nav="usage">调用记录</button><div class="rail-bottom"></div></nav><main class="content-area"><section data-panel="usage"><p>调用记录列表</p></section></main>'
    history.replaceState(null, '', '/console/#usage/analysis')
    window.eval(source)
    document.dispatchEvent(new Event('relayfor:ready'))

    expect(location.hash).toBe('#usage')
    expect(document.querySelector('[data-panel="usage"] [data-relayfor-inline-tabs]')).toBeNull()
    expect(document.querySelector('[data-panel="usage"] .relayfor-inline-host')).toBeNull()
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
