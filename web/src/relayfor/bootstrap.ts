import '@/i18n/config'

import { api } from '@/lib/api'
import { bootstrapAuthentication } from '@/lib/auth-session'
import { requireServerSuccess } from '@/lib/server-error-message'
import { applySiteLogoToDom } from '@/lib/site-logo'
import { useAuthStore } from '@/stores/auth-store'

import { gatewayAuthenticated, gatewayExportLogs, gatewayLogout, gatewayRequest } from './gateway-request'
import { installNativeFeatureRuntime } from './native-features'

declare global {
  interface Window {
    RelayForQuotaPerUnit: number
    RelayForGateway: {
      request: typeof gatewayRequest
      authenticated: typeof gatewayAuthenticated
      logout: typeof gatewayLogout
      exportLogs: typeof gatewayExportLogs
      applyLogo: typeof applySiteLogoToDom
    }
    RelayForGatewayReady: Promise<void>
  }
}

// The source forms are visible before asynchronous boot finishes. Keep native
// submissions from navigating away (or putting credentials in a query string)
// when the server or a legacy script fails to load. Preventing the browser's
// default action still lets the original JavaScript submit handlers run.
document.body.dataset.gatewayReady = 'false'
document.addEventListener('submit', (event) => {
  if (document.body.dataset.gatewayReady !== 'true' || document.body.dataset.page === 'auth') {
    event.preventDefault()
  }
}, true)

async function bootOriginalFrontend(): Promise<void> {
  const setupResponse = await api.get('/api/setup')
  if (setupResponse.data?.success && !setupResponse.data.data?.status) {
    location.replace('/setup')
    return
  }
  await bootstrapAuthentication()
  const user = useAuthStore.getState().auth.user
  document.body.dataset.gatewayRole = String(user?.role || 0)
  const protectedPage = ['console', 'admin'].includes(document.body.dataset.page || '')
  if (protectedPage && !user) {
    location.replace('/sign-in?redirect=' + encodeURIComponent(location.pathname + location.hash))
    return
  }
  if (document.body.dataset.page === 'admin' && Number(user?.role) < 10) {
    location.replace('/console/')
    return
  }
  const statusResponse = await api.get('/api/status')
  const status = requireServerSuccess(statusResponse.data).data
  let pageTitle = ''
  if (document.body.dataset.page === 'admin') pageTitle = '管理后台'
  if (document.body.dataset.page === 'console') pageTitle = '控制台'
  document.title = pageTitle ? `${pageTitle} · ${status.system_name}` : status.system_name
  document.querySelectorAll('.rail-title,.public-brand strong,.auth-brand strong,[data-site-name]').forEach((element) => { element.textContent = status.system_name })
  void applySiteLogoToDom(status.logo || '/relayfor-logo.svg')
  const quotaPerUnit = Number(status.quota_per_unit)
  if (!Number.isFinite(quotaPerUnit) || quotaPerUnit <= 0) throw new Error('服务器额度换算配置无效')
  window.RelayForQuotaPerUnit = quotaPerUnit
  if (Number(user?.role) >= 10) {
    const groupsResponse = await api.get('/api/group/')
    const groupData = requireServerSuccess(groupsResponse.data).data
    const groups = Array.isArray(groupData) ? groupData.filter((item): item is string => typeof item === 'string') : Object.keys(groupData || {})
    document.querySelectorAll<HTMLSelectElement>('select[name="user_group"]').forEach((select) => {
      const isFilter = Boolean(select.querySelector('option[value=""]'))
      select.replaceChildren()
      if (isFilter) select.add(new Option('全部用户组', ''))
      for (const group of groups) select.add(new Option(group, group))
      if (!isFilter && groups.includes('default')) select.value = 'default'
    })
  }
  document.documentElement.dataset.relayforPalette = 'white'
  try {
    const palette = localStorage.getItem('newapi:theme:v1:relayfor-palette')
    if (palette === 'blue' || palette === 'yellow') document.documentElement.dataset.relayforPalette = palette
    const motion = localStorage.getItem('relayfor-motion')
    if (motion === 'on' || motion === 'off') document.documentElement.dataset.relayforMotion = motion
  } catch { /* Preferences can still be changed for this page. */ }
  const scripts = [...document.querySelectorAll<HTMLScriptElement>('script[data-relayfor-source-only]')]
  for (const source of scripts) {
    const url = new URL(source.src, location.href)
    if (url.origin !== location.origin || !url.pathname.startsWith('/relayfor-assets/')) throw new Error('Invalid RelayFor asset path')
    await new Promise<void>((resolve, reject) => {
      const script = document.createElement('script')
      script.src = url.href
      script.onload = () => resolve()
      script.onerror = () => reject(new Error('无法载入界面资源：' + url.pathname))
      document.head.append(script)
    })
  }
  document.body.dataset.gatewayReady = 'true'
  document.dispatchEvent(new Event('relayfor:ready'))
  delete document.documentElement.dataset.relayforBoot
}

window.RelayForGateway = { request: gatewayRequest, authenticated: gatewayAuthenticated, logout: gatewayLogout, exportLogs: gatewayExportLogs, applyLogo: applySiteLogoToDom }
installNativeFeatureRuntime()
window.RelayForGatewayReady = bootOriginalFrontend().catch((error: unknown) => {
  delete document.documentElement.dataset.relayforBoot
  const notice = document.createElement('div')
  notice.className = 'snackbar show error'
  notice.setAttribute('role', 'alert')
  const message = document.createElement('span')
  message.textContent = '页面初始化失败，服务或界面资源暂时不可用。请重新载入重试。'
  const reload = document.createElement('button')
  reload.type = 'button'
  reload.className = 'md-btn md-btn-text'
  reload.textContent = '重新载入'
  reload.addEventListener('click', () => location.reload())
  notice.append(message, reload)
  document.body.append(notice)
  console.error('RelayFor initialization failed:', error)
})
