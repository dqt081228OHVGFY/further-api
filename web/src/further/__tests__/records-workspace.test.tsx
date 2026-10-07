import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { RecordsScreen } from '../feature-workspace/records'
import { FeatureWorkspace } from '../feature-workspace'
import { useAuthStore } from '@/stores/auth-store'

const api = vi.hoisted(() => ({ read: vi.fn(), write: vi.fn() }))
vi.mock('../feature-workspace/api', async (original) => ({
  ...(await original<typeof import('../feature-workspace/api')>()),
  ...api,
}))
const vendor = { id: 7, name: 'Model Vendor', description: '用于测试', model_count: 3, icon: 'vendor' }

beforeEach(() => {
  api.read.mockReset().mockResolvedValue({ items: [vendor], total: 1 })
  api.write.mockReset().mockResolvedValue({})
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value (this: HTMLDialogElement) {
      this.setAttribute('open', '')
    },
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value (this: HTMLDialogElement) {
      this.removeAttribute('open')
    },
  })
})

describe('further-api administration records', () => {
  it('opens the dedicated identity service editor and fills discovered endpoints from its workspace route', async () => {
    const previous = useAuthStore.getState()
    useAuthStore.setState({ auth: { ...previous.auth, user: { id: 1, username: 'qa', role: 100 } } })
    api.read.mockResolvedValue([])
    api.write.mockResolvedValue({ discovery: {
      authorization_endpoint: 'https://identity.example/authorize',
      token_endpoint: 'https://identity.example/token',
      userinfo_endpoint: 'https://identity.example/user',
    } })
    const workspace = render(<FeatureWorkspace path='/system-settings/auth/custom-oauth' />)
    try {
      expect(await screen.findByRole('heading', { name: '自定义身份服务' })).toBeVisible()
      fireEvent.click(screen.getByRole('button', { name: '添加登录服务' }))
      const dialog = await screen.findByRole('dialog', { name: '添加登录服务' })
      fireEvent.change(within(dialog).getByLabelText('身份服务发现地址（可选）'), {
        target: { value: 'https://identity.example/.well-known/openid-configuration' },
      })
      fireEvent.click(within(dialog).getByRole('button', { name: '自动填写地址' }))
      await waitFor(() => expect(within(dialog).getByLabelText('授权地址')).toHaveValue('https://identity.example/authorize'))
      expect(within(dialog).getByLabelText('令牌交换地址')).toHaveValue('https://identity.example/token')
      expect(within(dialog).getByLabelText('账户信息地址')).toHaveValue('https://identity.example/user')
      expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/custom-oauth-provider/discovery', 'POST', {
        well_known_url: 'https://identity.example/.well-known/openid-configuration',
      })
    } finally {
      workspace.unmount()
      useAuthStore.setState(previous)
    }
  })
  it('updates the unique vendor editor and retains the association count in the list', async () => {
    render(<RecordsScreen pathname='/models/vendors' />)
    expect(await screen.findByRole('heading', { name: 'Model Vendor', level: 2 })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '编辑Model Vendor' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('厂商名称'), { target: { value: 'Updated Vendor' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保存' }))
    await waitFor(() => expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/vendors/', 'PUT', expect.objectContaining({ id: 7, name: 'Updated Vendor' })))
    expect(await screen.findByText('已保存')).toBeVisible()
    expect(screen.getByText('关联模型')).toBeVisible()
  })

  it('deletes a vendor only after the visible confirmation', async () => {
    render(<RecordsScreen pathname='/models/vendors' />)
    fireEvent.click(await screen.findByRole('button', { name: '删除Model Vendor' }))
    expect(api.write).not.toHaveBeenCalled()
    const dialog = screen.getByRole('dialog', { name: '删除配置' })
    fireEvent.click(within(dialog).getByRole('button', { name: '确认删除' }))
    await waitFor(() => expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/vendors/7', 'DELETE'))
  })

  it('paginates a complete vendor list without repeating the first page', async () => {
    api.read.mockResolvedValue(Array.from({ length: 26 }, (_, index) => ({ ...vendor, id: index + 1, name: `厂商 ${index + 1}` })))
    render(<RecordsScreen pathname='/models/vendors' />)
    expect(await screen.findByRole('heading', { name: '厂商 1', level: 2 })).toBeVisible()
    expect(screen.queryByRole('heading', { name: '厂商 26', level: 2 })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '下一页' }))
    expect(await screen.findByRole('heading', { name: '厂商 26', level: 2 })).toBeVisible()
    expect(screen.queryByRole('heading', { name: '厂商 1', level: 2 })).toBeNull()
  })
})
