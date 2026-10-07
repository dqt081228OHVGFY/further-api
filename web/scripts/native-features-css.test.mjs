import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { test } from 'node:test'

import postcss from 'postcss'

import { publishNativeFeatureStyles, transformNativeFeatureCss } from './native-features-css.mjs'

test('document and dark selectors target the shadow host and both feature roots', () => {
  const css = transformNativeFeatureCss(`
    :root { --tone: blue }
    html.dark[data-theme='dark'] { --tone: navy }
    html[data-theme='dark'] body, body[data-scroll-locked] { color: red }
    .dark .card, .dark\\:block:is(.dark *) { color: blue }
    .card:not(.dark) { color: green }
  `)
  const selectors = []
  postcss.parse(css).walkRules((rule) => selectors.push(rule.selector))
  assert.deepEqual(selectors, [
    ':host',
    ":host(.dark[data-theme='dark'])",
    ":host([data-theme='dark']) :is([data-native-feature-root],[data-native-feature-portal]),:is([data-native-feature-root],[data-native-feature-portal])[data-scroll-locked]",
    ':host(.dark) .card, .dark\\:block:is(:host(.dark) *)',
    '.card:not(.dark)',
  ])
})

test('font, property, animation and asset references survive CSS conversion', () => {
  const css = transformNativeFeatureCss(`
    @font-face { font-family: Example; src: url(../font/example.woff2) }
    @property --angle { syntax: '<angle>'; inherits: false; initial-value: 0deg }
    @keyframes spin { from { transform: rotate(0) } to { transform: rotate(360deg) } }
    :root { --angle: 90deg }
  `, { sourceFile: path.join('dist', 'static', 'css', 'index.css'), distRoot: 'dist' })
  assert.match(css, /@font-face/)
  assert.match(css, /@property --angle/)
  assert.match(css, /@keyframes spin \{ from/)
  assert.match(css, /url\(\/static\/font\/example\.woff2\)/)
  assert.match(css, /:host \{ --angle: 90deg \}/)
})

test('embedded toast feedback and loading indicators keep their animations under system motion preferences', async () => {
  const vendorCss = await readFile(path.resolve(import.meta.dirname, '../node_modules/sonner/dist/styles.css'), 'utf8')
  assert.match(vendorCss, /prefers-reduced-motion/)
  const css = transformNativeFeatureCss(vendorCss)
  assert.doesNotMatch(css, /prefers-reduced-motion/)
  assert.match(css, /@keyframes sonner-spin/)
  assert.match(css, /animation: sonner-spin 1\.2s linear infinite/)
  assert.match(css, /transition: transform 400ms, opacity 400ms, height 400ms, box-shadow 200ms/)
  assert.match(css, /@media \(hover: none\) and \(pointer: coarse\)/)
})

test('manifest points to a single isolated stylesheet containing main and lazy CSS', async () => {
  const distRoot = await mkdtemp(path.join(tmpdir(), 'further-native-css-'))
  try {
    const cssRoot = path.join(distRoot, 'static', 'css')
    const assetRoot = path.join(distRoot, 'further-assets')
    await mkdir(path.join(cssRoot, 'async'), { recursive: true })
    await mkdir(assetRoot)
    await writeFile(path.join(cssRoot, 'index.123.css'), '@font-face{font-family:Example;src:url(/static/font/example.woff2)}@property --angle{syntax:"<angle>";inherits:false;initial-value:0deg}:root{--base:1}.base{display:block}')
    await writeFile(path.join(cssRoot, 'async', 'feature.123.css'), '.dark .feature{color:red}')
    await writeFile(path.join(assetRoot, 'app.css'), 'body{background:red}')
    const overrideFile = path.join(distRoot, 'override.css')
    await writeFile(overrideFile, '[data-native-feature-root]{border-radius:20px}')

    const { manifest, cssFiles } = await publishNativeFeatureStyles({ distRoot, overrideFile })
    const css = await readFile(path.join(assetRoot, 'native-features.css'), 'utf8')
    const foundationsCss = await readFile(path.join(assetRoot, 'native-features-foundations.css'), 'utf8')
    const savedManifest = JSON.parse(await readFile(path.join(assetRoot, 'native-features-manifest.json'), 'utf8'))
    assert.equal(cssFiles.length, 2)
    assert.match(css, /:host\{--base:1\}/)
    assert.match(css, /:host\(\.dark\) \.feature\{color:red\}/)
    assert.match(css, /\[data-native-feature-root\]\{border-radius:20px\}/)
    assert.doesNotMatch(css, /background:red/)
    const foundations = postcss.parse(foundationsCss)
    const globalAtRules = []
    const globalSelectors = []
    foundations.walkAtRules((rule) => globalAtRules.push(rule.name))
    foundations.walkRules((rule) => globalSelectors.push(rule.selector))
    assert.deepEqual(globalAtRules, ['font-face', 'property'])
    assert.deepEqual(globalSelectors, [])
    assert.doesNotMatch(foundationsCss, /--base|\.base|\.feature|border-radius/)
    assert.deepEqual(savedManifest, manifest)
    assert.match(manifest.styles[0], /^\/further-assets\/native-features\.css\?v=[a-f0-9]{16}$/)
    assert.match(manifest.globals[0], /^\/further-assets\/native-features-foundations\.css\?v=[a-f0-9]{16}$/)
  } finally {
    await rm(distRoot, { recursive: true, force: true })
  }
})


test('embedded workspaces fill flex roots without a second set of page gutters', async () => {
  const workspace = postcss.parse(await readFile(path.resolve(import.meta.dirname, '../src/further/feature-workspace/workspace.css'), 'utf8'))
  const integration = postcss.parse(await readFile(path.resolve(import.meta.dirname, '../src/further/native-features.css'), 'utf8'))
  const values = (css, selector) => {
    const result = {}
    css.walkRules(rule => {
      if (!postcss.list.comma(rule.selector).includes(selector)) return
      rule.walkDecls(decl => { result[decl.prop] = decl.value })
    })
    return result
  }
  const page = values(workspace, '.rfw-page')
  assert.equal(page.width, '100%', 'auto width with auto flex-item margins caused the page to shrink to its contents')
  assert.equal(page.margin, '0')
  assert.equal(page.padding, '0')
  assert.equal(page['max-width'], 'none')
  const root = values(integration, '[data-native-feature-root]')
  assert.equal(root.width, '100%')
  assert.equal(root['align-items'], 'stretch')
  assert.equal(root.container, 'further-native / inline-size')
  const embedded = values(integration, '[data-native-feature-root] > .rfw-page')
  for (const [property, value] of Object.entries({ width: '100%', margin: '0', padding: '0', 'max-width': 'none', 'min-width': '0', 'box-sizing': 'border-box' })) assert.equal(embedded[property], value)
  assert.equal(values(workspace, '.rfw-header h1')['line-height'], '1.45')
})

test('native form columns respond to the host width, while typography inherits the Further shell', async () => {
  const source = await readFile(path.resolve(import.meta.dirname, '../src/further/native-features.css'), 'utf8')
  const css = postcss.parse(source)
  const smallPaneRules = []
  css.walkAtRules('container', rule => {
    if (!rule.params.includes('further-native') || !rule.params.includes('720px')) return
    rule.walkDecls('grid-template-columns', declaration => smallPaneRules.push([declaration.parent.selector, declaration.value]))
  })
  assert.ok(smallPaneRules.some(([selector, value]) => selector.includes('.rfw-settings-section-fields') && value === 'minmax(0,1fr)'))
  assert.ok(smallPaneRules.some(([selector, value]) => selector.includes('.rfw-record-grid') && value === 'minmax(0,1fr)'))
  let inheritsShellFont = false
  css.walkDecls('font-family', declaration => {
    if (declaration.parent.selector.includes('[data-native-feature-root]')) inheritsShellFont ||= declaration.value === 'inherit'
  })
  assert.ok(inheritsShellFont)
  assert.doesNotMatch(source, /font-family:\s*'Segoe UI/)
})
