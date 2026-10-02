import { DEFAULT_LOGO } from '@/lib/constants'
import { applyFaviconToDom } from '@/lib/dom-utils'

// Share successful and failed loads across the bridge and React consumers.
// A failed custom logo resolves to the bundled mark, never a broken image URL.
const logoLoads = new Map<string, Promise<string>>()
let requestedDomLogo = DEFAULT_LOGO

export function loadSiteLogo(value: string): Promise<string> {
  const url = value.trim() || DEFAULT_LOGO
  if (url === DEFAULT_LOGO) return Promise.resolve(DEFAULT_LOGO)
  const existing = logoLoads.get(url)
  if (existing) return existing
  const pending = new Promise<string>((resolve) => {
    const image = new Image()
    image.onload = () => resolve(url)
    image.onerror = () => resolve(DEFAULT_LOGO)
    image.src = url
  })
  logoLoads.set(url, pending)
  return pending
}

export async function applySiteLogoToDom(value: string): Promise<void> {
  const requested = value.trim() || DEFAULT_LOGO
  requestedDomLogo = requested
  const resolved = await loadSiteLogo(requested)
  // Ignore an older image that finishes after the administrator changes the URL.
  if (requestedDomLogo !== requested) return
  document.querySelectorAll<HTMLElement>('.rail-logo').forEach((element) => {
    let image = element.querySelector('img')
    if (!image) {
      image = document.createElement('img')
      image.alt = ''
      image.width = 40
      image.height = 40
      element.replaceChildren(image)
    }
    if (image.getAttribute('src') !== resolved) image.src = resolved
    image.onerror = () => {
      if (image.getAttribute('src') !== DEFAULT_LOGO) image.src = DEFAULT_LOGO
    }
  })
  applyFaviconToDom(resolved)
}
