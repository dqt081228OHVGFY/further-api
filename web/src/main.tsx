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
import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'

import { resolveAuthentication } from '@/lib/auth-session'
import { installBuildMetadata } from '@/lib/build-metadata'
import '@/lib/dayjs'
import { applyFaviconToDom } from '@/lib/dom-utils'
import { initializeFrontendCache } from '@/lib/frontend-cache'
import { createAppQueryClient } from '@/lib/query-client'
import { publishWorkspaceStatus, STATUS_QUERY_KEY } from '@/lib/status-query'
import { loadWorkspaceBootstrap } from '@/further/workspace-bootstrap'
import { initializeWorkspacePalette } from '@/further/appearance-preferences'
import { normalizeNativeFeaturePath } from '@/further/native-feature-paths'
import {
  activityShellTarget,
  workspaceShellTarget,
} from '@/further/workspace-target'
import { useAuthStore } from '@/stores/auth-store'

import { DirectionProvider } from './context/direction-provider'
import { FontProvider } from './context/font-provider'
import { ThemeProvider } from './context/theme-provider'
import './i18n/config'

// Styles
import './styles/index.css'

initializeFrontendCache()
initializeWorkspacePalette()
installBuildMetadata()

const queryClient = createAppQueryClient()

const rootElement = (() => {
  let element = document.querySelector<HTMLElement>('#root')
  if (!element && document.body.dataset.page === 'public' && location.pathname !== '/') {
    element = document.createElement('div')
    element.id = 'root'
    document.body.replaceChildren(element)
  }
  return element
})()

async function mountPublicApplication(element: HTMLElement): Promise<void> {
  document.title = 'further-api'
  applyFaviconToDom('/further-api-logo.svg?v=handshake-20261004')
  const styles = document.querySelector<HTMLMetaElement>('meta[name="further-app-styles"]')?.content.split(',') || []
  for (const href of styles) {
    if (!href.startsWith('/static/') || document.querySelector(`link[href="${href}"]`)) continue
    const link = document.createElement('link')
    link.rel = 'stylesheet'
    link.href = href
    document.head.append(link)
  }
  const status = document.createElement('p')
  status.className = 'further-startup'
  status.setAttribute('role', 'status')
  status.textContent = '正在打开 further-api…'
  element.replaceChildren(status)
  await resolveAuthentication()
  const bootstrap = await loadWorkspaceBootstrap()
  publishWorkspaceStatus(bootstrap.status)
  queryClient.setQueryData(STATUS_QUERY_KEY, bootstrap.status)

  if (location.pathname.replace(/\/+$/, '') === '/welcome' && !useAuthStore.getState().auth.user) {
    location.replace(`/auth/?mode=login&redirect=${encodeURIComponent(location.pathname + location.search)}`)
    return
  }

  let protectedFeature: string | null = null
  try {
    const requestedFeature = location.pathname + location.search
    protectedFeature = activityShellTarget(requestedFeature)
      ? requestedFeature
      : normalizeNativeFeaturePath(requestedFeature)
  } catch { /* Public and authentication pages mount below. */ }
  if (protectedFeature) {
    const user = useAuthStore.getState().auth.user
    location.replace(user
      ? workspaceShellTarget(protectedFeature, Number(user.role))
      : `/auth/?mode=login&redirect=${encodeURIComponent(protectedFeature)}`)
    return
  }
  const { FurtherPublicApp } = await import('@/further/public-pages')
  ReactDOM.createRoot(element).render(
    <StrictMode><QueryClientProvider client={queryClient}>
      <ThemeProvider><FontProvider><DirectionProvider><FurtherPublicApp /></DirectionProvider></FontProvider></ThemeProvider>
    </QueryClientProvider></StrictMode>
  )
  document.body.dataset.gatewayReady = 'true'
}

if (rootElement) {
  void mountPublicApplication(rootElement).catch(() => {
    const notice = document.createElement('div')
    notice.className = 'further-startup further-error'
    notice.setAttribute('role', 'alert')
    const text = document.createElement('p')
    text.textContent = '页面暂时无法打开，请重新载入重试。'
    const retry = document.createElement('button')
    retry.className = 'further-button'
    retry.textContent = '重新载入'
    retry.addEventListener('click', () => location.reload())
    notice.append(text, retry)
    rootElement.replaceChildren(notice)
  })
}
