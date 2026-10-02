import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { JSDOM } from 'jsdom'

const source = resolve(import.meta.dirname, '../../relayfor-ui')
const html = readFileSync(resolve(source, 'index.html'), 'utf8')
const site = readFileSync(resolve(source, 'assets/site.js'), 'utf8')
const home = readFileSync(resolve(source, 'assets/home.js'), 'utf8')
const dom = new JSDOM(html, { url: 'http://127.0.0.1:3000/', runScripts: 'outside-only' })
const { window } = dom
window.matchMedia = () => ({ matches: true })
window.ProxyLan = {
  setText(selector, value) {
    window.document.querySelectorAll(selector).forEach((element) => {
      element.textContent = value
    })
  },
}

window.eval(site)
window.ProxyLanSite.applyContext({ system_name: '星桥' })
const title = window.document.querySelector('#home-title')
assert.equal(title.textContent, '欢迎来到星桥站')
assert.equal(title.querySelector('br'), null)
assert.equal(window.document.querySelector('.home-core-node [data-site-name]').textContent, '星桥')
assert.equal(window.document.querySelector('.public-brand strong').textContent, '星桥')
assert.equal(window.document.querySelector('.home-top-links'), null)

window.eval(home)
const anthropic = window.document.querySelector('[data-home-protocol="anthropic"]')
anthropic.click()
assert.equal(anthropic.getAttribute('aria-selected'), 'true')
assert.equal(window.document.querySelector('[data-home-endpoint]').textContent, '/v1/messages')
assert.match(window.document.querySelector('[data-home-code]').textContent, /anthropic-version: 2023-06-01/)
window.close()
console.log('RelayFor home identity and protocol checks passed')
