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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { beforeEach, describe, expect, it } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import { setRelayForMotionEnabled } from '@/hooks/use-relayfor-motion'
import en from '@/i18n/locales/en.json'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { RelayForLanding } from '../components/relayfor-landing'

async function renderLanding(
  isAuthenticated: boolean,
  status: Record<string, unknown> = {}
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  client.setQueryData(STATUS_QUERY_KEY, {
    docs_link: 'https://docs.example.test/api',
    ...status,
  })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <RelayForLanding
          isAuthenticated={isAuthenticated}
          footer={<footer>Attribution</footer>}
        />
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <ThemeCustomizationProvider>
          <RouterProvider router={router} />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  window.localStorage.clear()
  delete document.documentElement.dataset.relayforMotion
  document.documentElement.setAttribute('data-relayfor-palette', 'white')
  i18next.addResourceBundle('en', 'translation', en.translation, true, true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore.getState().setConfig({
    systemName: 'Acme Gateway',
    logo: '/acme-icon.svg',
  })
})

describe('RelayFor homepage', () => {
  it('uses the configured name and icon when the status branding changes', async () => {
    await renderLanding(false)

    expect(
      screen.getByRole('link', { name: 'Back to Acme Gateway home' })
    ).toHaveTextContent('Acme Gateway')
    expect(
      screen.getByRole('img', { name: 'Acme Gateway logo' })
    ).toHaveAttribute('src', '/acme-icon.svg')
    for (const link of screen.getAllByRole('link', { name: 'Get started' })) {
      expect(link).toHaveAttribute('href', '/sign-up')
    }
    expect(document.querySelector('.rf-mobile-login')).toHaveAttribute(
      'href',
      '/sign-in'
    )
    expect(screen.getByRole('link', { name: 'Features' })).toHaveAttribute(
      'href',
      '#rf-capabilities'
    )
    expect(document.querySelector('#rf-capabilities')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Quick start' })).toHaveAttribute(
      'href',
      '#rf-quick-start'
    )
    expect(document.querySelector('#rf-quick-start')).toBeInTheDocument()

    useSystemConfigStore
      .getState()
      .setConfig({ systemName: 'Second Name', logo: '/second.svg' })
    await waitFor(() =>
      expect(
        screen.getByRole('link', { name: 'Back to Second Name home' })
      ).toHaveTextContent('Second Name')
    )
    expect(
      screen.getByRole('img', { name: 'Second Name logo' })
    ).toHaveAttribute('src', '/second.svg')
    expect(document.querySelector('.rf-flow-hub img')).toHaveAttribute(
      'src',
      '/second.svg'
    )
  })

  it('links authenticated users to New API pages and configured docs', async () => {
    await renderLanding(true)

    for (const link of screen.getAllByRole('link', {
      name: 'Open dashboard',
    })) {
      expect(link).toHaveAttribute('href', '/dashboard')
    }
    expect(
      screen.getByRole('link', { name: /API key management/ })
    ).toHaveAttribute('href', '/keys')
    expect(screen.getByRole('link', { name: /Usage records/ })).toHaveAttribute(
      'href',
      '/usage-logs'
    )
    expect(screen.queryByRole('link', { name: /Balance & billing/ })).toBeNull()
    expect(
      screen.getByRole('link', { name: /Open API documentation/ })
    ).toHaveAttribute('href', 'https://docs.example.test/api')
    expect(
      screen.getByRole('link', { name: 'Browse dashboard' })
    ).toHaveAttribute('href', '/dashboard')
    expect(
      screen.getByRole('button', { name: 'Copy API example' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('group', { name: 'Unified API connection diagram' })
    ).toBeInTheDocument()
  })

  it('uses the official docs URL when site settings contain an unsafe link', async () => {
    await renderLanding(false, { docs_link: 'javascript:alert(1)' })

    expect(
      screen.getByRole('link', { name: 'Open API documentation' })
    ).toHaveAttribute('href', 'https://docs.newapi.pro')
  })

  it.each([
    {
      condition: 'registration is closed',
      status: { register_enabled: false },
    },
    {
      condition: 'self-use mode is enabled',
      status: { self_use_mode_enabled: true },
    },
  ])('sends guests to sign-in when $condition', async ({ status }) => {
    await renderLanding(false, status)

    for (const link of screen.getAllByRole('link', { name: 'Sign in' })) {
      expect(link).toHaveAttribute('href', '/sign-in')
    }
    expect(
      screen.queryByRole('link', { name: 'Get started' })
    ).not.toBeInTheDocument()
  })

  it('keeps the public palette control visible and applies a selected color', async () => {
    await renderLanding(false)
    const user = userEvent.setup()

    expect(
      screen.getByRole('button', { name: 'Change language' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Toggle theme' })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Color palette' }))
    await user.click(screen.getByRole('menuitem', { name: 'Blue' }))

    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'blue'
    )
  })

  it('starts and clears a pointer-origin ripple when motion is enabled', async () => {
    setRelayForMotionEnabled(true)
    await renderLanding(false)
    const action = screen.getAllByRole('link', { name: 'Get started' })[0]
    action.getBoundingClientRect = () => new DOMRect(0, 0, 120, 40)

    fireEvent.pointerDown(action, {
      button: 0,
      isPrimary: true,
      clientX: 12,
      clientY: 12,
    })

    const ripple = action.querySelector('.rf-ripple')
    expect(ripple).toBeInTheDocument()
    fireEvent.animationEnd(ripple as Element)
    expect(ripple).not.toBeInTheDocument()
  })
})
