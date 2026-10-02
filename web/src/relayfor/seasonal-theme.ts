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
      'link[data-relayfor-workspace-colors], link[href^="/relayfor-assets/workspace-colors.css"]'
    )
  ) {
    return
  }
  const version = document.querySelector<HTMLMetaElement>(
    'meta[name="relayfor-asset-version"]'
  )?.content
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = `/relayfor-assets/workspace-colors.css${version ? `?v=${encodeURIComponent(version)}` : ''}`
  link.dataset.relayforWorkspaceColors = ''
  document.head.append(link)
}

// React-only pages share the original shell's server-authoritative seasonal theme.
// Isolated feature roots inherit it from the surrounding shell instead.
export function watchSeasonalTheme(): () => void {
  ensureWorkspaceColors()
  let stopped = false
  let pending: AbortController | undefined
  async function refresh() {
    if (stopped || pending || document.hidden) return
    const controller = new AbortController()
    pending = controller
    const timeout = window.setTimeout(() => controller.abort(), 8000)
    try {
      const response = await fetch('/api/relayfor/activities/theme', {
        credentials: 'same-origin',
        signal: controller.signal,
      })
      if (!response.ok) throw new Error('Seasonal theme unavailable')
      const result = await response.json()
      if (stopped) return
      const key = result.success === true ? result.data?.theme : ''
      if (typeof key === 'string' && holidayKeys.has(key)) {
        document.documentElement.dataset.holiday = key
      } else {
        delete document.documentElement.dataset.holiday
      }
    } catch {
      if (!stopped) delete document.documentElement.dataset.holiday
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
