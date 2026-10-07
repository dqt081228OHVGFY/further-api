import { beforeEach, describe, expect, it, vi } from 'vitest'
import { workspaceGet } from '../workspace-get'

const mock = vi.hoisted(() => ({ get: vi.fn(), token: 'session-a' }))
vi.mock('@/lib/api', () => ({ api: { get: mock.get } }))
vi.mock('@/stores/auth-store', () => ({ useAuthStore: { getState: () => ({ auth: { accessToken: mock.token } }) } }))

beforeEach(() => { mock.get.mockReset(); mock.token = 'session-a' })

describe('workspace concurrent GET ownership', () => {
  it('joins equivalent simultaneous channel reads without caching the next refresh', async () => {
    let finish!: (value: unknown) => void
    mock.get.mockImplementationOnce(() => new Promise(resolve => { finish = resolve })).mockResolvedValue({ data: { version: 2 } })
    const first = workspaceGet('/api/channel/?p=1&page_size=100')
    const duplicate = workspaceGet('/api/channel/?page_size=100&p=1')
    expect(duplicate).toBe(first)
    await vi.waitFor(() => expect(mock.get).toHaveBeenCalledOnce())
    finish({ data: { version: 1 } })
    await Promise.all([first, duplicate])
    await expect(workspaceGet('/api/channel/?p=1&page_size=100')).resolves.toMatchObject({ data: { version: 2 } })
    expect(mock.get).toHaveBeenCalledTimes(2)
  })

  it('keeps different parameters and authenticated sessions separate', async () => {
    mock.get.mockResolvedValue({ data: {} })
    const first = workspaceGet('/api/token/', { p: 1 })
    const otherPage = workspaceGet('/api/token/', { p: 2 })
    mock.token = 'session-b'
    const otherSession = workspaceGet('/api/token/', { p: 1 })
    await Promise.all([first, otherPage, otherSession])
    expect(mock.get).toHaveBeenCalledTimes(3)
  })

  it('discards a rejected request so a retry can reach the server', async () => {
    mock.get.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ data: { ready: true } })
    await expect(workspaceGet('/api/user/2fa/status')).rejects.toThrow('offline')
    await expect(workspaceGet('/api/user/2fa/status')).resolves.toMatchObject({ data: { ready: true } })
    expect(mock.get).toHaveBeenCalledTimes(2)
  })
})
