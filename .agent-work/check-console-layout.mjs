import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
const require = createRequire(new URL('../web/package.json', import.meta.url))
const { JSDOM } = require('jsdom')
const postcss = require('postcss')
const doc = new JSDOM(readFileSync(new URL('../relayfor-ui/console/index.html', import.meta.url), 'utf8')).window.document
const ids = [...doc.querySelectorAll('[id]')].map((element) => element.id)
const duplicateIds = ids.filter((id, index) => ids.indexOf(id) !== index)
if (duplicateIds.length) throw new Error(`Duplicate IDs: ${duplicateIds}`)
for (const button of doc.querySelectorAll('[data-doc-anchor]')) {
  if (!doc.getElementById(button.dataset.docAnchor)) throw new Error(`Missing scroll target ${button.dataset.docAnchor}`)
}
const selectors = ['[data-profile-form]', '[data-preferences-form]', '[data-create-token]', '[data-redeem]', '[data-redeem-batch]', '[data-user-log-filter]', '[data-token-rows]', '[data-model-pricing-rows]', '[data-log-rows]', '[data-account-security-frame]', 'link[href="/relayfor-assets/console-workspace.css"]']
selectors.forEach((selector) => {
  if (doc.querySelectorAll(selector).length !== 1) throw new Error(`Broken unique contract ${selector}`)
})
const css = postcss.parse(readFileSync(new URL('../relayfor-ui/assets/console-workspace.css', import.meta.url), 'utf8'))
console.log(JSON.stringify({ views: [...doc.querySelectorAll('[data-panel]')].map((element) => element.dataset.panel), forms: doc.querySelectorAll('form').length, scrollTargets: doc.querySelectorAll('[data-doc-anchor]').length, duplicateIds: duplicateIds.length, cssRules: css.nodes.length }))
