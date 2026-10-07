import { createHash } from 'node:crypto'
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises'
import path from 'node:path'

import postcss from 'postcss'
import selectorParser from 'postcss-selector-parser'

const rootSelector = selectorParser().astSync(':is([data-native-feature-root],[data-native-feature-portal])').first.first

function isDocumentRoot(node) {
  return (node.type === 'pseudo' && node.value === ':root') ||
    (node.type === 'tag' && node.value.toLowerCase() === 'html')
}

function rewriteCompoundAsHost(nodes, marker) {
  const first = nodes[0]
  const host = selectorParser.pseudo({ value: ':host' })
  const argument = selectorParser.selector()
  const pseudoElements = []

  for (const node of nodes) {
    if (node === marker && marker.type !== 'class') continue
    if (node.type === 'pseudo' && node.value.startsWith('::')) pseudoElements.push(node.clone())
    else argument.append(node.clone())
  }
  if (argument.nodes.length > 0) host.append(argument)
  first.replaceWith(host)
  for (const node of nodes.slice(1)) node.remove()
  for (const pseudoElement of pseudoElements) host.parent.insertAfter(host, pseudoElement)
}

function rewriteSelector(selector) {
  // Tailwind's dark utilities use :is(.dark *); the host is their ancestor
  // after the React tree moves into the shadow, so rewrite those inner lists.
  for (const node of selector.nodes) {
    if (node.type === 'pseudo' && (node.value === ':is' || node.value === ':where')) {
      for (const nested of node.nodes || []) rewriteSelector(nested)
    }
  }

  const compounds = []
  let compound = []
  for (const node of selector.nodes) {
    if (node.type === 'combinator') {
      if (compound.length > 0) compounds.push(compound)
      compound = []
    } else compound.push(node)
  }
  if (compound.length > 0) compounds.push(compound)

  for (const [index, nodes] of compounds.entries()) {
    const documentRoot = nodes.find(isDocumentRoot)
    const darkRoot = index === 0 && nodes[0]?.type === 'class' && nodes[0].value === 'dark' &&
      (compounds.length > 1 || selector.parent?.type === 'root')
    if (documentRoot || darkRoot) {
      rewriteCompoundAsHost(nodes, documentRoot || nodes[0])
      continue
    }
    for (const node of nodes) {
      if (node.type === 'tag' && node.value.toLowerCase() === 'body') node.replaceWith(rootSelector.clone())
    }
  }
}

function absoluteCssUrls(root, sourceFile, distRoot) {
  root.walkDecls((declaration) => {
    if (!declaration.value.includes('url(')) return
    declaration.value = declaration.value.replaceAll(/url\(\s*(['"]?)([^'"()]+)\1\s*\)/g, (match, quote, rawUrl) => {
      const url = rawUrl.trim()
      if (/^(?:\/|#|data:|[a-z][a-z\d+.-]*:)/i.test(url)) return match
      const suffixStart = url.search(/[?#]/)
      const pathname = suffixStart < 0 ? url : url.slice(0, suffixStart)
      const suffix = suffixStart < 0 ? '' : url.slice(suffixStart)
      const absolute = `/${path.relative(distRoot, path.resolve(path.dirname(sourceFile), pathname)).replaceAll('\\', '/')}`
      return `url(${quote}${absolute}${suffix}${quote})`
    })
  })
}

export function transformNativeFeatureCss(css, { sourceFile, distRoot } = {}) {
  const root = postcss.parse(css, { from: sourceFile })
  // Embedded vendor styles, including toast loaders, follow the same full
  // motion policy as the workspace without changing installed dependencies.
  root.walkAtRules('media', (rule) => {
    const preference = rule.params.match(/^\(\s*prefers-reduced-motion\s*(?::\s*(reduce|no-preference)\s*)?\)$/i)
    if (!preference) return
    if (preference[1]?.toLowerCase() === 'no-preference') rule.replaceWith(...rule.nodes)
    else rule.remove()
  })
  root.walkRules((rule) => {
    // Preserve animation steps and all other at-rules exactly as emitted.
    if (rule.parent?.type === 'atrule' && rule.parent.name.endsWith('keyframes')) return
    rule.selector = selectorParser((selectors) => {
      for (const selector of selectors.nodes) rewriteSelector(selector)
    }).processSync(rule.selector)
  })
  if (sourceFile && distRoot) absoluteCssUrls(root, sourceFile, distRoot)
  return root.toString()
}

async function listCssFiles(directory) {
  const result = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const file = path.join(directory, entry.name)
    if (entry.isDirectory()) result.push(...await listCssFiles(file))
    else if (entry.isFile() && entry.name.endsWith('.css')) result.push(file)
  }
  return result
}

export async function publishNativeFeatureStyles({ distRoot, overrideFile }) {
  const cssRoot = path.join(distRoot, 'static', 'css')
  const cssFiles = (await listCssFiles(cssRoot)).sort((a, b) => {
    // The main stylesheet defines the Tailwind layer order and theme tokens.
    const aMain = path.dirname(a) === cssRoot ? 0 : 1
    const bMain = path.dirname(b) === cssRoot ? 0 : 1
    return aMain - bMain || a.localeCompare(b)
  })
  if (cssFiles.length === 0) throw new Error('Native features require compiled React CSS')

  const parts = []
  const foundations = []
  for (const file of cssFiles) {
    const transformed = transformNativeFeatureCss(await readFile(file, 'utf8'), { sourceFile: file, distRoot })
    parts.push(transformed)
    postcss.parse(transformed).walkAtRules((rule) => {
      if (rule.name === 'font-face' || rule.name === 'property') foundations.push(rule.toString())
    })
  }
  parts.push(await readFile(overrideFile, 'utf8'))
  const css = `${parts.join('\n')}\n`
  const foundationCss = `${foundations.join('\n')}\n`
  const version = createHash('sha256').update(css).digest('hex').slice(0, 16)
  const foundationVersion = createHash('sha256').update(foundationCss).digest('hex').slice(0, 16)
  const outputRoot = path.join(distRoot, 'further-assets')
  await mkdir(outputRoot, { recursive: true })
  await writeFile(path.join(outputRoot, 'native-features.css'), css)
  await writeFile(path.join(outputRoot, 'native-features-foundations.css'), foundationCss)
  const manifest = {
    styles: [`/further-assets/native-features.css?v=${version}`],
    globals: [`/further-assets/native-features-foundations.css?v=${foundationVersion}`],
  }
  await writeFile(path.join(outputRoot, 'native-features-manifest.json'), JSON.stringify(manifest))
  return { cssFiles, manifest }
}
