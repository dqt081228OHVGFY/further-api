import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, describe, expect, it, vi } from 'vitest'

const furtherRoot = resolve(import.meta.dirname, '../../../../further-ui')
const html = readFileSync(resolve(furtherRoot, 'console/index.html'), 'utf8')
const script = readFileSync(resolve(furtherRoot, 'assets/console.js'), 'utf8')

type LogState = {
  page: number
  pageSize: number
  keyword: string
  statusCode: string
  exclude200: boolean
  startTime: number
  endTime: number
  sortBy: string
  sortOrder: string
  totalPages: number
}

const record = (model: string, id = 1) => ({
  id, model, token_name: '应用 Key', request_id: `request-${id}`,
  created_at: 1791043200, prompt_tokens: 100, completion_tokens: 50,
  cached_tokens: 20, cache_write_tokens: 10, quota: 250000,
  duration_ms: 430, status_code: 200,
})

function logPage(model = 'model-one', total = 30) {
  return {
    items: [record(model)], total, total_pages: Math.ceil(total / 25),
    summary: { requests: total, quota: 500000, prompt_tokens: 200, completion_tokens: 100, cached_tokens: 40, cache_write_tokens: 20 },
  }
}

function mountLogs(preferences = { pageSize: 25, hideSuccessfulLogs: false }) {
  document.body.innerHTML = new DOMParser().parseFromString(html, 'text/html').body.innerHTML
  document.body.dataset.page = 'console'
  const api = {
    preferences: () => preferences,
    request: vi.fn<(path: string) => Promise<unknown>>().mockResolvedValue(logPage()),
    escapeHTML: (value: unknown) => String(value ?? '').replaceAll(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]!),
    date: (value: unknown) => String(value ?? ''),
    number: (value: unknown) => String(value ?? 0),
    quotaPerUSD: () => 500000,
    setText: (selector: string, text: string) => { document.querySelectorAll(selector).forEach(node => { node.textContent = text }) },
    showBusy: vi.fn((button: HTMLButtonElement, busy: boolean) => { button.disabled = busy }),
    toast: vi.fn(), openDialog: vi.fn(),
  }
  const gateway = { exportLogs: vi.fn() }
  // Execute the production controller and real page fields, without starting
  // unrelated account, activity or model requests.
  const controller = new Function('window', 'document', `return ${script.replace(
    /\}\(\)\);\s*$/,
    'return { loadLogs, bindLogFilters, bindAccountActions, logState, userLogPeriodRange, localDateTimeTimestamp, localDateTimeInputValue }; }());'
  )}`)({
    FurtherUI: api, FurtherUIConsoleModels: { load: vi.fn() },
    FurtherUIResources: { create: vi.fn() }, FurtherGateway: gateway,
    setTimeout: window.setTimeout.bind(window),
  }, document) as {
    loadLogs: () => Promise<void>
    bindLogFilters: () => void
    bindAccountActions: () => void
    logState: LogState
    userLogPeriodRange: (period: string, now: Date) => { start: Date; end: Date }
    localDateTimeTimestamp: (value: string) => number
    localDateTimeInputValue: (value: Date) => string
  }
  const form = document.querySelector<HTMLFormElement>('[data-user-log-filter]')!
  const field = (name: string) => form.elements.namedItem(name) as HTMLInputElement
  const submit = () => form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  const params = () => new URL(api.request.mock.calls.at(-1)![0], 'http://further.local').searchParams
  return { api, gateway, controller, form, field, submit, params }
}

afterEach(() => {
  document.body.replaceChildren()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('account calling-record workspace', () => {
  it('marks a failed SSE response as an error while preserving its real HTTP 200 status', async () => {
    const { api, controller } = mountLogs()
    api.request.mockResolvedValue({ ...logPage(), items: [{ ...record('sse-model'), failed: true, error_code: 'stream_failed' }] })
    await controller.loadLogs()
    const status = document.querySelector('[data-log-rows] .usage-status')
    expect(status).toHaveClass('error')
    expect(status).toHaveTextContent('200')
    expect(document.querySelector('[data-log-rows]')?.textContent).toContain('stream_failed')
    document.querySelector<HTMLButtonElement>('[data-user-log-detail-id]')!.click()
    expect(document.querySelector('.user-logs-record-context')?.textContent).toContain('HTTP 200')
  })
  it('labels a historical error without a saved HTTP status as failed and leaves its detail status unknown', async () => {
    const { api, controller } = mountLogs()
    api.request.mockResolvedValue({ ...logPage(), items: [{ ...record('legacy-model'), failed: true, status_code: 0 }] })
    await controller.loadLogs()
    const status = document.querySelector('[data-log-rows] .usage-status')
    expect(status).toHaveClass('error')
    expect(status).toHaveTextContent('失败')
    expect(status).not.toHaveTextContent('200')
    document.querySelector<HTMLButtonElement>('[data-user-log-detail-id]')!.click()
    const context = document.querySelector('.user-logs-record-context')?.textContent
    expect(context).toContain('HTTP 未记录')
    expect(context).not.toContain('HTTP 200')
  })
  it('keeps query fields in the full-width account workspace and labels every field', () => {
    const { form } = mountLogs()
    expect(form.closest('.user-logs-workspace')).not.toBeNull()
    expect(form.closest('.console-records-filter')).toBeNull()
    expect(form.querySelectorAll('fieldset')).toHaveLength(2)
    for (const name of ['keyword', 'status_code', 'start_time', 'end_time', 'page_size']) {
      expect((form.elements.namedItem(name) as HTMLInputElement).labels?.length).toBeGreaterThan(0)
    }
    expect(document.querySelector('[data-user-log-periods]')?.textContent).toContain('快捷查看消费')
  })

  it('queries the current account with all submitted filters in UTC+8', async () => {
    const { api, controller, field, submit, params } = mountLogs()
    controller.bindLogFilters()
    field('keyword').value = ' model-two '
    field('status_code').value = '429'
    field('start_time').value = '2026-10-04T00:15:00'
    field('end_time').value = '2026-10-04T23:59:59'
    field('page_size').value = '50'
    submit()
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(1))
    expect(api.request.mock.calls[0][0]).toMatch(/^\/api\/user\/logs\?/)
    expect(Object.fromEntries(params())).toEqual({
      page: '1', page_size: '50', keyword: 'model-two', status_code: '429',
      start_timestamp: String(Date.parse('2026-10-04T00:15:00+08:00') / 1000),
      end_timestamp: String(Date.parse('2026-10-04T23:59:59+08:00') / 1000),
      sort_by: 'created_at', sort_order: 'desc',
    })
    await vi.waitFor(() => expect(document.querySelector('[data-user-log-applied-filter]')?.textContent).toContain('UTC+8'))
  })

  it('makes the status code and errors-only filters mutually exclusive', async () => {
    const { api, controller, field, submit, params } = mountLogs()
    controller.bindLogFilters()
    field('status_code').value = '200'
    field('exclude_200').checked = true
    field('exclude_200').dispatchEvent(new Event('change', { bubbles: true }))
    expect(field('status_code')).toBeDisabled()
    expect(field('status_code')).toHaveValue(null)
    submit()
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(1))
    expect(params().get('only_errors')).toBe('true')
    expect(params().has('status_code')).toBe(false)
  })

  it.each([
    ['today', '2026-10-05T00:00:00+08:00', '2026-10-05T23:59:59+08:00'],
    ['week', '2026-10-05T00:00:00+08:00', '2026-10-11T23:59:59+08:00'],
    ['month', '2026-10-01T00:00:00+08:00', '2026-10-31T23:59:59+08:00'],
  ])('queries the Beijing %s even while the UTC calendar is still Sunday', async (period, start, end) => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-10-04T17:30:00Z'))
    const { api, controller, params } = mountLogs()
    controller.bindLogFilters()
    const button = document.querySelector<HTMLButtonElement>(`[data-log-period="${period}"]`)!
    button.click()
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(1))
    expect(params().get('start_timestamp')).toBe(String(Date.parse(start) / 1000))
    expect(params().get('end_timestamp')).toBe(String(Date.parse(end) / 1000))
    expect(button).toHaveAttribute('aria-pressed', 'true')
  })

  it('rejects reversed time ranges without changing the active query or requesting data', () => {
    const { api, controller, field, submit } = mountLogs()
    controller.bindLogFilters()
    controller.logState.keyword = 'previous-query'
    field('keyword').value = 'new-query'
    field('start_time').value = '2026-10-05T00:00'
    field('end_time').value = '2026-10-04T23:59'
    submit()
    expect(api.request).not.toHaveBeenCalled()
    expect(controller.logState.keyword).toBe('previous-query')
    expect(field('end_time')).toHaveAttribute('aria-invalid', 'true')
    expect(document.querySelector('[data-user-log-filter-feedback]')).toHaveTextContent('开始时间不能晚于结束时间')
    expect(api.toast).toHaveBeenCalledWith('开始时间不能晚于结束时间。', 'danger')
  })

  it('preserves the submitted query through pagination and column sorting', async () => {
    const { api, controller, params } = mountLogs()
    controller.bindLogFilters()
    controller.logState.keyword = 'preserved-model'
    controller.logState.exclude200 = true
    await controller.loadLogs()
    document.querySelector<HTMLButtonElement>('[data-user-log-next]')!.click()
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(2))
    expect(params().get('page')).toBe('2')
    expect(params().get('keyword')).toBe('preserved-model')
    document.querySelector<HTMLButtonElement>('[data-user-log-sort="quota"]')!.click()
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(3))
    expect(params().get('page')).toBe('1')
    expect(params().get('sort_by')).toBe('quota')
    expect(params().get('sort_order')).toBe('desc')
    expect(params().get('only_errors')).toBe('true')
    expect(document.querySelector('[data-user-log-sort="quota"]')?.closest('th')).toHaveAttribute('aria-sort', 'descending')
  })

  it('shares duplicate pending queries and keeps a late older response from replacing the current results', async () => {
    const { api, controller } = mountLogs()
    let finishOld!: (value: unknown) => void
    api.request.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
    const older = controller.loadLogs()
    expect(controller.loadLogs()).toBe(older)
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(1))
    controller.logState.keyword = 'latest-model'
    api.request.mockResolvedValueOnce(logPage('latest-model'))
    await controller.loadLogs()
    finishOld(logPage('older-model'))
    await older
    expect(document.querySelector('[data-log-rows]')).toHaveTextContent('latest-model')
    expect(document.querySelector('[data-log-rows]')).not.toHaveTextContent('older-model')
    expect(document.querySelector('.user-logs-results')).toHaveAttribute('data-loading', 'false')
  })

  it('reports a failed query, clears its busy state and allows the same query to retry', async () => {
    const { api, controller, form, submit } = mountLogs()
    controller.bindLogFilters()
    api.request.mockRejectedValueOnce(new Error('服务暂时不可用'))
    submit()
    await vi.waitFor(() => expect(api.toast).toHaveBeenCalledWith('服务暂时不可用', 'danger'))
    expect(form.querySelector('[type="submit"]')).not.toBeDisabled()
    expect(document.querySelector('.user-logs-results')).toHaveAttribute('aria-busy', 'false')
    submit()
    await vi.waitFor(() => expect(document.querySelector('[data-log-rows]')).toHaveTextContent('model-one'))
    expect(api.request).toHaveBeenCalledTimes(2)
  })

  it('shows range totals and retains request, token and fee details with escaped IDs', async () => {
    const { api, controller } = mountLogs()
    const payload = logPage()
    payload.items[0].request_id = 'request-<script>unsafe</script>'
    api.request.mockResolvedValue(payload)
    await controller.loadLogs()
    expect(document.querySelector('[data-user-log-cost]')).toHaveTextContent('$1.00')
    expect(document.querySelector('[data-user-log-cache-write]')).toHaveTextContent('20')
    document.querySelector<HTMLButtonElement>('[data-user-log-detail-id]')!.click()
    expect(api.openDialog).toHaveBeenCalledWith('usage-cost-dialog')
    expect(document.querySelector('.user-logs-record-context')).toHaveTextContent('request-<script>unsafe</script>')
    expect(document.querySelector('.user-logs-record-context script')).toBeNull()
    expect(document.querySelector('[data-usage-cost-breakdown]')).toHaveTextContent('Token 用量')
  })

  it('restores saved page-size and successful-log preferences on reset', async () => {
    const { api, controller, field, params, form } = mountLogs({ pageSize: 50, hideSuccessfulLogs: true })
    controller.bindLogFilters()
    controller.logState.page = 3
    controller.logState.keyword = 'old-model'
    field('keyword').value = 'draft-model'
    form.reset()
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(1))
    expect(params().get('page')).toBe('1')
    expect(params().get('page_size')).toBe('50')
    expect(params().get('only_errors')).toBe('true')
    expect(params().has('keyword')).toBe(false)
    expect(field('exclude_200')).toBeChecked()
    expect(field('status_code')).toBeDisabled()
  })

  it('exports the current account CSV from the new results toolbar', async () => {
    const { api, gateway, controller } = mountLogs()
    const createObjectURL = vi.fn(() => 'blob:further-api-test')
    const revokeObjectURL = vi.fn()
    const OriginalURL = URL
    vi.stubGlobal('URL', class extends OriginalURL { static createObjectURL = createObjectURL; static revokeObjectURL = revokeObjectURL })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    gateway.exportLogs.mockResolvedValue(new Response('created_at,model\n1,model-one', { headers: { 'Content-Type': 'text/csv' } }))
    controller.bindAccountActions()
    const exportButton = document.querySelector<HTMLButtonElement>('[data-user-log-export]')!
    exportButton.click()
    await vi.waitFor(() => expect(api.toast).toHaveBeenCalledWith('调用记录已导出'))
    expect(gateway.exportLogs).toHaveBeenCalledTimes(1)
    expect(createObjectURL).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:further-api-test')
    expect(exportButton).not.toBeDisabled()
  })
})
