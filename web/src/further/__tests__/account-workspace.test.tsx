import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SecurityScreen } from '../feature-workspace/security'
import { Modal } from '../feature-workspace/ui'

const calls = vi.hoisted(() => ({
  read: vi.fn(),
  write: vi.fn(),
  methods: vi.fn(),
  verify: vi.fn(),
  token: vi.fn(),
  password: vi.fn(),
  toast: vi.fn(),
}))
vi.mock('../feature-workspace/api', async (original) => ({
  ...(await original<typeof import('../feature-workspace/api')>()),
  read: calls.read,
  write: calls.write,
}))
vi.mock('@/lib/api', () => ({
  api: {},
  clearAuthentication: vi.fn(),
  disable2FA: vi.fn(),
  regenerate2FABackupCodes: vi.fn(),
}))
vi.mock('@/features/auth/secure-verification/api', () => ({
  checkVerificationMethods: calls.methods,
  verify: calls.verify,
}))
vi.mock('@/features/auth/passkey/api', () => ({
  beginPasskeyRegistration: vi.fn(),
  deletePasskey: vi.fn(),
  finishPasskeyRegistration: vi.fn(),
}))
vi.mock('@/lib/passkey', () => ({
  buildRegistrationResult: vi.fn(),
  createCredential: vi.fn(),
  prepareCredentialCreationOptions: vi.fn(),
}))
vi.mock('@/features/profile/api', () => ({
  bindEmail: vi.fn(),
  changeAccountPassword: calls.password,
  deleteUserAccount: vi.fn(),
  resendEmailBinding: vi.fn(),
  startEmailBinding: vi.fn(),
  unbindCustomOAuth: vi.fn(),
}))
vi.mock('@/features/security/api', () => ({
  createAccessToken: calls.token,
  enable2FA: vi.fn(),
  revokeAccessToken: vi.fn(),
  setup2FA: vi.fn(),
}))
vi.mock('sonner', () => ({
  toast: { success: calls.toast, error: calls.toast },
}))
const snapshot: Record<string, unknown> = {
  '/api/user/self': { has_password: true, email: 'me@example.com' },
  '/api/user/2fa/status': { enabled: false },
  '/api/user/passkey': { enabled: false },
  '/api/user/token/status': { exists: false },
  '/api/user/oauth/bindings': [],
  '/api/user/sessions': [
    {
      sid: 'current',
      current: true,
      user_agent: 'Windows Chrome/123',
      ip: '127.0.0.1',
    },
    {
      sid: 'other/device',
      current: false,
      user_agent: 'iPhone Safari/123',
      ip: '127.0.0.2',
    },
  ],
}
beforeEach(() => {
  vi.resetAllMocks()
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', {
    configurable: true,
    value() {
      this.setAttribute('open', '')
    },
  })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', {
    configurable: true,
    value() {
      this.removeAttribute('open')
    },
  })
  calls.read.mockImplementation(async (path: string) => snapshot[path])
  calls.methods.mockResolvedValue({
    scope: 'access_token.generate',
    methods: [{ method: 'password', available: true }],
    oauth_providers: [],
    password_encryption_enabled: false,
  })
  calls.verify.mockResolvedValue({ proof_token: 'single-use-proof' })
  calls.token.mockResolvedValue('test-account-access-secret')
  calls.write.mockResolvedValue(undefined)
})
describe('further-api account security', () => {
  it('keeps available account actions usable when one status endpoint fails', async () => {
    calls.read.mockImplementation(async (path: string) => {
      if (path === '/api/user/passkey') throw new Error('设备信息暂时不可用')
      return snapshot[path]
    })
    render(<SecurityScreen />)
    expect(await screen.findByText('设备信息暂时不可用')).toBeVisible()
    expect(screen.getByRole('button', { name: '绑定设备' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '更新密码' })).toBeEnabled()
    expect(screen.getByRole('heading', { name: '登录设备' })).toBeVisible()
  })
  it('retains the generated secret and inline feedback during refresh', async () => {
    render(<SecurityScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '生成令牌' }))
    fireEvent.change(await screen.findByLabelText('当前账户密码'), {
      target: { value: 'password-value' },
    })
    calls.read.mockImplementation(() => new Promise(() => {}))
    fireEvent.click(screen.getByRole('button', { name: '验证并继续' }))
    const dialog = await screen.findByRole('dialog', {
      name: '保存账户访问令牌',
    })
    expect(within(dialog).getByText('test-account-access-secret')).toBeVisible()
    expect(within(dialog).getByRole('status')).toHaveTextContent(
      '生成账户访问令牌已完成'
    )
    expect(calls.token).toHaveBeenCalledWith(
      'single-use-proof',
      expect.any(AbortSignal)
    )
    expect(calls.verify).toHaveBeenCalledWith(
      { method: 'password', password: 'password-value' },
      { scope: 'access_token.generate' },
      false,
      expect.any(AbortSignal)
    )
    expect(calls.toast).toHaveBeenCalledWith('生成账户访问令牌已完成。')
  })
  it('rejects mismatched passwords before identity verification', async () => {
    render(<SecurityScreen />)
    fireEvent.change(await screen.findByLabelText('新密码'), {
      target: { value: 'password-one' },
    })
    fireEvent.change(screen.getByLabelText('再次输入'), {
      target: { value: 'password-two' },
    })
    fireEvent.click(screen.getByRole('button', { name: '更新密码' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      '两次输入的密码不一致'
    )
    expect(calls.methods).not.toHaveBeenCalled()
    expect(calls.password).not.toHaveBeenCalled()
  })
  it('confirms and URL-encodes a device session before revoking', async () => {
    render(<SecurityScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '退出登录' }))
    expect(calls.write).not.toHaveBeenCalled()
    fireEvent.click(
      within(screen.getByRole('dialog', { name: '退出此设备' })).getByRole(
        'button',
        { name: '确认退出' }
      )
    )
    await waitFor(() =>
      expect(calls.write).toHaveBeenCalledWith(
        '/api/user/sessions/other%2Fdevice',
        'DELETE'
      )
    )
    expect(await screen.findByRole('status')).toHaveTextContent(
      '所选设备已退出登录'
    )
  })
  it('prevents Escape and close from dismissing an action in flight', () => {
    const close = vi.fn()
    render(
      <Modal title='处理中' busy close={close}>
        <p>请稍候</p>
      </Modal>
    )
    const cancel = new Event('cancel', { bubbles: false, cancelable: true })
    fireEvent(screen.getByRole('dialog'), cancel)
    expect(cancel.defaultPrevented).toBe(true)
    expect(screen.getByRole('button', { name: '关闭' })).toBeDisabled()
    expect(close).not.toHaveBeenCalled()
  })
  it('closes a completed session operation even while the background refresh is pending', async () => {
    render(<SecurityScreen />)
    fireEvent.click(await screen.findByRole('button', { name: '退出登录' }))
    calls.read.mockImplementation(() => new Promise(() => {}))
    fireEvent.click(within(screen.getByRole('dialog', { name: '退出此设备' })).getByRole('button', { name: '确认退出' }))
    expect(await screen.findByText('所选设备已退出登录。')).toBeVisible()
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(calls.write).toHaveBeenCalledExactlyOnceWith('/api/user/sessions/other%2Fdevice', 'DELETE')
  })
  it('plays an exit state before removing the modal', async () => {
    const close = vi.fn()
    vi.spyOn(window, 'matchMedia').mockReturnValue({
      matches: false,
    } as MediaQueryList)
    render(
      <Modal title='确认操作' close={close}>
        <p>内容</p>
      </Modal>
    )
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.getByRole('dialog')).toHaveClass('rfw-dialog-closing')
    expect(close).not.toHaveBeenCalled()
    await waitFor(() => expect(close).toHaveBeenCalledOnce())
  })
  it('keeps full compositor motion and animated exit despite legacy motion-off preferences', async () => {
    const close = vi.fn()
    const cancel = vi.fn()
    const animate = vi.fn(() => ({ cancel }))
    Object.defineProperty(HTMLDialogElement.prototype, 'animate', {
      configurable: true,
      value: animate,
    })
    document.documentElement.dataset.furtherMotion = 'off'
    document.documentElement.dataset.reducedMotion = 'true'
    try {
      render(
        <Modal title='启用动画' close={close}>
          <p>内容</p>
        </Modal>
      )
      const dialog = screen.getByRole('dialog')
      expect(dialog).toHaveAttribute('data-rfw-motion-engine', 'web')
      expect(animate).toHaveBeenCalledWith(
        [
          { opacity: 0, translate: 'none', scale: '1' },
          { opacity: 1, translate: 'none', scale: '1' },
        ],
        expect.objectContaining({ duration: 220 })
      )
      fireEvent.click(screen.getByRole('button', { name: '关闭' }))
      expect(close).not.toHaveBeenCalled()
      expect(animate).toHaveBeenLastCalledWith(
        expect.any(Array),
        expect.objectContaining({ duration: 140 })
      )
      await waitFor(() => expect(close).toHaveBeenCalledOnce())
    } finally {
      delete document.documentElement.dataset.furtherMotion
      delete document.documentElement.dataset.reducedMotion
      delete (HTMLDialogElement.prototype as Partial<HTMLElement>).animate
    }
  })
})
