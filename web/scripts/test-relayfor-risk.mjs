import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { JSDOM } from 'jsdom'

const script = readFileSync(resolve(import.meta.dirname, '../../relayfor-ui/assets/risk.js'), 'utf8')
const dom = new JSDOM('<body><section id="risk"></section></body>', {
  url: 'http://127.0.0.1:3000/admin/#risk',
  runScripts: 'outside-only',
})
const { window } = dom
window.ProxyLan = {
  escapeHTML: (value) => String(value),
  request: async () => ({ items: [], total: 0 }),
  showBusy() {},
  setText(selector, value) { window.document.querySelectorAll(selector).forEach((node) => { node.textContent = value }) },
  closeDialog() {},
  openDialog() {},
  toast() {},
}
window.eval(script)
const panel = window.document.querySelector('#risk')
window.ProxyLanRisk.mount(panel)
const list = panel.querySelector('.risk-list-card')
assert.equal(list.parentElement, panel)
assert.equal(list.querySelector('.md-card-title').textContent, '风险账户列表')
panel.querySelector('[data-risk-tab="warnings"]').click()
assert.equal(panel.querySelector('[data-risk-warnings]').hidden, false)
assert.equal(list.hidden, false)
panel.querySelector('[data-risk-tab="accounts"]').click()
assert.equal(panel.querySelector('[data-risk-accounts]').hidden, false)
assert.equal(list.hidden, false)
window.close()
console.log('RelayFor risk list remains available under both tabs')
