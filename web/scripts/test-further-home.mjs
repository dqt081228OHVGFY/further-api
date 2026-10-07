import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { JSDOM } from 'jsdom'

const source = resolve(import.meta.dirname, '../../further-ui')
const html = readFileSync(resolve(source, 'index.html'), 'utf8')
const site = readFileSync(resolve(source, 'assets/site.js'), 'utf8')
const home = readFileSync(resolve(source, 'assets/home.js'), 'utf8')
const dom = new JSDOM(html, { url: 'http://127.0.0.1:3000/', runScripts: 'outside-only' })
const { window } = dom
window.matchMedia = () => ({ matches: true })
window.FurtherUI = {
  setText(selector, value) {
    window.document.querySelectorAll(selector).forEach((element) => {
      element.textContent = value
    })
  },
}

window.eval(site)
window.FurtherUISite.applyContext({ system_name: '星桥' })
const title = window.document.querySelector('#home-title')
assert.ok(title.textContent.trim().length > 8)
assert.equal(window.document.querySelector('.public-brand strong').textContent, 'further-api')
assert.equal(window.document.querySelector('.home-top-links'), null)
for (const control of window.document.querySelectorAll('header a,header button')) {
  assert.ok(!['产品能力','模型目录','关于','登录'].includes(control.textContent.trim()))
}

window.eval(home)
assert.ok(window.document.querySelectorAll('.home-reveal-card').length >= 3)
const anthropic = window.document.querySelector('[data-home-protocol="anthropic"]')
anthropic.click()
assert.equal(anthropic.getAttribute('aria-selected'), 'true')
assert.equal(window.document.querySelector('[data-home-endpoint]').textContent, '/v1/messages')
assert.match(window.document.querySelector('[data-home-code]').textContent, /anthropic-version: 2023-06-01/)
window.close()
console.log('Further home identity and protocol checks passed')
