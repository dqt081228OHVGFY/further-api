import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AnalyticsScreen, LogsScreen } from '../feature-workspace/analysis'

const api = vi.hoisted(() => ({ read: vi.fn() }))
vi.mock('../feature-workspace/api', async (original) => ({
  ...(await original<typeof import('../feature-workspace/api')>()),
  ...api,
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (select: (state: unknown) => unknown) =>
    select({ auth: { user: { role: 100 } } }),
}))
beforeEach(() => {
  vi.resetAllMocks()
  api.read.mockResolvedValue({ items: [], total: 0 })
  window.FurtherQuotaPerUnit = 500000
})
afterEach(() => vi.useRealTimers())

describe('further-api analysis contracts', () => {
  it.each([
    ['/usage-logs/common', '/api/log', '模型名称', 'model_name', 'model-one'],
    ['/usage-logs/drawing', '/api/mj', '绘图任务编号', 'mj_id', 'drawing-one'],
    ['/usage-logs/task', '/api/task', '任务编号', 'task_id', 'task-one'],
    ['/usage-logs/audit', '/api/audit', '用户名', 'username', 'member-one'],
  ])(
    'sends the actual backend filter for %s',
    async (path, endpoint, label, parameter, value) => {
      render(<LogsScreen pathname={path} />)
      const input = await screen.findByRole('textbox', { name: label })
      fireEvent.change(input, { target: { value: ` ${value} ` } })
      fireEvent.click(screen.getByRole('button', { name: '查询' }))
      await waitFor(() =>
        expect(api.read).toHaveBeenCalledWith(
          endpoint,
          expect.objectContaining({ [parameter]: value, p: 1, page_size: 25 })
        )
      )
    }
  )

  it('uses Beijing dates before 08:00 and groups user records by their username', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-10-03T17:00:00Z'))
    api.read.mockResolvedValue([
      { model_name: '', username: '用户甲', quota: 1000000, count: 2 },
      { model_name: '', username: '用户乙', quota: 500000, count: 1 },
    ])
    render(<AnalyticsScreen pathname='/dashboard/users' />)
    expect(await screen.findByText('用户甲')).toBeVisible()
    expect(screen.getByText('用户乙')).toBeVisible()
    expect(screen.getByLabelText('结束日期')).toHaveValue('2026-10-04')
    expect(api.read).toHaveBeenCalledWith('/api/data/users', {
      start_timestamp: Date.parse('2026-09-28T00:00:00+08:00') / 1000,
      end_timestamp: Date.parse('2026-10-04T23:59:59+08:00') / 1000,
    })
  })
})
