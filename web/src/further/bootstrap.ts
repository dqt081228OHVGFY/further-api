import '@/i18n/config'

import { bootstrapAuthentication } from '@/lib/auth-session'
import { applySiteLogoToDom } from '@/lib/site-logo'
import { useAuthStore } from '@/stores/auth-store'

import { gatewayAuthenticated, gatewayExportLogs, gatewayLogout, gatewayRequest } from './gateway-request'
import { installNativeFeatureRuntime } from './native-features'
import { loadWorkspaceBootstrap } from './workspace-bootstrap'
import { initializeWorkspacePalette } from './appearance-preferences'
import { mountDisclosureMotion } from './disclosure-motion'
import { postLoginDestination } from './welcome-flow'
import { sanitizeAuthRedirect } from '@/features/auth/lib/auth-redirect'
import { MANAGEMENT_ENTRY_LABEL, managementDestination, workspaceShellTarget } from './workspace-target'

declare global {
  interface Window {
    FurtherQuotaPerUnit: number
    FurtherGateway: {
      request: typeof gatewayRequest
      authenticated: typeof gatewayAuthenticated
      logout: typeof gatewayLogout
      exportLogs: typeof gatewayExportLogs
      applyLogo: typeof applySiteLogoToDom
      postLoginDestination: typeof postLoginDestination
      managementDestination: typeof managementDestination
      managementLabel: typeof MANAGEMENT_ENTRY_LABEL
      workspaceShellTarget: typeof workspaceShellTarget
      sanitizeAuthRedirect: typeof sanitizeAuthRedirect
    }
    FurtherGatewayReady: Promise<void>
  }
}

// The source forms are visible before asynchronous boot finishes. Keep native
// submissions from navigating away (or putting credentials in a query string)
// when the server or a legacy script fails to load. Preventing the browser's
// default action still lets the original JavaScript submit handlers run.
document.body.dataset.gatewayReady = 'false'
mountDisclosureMotion(document)
const workspaceStartup = ['console','admin'].includes(document.body.dataset.page || '') ? document.createElement('div') : null
if(workspaceStartup) {
  workspaceStartup.className='workspace-startup-status'
  workspaceStartup.dataset.loading='true'
  workspaceStartup.setAttribute('role','status')
  workspaceStartup.textContent='正在打开你的工作区…'
  document.querySelector('.main-content')?.append(workspaceStartup)
  window.addEventListener('further:shell-ready',()=>{
    delete workspaceStartup.dataset.loading
    workspaceStartup.classList.add('is-leaving')
    window.setTimeout(()=>workspaceStartup.remove(),180)
  },{once:true})
}
document.addEventListener('submit', (event) => {
  if (document.body.dataset.gatewayReady !== 'true' || document.body.dataset.page === 'auth') {
    event.preventDefault()
  }
}, true)

async function bootWorkspaceFrontend(): Promise<void> {
  if (document.body.dataset.page === 'public' && location.pathname !== '/') return
  await bootstrapAuthentication()
  const bootstrap = await loadWorkspaceBootstrap()
  if (!bootstrap.setup.status) {
    location.replace('/setup')
    return
  }
  const user = useAuthStore.getState().auth.user
  document.body.dataset.gatewayRole = String(user?.role || 0)
  const protectedPage = ['console', 'admin'].includes(document.body.dataset.page || '')
  if (protectedPage && !user) {
    location.replace(`/sign-in?redirect=${  encodeURIComponent(location.pathname + location.hash)}`)
    return
  }
  if (document.body.dataset.page === 'admin' && Number(user?.role) < 10) {
    const feature = location.hash.match(/^#(?:workspace|native|gateway)\/(.+)$/)
    let requested = ''
    try {
      if (feature) requested = decodeURIComponent(feature[1])
      else if (location.hash === '#overview/analysis') requested = '/dashboard/models'
      else if (location.hash.slice(1).split('/')[0] === 'logs') requested = '/usage-logs/common'
    } catch { /* Invalid saved links return to the account overview. */ }
    location.replace(workspaceShellTarget(requested, Number(user?.role)))
    return
  }
  if (protectedPage && user) {
    const current = location.pathname + location.search + location.hash
    const destination = postLoginDestination(user, current)
    if (destination !== current) { location.replace(destination); return }
  }
  const status = bootstrap.status
  let pageTitle = ''
  if (protectedPage) pageTitle = '管理页面'
  document.title = pageTitle ? `${pageTitle} · further-api` : 'further-api'
  document.querySelectorAll('.rail-title,.public-brand strong,.auth-brand strong,[data-site-name]').forEach((element) => { element.textContent = 'further-api' })
  void applySiteLogoToDom('/further-api-logo.svg?v=handshake-20261004')
  const quotaPerUnit = Number(status.quota_per_unit)
  if (!Number.isFinite(quotaPerUnit) || quotaPerUnit <= 0) throw new Error('服务器额度换算配置无效')
  window.FurtherQuotaPerUnit = quotaPerUnit
  initializeWorkspacePalette()
  document.documentElement.dataset.furtherMotion = 'on'
  document.documentElement.dataset.reducedMotion = 'false'
  const scripts = [...document.querySelectorAll<HTMLScriptElement>('script[data-further-source-only]')]
  for (const source of scripts) {
    const url = new URL(source.src, location.href)
    if (url.origin !== location.origin || !url.pathname.startsWith('/further-assets/')) throw new Error('Invalid Further asset path')
    await new Promise<void>((resolve, reject) => {
      const script = document.createElement('script')
      script.src = url.href
      const cleanup = () => {
        script.removeEventListener('load', loaded)
        script.removeEventListener('error', failed)
      }
      const loaded = () => { cleanup(); resolve() }
      const failed = () => {
        cleanup()
        reject(new Error(`无法载入界面资源：${url.pathname}`))
      }
      script.addEventListener('load', loaded, { once: true })
      script.addEventListener('error', failed, { once: true })
      document.head.append(script)
    })
  }
  document.body.dataset.gatewayReady = 'true'
  document.dispatchEvent(new Event('further:ready'))
  delete document.documentElement.dataset.furtherBoot
}

window.FurtherGateway = { request: gatewayRequest, authenticated: gatewayAuthenticated, logout: gatewayLogout, exportLogs: gatewayExportLogs, applyLogo: applySiteLogoToDom, postLoginDestination, managementDestination, managementLabel: MANAGEMENT_ENTRY_LABEL, workspaceShellTarget, sanitizeAuthRedirect }
installNativeFeatureRuntime()
window.FurtherGatewayReady = bootWorkspaceFrontend().catch((error: unknown) => {
  workspaceStartup?.remove()
  delete document.documentElement.dataset.furtherBoot
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
  // oxlint-disable-next-line no-console -- Preserve diagnostics for a failed authentication or application bootstrap.
  console.error('further-api initialization failed:', error)
})
