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
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  Blocks,
  Code2,
  KeyRound,
  ReceiptText,
  ShieldCheck,
} from 'lucide-react'
import { useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { LanguageSwitcher } from '@/components/language-switcher'
import { RelayForMotionSwitch } from '@/components/relayfor-motion-switch'
import { RelayForPaletteSwitch } from '@/components/relayfor-palette-switch'
import { ThemeSwitch } from '@/components/theme-switch'
import { relayForMotionEnabled } from '@/hooks/use-relayfor-motion'
import { useStatus } from '@/hooks/use-status'
import { useSystemConfig } from '@/hooks/use-system-config'

type RelayForLandingProps = {
  isAuthenticated: boolean
  footer: ReactNode
}

const RIPPLE_TARGETS = [
  '.rf-ripple-target',
  '.rf-header-actions button',
  '.rf-nav a',
  '.rf-text-link',
  '.rf-mobile-login',
].join(',')

// Same pointer origin, 560 ms easing and cleanup timing as mirror/app.js.
function addMaterialRipple(
  host: HTMLElement,
  clientX: number,
  clientY: number
) {
  if (!relayForMotionEnabled()) return
  const bounds = host.getBoundingClientRect()
  if (bounds.width <= 0 || bounds.height <= 0) return
  const x = Math.min(Math.max(clientX - bounds.left, 0), bounds.width)
  const y = Math.min(Math.max(clientY - bounds.top, 0), bounds.height)
  const radius = Math.hypot(
    Math.max(x, bounds.width - x),
    Math.max(y, bounds.height - y)
  )
  const size = Math.max(24, radius * 2)
  const ripple = document.createElement('span')
  ripple.className = 'rf-ripple'
  ripple.setAttribute('aria-hidden', 'true')
  ripple.style.width = `${size}px`
  ripple.style.height = `${size}px`
  ripple.style.left = `${x - size / 2}px`
  ripple.style.top = `${y - size / 2}px`
  host.classList.add('rf-ripple-target')
  host.appendChild(ripple)
  ripple.addEventListener('animationend', () => ripple.remove(), { once: true })
  window.setTimeout(() => ripple.remove(), 700)
}

export function RelayForLanding(props: RelayForLandingProps) {
  const { t } = useTranslation()
  const { systemName, logo } = useSystemConfig()
  const { status } = useStatus()
  const rootRef = useRef<HTMLDivElement>(null)
  const registrationAvailable =
    status?.register_enabled !== false && status?.self_use_mode_enabled !== true
  let actionRoute: '/dashboard' | '/sign-up' | '/sign-in' = '/sign-up'
  let actionLabel = t('home.relay.getStarted')
  if (props.isAuthenticated) {
    actionRoute = '/dashboard'
    actionLabel = t('home.relay.openConsole')
  } else if (!registrationAvailable) {
    actionRoute = '/sign-in'
    actionLabel = t('Sign in')
  }
  const configuredDocsLink = status?.docs_link
  const docsUrl =
    typeof configuredDocsLink === 'string' &&
    (/^https?:\/\//i.test(configuredDocsLink) ||
      (configuredDocsLink.startsWith('/') &&
        !configuredDocsLink.startsWith('//')))
      ? configuredDocsLink
      : 'https://docs.newapi.pro'
  const apiOrigin =
    typeof window === 'undefined' ? 'YOUR_BASE_URL' : window.location.origin
  const snippet = `curl ${apiOrigin}/v1/responses \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "YOUR_MODEL",\n    "input": "Hello, world."\n  }'`

  useEffect(() => {
    const root = rootRef.current
    if (!root) return
    const onPointerDown = (event: PointerEvent) => {
      if (event.button !== 0 || event.isPrimary === false) return
      if (!(event.target instanceof Element)) return
      const host = event.target.closest<HTMLElement>(RIPPLE_TARGETS)
      if (!host || host.matches(':disabled, [aria-disabled="true"]')) return
      addMaterialRipple(host, event.clientX, event.clientY)
    }
    const onKeyboardClick = (event: MouseEvent) => {
      if (event.detail !== 0 || !(event.target instanceof Element)) return
      const host = event.target.closest<HTMLElement>(RIPPLE_TARGETS)
      if (!host || host.matches(':disabled, [aria-disabled="true"]')) return
      const bounds = host.getBoundingClientRect()
      addMaterialRipple(
        host,
        bounds.left + bounds.width / 2,
        bounds.top + bounds.height / 2
      )
    }
    root.addEventListener('pointerdown', onPointerDown)
    root.addEventListener('click', onKeyboardClick, true)
    return () => {
      root.removeEventListener('pointerdown', onPointerDown)
      root.removeEventListener('click', onKeyboardClick, true)
    }
  }, [])

  const features = [
    {
      title: t('home.relay.modelsTitle'),
      description: t('home.relay.modelsDescription'),
      action: t('home.relay.viewModels'),
      to: '/pricing' as const,
      icon: Blocks,
    },
    {
      title: t('home.relay.keysTitle'),
      description: t('home.relay.keysDescription'),
      action: t('home.relay.manageKeys'),
      to: '/keys' as const,
      icon: KeyRound,
    },
    {
      title: t('home.relay.usageTitle'),
      description: t('home.relay.usageDescription'),
      action: t('home.relay.viewUsage'),
      to: '/usage-logs' as const,
      icon: ReceiptText,
    },
  ]

  return (
    <div className='relayfor-home' ref={rootRef}>
      <a className='rf-skip' href='#rf-main'>
        {t('home.relay.skipToContent')}
      </a>
      <div className='rf-shell'>
        <header className='rf-header'>
          <Link
            to='/'
            className='rf-brand'
            aria-label={t('home.relay.backHome', { name: systemName })}
          >
            <img
              className='rf-brand-mark'
              src={logo}
              alt={t('home.relay.siteLogo', { name: systemName })}
              width={44}
              height={44}
            />
            <span className='rf-brand-copy'>
              <strong>{systemName}</strong>
              <small>{t('home.relay.gateway')}</small>
            </span>
          </Link>
          <nav className='rf-nav' aria-label={t('home.relay.homeNavigation')}>
            <a href='#rf-capabilities'>{t('home.relay.features')}</a>
            <a href='#rf-quick-start'>{t('home.relay.quickStart')}</a>
            <Link to='/pricing'>{t('home.relay.modelsAndPricing')}</Link>
          </nav>
          <div className='rf-header-actions'>
            <RelayForPaletteSwitch />
            <RelayForMotionSwitch />
            <LanguageSwitcher />
            <ThemeSwitch />
            {!props.isAuthenticated && (
              <Link to='/sign-in' className='rf-login'>
                {t('Sign in')}
              </Link>
            )}
            <Link to={actionRoute} className='rf-button rf-ripple-target'>
              {actionLabel}
              <ArrowRight aria-hidden='true' size={17} />
            </Link>
          </div>
        </header>

        <main id='rf-main'>
          <section className='rf-hero' aria-labelledby='rf-hero-title'>
            <div className='rf-hero-copy'>
              <p className='rf-eyebrow'>
                <span aria-hidden='true' />
                {t('home.relay.eyebrow')}
              </p>
              <h1 id='rf-hero-title'>
                {t('home.relay.heroTitleStart')}
                <br />
                <span>{t('home.relay.heroTitleEnd')}</span>
              </h1>
              <p className='rf-lead'>{t('home.relay.heroDescription')}</p>
              <div className='rf-hero-actions'>
                <Link
                  to={actionRoute}
                  className='rf-button rf-button-large rf-ripple-target'
                >
                  {actionLabel}
                  <ArrowRight aria-hidden='true' size={18} />
                </Link>
                <a
                  href='#rf-quick-start'
                  className='rf-button rf-button-outline rf-ripple-target'
                >
                  <Code2 aria-hidden='true' size={17} />
                  {t('home.relay.viewExample')}
                </a>
                {!props.isAuthenticated && registrationAvailable && (
                  <Link to='/sign-in' className='rf-mobile-login'>
                    {t('Sign in')}
                  </Link>
                )}
              </div>
              <div className='rf-hero-notes'>
                <span>
                  <KeyRound aria-hidden='true' size={17} />
                  {t('home.relay.independentKeys')}
                </span>
                <span>
                  <ReceiptText aria-hidden='true' size={17} />
                  {t('home.relay.clearUsage')}
                </span>
              </div>
            </div>
            <div
              className='rf-connection'
              role='group'
              aria-label={t('home.relay.connectionDiagram')}
            >
              <div className='rf-connection-heading'>
                <span>{t('home.relay.nextRequest')}</span>
                <span className='rf-protocol-label'>
                  {t('home.relay.unifiedInterface')}
                </span>
              </div>
              <div className='rf-flow'>
                <div className='rf-flow-source'>
                  <Code2 aria-hidden='true' size={24} />
                  <span>{t('home.relay.yourApp')}</span>
                </div>
                <div className='rf-flow-line' aria-hidden='true' />
                <div className='rf-flow-hub'>
                  <img src={logo} alt='' width={62} height={62} />
                  <span>{systemName}</span>
                </div>
                <div className='rf-flow-line' aria-hidden='true' />
                <div className='rf-flow-targets'>
                  <span>OpenAI</span>
                  <span>Anthropic</span>
                  <span>{t('home.relay.compatibleModels')}</span>
                </div>
              </div>
              <div className='rf-request'>
                <div className='rf-request-top'>
                  <span className='rf-method'>POST</span>
                  <code>/v1/responses</code>
                  <span>{t('home.relay.requestExample')}</span>
                </div>
                <pre aria-label={t('home.relay.requestJson')}>
                  <code>
                    {
                      '{\n  "model": "YOUR_MODEL",\n  "input": "Hello, world."\n}'
                    }
                  </code>
                </pre>
                <div className='rf-request-footer'>
                  <KeyRound aria-hidden='true' size={16} />
                  <code>Authorization: Bearer YOUR_API_KEY</code>
                </div>
              </div>
              <p className='rf-connection-foot'>
                {t('home.relay.connectionFoot')}
              </p>
            </div>
          </section>

          <section
            className='rf-capabilities'
            id='rf-capabilities'
            aria-labelledby='rf-capabilities-title'
          >
            <div className='rf-section-head'>
              <div>
                <p className='rf-kicker'>
                  {t('home.relay.capabilitiesEyebrow')}
                </p>
                <h2 id='rf-capabilities-title'>
                  {t('home.relay.capabilitiesTitle')}
                </h2>
              </div>
              <Link to='/dashboard' className='rf-text-link'>
                {t('home.relay.browseConsole')}
                <ArrowRight aria-hidden='true' size={18} />
              </Link>
            </div>
            <div className='rf-feature-grid'>
              {features.map((feature) => (
                <Link
                  key={feature.to}
                  to={feature.to}
                  className='rf-feature rf-ripple-target'
                >
                  <feature.icon
                    className='rf-feature-icon'
                    aria-hidden='true'
                    size={23}
                  />
                  <h3>{feature.title}</h3>
                  <p>{feature.description}</p>
                  <span className='rf-feature-link'>
                    {feature.action}
                    <ArrowRight aria-hidden='true' size={18} />
                  </span>
                </Link>
              ))}
            </div>
          </section>

          <section
            className='rf-start'
            id='rf-quick-start'
            aria-labelledby='rf-start-title'
          >
            <div>
              <p className='rf-kicker'>{t('home.relay.startEyebrow')}</p>
              <h2 id='rf-start-title'>{t('home.relay.startTitle')}</h2>
              <ol className='rf-steps'>
                <li>
                  <span>01</span>
                  <div>
                    <h3>{t('home.relay.stepAccountTitle')}</h3>
                    <p>{t('home.relay.stepAccountDescription')}</p>
                  </div>
                </li>
                <li>
                  <span>02</span>
                  <div>
                    <h3>{t('home.relay.stepKeyTitle')}</h3>
                    <p>{t('home.relay.stepKeyDescription')}</p>
                  </div>
                </li>
                <li>
                  <span>03</span>
                  <div>
                    <h3>{t('home.relay.stepRequestTitle')}</h3>
                    <p>{t('home.relay.stepRequestDescription')}</p>
                  </div>
                </li>
              </ol>
              <a
                className='rf-text-link'
                href={docsUrl}
                target={docsUrl.startsWith('http') ? '_blank' : undefined}
                rel='noreferrer'
              >
                {t('home.relay.openDocs')}
                <ArrowRight aria-hidden='true' size={18} />
              </a>
            </div>
            <div className='rf-code-panel'>
              <div className='rf-code-toolbar'>
                <span>
                  <Code2 aria-hidden='true' size={18} />
                  cURL · Responses
                </span>
                <CopyButton
                  value={snippet}
                  variant='ghost'
                  size='sm'
                  className='rf-copy rf-ripple-target'
                  aria-label={t('home.relay.copyExample')}
                >
                  {t('home.relay.copy')}
                </CopyButton>
              </div>
              <pre tabIndex={0} aria-label={t('home.relay.curlExample')}>
                <code>{snippet}</code>
              </pre>
              <div className='rf-code-note'>
                <ShieldCheck aria-hidden='true' size={18} />
                <p>{t('home.relay.codeNote')}</p>
              </div>
            </div>
          </section>

          <section className='rf-closing' aria-labelledby='rf-closing-title'>
            <div>
              <span className='rf-kicker'>
                {t('home.relay.closingEyebrow')}
              </span>
              <h2 id='rf-closing-title'>{t('home.relay.closingTitle')}</h2>
              <p>{t('home.relay.closingDescription')}</p>
            </div>
            <Link to={actionRoute} className='rf-button rf-ripple-target'>
              {actionLabel}
              <ArrowRight aria-hidden='true' size={18} />
            </Link>
          </section>
        </main>
      </div>
      {props.footer}
    </div>
  )
}
