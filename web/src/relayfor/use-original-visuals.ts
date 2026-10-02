import { useEffect } from 'react'

import { ensureWorkspaceColors } from './seasonal-theme'

let originalMotionPromise: Promise<void> | undefined

export function useOriginalRelayForVisuals(enabled = true): void {
  useEffect(() => {
    if (!enabled) return
    const version = document.querySelector<HTMLMetaElement>(
      'meta[name="relayfor-asset-version"]'
    )?.content
    const assetURL = (asset: string) =>
      version ? `${asset}?v=${version}` : asset
    for (const href of [
      '/relayfor-assets/app.css',
      '/relayfor-assets/gateway-theme.css',
      '/relayfor-assets/material.css',
    ]) {
      if (document.querySelector(`link[href="${assetURL(href)}"]`)) continue
      const link = document.createElement('link')
      link.rel = 'stylesheet'
      link.href = assetURL(href)
      document.head.append(link)
    }
    // Keep shared colors last when the legacy auth surface is mounted later.
    document.querySelector('link[data-relayfor-workspace-colors]')?.remove()
    ensureWorkspaceColors()
    originalMotionPromise ??= new Promise<void>((resolve, reject) => {
      const script = document.createElement('script')
      script.src = assetURL('/relayfor-assets/app.js')
      script.onload = () => resolve()
      script.onerror = () =>
        reject(new Error('Original RelayFor motion could not load'))
      document.head.append(script)
    })
    void originalMotionPromise
      .then(() => {
        const runtime = (
          window as Window & { ProxyLan?: { initializeVisuals: () => void } }
        ).ProxyLan
        runtime?.initializeVisuals()
      })
      .catch(() => {
        /* Page controls remain usable if a visual asset fails. */
      })
  }, [enabled])
}
