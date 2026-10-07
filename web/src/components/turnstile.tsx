/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useRef } from 'react'

declare global {
  interface Window {
    turnstile?: {
      render: (element: HTMLElement, options: Record<string, unknown>) => string
      remove?: (widgetId: string) => void
    }
  }
}

interface TurnstileProps {
  siteKey: string
  onVerify: (token: string) => void
  onExpire?: () => void
  className?: string
}

export function Turnstile({
  siteKey,
  onVerify,
  onExpire,
  className,
}: TurnstileProps) {
  const ref = useRef<HTMLDivElement | null>(null)
  const callbacks = useRef({ onVerify, onExpire })

  useEffect(() => {
    callbacks.current = { onVerify, onExpire }
  }, [onVerify, onExpire])

  useEffect(() => {
    const container = ref.current
    let active = true
    let widgetId: string | undefined
    let script: HTMLScriptElement | null = null
    const render = () => {
      if (!active || !container || !window.turnstile || widgetId !== undefined) return
      try {
        widgetId = window.turnstile.render(container, {
          sitekey: siteKey,
          callback: (token: string) => { if (active) callbacks.current.onVerify(token) },
          'error-callback': () => { if (active) callbacks.current.onExpire?.() },
          'expired-callback': () => { if (active) callbacks.current.onExpire?.() },
        })
      } catch {
        /* empty */
      }
    }

    if (window.turnstile) {
      render()
    } else {
      script = document.querySelector<HTMLScriptElement>('#cf-turnstile')
      if (!script) {
        script = document.createElement('script')
        script.id = 'cf-turnstile'
        script.src =
          'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
        script.async = true
        script.defer = true
      }
      script.addEventListener('load', render, { once: true })
      if (!script.isConnected) document.head.append(script)
    }
    return () => {
      active = false
      script?.removeEventListener('load', render)
      if (widgetId !== undefined) window.turnstile?.remove?.(widgetId)
    }
  }, [siteKey])

  return <div ref={ref} className={className} />
}
