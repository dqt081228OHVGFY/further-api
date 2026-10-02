import { copyFile, cp, mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { publishNativeFeatureStyles } from './native-features-css.mjs'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const sourceRoot = path.resolve(webRoot, '../relayfor-ui')
const distRoot = path.join(webRoot, 'dist')
const assetRoot = path.join(sourceRoot, 'assets')
const digest = createHash('sha256')
async function hashAssets(directory) {
  for (const entry of (await readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
    const file = path.join(directory, entry.name)
    if (entry.isDirectory()) await hashAssets(file)
    else if (entry.isFile()) {
      digest.update(path.relative(assetRoot, file))
      digest.update(await readFile(file))
    }
  }
}
await hashAssets(assetRoot)
const assetVersion = digest.digest('hex').slice(0, 16)
const versionAssetUrls = (value) => value.replaceAll(/\/relayfor-assets\/[\w./-]+/g, (url) => `${url}?v=${assetVersion}`)
const runtimePage = await readFile(path.join(distRoot, 'relayfor-bridge.html'), 'utf8')
const runtimeScripts = [...runtimePage.matchAll(/<script\b[^>]*\bsrc=["'][^"']+["'][^>]*><\/script>/g)].map((match) => match[0]).join('\n')
if (!runtimeScripts.includes('relayfor-bridge')) throw new Error('RelayFor runtime entry was not emitted')

// New API's router is used for additional feature content. The visible website
// uses the original RelayFor documents and their original DOM/motion scripts.
const generatedIndex = await readFile(path.join(distRoot, 'index.html'), 'utf8')
if (!/data-page=["']public["']/.test(generatedIndex)) {
  await copyFile(path.join(distRoot, 'index.html'), path.join(distRoot, 'gateway.html'))
}
let gatewayPage = await readFile(path.join(distRoot, 'gateway.html'), 'utf8')
gatewayPage = gatewayPage.replaceAll(/<meta name="relayfor-asset-version"[^>]*>/g, '')
gatewayPage = gatewayPage.replace('</head>', `<meta name="relayfor-asset-version" content="${assetVersion}"></head>`)
await writeFile(path.join(distRoot, 'gateway.html'), gatewayPage)
await cp(path.join(sourceRoot, 'assets'), path.join(distRoot, 'relayfor-assets'), { recursive: true })
const cssFile = path.join(distRoot, 'relayfor-assets', 'app.css')
await writeFile(cssFile, versionAssetUrls(await readFile(cssFile, 'utf8')))
// The public authentication surface stays on RelayFor's original document too.
// It intentionally uses the same app.css/app.js/site.js bridge as the other
// restored pages so login and registration keep the template's motion and
// controls instead of falling through to New API's React auth shell.
for (const page of ['index.html', 'auth/index.html', 'console/index.html', 'admin/index.html', 'prepaid/index.html']) {
  let html
  try {
    html = await readFile(path.join(sourceRoot, page), 'utf8')
  } catch (error) {
    if (page === 'prepaid/index.html' && error.code === 'ENOENT') continue
    throw error
  }
  html = versionAssetUrls(html).replace('</head>', `<meta name="relayfor-asset-version" content="${assetVersion}">${runtimeScripts}\n</head>`)
  if (page === 'console/index.html' || page === 'admin/index.html') {
    // Avoid painting the document's default overview before the hash-selected
    // panel and its handlers have mounted. Keep the already-themed shell visible.
    html = html.replace('<html', '<html data-relayfor-boot="pending"')
    html = html.replace('</head>', '<style>html[data-relayfor-boot="pending"] .content-area,html[data-relayfor-boot="pending"] [data-page-title],html[data-relayfor-boot="pending"] [data-page-subtitle]{visibility:hidden}</style></head>')
  }
  await mkdir(path.dirname(path.join(distRoot, page)), { recursive: true })
  await writeFile(path.join(distRoot, page), html)
}
await publishNativeFeatureStyles({
  distRoot,
  overrideFile: path.join(webRoot, 'src', 'relayfor', 'native-features.css'),
})
console.log('RelayFor original pages and animations published into web/dist')
