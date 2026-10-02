import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../relayfor-ui/assets/admin/multi-user-stats.js'), 'utf8')
const markup = readFileSync(resolve(process.cwd(), '../relayfor-ui/admin/index.html'), 'utf8')
const shellRuntime = readFileSync(resolve(process.cwd(), '../relayfor-ui/assets/app.js'), 'utf8')
const request = vi.fn()
const createObjectURL = vi.fn()
const revokeObjectURL = vi.fn()
type RuntimeWindow = Window & { ProxyLan: unknown; ProxyLanMultiUserStats: { mount(): void; query(): Promise<void>; exportCSV(): void } }
let exportedBlob: Blob | null = null
let filename = ''

beforeEach(() => {
  document.body.innerHTML = '<nav class="nav-rail"><button class="nav-item" data-nav="users">用户</button></nav><div class="content-area"></div>'
  request.mockReset()
  exportedBlob = null; filename = ''
  createObjectURL.mockReset().mockImplementation((blob: Blob) => { exportedBlob = blob; return 'blob:stats-export' })
  revokeObjectURL.mockReset()
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) { filename = this.download })
  ;(window as unknown as RuntimeWindow).ProxyLan = {
    request, initializeVisuals() {}, showBusy() {}, toast() {},
    number(value: number) { return String(value || 0) }, moneyFromQuota(value: number) { return '$' + Number(value || 0) / 500000 },
    usdFromQuota(value: number) { return Number(value || 0) / 500000 },
    escapeHTML(value: unknown) { return String(value).replaceAll(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char] ?? char) },
  }
  window.eval(runtime)
  ;(window as unknown as RuntimeWindow).ProxyLanMultiUserStats.mount()
  const form = document.querySelector('[data-multi-user-form]') as HTMLFormElement
  ;(form.elements.namedItem('usernames') as HTMLInputElement).value = 'alice, bob'
  ;(form.elements.namedItem('start_time') as HTMLInputElement).value = '2026-10-01'
  ;(form.elements.namedItem('end_time') as HTMLInputElement).value = '2026-10-02'
})

const result = {
  items: [
    { user_id: 1, username: 'alice', request_count: 10, error_count: 1, quota: 500000, prompt_tokens: 200, completion_tokens: 50, request_time: 12340 },
    { user_id: 2, username: 'bob', request_count: 5, error_count: 0, quota: 250000, prompt_tokens: 100, completion_tokens: 20, request_time: 8000 },
  ],
  model_usage: [
    { user_id: 1, username: 'alice', model_name: 'model-a', request_count: 10, error_count: 1, quota: 500000, prompt_tokens: 200, completion_tokens: 50, request_time: 12340 },
    { user_id: 2, username: 'bob', model_name: 'model-b', request_count: 5, error_count: 0, quota: 250000, prompt_tokens: 100, completion_tokens: 20, request_time: 8000 },
  ], unknown_usernames: [], no_usage_usernames: [],
}

describe('reference multi-user statistics page backed by real aggregates', () => {
  it('supports a direct hash and navigation through the real shell', () => {
    document.body.dataset.page = 'admin'
    document.querySelector('.nav-rail')?.setAttribute('data-shell', '')
    const title = document.createElement('div')
    title.dataset.pageTitle = ''; document.body.append(title)
    vi.stubGlobal('CSS', { escape(value: string) { return value } })
    vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener() {}, removeEventListener() {} }))
    history.replaceState(null, '', '#multi-user-stats')
    window.eval(shellRuntime)
    const shell = (window as unknown as { ProxyLan: { mountShell(): void } }).ProxyLan
    shell.mountShell()
    expect(document.querySelector('[data-panel="multi-user-stats"]')?.classList.contains('active')).toBe(true)
    expect(title.textContent).toBe('多用户令牌统计')
    expect(location.hash).toBe('#multi-user-stats')
    expect(document.querySelector('.nav-rail [data-nav="multi-user-stats"]')?.classList.contains('active')).toBe(true)
    history.replaceState(null, '', '/')
    vi.unstubAllGlobals()
  })

  it('mounts one navigation panel before the shell script and renders actual user cards and chart proportions', async () => {
    const stats = (window as unknown as RuntimeWindow).ProxyLanMultiUserStats
    stats.mount()
    expect(document.querySelectorAll('[data-panel="multi-user-stats"]')).toHaveLength(1)
    expect(document.querySelectorAll('.nav-rail [data-nav="multi-user-stats"]')).toHaveLength(1)
    expect(markup.indexOf('/admin/multi-user-stats.js')).toBeLessThan(markup.indexOf('/admin/admin.js'))
    request.mockResolvedValue(result)
    await stats.query()
    const requestURL = new URL(request.mock.calls[0][0], 'https://local.test')
    expect(requestURL.pathname).toBe('/api/admin/multi-user-stats')
    expect(JSON.parse(requestURL.searchParams.get('usernames_json') ?? '[]')).toEqual(['alice', 'bob'])
    expect(requestURL.searchParams.has('usernames')).toBe(false)
    expect(requestURL.searchParams.get('start_time')).toBe('2026-10-01')
    expect(document.querySelectorAll('.multi-user-card')).toHaveLength(2)
    expect(document.querySelector('.multi-user-card')?.textContent).toContain('12.34 s')
    const bars = [...document.querySelectorAll('.multi-user-bar-row svg rect:last-child')]
    expect(bars.map((bar) => bar.getAttribute('width'))).toEqual(['100.0000', '50.0000'])
    const segments = [...document.querySelectorAll('.multi-user-donut circle')]
    expect(segments).toHaveLength(2)
    expect(Number(segments[0].getAttribute('stroke-dasharray')?.split(' ')[0])).toBeCloseTo(66.666666, 4)
    expect(document.querySelectorAll('.multi-user-table tbody tr')).toHaveLength(2)
  })

  it('exports the actual queried snapshot and both user and model metrics', async () => {
    request.mockResolvedValue(result)
    const stats = (window as unknown as RuntimeWindow).ProxyLanMultiUserStats
    await stats.query()
    const form = document.querySelector('[data-multi-user-form]') as HTMLFormElement
    ;(form.elements.namedItem('start_time') as HTMLInputElement).value = '2025-01-01'
    stats.exportCSV()
    expect(filename).toBe('multi_user_stats_2026-10-01_2026-10-02.csv')
    expect(exportedBlob).not.toBeNull()
    const contents = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.addEventListener('load', () => resolve(String(reader.result)))
      reader.addEventListener('error', reject)
      reader.readAsText(exportedBlob as Blob)
    })
    expect(contents).toContain('"alice","10","1","500000","1","200","50","12340"')
    expect(contents).toContain('"bob","model-b","5"')
    expect(contents).not.toContain('2025-01-01')
  })

  it('shows unknown and unused accounts without inventing zero-valued cards', async () => {
    request.mockResolvedValue({ items: [], model_usage: [], unknown_usernames: ['missing'], no_usage_usernames: ['quiet'] })
    await (window as unknown as RuntimeWindow).ProxyLanMultiUserStats.query()
    expect(document.querySelector('[data-multi-user-message]')?.textContent).toContain('未找到账户：missing')
    expect(document.querySelector('[data-multi-user-message]')?.textContent).toContain('没有调用记录：quiet')
    expect(document.querySelectorAll('.multi-user-card')).toHaveLength(0)
    expect((document.querySelector('[data-multi-user-export]') as HTMLButtonElement).disabled).toBe(true)
  })

  it('validates dates and clears stale results after a real server failure', async () => {
    const stats = (window as unknown as RuntimeWindow).ProxyLanMultiUserStats
    request.mockResolvedValueOnce(result).mockRejectedValueOnce(new Error('统计数据库暂时不可用'))
    await stats.query()
    await stats.query()
    expect(document.querySelectorAll('.multi-user-card')).toHaveLength(0)
    expect(document.querySelector('[data-multi-user-message]')?.textContent).toContain('统计数据库暂时不可用')
    expect((document.querySelector('[data-multi-user-export]') as HTMLButtonElement).disabled).toBe(true)
    const form = document.querySelector('[data-multi-user-form]') as HTMLFormElement
    ;(form.elements.namedItem('end_time') as HTMLInputElement).value = '2026-09-01'
    await stats.query()
    expect(request).toHaveBeenCalledTimes(2)
    expect(document.querySelector('[data-multi-user-message]')?.textContent).toContain('结束日期不能早于开始日期')
  })

  it('preserves comma-containing account names and escaped quotes in the array query', async () => {
    const form = document.querySelector('[data-multi-user-form]') as HTMLFormElement
    ;(form.elements.namedItem('usernames') as HTMLInputElement).value = '"alice,bob", "张，三", "name""quoted", carol'
    request.mockResolvedValue({ ...result, items: [{ ...result.items[0], username: 'alice,bob' }, { ...result.items[1], username: '张，三' }] })
    await (window as unknown as RuntimeWindow).ProxyLanMultiUserStats.query()
    const requestURL = new URL(request.mock.calls[0][0], 'https://local.test')
    expect(JSON.parse(requestURL.searchParams.get('usernames_json') ?? '[]')).toEqual(['alice,bob', '张，三', 'name"quoted', 'carol'])
    expect(document.querySelectorAll('.multi-user-card')).toHaveLength(2)
    expect(document.querySelector('.multi-user-card')?.textContent).toContain('alice,bob')
    expect(document.querySelector('[data-multi-user-message]')?.textContent).not.toContain('未找到账户')
  })

  it.each(['date', 'quotes'])('rejects an older response after a newer invalid %s submission', async (invalid) => {
    let finishEarlier: (value: typeof result) => void = () => {}
    request.mockImplementationOnce(() => new Promise<typeof result>((resolve) => { finishEarlier = resolve }))
    const stats = (window as unknown as RuntimeWindow).ProxyLanMultiUserStats
    const first = stats.query()
    expect(request).toHaveBeenCalledOnce()
    const form = document.querySelector('[data-multi-user-form]') as HTMLFormElement
    if (invalid === 'date') (form.elements.namedItem('end_time') as HTMLInputElement).value = '2026-09-01'
    else (form.elements.namedItem('usernames') as HTMLInputElement).value = '"unfinished'
    await stats.query()
    const error = document.querySelector('[data-multi-user-message]')?.textContent
    expect(error).toContain(invalid === 'date' ? '结束日期不能早于开始日期' : '双引号尚未闭合')
    expect(document.querySelector('[data-panel="multi-user-stats"]')?.hasAttribute('aria-busy')).toBe(false)
    finishEarlier(result)
    await first
    expect(request).toHaveBeenCalledOnce()
    expect(document.querySelector('[data-multi-user-message]')?.textContent).toBe(error)
    expect(document.querySelectorAll('.multi-user-card')).toHaveLength(0)
    expect((document.querySelector('[data-multi-user-export]') as HTMLButtonElement).disabled).toBe(true)
    stats.exportCSV()
    expect(createObjectURL).not.toHaveBeenCalled()
  })
})
