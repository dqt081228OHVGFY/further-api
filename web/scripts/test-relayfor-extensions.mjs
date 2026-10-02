import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { JSDOM } from 'jsdom'

const root = resolve(import.meta.dirname, '../../relayfor-ui')
const script = readFileSync(resolve(root, 'assets/admin/admin-feature-directory.js'), 'utf8') + '\n' + readFileSync(resolve(root, 'assets/extensions.js'), 'utf8')
const shell = readFileSync(resolve(root, 'assets/app.js'), 'utf8')
function mount(page, role) {
  const dom = new JSDOM(
    readFileSync(resolve(root, `${page}/index.html`), 'utf8'),
    {
      url: `http://localhost/${page}/#overview`,
      runScripts: 'outside-only',
      pretendToBeVisual: true,
    }
  )
  const { window } = dom
  window.CSS ||= {}
  window.CSS.escape ||= (value) => value
  window.matchMedia = () => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
  })
  window.document.body.dataset.gatewayRole = String(role)
  const calls = []
  window.RelayForFeatures = {
    entries: [
      {
        title: '支付网关',
        path: '/system-settings/billing/payment',
        group: '系统设置 · 计费',
        role: 100,
      },
      { title: '账户安全', path: '/security', group: '账户' },
    ],
    async mount(host, options) {
      calls.push(options.path)
      const shadow = host.shadowRoot || host.attachShadow({ mode: 'open' })
      shadow.textContent = options.path
      return { navigate() {}, destroy() {} }
    },
  }
  window.ProxyLanRisk = {
    mount(panel) {
      panel.dataset.nativeRisk = 'true'
    },
  }
  window.eval(shell)
  window.eval(script)
  window.document.dispatchEvent(new window.Event('relayfor:ready'))
  window.ProxyLan.mountShell()
  return { dom, calls }
}
const admin = mount('admin', 100)
const a = admin.dom.window.document
assert.equal(admin.calls.length, 0, 'features load on demand')
assert.ok(
  a.querySelector('[data-settings-page="payment"]'),
  'payment directory uses the local settings page'
)
const tabs = a.querySelector(
  '[data-panel="models"] [data-relayfor-inline-tabs]'
)
tabs.lastElementChild.click()
await Promise.resolve()
assert.equal(admin.calls.at(-1), '/models/metadata')
assert.ok(
  a.querySelector('[data-panel="models"] .relayfor-feature-region').shadowRoot
)
assert.equal(a.querySelectorAll('iframe').length, 0)
tabs.firstElementChild.click()
assert.equal(
  a
    .querySelector('[data-panel="models"]')
    .classList.contains('relayfor-inline-active'),
  false
)
admin.dom.window.dispatchEvent(
  new admin.dom.window.CustomEvent('relayfor:settings-subpage', {
    detail: { name: 'payment' },
  })
)
await Promise.resolve()
assert.equal(admin.calls.includes('/system-settings/billing/payment'), false)
const payment = [
  ...a.querySelectorAll('.relayfor-native-directory button'),
].find((button) => button.textContent === '支付网关')
assert.equal(payment, undefined, 'billing is removed from more features')
assert.ok(a.querySelector('[data-payment-settings]'), 'local payment form is present')
assert.equal(a.querySelectorAll('iframe').length, 0)
const user = mount('console', 1)
const c = user.dom.window.document
assert.equal(c.querySelector('[data-nav="native"]'), null)
assert.equal(c.querySelector('[data-panel="native"]'), null)
assert.equal(
  [...c.querySelectorAll('.relayfor-native-directory button')].some(
    (button) => button.textContent === '支付网关'
  ),
  false
)
c.querySelector(
  '[data-panel="tokens"] [data-relayfor-inline-tabs]'
).lastElementChild.click()
await Promise.resolve()
assert.equal(user.calls.at(-1), '/keys')
assert.ok(c.querySelector('[data-account-security-host]'))
assert.equal(c.querySelectorAll('iframe').length, 0)
admin.dom.window.close()
user.dom.window.close()
console.log('RelayFor same-document feature navigation checks passed')
