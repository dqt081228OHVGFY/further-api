import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../relayfor-ui/assets/admin/admin-token-stats.js'), 'utf8')
const request = vi.fn()
const writeText = vi.fn()
const toast = vi.fn()
type StatsWindow = Window & { ProxyLan: unknown; ProxyLanAdminTokenStats: { bind(): void; load(): Promise<void> } }

beforeEach(() => {
  document.body.innerHTML = '<section data-panel="users"><div class="card-head">用户</div><div class="md-card"><form data-admin-user-filter></form></div></section>'
  request.mockReset()
  writeText.mockReset().mockResolvedValue(undefined)
  toast.mockReset()
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  ;(window as unknown as StatsWindow).ProxyLan = {
    request, toast, showBusy() {}, number(value: number) { return String(value || 0) },
    moneyFromQuota(value: number) { return '$' + value }, date(value: number) { return String(value) },
    escapeHTML(value: unknown) { return String(value).replaceAll(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]!) },
  }
  window.eval(runtime)
})

const payload = {
  page: 1, page_size: 25, total: 2, total_pages: 1,
  summary: { active_users: 2, requests: 15, errors: 1, prompt_tokens: 300, completion_tokens: 70, quota: 45 },
  items: [
    { user_id: 1, username: 'alice', token_id: 7, token_name: 'production', requests: 10, errors: 1, prompt_tokens: 200, completion_tokens: 50, quota: 30, last_used_at: 1000 },
    { user_id: 2, username: 'bob', token_id: 9, token_name: 'staging', requests: 5, errors: 0, prompt_tokens: 100, completion_tokens: 20, quota: 15, last_used_at: 1001 },
  ],
}

describe('multi-user token statistics', () => {
  it('loads real aggregates, mounts once, and copies only the selected token statistics', async () => {
    request.mockResolvedValue(payload)
    const stats = (window as unknown as StatsWindow).ProxyLanAdminTokenStats
    stats.bind(); stats.bind()
    await stats.load()
    expect(document.querySelectorAll('[data-token-usage-card]')).toHaveLength(1)
    expect(request).toHaveBeenCalledWith('/api/admin/token-usage?page=1&page_size=25&days=7')
    expect(document.querySelector('[data-token-usage-summary]')!.textContent).toContain('370')
    const choice = document.querySelector('[data-token-usage-select="1"]') as HTMLInputElement
    choice.click()
    expect(document.querySelector('[data-token-usage-copy-label]')!.textContent).toContain('1 个令牌')
    ;(document.querySelector('[data-token-usage-copy]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    const report = writeText.mock.calls[0][0] as string
    expect(report).toContain('bob\t9\tstaging')
    expect(report).not.toContain('alice')
    expect(report).toContain('最近 7 天')
  })

  it('copies both users when all rows are selected and provides a manual copy fallback', async () => {
    request.mockResolvedValue(payload)
    writeText.mockRejectedValue(new Error('clipboard blocked'))
    const stats = (window as unknown as StatsWindow).ProxyLanAdminTokenStats
    await stats.load()
    ;(document.querySelector('[data-token-usage-select-all]') as HTMLInputElement).click()
    ;(document.querySelector('[data-token-usage-copy]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(document.querySelector('[data-token-usage-copy-output]')!.classList.contains('hidden')).toBe(false))
    const report = (document.querySelector('[data-token-usage-copy-output]') as HTMLTextAreaElement).value
    expect(report).toContain('alice\t7\tproduction')
    expect(report).toContain('bob\t9\tstaging')
  })

  it('clears stale statistics after a failure and allows a real retry', async () => {
    request.mockResolvedValueOnce(payload).mockRejectedValueOnce(new Error('统计服务暂时不可用')).mockResolvedValueOnce(payload)
    const stats = (window as unknown as StatsWindow).ProxyLanAdminTokenStats
    await stats.load()
    await stats.load()
    expect(document.querySelector('[data-token-usage-rows]')!.textContent).toContain('统计服务暂时不可用')
    expect((document.querySelector('[data-token-usage-copy]') as HTMLButtonElement).disabled).toBe(true)
    expect(document.querySelector('[data-token-usage-rows]')!.textContent).not.toContain('alice')
    ;(document.querySelector('[data-token-usage-retry]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(document.querySelector('[data-token-usage-rows]')!.textContent).toContain('alice'))
  })

  it('neutralizes spreadsheet formulas and cell separators in copied names', async () => {
    request.mockResolvedValue({ ...payload, items: [{ ...payload.items[0], username: '=SUM(1,2)', token_name: '@malicious\tname\nnext' }] })
    await (window as unknown as StatsWindow).ProxyLanAdminTokenStats.load()
    ;(document.querySelector('[data-token-usage-copy]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    expect(writeText.mock.calls[0][0]).toContain("'=SUM(1,2)")
    expect(writeText.mock.calls[0][0]).toContain("'@malicious name next")
  })
})
