import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const pendingImages: HTMLImageElement[] = []

beforeEach(() => {
  vi.resetModules()
  pendingImages.length = 0
  document.head.innerHTML = '<link rel="icon" type="image/svg+xml" href="/relayfor-logo.svg">'
  document.body.innerHTML = '<span class="rail-logo"><img src="/relayfor-logo.svg" alt=""></span>'
  vi.stubGlobal('Image', function () {
    const image = document.createElement('img')
    pendingImages.push(image)
    return image
  })
})

afterEach(() => { vi.unstubAllGlobals() })

describe('site branding during failures and repeated initialization', () => {
  it.each(['index.html', 'auth/index.html', 'console/index.html', 'admin/index.html', 'prepaid/index.html'])(
    'renders a logo and favicon before any request or script runs in %s',
    (page) => {
      const html = readFileSync(resolve(process.cwd(), '../relayfor-ui', page), 'utf8')
      const parsed = new DOMParser().parseFromString(html, 'text/html')
      expect(parsed.querySelector('.rail-logo img')?.getAttribute('src')).toBe('/relayfor-logo.svg')
      expect(parsed.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe('/relayfor-logo.svg')
    }
  )

  it('keeps the visible mark until a custom image loads and reuses the existing image', async () => {
    const { applySiteLogoToDom } = await import('../site-logo')
    const original = document.querySelector('.rail-logo img')
    const first = applySiteLogoToDom('/custom-logo.svg')
    const repeated = applySiteLogoToDom('/custom-logo.svg')
    expect(pendingImages).toHaveLength(1)
    expect(original).toHaveAttribute('src', '/relayfor-logo.svg')
    pendingImages[0].dispatchEvent(new Event('load'))
    await Promise.all([first, repeated])
    expect(document.querySelector('.rail-logo img')).toBe(original)
    expect(original).toHaveAttribute('src', '/custom-logo.svg')
    expect(document.querySelectorAll('link[rel~="icon"]')).toHaveLength(1)
    expect(document.querySelector('link[rel="icon"]')).toHaveAttribute('href', '/custom-logo.svg')
  })

  it('falls back on a broken custom URL and shares the failed load across consumers', async () => {
    const { applySiteLogoToDom, loadSiteLogo } = await import('../site-logo')
    const update = applySiteLogoToDom('/broken.svg')
    const reactConsumer = loadSiteLogo('/broken.svg')
    expect(pendingImages).toHaveLength(1)
    pendingImages[0].dispatchEvent(new Event('error'))
    await update
    expect(await reactConsumer).toBe('/relayfor-logo.svg')
    expect(document.querySelector('.rail-logo img')).toHaveAttribute('src', '/relayfor-logo.svg')
    expect(document.querySelector('link[rel="icon"]')).toHaveAttribute('href', '/relayfor-logo.svg')
    await applySiteLogoToDom('/broken.svg')
    expect(pendingImages).toHaveLength(1)
  })

  it('ignores an older image that completes after a newer branding change', async () => {
    const { applySiteLogoToDom } = await import('../site-logo')
    const older = applySiteLogoToDom('/old.svg')
    const newer = applySiteLogoToDom('/new.svg')
    pendingImages[1].dispatchEvent(new Event('load'))
    await newer
    pendingImages[0].dispatchEvent(new Event('load'))
    await older
    expect(document.querySelector('.rail-logo img')).toHaveAttribute('src', '/new.svg')
    expect(document.querySelector('link[rel="icon"]')).toHaveAttribute('href', '/new.svg')
  })
})
