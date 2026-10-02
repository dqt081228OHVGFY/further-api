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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  ThemeCustomizationProvider,
  useThemeCustomization,
} from '@/context/theme-customization-provider'
import { ThemeProvider, useTheme } from '@/context/theme-provider'
import { initializeFrontendCache } from '@/lib/frontend-cache'

const savedPreferences = {
  'newapi:theme:v1:mode': 'dark',
  'newapi:theme:v1:relayfor-palette': 'yellow',
  'newapi:theme:v1:preset': 'rose-garden',
  'newapi:theme:v1:font': 'serif',
  'newapi:theme:v1:radius': 'lg',
  'newapi:theme:v1:scale': 'sm',
  'newapi:theme:v1:content-layout': 'centered',
}

function ThemeControls() {
  const theme = useTheme()
  const customization = useThemeCustomization()

  return (
    <>
      <output aria-label='Theme mode'>{theme.theme}</output>
      <output aria-label='RelayFor palette'>
        {customization.customization.palette}
      </output>
      <button
        type='button'
        onClick={() => {
          theme.setTheme('dark')
          customization.setPalette('yellow')
          customization.setPreset('rose-garden')
          customization.setFont('serif')
          customization.setRadius('lg')
          customization.setScale('sm')
          customization.setContentLayout('centered')
        }}
      >
        Customize
      </button>
      <button type='button' onClick={() => customization.setPalette('blue')}>
        Select blue
      </button>
      <button
        type='button'
        onClick={() => {
          theme.resetTheme()
          customization.resetCustomization()
        }}
      >
        Reset
      </button>
    </>
  )
}

function ThemeFixture() {
  return (
    <ThemeProvider>
      <ThemeCustomizationProvider>
        <ThemeControls />
      </ThemeCustomizationProvider>
    </ThemeProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  localStorage.clear()
  for (const cookie of document.cookie.split(';')) {
    const name = cookie.trim().split('=')[0]
    document.cookie = `${name}=; path=/; max-age=0`
  }
  document.documentElement.classList.remove('light', 'dark')
  document.documentElement.removeAttribute('data-relayfor-palette')
  for (const name of document.body.getAttributeNames()) {
    if (name.startsWith('data-theme-')) document.body.removeAttribute(name)
  }
})

describe('theme preference persistence', () => {
  it('starts with defaults when only shared legacy theme cookies exist', () => {
    document.cookie = 'theme_preset=ocean-breeze; path=/'
    document.cookie = 'vite-ui-theme=dark; path=/'
    document.cookie = 'theme_font=serif; path=/'
    document.cookie = 'theme_radius=xl; path=/'
    document.cookie = 'theme_scale=lg; path=/'
    document.cookie = 'theme_content_layout=centered; path=/'

    render(<ThemeFixture />)

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('light')
    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent('white')
    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'white'
    )
    expect(document.documentElement).toHaveClass('light')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
    expect(document.body).toHaveAttribute('data-theme-font', 'sans')
    expect(document.body).not.toHaveAttribute('data-theme-radius')
    expect(document.body).not.toHaveAttribute('data-theme-scale')
    expect(document.body).toHaveAttribute('data-theme-content-layout', 'full')
  })

  it('restores all customized preferences after remounting without writing cookies', async () => {
    const user = userEvent.setup()
    const first = render(<ThemeFixture />)

    await user.click(screen.getByRole('button', { name: 'Customize' }))
    first.unmount()
    render(<ThemeFixture />)

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('dark')
    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent(
      'yellow'
    )
    expect(document.documentElement).toHaveClass('dark')
    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'yellow'
    )
    expect(document.body).toHaveAttribute('data-theme-preset', 'rose-garden')
    expect(document.body).toHaveAttribute('data-theme-font', 'serif')
    expect(document.body).toHaveAttribute('data-theme-radius', 'lg')
    expect(document.body).toHaveAttribute('data-theme-scale', 'sm')
    expect(document.body).toHaveAttribute(
      'data-theme-content-layout',
      'centered'
    )
    for (const [key, value] of Object.entries(savedPreferences)) {
      expect(localStorage.getItem(key)).toBe(value)
    }
    expect(document.cookie).toBe('')
  })

  it('keeps a reset after remounting even when the legacy ocean cookie remains', async () => {
    for (const [key, value] of Object.entries(savedPreferences)) {
      localStorage.setItem(key, value)
    }
    localStorage.setItem('unrelated-preference', 'keep')
    document.cookie = 'theme_preset=ocean-breeze; path=/'
    const user = userEvent.setup()
    const first = render(<ThemeFixture />)

    await user.click(screen.getByRole('button', { name: 'Reset' }))
    first.unmount()
    render(<ThemeFixture />)

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('light')
    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent('white')
    expect(document.documentElement).toHaveClass('light')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
    expect(document.body).toHaveAttribute('data-theme-font', 'sans')
    expect(document.body).not.toHaveAttribute('data-theme-radius')
    expect(document.body).not.toHaveAttribute('data-theme-scale')
    expect(document.body).toHaveAttribute('data-theme-content-layout', 'full')
    for (const key of Object.keys(savedPreferences)) {
      expect(localStorage.getItem(key)).toBeNull()
    }
    expect(localStorage.getItem('unrelated-preference')).toBe('keep')
    expect(document.cookie).toContain('theme_preset=ocean-breeze')
  })

  it.each(['', 'unknown'])(
    'uses defaults when stored preferences are %j',
    (value) => {
      for (const key of Object.keys(savedPreferences)) {
        localStorage.setItem(key, value)
      }

      render(<ThemeFixture />)

      expect(screen.getByLabelText('Theme mode')).toHaveTextContent('light')
      expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent(
        'white'
      )
      expect(document.body).not.toHaveAttribute('data-theme-preset')
      expect(document.body).toHaveAttribute('data-theme-font', 'sans')
      expect(document.body).not.toHaveAttribute('data-theme-radius')
      expect(document.body).not.toHaveAttribute('data-theme-scale')
      expect(document.body).toHaveAttribute('data-theme-content-layout', 'full')
    }
  )

  it('renders defaults when reading local storage fails', () => {
    vi.spyOn(localStorage, 'getItem').mockImplementation(() => {
      throw new DOMException('Storage unavailable', 'SecurityError')
    })

    render(<ThemeFixture />)

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('light')
    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent('white')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
  })

  it('still applies and resets preferences when storage writes fail', async () => {
    vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
      throw new DOMException('Storage full', 'QuotaExceededError')
    })
    vi.spyOn(localStorage, 'removeItem').mockImplementation(() => {
      throw new DOMException('Storage unavailable', 'SecurityError')
    })
    const user = userEvent.setup()
    render(<ThemeFixture />)

    await user.click(screen.getByRole('button', { name: 'Customize' }))

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('dark')
    expect(document.body).toHaveAttribute('data-theme-preset', 'rose-garden')

    await user.click(screen.getByRole('button', { name: 'Reset' }))

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('light')
    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent('white')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
  })

  it('preserves saved theme preferences during frontend cache initialization', () => {
    for (const [key, value] of Object.entries(savedPreferences)) {
      localStorage.setItem(key, value)
    }
    localStorage.setItem('stale-ui-cache', 'old')

    initializeFrontendCache()
    initializeFrontendCache()
    render(<ThemeFixture />)

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('dark')
    expect(document.body).toHaveAttribute('data-theme-preset', 'rose-garden')
    for (const [key, value] of Object.entries(savedPreferences)) {
      expect(localStorage.getItem(key)).toBe(value)
    }
    expect(localStorage.getItem('stale-ui-cache')).toBeNull()
  })

  it('switches the RelayFor palette without changing mode or the New API preset', async () => {
    const user = userEvent.setup()
    const first = render(<ThemeFixture />)

    await user.click(screen.getByRole('button', { name: 'Select blue' }))

    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'blue'
    )
    expect(localStorage.getItem('newapi:theme:v1:relayfor-palette')).toBe(
      'blue'
    )
    expect(localStorage.getItem('newapi:theme:v1:preset')).toBeNull()
    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('light')

    first.unmount()
    render(<ThemeFixture />)

    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent('blue')
    expect(document.body).not.toHaveAttribute('data-theme-preset')
  })

  it('keeps an existing system mode while restoring the independent palette', () => {
    localStorage.setItem('newapi:theme:v1:mode', 'system')
    localStorage.setItem('newapi:theme:v1:relayfor-palette', 'yellow')

    render(<ThemeFixture />)

    expect(screen.getByLabelText('Theme mode')).toHaveTextContent('system')
    expect(screen.getByLabelText('RelayFor palette')).toHaveTextContent(
      'yellow'
    )
    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'yellow'
    )
  })
})
