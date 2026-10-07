import { applyWorkspaceSeason, initializeWorkspacePalette } from './appearance-preferences'

const holidayKeys = new Set([
  'duanwu',
  'qixi',
  'mid_autumn',
  'chongyang',
  'new_year_eve',
  'national_day',
])

export function ensureWorkspaceColors(): void {
  if (
    document.querySelector(
      'link[data-further-workspace-colors], link[href^="/further-assets/workspace-colors.css"]'
    )
  ) {
    return
  }
  const version = document.querySelector<HTMLMetaElement>(
    'meta[name="further-asset-version"]'
  )?.content
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = `/further-assets/workspace-colors.css${version ? `?v=${encodeURIComponent(version)}` : ''}`
  link.dataset.furtherWorkspaceColors = ''
  document.head.append(link)
}

// React-only pages share the original shell's server-authoritative seasonal theme.
// Isolated feature roots inherit it from the surrounding shell instead.
export function watchSeasonalTheme(): () => void {
  initializeWorkspacePalette()
  ensureWorkspaceColors()
  let stopped = false
  let pending: AbortController | undefined
  async function refresh() {
    if (stopped || pending || document.hidden) return
    const controller = new AbortController()
    pending = controller
    const timeout = window.setTimeout(() => controller.abort(), 8000)
    try {
      const response = await fetch('/api/further/activities/theme', {
        credentials: 'same-origin',
        signal: controller.signal,
      })
      if (!response.ok) throw new Error('Seasonal theme unavailable')
      const result = await response.json()
      if (stopped) return
      const key = result.success === true ? result.data?.theme : ''
      applyWorkspaceSeason(typeof key === 'string' && holidayKeys.has(key) ? key : '')
    } catch {
      // Retain the last verified season during a temporary network failure.
      // A successful empty theme clears it when the holiday actually ends.
    } finally {
      window.clearTimeout(timeout)
      pending = undefined
    }
  }
  void refresh()
  const timer = window.setInterval(() => void refresh(), 60000)
  const refreshVisible = () => {
    if (!document.hidden) void refresh()
  }
  window.addEventListener('focus', refreshVisible)
  document.addEventListener('visibilitychange', refreshVisible)
  return () => {
    stopped = true
    pending?.abort()
    window.clearInterval(timer)
    window.removeEventListener('focus', refreshVisible)
    document.removeEventListener('visibilitychange', refreshVisible)
  }
}
