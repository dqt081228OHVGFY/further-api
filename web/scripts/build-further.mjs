import { cp, mkdir, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { publishNativeFeatureStyles } from './native-features-css.mjs'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const sourceRoot = path.resolve(webRoot, '../further-ui')
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
const versionAssetUrls = (value) => value.replaceAll(/\/further-assets\/[\w./-]+/g, (url) => `${url}?v=${assetVersion}`)
const runtimePage = await readFile(path.join(distRoot, 'further-bridge.html'), 'utf8')
const runtimeScripts = [...runtimePage.matchAll(/<script\b[^>]*\bsrc=["'][^"']+["'][^>]*><\/script>/g)].map((match) => match[0]).join('\n')
if (!runtimeScripts.includes('further-bridge')) throw new Error('Further runtime entry was not emitted')

// Publish the independent application entry for public routes, OAuth and setup.
// Every HTML entry includes it; main.tsx only mounts on the application document.
const generatedIndex = await readFile(path.join(distRoot, 'index.html'), 'utf8')
const applicationScripts = [...generatedIndex.matchAll(/<script\b[^>]*\bsrc=["'][^"']+["'][^>]*><\/script>/g)].map((match) => match[0]).join('\n')
const applicationStyles = [...generatedIndex.matchAll(/<link\b[^>]*\bhref=["']([^"']+\.css(?:\?[^"']*)?)["'][^>]*>/g)].map((match) => match[1])
const scripts = [...new Map([...runtimeScripts.matchAll(/<script\b[^>]*\bsrc=["']([^"']+)["'][^>]*><\/script>/g), ...applicationScripts.matchAll(/<script\b[^>]*\bsrc=["']([^"']+)["'][^>]*><\/script>/g)].map((match) => [match[1], match[0]])).values()].join('\n')
if (!applicationScripts) throw new Error('further-api application entry was not emitted')
await rm(path.join(distRoot, 'gateway.html'), { force: true })
await rm(path.join(distRoot, 'further-bridge.html'), { force: true })
await cp(path.join(sourceRoot, 'assets'), path.join(distRoot, 'further-assets'), { recursive: true })
const cssFile = path.join(distRoot, 'further-assets', 'app.css')
await writeFile(cssFile, versionAssetUrls(await readFile(cssFile, 'utf8')))
// Publish the gateway workspace documents and their shared application runtime.
for (const page of ['index.html', 'auth/index.html', 'console/index.html', 'admin/index.html', 'prepaid/index.html', 'welcome/index.html']) {
  let html
  try {
    html = await readFile(path.join(sourceRoot, page), 'utf8')
  } catch (error) {
    if (page === 'prepaid/index.html' && error.code === 'ENOENT') continue
    throw error
  }
  const preferenceScript = await readFile(path.join(assetRoot, 'theme-preference.js'), 'utf8')
  html = versionAssetUrls(html)
  const workspacePreloads = [...html.matchAll(/<script\b[^>]*data-further-source-only[^>]*\bsrc=["']([^"']+)["'][^>]*>/g)]
    .map((match) => `<link rel="preload" as="script" href="${match[1]}">`).join('\n')
  html = html.replace('<head>', `<head><script>${preferenceScript}</script>${workspacePreloads}`).replace('</head>', `<meta name="further-asset-version" content="${assetVersion}"><meta name="further-app-styles" content="${applicationStyles.join(',')}">${scripts}\n</head>`)
  if (page === 'console/index.html' || page === 'admin/index.html') {
    // Avoid painting the document's default overview before the hash-selected
    // panel and its handlers have mounted. Keep the already-themed shell visible.
    html = html.replace('<html', '<html data-further-boot="pending"')
    html = html.replace('</head>', '<style>html[data-further-boot="pending"] .view{visibility:hidden}</style></head>')
  }
  await mkdir(path.dirname(path.join(distRoot, page)), { recursive: true })
  await writeFile(path.join(distRoot, page), html)
}
await publishNativeFeatureStyles({
  distRoot,
  overrideFile: path.join(webRoot, 'src', 'further', 'native-features.css'),
})
console.log('further-api application and workspace documents published into web/dist')
