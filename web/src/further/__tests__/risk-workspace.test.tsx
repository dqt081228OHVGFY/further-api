import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RiskScreen } from '../feature-workspace/risk'

const calls = vi.hoisted(() => ({ read: vi.fn(), write: vi.fn(), toast: vi.fn(), role: 100 }))
vi.mock('../feature-workspace/api', async (original) => ({
  ...(await original<typeof import('../feature-workspace/api')>()),
  read: calls.read,
  write: calls.write,
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (select: (state: unknown) => unknown) => select({ auth: { user: { role: calls.role } } }),
}))
vi.mock('sonner', () => ({ toast: { success: calls.toast, error: calls.toast } }))

const account = { user_id: 1, username: 'demo', score: 60, level: 'high', status: 'unreviewed', request_count: 28, ip_count: 2, last_request_at: 1700000000, signals: { repeated_input: 40 } }
const list = { items: [account], total: 1 }
const policy = { options: { CheckSensitiveEnabled: 'true', CheckSensitiveOnPromptEnabled: 'false', SensitiveWords: '旧词条' } }
function deferred() {
  let resolve!: (value: unknown) => void
  const promise = new Promise((finish) => { resolve = finish })
  return { promise, resolve }
}

beforeEach(() => {
  vi.resetAllMocks()
  calls.role = 100
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: () => ({ matches: false }) })
  calls.read.mockImplementation(async (path: string) => path === '/api/option/request_policy' ? policy : list)
  calls.write.mockResolvedValue({})
})

describe('further-api risk workspace', () => {
  it('names every tab panel and exposes all review filters and options', async () => {
    render(<RiskScreen />)
    const overview = await screen.findByRole('tabpanel', { name: '风险总览' })
    await within(overview).findByRole('heading', { name: 'demo' })
    expect(within(overview).getByRole('combobox', { name: '复核状态' })).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: '反测活复核' }))
    const review = screen.getByRole('tabpanel', { name: '反测活复核' })
    expect(within(review).getAllByRole('combobox')).toHaveLength(3)
    expect(within(review).getByRole('combobox', { name: '风险等级' })).toHaveTextContent('全部等级正常需要关注高风险严重风险')
    expect(within(review).getByRole('combobox', { name: '采样窗口' })).toHaveTextContent('最近 7 天最近 3 天')
    expect(within(review).getByRole('button', { name: '上一页' })).toBeVisible()
    expect(within(review).getByText(/复核标记会保存到服务器/)).toBeVisible()
    fireEvent.keyDown(screen.getByRole('tab', { name: '反测活复核' }), { key: 'End' })
    const policyPanel = await screen.findByRole('tabpanel', { name: '检测策略' })
    expect(await within(policyPanel).findByRole('button', { name: '保存检测策略' })).toBeVisible()
    expect(screen.queryByRole('tabpanel', { name: '反测活复核' })).toBeNull()
  })

  it('keeps pagination, sampling and review-status filters on their real API contracts', async () => {
    calls.read.mockResolvedValue({ items: [account], total: 25 })
    render(<RiskScreen />)
    await screen.findByRole('heading', { name: 'demo' })
    fireEvent.click(screen.getByRole('tab', { name: '反测活复核' }))
    await waitFor(() => expect(calls.read).toHaveBeenLastCalledWith('/api/risk/users', expect.objectContaining({ page: 1, page_size: 12, level: '' })))
    fireEvent.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(calls.read).toHaveBeenLastCalledWith('/api/risk/users', expect.objectContaining({ page: 2, page_size: 12 })))
    fireEvent.change(screen.getByRole('combobox', { name: '复核状态' }), { target: { value: 'confirmed' } })
    await waitFor(() => expect(calls.read).toHaveBeenLastCalledWith('/api/risk/users', expect.objectContaining({ page: 1, status: 'confirmed' })))
    fireEvent.change(screen.getByRole('combobox', { name: '采样窗口' }), { target: { value: '3d' } })
    await waitFor(() => expect(calls.read).toHaveBeenLastCalledWith('/api/risk/users', expect.objectContaining({ window: '3d', page: 1, status: 'confirmed' })))
    const search = screen.getByRole('textbox', { name: '搜索风险账户' })
    fireEvent.change(search, { target: { value: ' demo ' } })
    fireEvent.submit(search.closest('form')!)
    await waitFor(() => expect(calls.read).toHaveBeenLastCalledWith('/api/risk/users', expect.objectContaining({ username: 'demo', window: '3d', status: 'confirmed', page_size: 12 })))
  })

  it('retains the complete policy controls and saves only changed fields', async () => {
    render(<RiskScreen />)
    await screen.findByRole('heading', { name: 'demo' })
    fireEvent.click(screen.getByRole('tab', { name: '检测策略' }))
    const words = await screen.findByLabelText(/拦截词条/)
    await waitFor(() => expect(words).toHaveValue('旧词条'))
    fireEvent.click(screen.getByRole('checkbox', { name: /检测请求提示词/ }))
    fireEvent.change(words, { target: { value: '第一条\n第二条' } })
    fireEvent.click(screen.getByRole('button', { name: '保存检测策略' }))
    await waitFor(() => expect(calls.write).toHaveBeenCalledExactlyOnceWith('/api/option/request_policy', 'PATCH', { options: { CheckSensitiveOnPromptEnabled: 'true', SensitiveWords: '第一条\n第二条' } }))
    expect(await screen.findByText('检测策略已保存。')).toBeVisible()
    expect(screen.getByRole('button', { name: '保存检测策略' })).toBeDisabled()
  })

  it('keeps administrator policy permissions while leaving review views available', async () => {
    calls.role = 10
    render(<RiskScreen />)
    await screen.findByRole('heading', { name: 'demo' })
    fireEvent.click(screen.getByRole('tab', { name: '检测策略' }))
    expect(await screen.findByRole('heading', { name: '检测策略由超级管理员维护' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '保存检测策略' })).toBeNull()
    expect(calls.read.mock.calls.some(([path]) => path === '/api/option/request_policy')).toBe(false)
    fireEvent.click(screen.getByRole('tab', { name: '反测活复核' }))
    expect(screen.getByRole('tabpanel', { name: '反测活复核' })).toBeVisible()
  })

  it('lets the running native surface own first entry and resumes local entry for later views', async () => {
    const { container } = render(<div data-native-feature-root='' data-native-motion-engine='web' data-native-motion-running='true'><RiskScreen /></div>)
    await screen.findByRole('heading', { name: 'demo' })
    await waitFor(() => expect(container.querySelector('.frisk-workspace')).toHaveAttribute('data-risk-entry', 'initial'))
    container.firstElementChild!.removeAttribute('data-native-motion-running')
    fireEvent.click(screen.getByRole('tab', { name: '反测活复核' }))
    expect(container.querySelector('.frisk-workspace')).toHaveAttribute('data-risk-entry', 'local')
  })

  it('retains the existing accounts while an explicit refresh is in flight', async () => {
    render(<RiskScreen />)
    await screen.findByRole('heading', { name: 'demo' })
    const update = deferred()
    calls.read.mockReturnValueOnce(update.promise)
    fireEvent.click(screen.getByRole('button', { name: '刷新记录' }))
    expect(screen.getByRole('heading', { name: 'demo' })).toBeVisible()
    expect(screen.getByLabelText('正在更新风险记录')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '查看 demo 的证据' })).toBeDisabled()
    update.resolve({ items: [{ ...account, username: '最新账户' }], total: 1 })
    expect(await screen.findByRole('heading', { name: '最新账户' })).toBeVisible()
    expect(screen.queryByLabelText('正在更新风险记录')).toBeNull()
  })

  it('keeps an unsaved policy draft when moving between views without another request', async () => {
    render(<RiskScreen />)
    await screen.findByRole('heading', { name: 'demo' })
    fireEvent.click(screen.getByRole('tab', { name: '检测策略' }))
    const words = await screen.findByLabelText(/拦截词条/)
    await waitFor(() => expect(words).toHaveValue('旧词条'))
    fireEvent.change(words, { target: { value: '未保存词条' } })
    calls.read.mockClear()
    fireEvent.click(screen.getByRole('tab', { name: '风险总览' }))
    expect(screen.getByRole('heading', { name: 'demo' })).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: '检测策略' }))
    expect(screen.getByLabelText(/拦截词条/)).toHaveValue('未保存词条')
    expect(screen.getByRole('button', { name: '读取最新' })).toBeDisabled()
    expect(calls.read).not.toHaveBeenCalled()
  })

  it('requires a successful policy read before allowing edits or writes', async () => {
    calls.read.mockImplementation(async (path: string) => {
      if (path === '/api/option/request_policy') throw new Error('检测规则暂时不可用')
      return list
    })
    render(<RiskScreen />)
    await screen.findByRole('heading', { name: 'demo' })
    fireEvent.click(screen.getByRole('tab', { name: '检测策略' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('检测规则暂时不可用')
    expect(screen.queryByLabelText(/拦截词条/)).toBeNull()
    expect(screen.queryByRole('button', { name: '保存检测策略' })).toBeNull()
    expect(screen.getByRole('button', { name: '重新读取策略' })).toBeEnabled()
    expect(calls.write).not.toHaveBeenCalled()
  })

  it('discards late detail results after closing and opening another account', async () => {
    const oldDetail = deferred()
    const other = { ...account, user_id: 2, username: 'second', score: 30 }
    calls.read.mockImplementation(async (path: string) => {
      if (path === '/api/risk/users/1') return oldDetail.promise
      if (path === '/api/risk/users/2') return { ...other, evidence: { requests: 22 } }
      return { items: [account, other], total: 2 }
    })
    render(<RiskScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '查看 demo 的证据' }))
    const first = await screen.findByRole('dialog', { name: '行为证据 · demo' })
    fireEvent.click(within(first).getByRole('button', { name: '关闭' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    fireEvent.click(screen.getByRole('button', { name: '查看 second 的证据' }))
    const second = await screen.findByRole('dialog', { name: '行为证据 · second' })
    await waitFor(() => expect(within(second).getByText('22')).toBeVisible())
    oldDetail.resolve({ ...account, evidence: { requests: 999 } })
    await Promise.resolve()
    expect(within(second).queryByText('999')).toBeNull()
    expect(within(second).getByText('22')).toBeVisible()
  })

  it('finishes the review exit animation without waiting for the background list refresh', async () => {
    render(<RiskScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '复核 demo' }))
    calls.read.mockReturnValue(new Promise(() => {}))
    fireEvent.click(screen.getByRole('button', { name: '保存复核结果' }))
    expect(await screen.findByText('复核结果已保存。')).toBeVisible()
    await waitFor(() => expect(screen.getByRole('dialog')).toHaveClass('rfw-dialog-closing'))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(calls.write).toHaveBeenCalledExactlyOnceWith('/api/risk/users/1/status', 'PATCH', { status: 'watching' })
    expect(calls.toast).toHaveBeenCalledWith('「demo」已标记为持续观察')
  })
})
