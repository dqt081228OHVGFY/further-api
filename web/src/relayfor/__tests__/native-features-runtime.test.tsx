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
import { afterEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { useAuthStore } from '@/stores/auth-store'

import {
  installNativeFeatureRuntime,
  type NativeFeatureHandle,
} from '../native-features'

const auth = vi.hoisted(() => ({ resolveAuthentication: vi.fn() }))

vi.mock('@/lib/auth-session', () => auth)
vi.mock('@/routeTree.gen', async () => {
  const React = await import('react')
  const { createRootRoute, createRoute, Outlet } =
    await import('@tanstack/react-router')
  const root = createRootRoute({ component: Outlet })
  const page = (path: string, label: string) =>
    createRoute({
      getParentRoute: () => root,
      path,
      component: () =>
        React.createElement('div', { 'data-native-test-page': label }, label),
    })
  return {
    routeTree: root.addChildren([
      page('/keys', 'keys'),
      page('/dashboard/models', 'models'),
    ]),
  }
})

const hosts: HTMLElement[] = []
const handles: NativeFeatureHandle[] = []
let styleLinkCount = 0
let failStyleLinkAt = -1

function host() {
  const element = document.createElement('div')
  document.body.append(element)
  hosts.push(element)
  return element
}

function manifest(
  styles: string[] = ['/relayfor-assets/native.css'],
  globals?: string[]
): Response {
  return {
    ok: true,
    json: async () => (globals ? { styles, globals } : { styles }),
  } as Response
}

function completeStyleLinks(nodes: (Node | string)[]) {
  nodes.forEach((node) => {
    if (!(node instanceof HTMLLinkElement)) return
    styleLinkCount += 1
    const event = styleLinkCount === failStyleLinkAt ? 'error' : 'load'
    queueMicrotask(() => node.dispatchEvent(new Event(event)))
  })
}

function dispatchStyleEvents() {
  const appendShadow = ShadowRoot.prototype.append
  vi.spyOn(ShadowRoot.prototype, 'append').mockImplementation(function (
    this: ShadowRoot,
    ...nodes: (Node | string)[]
  ) {
    appendShadow.call(this, ...nodes)
    completeStyleLinks(nodes)
  })
  const appendElement = Element.prototype.append
  vi.spyOn(Element.prototype, 'append').mockImplementation(function (
    this: Element,
    ...nodes: (Node | string)[]
  ) {
    appendElement.call(this, ...nodes)
    completeStyleLinks(nodes)
  })
}

function authenticate() {
  useAuthStore.getState().auth.setBundle({
    access_token: 'test-access-token',
    token_type: 'Bearer',
    access_expires_at: Date.now() + 60_000,
    user: { id: 1, username: 'tester', role: 100 },
    session: {
      sid: 'test-session',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'vitest',
      created_at: Date.now(),
      last_active_at: Date.now(),
      expires_at: Date.now() + 60_000,
    },
  })
}

afterEach(() => {
  toast.dismiss()
  handles.splice(0).forEach((handle) => handle.destroy())
  hosts.splice(0).forEach((element) => element.remove())
  document
    .querySelectorAll(
      '[data-native-feature-notifications], [data-native-feature-global-style]'
    )
    .forEach((element) => element.remove())
  delete document.documentElement.dataset.festival
  delete document.documentElement.dataset.holiday
  delete document.documentElement.dataset.relayforPalette
  document.documentElement.style.removeProperty('--md-sys-color-primary')
  document.querySelector('meta[data-native-test-version]')?.remove()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('native feature runtime', () => {
  it('guards auth, retries manifest and CSS failures, reuses roots, and cleans up', async () => {
    useAuthStore.getState().auth.reset()
    auth.resolveAuthentication.mockReset().mockResolvedValue(undefined)
    styleLinkCount = 0
    failStyleLinkAt = -1
    dispatchStyleEvents()
    const version = document.createElement('meta')
    version.name = 'relayfor-asset-version'
    version.content = 'native-style-release-2'
    version.dataset.nativeTestVersion = ''
    document.head.append(version)
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    installNativeFeatureRuntime()
    const features = window.RelayForFeatures
    const element = host()

    await expect(features.mount(element, { path: '/keys' })).rejects.toThrow(
      '登录已失效'
    )
    expect(element.shadowRoot).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()

    authenticate()
    fetchMock
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(manifest([]))
      .mockResolvedValue(
        manifest(
          ['/relayfor-assets/native.css'],
          ['/relayfor-assets/native-features-foundations.css']
        )
      )
    await expect(features.mount(element, { path: '/keys' })).rejects.toThrow(
      'offline'
    )
    await expect(features.mount(element, { path: '/keys' })).rejects.toThrow(
      '功能样式配置无效'
    )
    expect(element.shadowRoot?.childNodes).toHaveLength(0)

    failStyleLinkAt = 1 // Global foundations fail before shadow styles.
    await expect(features.mount(element, { path: '/keys' })).rejects.toThrow(
      '功能样式暂时无法加载'
    )
    expect(
      document.querySelectorAll('[data-native-feature-global-style]')
    ).toHaveLength(0)

    failStyleLinkAt = 3 // Shadow style fails after the global retry succeeds.
    await expect(features.mount(element, { path: '/keys' })).rejects.toThrow(
      '功能样式暂时无法加载'
    )
    failStyleLinkAt = 5 // Notification shadow style fails and must clean up.
    await expect(features.mount(element, { path: '/keys' })).rejects.toThrow(
      '功能样式暂时无法加载'
    )
    expect(
      document.querySelectorAll('[data-native-feature-notifications]')
    ).toHaveLength(0)
    expect(element.shadowRoot?.childNodes).toHaveLength(0)

    failStyleLinkAt = -1
    const first = await features.mount(element, { path: '/keys' })
    handles.push(first)
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(fetchMock).toHaveBeenLastCalledWith(
      '/relayfor-assets/native-features-manifest.json?v=native-style-release-2',
      { credentials: 'same-origin', cache: 'no-cache' }
    )
    expect(styleLinkCount).toBe(7)
    expect(
      document.querySelectorAll('[data-native-feature-global-style]')
    ).toHaveLength(1)
    const noticeHost = document.querySelector<HTMLElement>(
      '[data-native-feature-notifications]'
    )!
    toast.success('界面提示成功测试', { id: 'native-success', duration: Infinity })
    toast.error('界面提示失败测试', { id: 'native-error', duration: Infinity })
    await vi.waitFor(() => {
      const toaster = noticeHost.shadowRoot?.querySelector<HTMLElement>('[data-sonner-toaster]')
      expect(toaster).toHaveAttribute('data-y-position', 'bottom')
      expect(toaster?.style.getPropertyValue('--normal-bg')).toContain('--md-sys-color-on-surface')
      expect(noticeHost.shadowRoot?.querySelector('[data-type="success"] [data-title]')).toHaveTextContent('界面提示成功测试')
      expect(noticeHost.shadowRoot?.querySelector('[data-type="error"] [data-title]')).toHaveTextContent('界面提示失败测试')
    })
    expect(noticeHost.shadowRoot?.querySelector('link[rel="stylesheet"]')).not.toBeNull()

    const second = await features.mount(element, {
      path: '/dashboard/models',
    })
    expect(second).toBe(first)
    expect(
      element.shadowRoot?.querySelectorAll('[data-native-feature-root]')
    ).toHaveLength(1)
    await vi.waitFor(() => {
      expect(
        element.shadowRoot?.querySelector('[data-native-test-page]')
      ).toHaveAttribute('data-native-test-page', 'models')
    })

    const another = host()
    const anotherHandle = await features.mount(another, { path: '/keys' })
    handles.push(anotherHandle)
    expect(
      document.querySelectorAll('[data-native-feature-global-style]')
    ).toHaveLength(1)

    document.documentElement.dataset.festival = 'midautumn'
    document.documentElement.dataset.holiday = 'mid_autumn'
    document.documentElement.dataset.relayforPalette = 'yellow'
    document.documentElement.style.setProperty('--md-sys-color-primary', '#abc')
    await vi.waitFor(() => {
      expect(element.dataset.festival).toBe('midautumn')
      expect(element.dataset.holiday).toBe('mid_autumn')
      expect(element.dataset.relayforPalette).toBe('yellow')
    })

    first.destroy()
    first.destroy() // Teardown is idempotent.
    expect(element.shadowRoot?.childNodes).toHaveLength(0)
    expect(element).not.toHaveClass('relayfor-native-feature-host')
    expect(element.dataset.festival).toBeUndefined()
    expect(element.dataset.holiday).toBeUndefined()
    expect(element.dataset.relayforPalette).toBeUndefined()

    const remount = await features.mount(element, { path: '/keys' })
    handles.push(remount)
    expect(remount).not.toBe(first)
    expect(
      element.shadowRoot?.querySelectorAll('[data-native-feature-root]')
    ).toHaveLength(1)
  })
})
