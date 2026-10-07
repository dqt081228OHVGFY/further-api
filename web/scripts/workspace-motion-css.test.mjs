import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { test } from 'node:test'
import postcss from 'postcss'

const root = resolve(import.meta.dirname, '../..')
const files = [
  'further-ui/assets/app.css', 'further-ui/assets/material.css', 'further-ui/assets/workspace-motion.css',
  'further-ui/assets/activities.css', 'further-ui/assets/home.css', 'further-ui/assets/home-material.css',
  'web/src/further/native-features.css', 'web/src/further/feature-workspace/workspace.css',
]
const sheets = await Promise.all(files.map(async (file) => ({ file, source: await readFile(resolve(root, file), 'utf8') })))
const compositorProperties = new Set(['opacity', 'transform', 'translate', 'rotate', 'scale'])

test('page, dialog and scene keyframes only animate compositor properties', () => {
  for (const { file, source } of sheets) {
    const css = postcss.parse(source, { from: file })
    css.walkAtRules(/keyframes$/, (rule) => {
      // Only inline batch bodies resize; page, dialog and floating menu surfaces stay compositor-only.
      if (file === 'further-ui/assets/app.css' && rule.params === 'rf-dropdown-collapse') {
        rule.walkDecls((decl) => assert.equal(decl.prop, 'height'))
        let owners = 0
        css.walkDecls('animation-name', (decl) => {
          if (decl.value !== rule.params) return
          assert.equal(decl.parent.selector, ".code-batch-items[data-dropdown-motion-engine='css'][data-dropdown-collapse='true']")
          owners += 1
        })
        assert.equal(owners, 1)
        return
      }
      rule.walkDecls((decl) => assert.ok(compositorProperties.has(decl.prop), `${file} ${rule.params} animates ${decl.prop}`))
    })
  }
})

test('large surfaces do not continuously resize, blur or reserve animation layers', () => {
  for (const { file, source } of sheets) {
    const css = postcss.parse(source, { from: file })
    css.walkDecls('transition', (decl) => {
      for (const value of postcss.list.comma(decl.value)) {
        const property = value.trim().split(/\s+/)[0]
        // The fixed-size navigation rail changes layout without scaling text.
        if (file === 'further-ui/assets/app.css' && decl.parent.selector === ':root[data-rail-ready="true"] .nav-rail' && ['width', 'padding'].includes(property)) continue
        assert.doesNotMatch(property, /^(all|width|height|top|left|padding|font-size|filter|backdrop-filter)$/, `${file} ${decl.parent.selector} transitions ${property}`)
      }
    })
    css.walkDecls('will-change', (decl) => {
      assert.ok(!/\.show(?:\b|\s)/.test(decl.parent.selector), `${file} keeps open-dialog layers alive`)
      assert.ok(postcss.list.comma(decl.value).every((property) => compositorProperties.has(property.trim())), `${file} has a noncompositor layer hint`)
    })
  }
})

test('idle decorations finish while actual busy indicators keep rotating', () => {
  let busyIndicator = false
  for (const { file, source } of sheets) {
    const css = postcss.parse(source, { from: file })
    css.walkDecls('animation', (decl) => {
      if (!decl.value.includes('infinite')) return
      assert.match(decl.parent.selector, /data-busy|loading|spinner/, `${file} ${decl.parent.selector} runs an idle frame loop`)
      busyIndicator = true
    })
  }
  assert.equal(busyIndicator, true)
})

test('festival scenes create animations only for visible and active artwork', () => {
  const sheet = sheets.find((sheet) => sheet.file.endsWith('/activities.css'))
  const css = postcss.parse(sheet.source, { from: sheet.file })
  css.walkDecls('animation', (decl) => {
    if (!/scene-|festival-atmosphere/.test(decl.parent.selector)) return
    assert.match(decl.parent.selector, /is-scene-visible/)
    assert.match(decl.parent.selector, /:not\(\.is-scene-paused\)/)
    if (decl.parent.selector.includes('festival-gallery-card')) assert.match(decl.parent.selector, /is-scene-engaged/)
  })
})

test('browser preferences never disable feedback and owners suppress only duplicate entry', () => {
  for (const { source } of sheets) assert.doesNotMatch(source, /prefers-reduced-motion|data-further-motion\s*=\s*['"]off/)
  const classic = sheets.find((sheet) => sheet.file.endsWith('/app.css')).source
  const native = sheets.find((sheet) => sheet.file.endsWith('/native-features.css')).source
  const local = sheets.find((sheet) => sheet.file.endsWith('/workspace.css')).source
  assert.match(classic, /data-dialog-motion-engine='web'/)
  assert.match(native, /data-native-motion-engine='web'/)
  assert.match(local, /rfw-page\[data-motion-enter='page'\]/)
  const material = sheets.find((sheet) => sheet.file.endsWith('/material.css')).source
  assert.doesNotMatch(material, /\.view\.active \.stat-card[^{}]*\{[^}]*animation:/)
})

test('homepage content stays still while sections reveal once', () => {
  const material = sheets.find((sheet) => sheet.file.endsWith('/home-material.css')).source
  const home = sheets.find((sheet) => sheet.file.endsWith('/home.css')).source
  assert.doesNotMatch(material, /animation-timeline:view\(\)/)
  assert.doesNotMatch(home, /--home-scroll-(opacity|lift|scale|blur)/)
  assert.match(home, /\.home-motion-surface\s*\{\s*opacity:1;\s*transform:none;/)
  assert.match(home, /\[data-home-scroll\]:not\(\.is-visible\)/)
  assert.doesNotMatch(home, /home-scroll-card:focus-within[^{}]*\{[^}]*transform:/)
  assert.doesNotMatch(material, /--home-hover-lift/)
})
