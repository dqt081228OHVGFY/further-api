import { createContext, useContext, useEffect, useLayoutEffect, useRef } from 'react'

// Internal feature routes render inside the existing Further content region.
// They keep the original route guards without mounting a second application shell.
export const NativeFeatureContext = createContext(false)
export const useNativeFeatureIsland = () => useContext(NativeFeatureContext)

export const NativeFeatureHostContext = createContext<HTMLElement | null>(null)
export type WorkspaceRefreshDetail = {
  name: string
  waitUntil: (request: Promise<unknown>) => void
}

export function useWorkspaceRefresh(refresh: () => unknown): void {
  const host = useContext(NativeFeatureHostContext)
  const current = useRef(refresh)
  useLayoutEffect(() => { current.current = refresh }, [refresh])
  useEffect(() => {
    if (!host) return
    const receive = (event: Event) => {
      const detail = (event as CustomEvent<WorkspaceRefreshDetail>).detail
      const panel = host.closest<HTMLElement>('[data-panel]')
      if (!panel?.classList.contains('active') || panel.dataset.panel !== detail?.name || host.closest('[hidden],.hidden')) return
      const request = Promise.resolve().then(() => {
        if (panel.classList.contains('active') && !host.closest('[hidden],.hidden')) return current.current()
      })
      if (typeof detail.waitUntil === 'function') detail.waitUntil(request)
      else void request.catch(() => {})
    }
    window.addEventListener('further-ui:refresh', receive)
    window.addEventListener('further-ui:page', receive)
    return () => {
      window.removeEventListener('further-ui:refresh', receive)
      window.removeEventListener('further-ui:page', receive)
    }
  }, [host])
}
