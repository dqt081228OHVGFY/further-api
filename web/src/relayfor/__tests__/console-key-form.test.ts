import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const relayforRoot = resolve(import.meta.dirname, '../../../../relayfor-ui')
const html = readFileSync(resolve(relayforRoot, 'console/index.html'), 'utf8')
const script = readFileSync(resolve(relayforRoot, 'assets/console.js'), 'utf8')
const css = readFileSync(resolve(relayforRoot, 'assets/app.css'), 'utf8')

describe('original RelayFor API key form', () => {
  it('does not expose independent quota controls', () => {
    expect(html).not.toContain('name="remain_quota"')
    expect(html).not.toContain('data-quota-usd-for="remain_quota"')
    expect(html).not.toContain('独立额度')
    expect(html).not.toContain('name="unlimited_quota"')
    expect(script).not.toContain('form.elements.remain_quota')
  })

  it('keeps account-following quota semantics in the submitted adapter payload', () => {
    expect(script).toContain('remain_quota: -1')
    expect(script).toContain('unlimited_quota: true')
  })

  it('uses an animated custom model whitelist chooser instead of a native select', () => {
    expect(html).toContain('data-model-picker=""')
    expect(html).toContain('data-model-picker-trigger=""')
    expect(html).toContain('name="model_limits"')
    expect(html).toContain('role="listbox"')
    expect(script).toContain("api.request('/api/user/models')")
    expect(script).toContain('data-model-picker-option')
    expect(css).toContain('.model-picker-menu.is-open')
    expect(css).toContain('.model-picker-menu[hidden]')
  })
})
