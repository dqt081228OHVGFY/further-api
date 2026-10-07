import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SETTINGS_PAGES, settingsPageFor } from '../feature-workspace/catalog'
import { settingPresetsFor, type SettingValues } from '../feature-workspace/settings-presets'
import { SettingsScreen } from '../feature-workspace/settings'
import { validateSetting } from '../feature-workspace/settings-validation'

const api = vi.hoisted(() => ({ read: vi.fn(), write: vi.fn() }))
vi.mock('../feature-workspace/api', async original => ({ ...(await original<typeof import('../feature-workspace/api')>()), ...api }))

const configured: SettingValues = {
  ServerAddress: 'https://gateway.example.test', SMTPServer: 'smtp.example.test',
  SMTPAccount: 'mail@example.test', SMTPTokenConfigured: 'true', SensitiveWords: 'blocked-word',
  'claude.default_max_tokens': '{"default":4096,"specific-model":16384}',
}

beforeEach(() => {
  api.read.mockReset()
  api.write.mockReset().mockResolvedValue({})
})

describe('settings presets', () => {
  it('offers only fields owned by each live form, with valid types and without credentials or business prices', () => {
    let pages = 0, choices = 0
    for (const [name, page] of Object.entries(SETTINGS_PAGES)) {
      const presets = settingPresetsFor(`/system-settings/${name}`, configured, 'https://browser.example.test')
      if (presets.length) pages++
      for (const preset of presets) {
        choices++
        expect(preset.unavailable, `${name}: ${preset.id}`).toBeUndefined()
        expect(Object.keys(preset.values).length).toBeGreaterThan(0)
        for (const [key, value] of Object.entries(preset.values)) {
          const field = page.fields.find(field => field.key === key)
          expect(field, `${name}: ${key}`).toBeDefined()
          expect(field!.kind).not.toBe('secret')
          expect(key).not.toMatch(/(?:secret|api_key|SMTPToken|ModelPrice|QuotaPerUnit|USDExchangeRate|QuotaFor|UnitPrice)/i)
          expect(validateSetting(field!, value), `${name}: ${key}`).toBe('')
          if (field!.kind === 'toggle') expect(['true', 'false']).toContain(value)
        }
      }
    }
    expect(pages).toBe(20)
    expect(choices).toBeGreaterThanOrEqual(26)
  })

  it('keeps SSRF protection effective even when domain IP filtering was previously disabled', () => {
    const current = { 'fetch_setting.apply_ip_filter_for_domain': 'false', 'fetch_setting.domain_list': '["trusted.example"]' }
    const [preset] = settingPresetsFor('/system-settings/security/ssrf', current, 'https://gateway.example')
    const updated: SettingValues = { ...current, ...preset.values }
    expect(updated['fetch_setting.apply_ip_filter_for_domain']).toBe('true')
    expect(updated['fetch_setting.allow_private_ip']).toBe('false')
    expect(JSON.parse(updated['fetch_setting.allowed_ports'])).toEqual(['443'])
    expect(updated['fetch_setting.domain_list']).toBe(current['fetch_setting.domain_list'])
  })

  it('preserves deployment paths, prior passkey domains and model-specific Claude limits', () => {
    const values = { ...configured, ServerAddress: 'https://gateway.example.test/prefix/', 'passkey.rp_id': 'old.example.test', 'passkey.legacy_rp_ids': 'older.example.test' }
    expect(settingPresetsFor('/system-settings/site/system-info', values, '')[0].values.TaskPublicAddress).toBe('https://gateway.example.test/prefix')
    const passkey = settingPresetsFor('/system-settings/auth/passkey', values, '')[0]
    expect(passkey.values['passkey.origins']).toBe('https://gateway.example.test')
    expect(passkey.values['passkey.legacy_rp_ids'].split(',')).toEqual(['older.example.test', 'old.example.test'])
    const claude = settingPresetsFor('/system-settings/models/claude', values, '')[0]
    expect(JSON.parse(claude.values['claude.default_max_tokens'])).toEqual({ default: 8192, 'specific-model': 16384 })
    expect(settingPresetsFor('/system-settings/auth/passkey', {}, 'http://127.0.0.1:3000')[0].unavailable).toBeTruthy()
    expect(settingPresetsFor('/system-settings/site/system-info', { ServerAddress: 'https://gateway.example/?token=private' }, '')[0].unavailable).toBeTruthy()
    expect(settingPresetsFor('/system-settings/models/claude', { 'claude.default_max_tokens': 'broken' }, '')[0].unavailable).toBeTruthy()
  })

  it('requires real mail credentials before offering email verification and never invents them', () => {
    expect(settingPresetsFor('/system-settings/auth/basic-auth', {}, '')[0].unavailable).toContain('系统邮件')
    const [preset] = settingPresetsFor('/system-settings/operations/email', { SMTPAccount: 'operator@example.test', SMTPFrom: 'custom@example.test', SMTPToken: 'unsaved-secret' }, '')
    expect(preset.values).not.toHaveProperty('SMTPAccount')
    expect(preset.values).not.toHaveProperty('SMTPFrom')
    expect(preset.values).not.toHaveProperty('SMTPToken')
  })

  it('fills SMTP transport for review and saves only the changed settings when requested', async () => {
    api.read.mockResolvedValue(Object.entries({ SMTPServer: 'smtp.existing.test', SMTPPort: '587', SMTPAccount: 'account@example.test', SMTPFrom: 'sender@example.test', SMTPTokenConfigured: 'true', SMTPSSLEnabled: 'false', SMTPStartTLSEnabled: 'true' }).map(([key, value]) => ({ key, value })))
    render(<SettingsScreen page={settingsPageFor('/system-settings/operations/email')!} pathname='/system-settings/operations/email' />)
    fireEvent.click(await screen.findByRole('button', { name: 'SSL · 465' }))
    expect(api.write).not.toHaveBeenCalled()
    expect(screen.getByLabelText('端口')).toHaveValue(465)
    expect(screen.getByLabelText('SMTP 服务器')).toHaveValue('smtp.existing.test')
    expect(screen.getByLabelText('授权密钥')).toHaveValue('')
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(api.write).toHaveBeenCalledTimes(3))
    expect(api.write.mock.calls.map(call => call[2])).toEqual([
      { key: 'SMTPStartTLSEnabled', value: 'false' },
      { key: 'SMTPPort', value: '465' },
      { key: 'SMTPSSLEnabled', value: 'true' },
    ])
  })

  it('keeps policy presets local until saving their linked values through the atomic policy endpoint', async () => {
    api.read.mockResolvedValue({ options: { RetryTimes: '0', AutomaticRetryStatusCodes: '502', 'channel_affinity_setting.enabled': 'false', 'channel_affinity_setting.default_ttl_seconds': '30', 'channel_affinity_setting.max_entries': '100' } })
    render(<SettingsScreen page={settingsPageFor('/system-settings/request-policies/routing')!} pathname='/system-settings/request-policies/routing' />)
    fireEvent.click(await screen.findByRole('button', { name: '稳健重试' }))
    expect(api.write).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '撤销更改' }))
    expect(screen.getByLabelText('重试次数')).toHaveValue(0)
    fireEvent.click(screen.getByRole('button', { name: '稳健重试' }))
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(api.write).toHaveBeenCalledExactlyOnceWith('/api/option/request_policy', 'PATCH', { options: { RetryTimes: '2', AutomaticRetryStatusCodes: '429,500,502,503,504', 'channel_affinity_setting.enabled': 'true', 'channel_affinity_setting.default_ttl_seconds': '300', 'channel_affinity_setting.max_entries': '100000' } }))
  })
})
