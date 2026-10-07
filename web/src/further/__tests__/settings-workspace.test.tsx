import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  type SettingsPage,
  settingsPageFor,
} from '../feature-workspace/catalog'
import { SettingsScreen } from '../feature-workspace/settings'
import { NativeFeatureHostContext } from '../native-feature-context'

const api = vi.hoisted(() => ({ read: vi.fn(), write: vi.fn() }))
vi.mock('../feature-workspace/api', async (original) => ({
  ...(await original<typeof import('../feature-workspace/api')>()),
  ...api,
}))

const page: SettingsPage = {
  title: '全局模型',
  description: '配置模型协议',
  fields: [
    { key: 'title', label: '显示名称' },
    { key: 'global.chat_completions_to_responses_policy', label: '协议转换策略', kind: 'multiline', format: 'json' },
  ],
}

beforeEach(() => {
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
  api.read.mockReset().mockResolvedValue([
    { key: 'title', value: '原名称' },
    { key: 'global.chat_completions_to_responses_policy', value: '{"model":1}' },
  ])
  api.write.mockReset().mockResolvedValue({})
})

describe('further-api settings workspace', () => {
  it.each([
    ['/system-settings/site/system-info', '任务公开地址', 'TaskPublicAddress', 'https://tasks.example.test'],
    ['/system-settings/auth/oauth', 'GitHub 应用标识', 'GitHubClientId', 'new-github-client'],
  ])('keeps shared site and LinuxDO values unchanged when saving the remaining %s fields', async (pathname, title, key, value) => {
    const stored: Record<string, string> = {
      ServerAddress: 'https://configured.example.test',
      LinuxDOOAuthEnabled: 'true',
      LinuxDOClientId: 'existing-linuxdo-client',
      LinuxDOClientSecret: 'server-only-existing-secret',
      TaskPublicAddress: 'https://previous-tasks.example.test',
      GitHubClientId: 'previous-github-client',
    }
    const preserved = { ...stored }
    api.read.mockImplementation(async () => Object.entries(stored).map(([key, value]) => ({ key, value })))
    api.write.mockImplementation(async (_path, _method, body) => { stored[body.key] = body.value; return {} })
    render(<SettingsScreen page={settingsPageFor(pathname)!} pathname={pathname} />)
    fireEvent.change(await screen.findByLabelText(title), { target: { value } })
    expect(screen.queryByLabelText('服务地址')).toBeNull()
    expect(screen.queryByLabelText('LinuxDO 应用标识')).toBeNull()
    expect(screen.queryByLabelText('LinuxDO 应用密钥')).toBeNull()
    expect(screen.getByRole('link', { name: /管理.*LinuxDO 登录/ })).toHaveAttribute('href', '#settings/site')
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/option/', 'PUT', { key, value }))
    for (const shared of ['ServerAddress', 'LinuxDOOAuthEnabled', 'LinuxDOClientId', 'LinuxDOClientSecret']) expect(stored[shared]).toBe(preserved[shared])
  })

  it('validates every changed field before writing any configuration', async () => {
    render(
      <SettingsScreen
        page={page}
        pathname='/system-settings/models/global'
      />
    )
    fireEvent.change(await screen.findByLabelText('显示名称'), {
      target: { value: '新名称' },
    })
    fireEvent.change(screen.getByLabelText('协议转换策略'), {
      target: { value: '{"model":}' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'JSON 格式不正确'
    )
    expect(api.write).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.getByLabelText('协议转换策略')).toHaveFocus())
  })

  it('keeps failed changes for retry and does not resubmit successfully saved fields', async () => {
    api.write
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('设置服务暂时不可用'))
      .mockResolvedValue({})
    render(
      <SettingsScreen
        page={page}
        pathname='/system-settings/models/global'
      />
    )
    fireEvent.change(await screen.findByLabelText('显示名称'), {
      target: { value: '新名称' },
    })
    fireEvent.change(screen.getByLabelText('协议转换策略'), {
      target: { value: '{"model":2}' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      '设置服务暂时不可用'
    )
    expect(screen.getByLabelText('协议转换策略')).toHaveValue('{"model":2}')
    expect(screen.getByRole('status')).toHaveTextContent('1 项更改等待保存')
    api.write.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() =>
      expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/option/', 'PUT', {
        key: 'global.chat_completions_to_responses_policy',
        value: '{"model":2}',
      })
    )
    expect(await screen.findByText('设置已保存')).toBeVisible()
  })

  it('requires a successful read before offering configuration edits', async () => {
    api.read.mockRejectedValueOnce(new Error('无读取权限'))
    render(
      <SettingsScreen
        page={page}
        pathname='/system-settings/models/global'
      />
    )
    expect(await screen.findByRole('alert')).toHaveTextContent('无读取权限')
    expect(screen.queryByRole('button', { name: '保存配置' })).toBeNull()
    expect(screen.queryByLabelText('协议转换策略')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '重新读取' }))
    expect(await screen.findByLabelText('协议转换策略')).toHaveValue('{"model":1}')
  })

  it('submits policy changes as one operation and retains unrelated server options', async () => {
    api.read.mockResolvedValue({
      options: { RetryTimes: '2', SensitiveWords: '原规则' },
    })
    const policies: SettingsPage = {
      title: '会话与重试',
      description: '',
      fields: [
        { key: 'RetryTimes', label: '重试次数', kind: 'number', min: 0 },
      ],
    }
    render(
      <SettingsScreen
        page={policies}
        pathname='/system-settings/request-policies/routing'
      />
    )
    fireEvent.change(await screen.findByLabelText('重试次数'), {
      target: { value: '3' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() =>
      expect(api.write).toHaveBeenCalledExactlyOnceWith(
        '/api/option/request_policy',
        'PATCH',
        { options: { RetryTimes: '3' } }
      )
    )
    expect(await screen.findByText('设置已保存')).toBeVisible()
  })

  it('searches localized field labels without losing unsaved values', async () => {
    render(
      <SettingsScreen
        page={page}
        pathname='/system-settings/models/global'
      />
    )
    fireEvent.change(await screen.findByLabelText('显示名称'), {
      target: { value: '待保存名称' },
    })
    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: '模型' },
    })
    expect(screen.queryByLabelText('显示名称')).toBeNull()
    expect(screen.getByRole('status')).toHaveTextContent('1 项更改等待保存')
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: '' } })
    expect(screen.getByLabelText('显示名称')).toHaveValue('待保存名称')
    fireEvent.click(screen.getByRole('button', { name: '撤销更改' }))
    expect(screen.getByLabelText('显示名称')).toHaveValue('原名称')
  })

  it('reloads when moving between settings pages with the same API type', async () => {
    const view = render(
      <SettingsScreen
        page={page}
        pathname='/system-settings/models/global'
      />
    )
    await screen.findByLabelText('显示名称')
    api.read.mockClear()
    view.rerender(
      <SettingsScreen
        page={page}
        pathname='/system-settings/billing/currency'
      />
    )
    await waitFor(() =>
      expect(api.read).toHaveBeenCalledExactlyOnceWith('/api/option/')
    )
  })
  it('stages an explicit secret removal, preserves failed intent, and retries without revealing the value', async () => {
    api.read.mockResolvedValue([{ key: 'SMTPTokenConfigured', value: 'true' }])
    const credentials: SettingsPage = {
      title: '邮件',
      description: '',
      fields: [{ key: 'SMTPToken', label: '邮件授权码', kind: 'secret' }],
    }
    render(
      <SettingsScreen
        page={credentials}
        pathname='/system-settings/operations/email'
      />
    )
    expect(await screen.findByLabelText('邮件授权码')).toHaveValue('')
    fireEvent.click(screen.getByRole('button', { name: '清除邮件授权码' }))
    expect(api.write).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '选择清除' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.write).not.toHaveBeenCalled()
    api.write.mockRejectedValueOnce(new Error('配置服务暂不可用'))
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      '配置服务暂不可用'
    )
    expect(screen.getByRole('button', { name: '保留现有配置' })).toBeEnabled()
    expect(screen.getByRole('status')).toHaveTextContent('1 项更改等待保存')
    api.write.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() =>
      expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/option/', 'PUT', {
        key: 'SMTPToken',
        value: '',
      })
    )
    expect(await screen.findByText('设置已保存')).toBeVisible()
    expect(screen.queryByRole('button', { name: '清除邮件授权码' })).toBeNull()
  })

  it('clears a successfully replaced secret from the local input and avoids resubmitting it after a later field fails', async () => {
    api.read.mockResolvedValue([
      { key: 'SMTPTokenConfigured', value: 'true' },
      { key: 'SMTPServer', value: 'old.example' },
    ])
    const credentials: SettingsPage = {
      title: '邮件',
      description: '',
      fields: [
        { key: 'SMTPToken', label: '邮件授权码', kind: 'secret' },
        { key: 'SMTPServer', label: '邮件服务器' },
      ],
    }
    api.write
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('服务器名称不正确'))
      .mockResolvedValue({})
    render(
      <SettingsScreen
        page={credentials}
        pathname='/system-settings/operations/email'
      />
    )
    fireEvent.change(await screen.findByLabelText('邮件授权码'), {
      target: { value: 'private-replacement' },
    })
    fireEvent.change(screen.getByLabelText('邮件服务器'), {
      target: { value: 'new.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      '服务器名称不正确'
    )
    expect(screen.getByLabelText('邮件授权码')).toHaveValue('')
    api.write.mockClear()
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() =>
      expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/option/', 'PUT', {
        key: 'SMTPServer',
        value: 'new.example',
      })
    )
  })

  it('previews passkey domain impact and requires confirmation before one atomic domain save', async () => {
    api.read.mockResolvedValue([
      { key: 'passkey.rp_id', value: 'gateway.example' },
      { key: 'passkey.legacy_rp_ids', value: 'old.example' },
      {
        key: 'passkey.origins',
        value: 'https://gateway.example,https://old.example',
      },
    ])
    const preview = {
      rp_id: 'gateway.example',
      previous_rp_id: 'gateway.example',
      effective_rp_id: 'gateway.example',
      legacy_rp_ids: '',
      origins: 'https://gateway.example',
      removed_rp_ids: ['old.example'],
      affected_credentials: 2,
      unknown_credentials: 0,
      confirmation_required: true,
      removal_confirmation: 'intent-proof',
    }
    api.write.mockResolvedValue(preview)
    render(
      <SettingsScreen
        page={settingsPageFor('/system-settings/auth/passkey')!}
        pathname='/system-settings/auth/passkey'
      />
    )
    fireEvent.change(await screen.findByLabelText('保留的旧服务域名'), {
      target: { value: '' },
    })
    fireEvent.change(screen.getByLabelText('允许的来源'), {
      target: { value: 'https://gateway.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    const dialog = await screen.findByRole('dialog', {
      name: '检查通行密钥域名变更',
    })
    expect(within(dialog).getByText(/受影响的凭据 2 个/)).toBeVisible()
    expect(within(dialog).queryByText('intent-proof')).toBeNull()
    const save = within(dialog).getByRole('button', {
      name: '确认保存域名与策略',
    })
    expect(save).toBeDisabled()
    expect(api.write).toHaveBeenCalledExactlyOnceWith(
      '/api/option/passkey/domains',
      'PUT',
      {
        rp_id: 'gateway.example',
        legacy_rp_ids: '',
        origins: 'https://gateway.example',
        preview: true,
      }
    )
    fireEvent.click(within(dialog).getByRole('checkbox'))
    fireEvent.click(save)
    await waitFor(() =>
      expect(api.write).toHaveBeenLastCalledWith(
        '/api/option/passkey/domains',
        'PUT',
        {
          rp_id: 'gateway.example',
          legacy_rp_ids: '',
          origins: 'https://gateway.example',
          preview: false,
          removal_confirmation: 'intent-proof',
        }
      )
    )
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(
      api.write.mock.calls.some((call) => call[0] === '/api/option/')
    ).toBe(false)
  })

  it('keeps a rejected domain draft and allows a fresh impact preview before retrying', async () => {
    api.read.mockResolvedValue([
      { key: 'passkey.rp_id', value: 'gateway.example' },
      { key: 'passkey.legacy_rp_ids', value: 'old.example' },
      { key: 'passkey.origins', value: 'https://gateway.example' },
    ])
    let previewCount = 0
    api.write.mockImplementation(
      async (_path: string, _method: string, body: { preview?: boolean }) => {
        if (!body.preview && previewCount === 1) {
          throw new Error('域名预览已过期，请重新检查')
        }
        if (body.preview) previewCount++
        return {
          rp_id: 'gateway.example',
          legacy_rp_ids: '',
          origins: 'https://gateway.example',
          confirmation_required: true,
          removal_confirmation: `proof-${previewCount}`,
          removed_rp_ids: ['old.example'],
          affected_credentials: 2,
        }
      }
    )
    render(
      <SettingsScreen
        page={settingsPageFor('/system-settings/auth/passkey')!}
        pathname='/system-settings/auth/passkey'
      />
    )
    fireEvent.change(await screen.findByLabelText('保留的旧服务域名'), {
      target: { value: '' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('checkbox'))
    fireEvent.click(
      within(dialog).getByRole('button', { name: '确认保存域名与策略' })
    )
    expect(await within(dialog).findByRole('alert')).toHaveTextContent(
      '域名预览已过期'
    )
    expect(screen.getByLabelText('保留的旧服务域名')).toHaveValue('')
    fireEvent.click(within(dialog).getByRole('button', { name: '重新检查' }))
    await waitFor(() =>
      expect(within(dialog).getByRole('checkbox')).not.toBeChecked()
    )
    fireEvent.click(within(dialog).getByRole('checkbox'))
    fireEvent.click(
      within(dialog).getByRole('button', { name: '确认保存域名与策略' })
    )
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(api.write).toHaveBeenLastCalledWith(
      '/api/option/passkey/domains',
      'PUT',
      expect.objectContaining({
        preview: false,
        removal_confirmation: 'proof-2',
      })
    )
  })

  it('saves OAuth credentials before enabling a previously unconfigured provider', async () => {
    api.read.mockResolvedValue([
      { key: 'GitHubOAuthEnabled', value: 'false' },
      { key: 'GitHubClientId', value: '' },
      { key: 'GitHubClientSecretConfigured', value: 'false' },
    ])
    const written: Record<string, string> = {}
    api.write.mockImplementation(
      async (_path, _method, body: { key: string; value: string }) => {
        if (
          body.key === 'GitHubOAuthEnabled' &&
          (!written.GitHubClientId || !written.GitHubClientSecret)
        ) {
          throw new Error('身份服务的凭据尚未配置')
        }
        written[body.key] = body.value
        return {}
      }
    )
    render(
      <SettingsScreen
        page={settingsPageFor('/system-settings/auth/oauth')!}
        pathname='/system-settings/auth/oauth'
      />
    )
    fireEvent.click(await screen.findByLabelText('GitHub 登录'))
    fireEvent.change(screen.getByLabelText('GitHub 应用标识'), {
      target: { value: 'client-id' },
    })
    fireEvent.change(screen.getByLabelText('GitHub 应用密钥'), {
      target: { value: 'replacement-secret' },
    })
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    expect(await screen.findByText('设置已保存')).toBeVisible()
    expect(api.write.mock.calls.map((call) => call[2].key)).toEqual([
      'GitHubClientId',
      'GitHubClientSecret',
      'GitHubOAuthEnabled',
    ])
    expect(screen.queryByRole('alert')).toBeNull()
  })
  it('keeps unsaved changes during global refresh and reloads the visible settings after undoing them', async () => {
    const panel = document.createElement('section')
    panel.dataset.panel = 'settings'
    panel.classList.add('active')
    const host = document.createElement('div')
    panel.append(host)
    document.body.append(panel)
    render(
      <NativeFeatureHostContext.Provider value={host}>
        <SettingsScreen
          page={page}
          pathname='/system-settings/models/global'
        />
      </NativeFeatureHostContext.Provider>,
      { container: host }
    )
    fireEvent.change(await screen.findByLabelText('显示名称'), {
      target: { value: '未保存名称' },
    })
    api.read.mockClear()
    const pending: Promise<unknown>[] = []
    window.dispatchEvent(
      new CustomEvent('further-ui:refresh', {
        detail: {
          name: 'settings',
          waitUntil: (request: Promise<unknown>) => pending.push(request),
        },
      })
    )
    await Promise.all(pending)
    expect(api.read).not.toHaveBeenCalled()
    expect(screen.getByLabelText('显示名称')).toHaveValue('未保存名称')
    fireEvent.click(screen.getByRole('button', { name: '撤销更改' }))
    window.dispatchEvent(
      new CustomEvent('further-ui:refresh', {
        detail: {
          name: 'settings',
          waitUntil: (request: Promise<unknown>) => pending.push(request),
        },
      })
    )
    await waitFor(() =>
      expect(api.read).toHaveBeenCalledExactlyOnceWith('/api/option/')
    )
    await waitFor(() =>
      expect(screen.getByLabelText('显示名称')).toHaveValue('原名称')
    )
    panel.remove()
  })
})
