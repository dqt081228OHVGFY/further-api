import { QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { createRef, type CSSProperties } from 'react'
import { createRoot, type Root } from 'react-dom/client'

import { PortalContainerContext } from '@/components/ui/portal-container'
import { Toaster } from '@/components/ui/sonner'
import { DirectionProvider } from '@/context/direction-provider'
import { FontProvider } from '@/context/font-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { resolveAuthentication } from '@/lib/auth-session'
import { createAppQueryClient } from '@/lib/query-client'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth-store'

import { NativeFeatureContext } from './native-feature-context'
import {
  NATIVE_FEATURE_ENTRIES,
  normalizeNativeFeaturePath,
  type NativeFeatureEntry,
} from './native-feature-paths'

export interface NativeFeatureHandle {
  navigate: (path: string) => Promise<void>
  destroy: () => void
}

export interface NativeFeatureOptions {
  path: string
}

declare global {
  interface Window {
    RelayForFeatures: {
      entries: readonly NativeFeatureEntry[]
      mount: (
        host: HTMLElement,
        options: NativeFeatureOptions
      ) => Promise<NativeFeatureHandle>
    }
  }
}

const queryClient = createAppQueryClient()
const mounts = new WeakMap<HTMLElement, Promise<NativeFeatureHandle>>()
type FeatureStyles = { styles: string[]; globals: string[] }
let manifestPromise: Promise<FeatureStyles> | undefined
const globalStylePromises = new Map<string, Promise<void>>()
let notificationRoot: Root | undefined
let notificationPromise: Promise<void> | undefined

function validatedStyleUrls(values: unknown): string[] {
  if (!Array.isArray(values)) throw new Error('功能样式配置无效。')
  return values.map((value: unknown) => {
    if (typeof value !== 'string') throw new Error('功能样式地址无效。')
    const url = new URL(value, location.origin)
    if (
      url.origin !== location.origin ||
      !url.pathname.startsWith('/relayfor-assets/')
    ) {
      throw new Error('功能样式地址无效。')
    }
    return url.href
  })
}

async function styles(): Promise<FeatureStyles> {
  const version = document.querySelector<HTMLMetaElement>(
    'meta[name="relayfor-asset-version"]'
  )?.content
  const manifestUrl = '/relayfor-assets/native-features-manifest.json' +
    (version ? `?v=${encodeURIComponent(version)}` : '')
  manifestPromise ??= fetch(manifestUrl, {
    credentials: 'same-origin',
    cache: 'no-cache',
  })
    .then(async (response) => {
      if (!response.ok) throw new Error('功能样式暂时无法加载，请重试。')
      const manifest: unknown = await response.json()
      if (
        !manifest ||
        typeof manifest !== 'object' ||
        !('styles' in manifest)
      ) {
        throw new Error('功能样式配置无效。')
      }
      const urls = validatedStyleUrls(manifest.styles)
      if (urls.length === 0) throw new Error('功能样式配置无效。')
      const globals =
        'globals' in manifest ? validatedStyleUrls(manifest.globals) : []
      return { styles: urls, globals }
    })
    .catch((error: unknown) => {
      manifestPromise = undefined
      throw error
    })
  return manifestPromise
}

function attachGlobalStyle(href: string): Promise<void> {
  const existing = globalStylePromises.get(href)
  if (existing) return existing
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = href
  link.dataset.nativeFeatureGlobalStyle = ''
  const pending = new Promise<void>((resolve, reject) => {
    link.onload = () => resolve()
    link.onerror = () => reject(new Error('功能样式暂时无法加载，请重试。'))
    document.head.append(link)
  }).catch((error: unknown) => {
    globalStylePromises.delete(href)
    link.remove()
    throw error
  })
  globalStylePromises.set(href, pending)
  return pending
}

async function attachStyles(shadow: ShadowRoot): Promise<void> {
  const { styles: urls, globals } = await styles()
  await Promise.all(globals.map(attachGlobalStyle))
  await Promise.all(
    urls.map(
      (href) =>
        new Promise<void>((resolve, reject) => {
          const link = document.createElement('link')
          link.rel = 'stylesheet'
          link.href = href
          link.onload = () => resolve()
          link.onerror = () =>
            reject(new Error('功能样式暂时无法加载，请重试。'))
          shadow.append(link)
        })
    )
  )
}

function synchronizeTheme(host: HTMLElement): () => void {
  const originalDark = host.classList.contains('dark')
  const attributes = [
    'dir',
    'data-theme',
    'data-relayfor-motion',
    'data-festival',
    'data-holiday',
    'data-relayfor-palette',
  ] as const
  const originalAttributes = attributes.map(
    (name) => [name, host.getAttribute(name)] as const
  )
  const sync = () => {
    const source = document.documentElement
    const dark =
      source.dataset.theme === 'dark' ||
      (!source.dataset.theme && source.classList.contains('dark'))
    host.classList.toggle('dark', dark)
    host.setAttribute('dir', source.getAttribute('dir') || 'ltr')
    host.dataset.theme = dark ? 'dark' : 'light'
    host.dataset.relayforMotion =
      source.dataset.relayforMotion ||
      (source.dataset.reducedMotion === 'true' ? 'off' : '')
    if (source.dataset.festival) {
      host.dataset.festival = source.dataset.festival
    } else {
      delete host.dataset.festival
    }
    if (source.dataset.holiday) host.dataset.holiday = source.dataset.holiday
    else delete host.dataset.holiday
    if (source.dataset.relayforPalette) {
      host.dataset.relayforPalette = source.dataset.relayforPalette
    } else {
      delete host.dataset.relayforPalette
    }
    // Material custom properties inherit through the host, including changes
    // to the document element's inline style.
  }
  sync()
  const observer = new MutationObserver(sync)
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: [
      'class',
      'dir',
      'style',
      'data-theme',
      'data-reduced-motion',
      'data-relayfor-motion',
      'data-relayfor-palette',
      'data-festival',
      'data-holiday',
    ],
  })
  return () => {
    observer.disconnect()
    host.classList.toggle('dark', originalDark)
    originalAttributes.forEach(([name, value]) => {
      if (value === null) host.removeAttribute(name)
      else host.setAttribute(name, value)
    })
  }
}

async function notifications(): Promise<void> {
  if (notificationRoot) return
  if (notificationPromise) return notificationPromise
  notificationPromise = mountNotifications().catch((error: unknown) => {
    notificationPromise = undefined
    throw error
  })
  return notificationPromise
}

async function mountNotifications(): Promise<void> {
  const host = document.createElement('div')
  host.dataset.nativeFeatureNotifications = ''
  document.body.append(host)
  const shadow = host.attachShadow({ mode: 'open' })
  const disconnectTheme = synchronizeTheme(host)
  try {
    await attachStyles(shadow)
    const root = document.createElement('div')
    root.dataset.nativeFeaturePortal = ''
    shadow.append(root)
    notificationRoot = createRoot(root)
    notificationRoot.render(
      <ThemeProvider isolated>
        <Toaster
          closeButton
          duration={5000}
          position='bottom-center'
          richColors
          style={{
            '--normal-bg': 'var(--md-sys-color-on-surface, #263952)',
            '--normal-text': 'var(--md-sys-color-surface, #fffefb)',
            '--normal-border': 'var(--md-sys-color-on-surface, #263952)',
            '--success-bg': 'var(--md-sys-color-on-surface, #263952)',
            '--success-text': 'var(--md-sys-color-surface, #fffefb)',
            '--success-border': 'var(--md-sys-color-on-surface, #263952)',
            '--error-bg': 'var(--md-sys-color-error-container, #f9dedc)',
            '--error-text': 'var(--md-sys-color-on-error-container, #410e0b)',
            '--error-border': 'var(--md-sys-color-error-container, #f9dedc)',
            '--border-radius': '12px',
            fontFamily: 'inherit',
          } as CSSProperties}
        />
      </ThemeProvider>
    )
  } catch (error) {
    notificationRoot?.unmount()
    notificationRoot = undefined
    disconnectTheme()
    host.remove()
    throw error
  }
}

async function mountFeature(
  host: HTMLElement,
  options: NativeFeatureOptions
): Promise<NativeFeatureHandle> {
  const path = normalizeNativeFeaturePath(options.path)
  await resolveAuthentication()
  const auth = useAuthStore.getState().auth
  if (!auth.user || !auth.accessToken) {
    throw new Error('登录已失效，请重新登录后打开此功能。')
  }
  const shadow = host.shadowRoot ?? host.attachShadow({ mode: 'open' })
  shadow.replaceChildren()
  const hadFeatureClass = host.classList.contains(
    'relayfor-native-feature-host'
  )
  const originalAriaBusy = host.getAttribute('aria-busy')
  const restoreHost = () => {
    host.classList.toggle('relayfor-native-feature-host', hadFeatureClass)
    if (originalAriaBusy === null) host.removeAttribute('aria-busy')
    else host.setAttribute('aria-busy', originalAriaBusy)
  }
  host.classList.add('relayfor-native-feature-host')
  host.setAttribute('aria-busy', 'true')
  const disconnectTheme = synchronizeTheme(host)
  let stopRouter: (() => void) | undefined
  let mountedRoot: Root | undefined
  try {
    await attachStyles(shadow)
    await notifications()
    const content = document.createElement('div')
    content.className = 'relayfor-native-feature'
    content.dataset.nativeFeatureRoot = ''
    const portal = document.createElement('div')
    portal.dataset.nativeFeaturePortal = ''
    shadow.append(content, portal)
    const portalRef = createRef<HTMLElement>()
    portalRef.current = portal
    const router = createRouter({
      routeTree,
      history: createMemoryHistory({ initialEntries: [path] }),
      context: { queryClient },
      defaultPreload: 'intent',
      defaultPreloadStaleTime: 0,
    })
    const unsubscribe = router.subscribe('onResolved', () => {
      if (router.state.location.pathname === '/sign-in') {
        location.assign(
          '/auth/?mode=login&redirect=' +
            encodeURIComponent(location.pathname + location.hash)
        )
      }
    })
    stopRouter = unsubscribe
    await router.load()
    const root = createRoot(content)
    mountedRoot = root
    root.render(
      <NativeFeatureContext value={true}>
        <PortalContainerContext value={portalRef}>
          <QueryClientProvider client={queryClient}>
            <ThemeProvider isolated>
              <FontProvider isolated>
                <DirectionProvider isolated>
                  <RouterProvider router={router} />
                </DirectionProvider>
              </FontProvider>
            </ThemeProvider>
          </QueryClientProvider>
        </PortalContainerContext>
      </NativeFeatureContext>
    )
    if (originalAriaBusy === null) host.removeAttribute('aria-busy')
    else host.setAttribute('aria-busy', originalAriaBusy)
    let destroyed = false
    return {
      navigate: async (nextPath: string) => {
        await router.navigate({ href: normalizeNativeFeaturePath(nextPath) })
      },
      destroy: () => {
        if (destroyed) return
        destroyed = true
        root.unmount()
        unsubscribe()
        disconnectTheme()
        shadow.replaceChildren()
        restoreHost()
        mounts.delete(host)
      },
    }
  } catch (error) {
    mountedRoot?.unmount()
    stopRouter?.()
    disconnectTheme()
    shadow.replaceChildren()
    restoreHost()
    throw error
  }
}

export function installNativeFeatureRuntime(): void {
  window.RelayForFeatures = {
    entries: NATIVE_FEATURE_ENTRIES,
    mount: (host, options) => {
      const existing = mounts.get(host)
      if (existing) {
        return existing.then(async (handle) => {
          await handle.navigate(options.path)
          return handle
        })
      }
      const pending = mountFeature(host, options).catch((error: unknown) => {
        mounts.delete(host)
        throw error
      })
      mounts.set(host, pending)
      return pending
    },
  }
}
