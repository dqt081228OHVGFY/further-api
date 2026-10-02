import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RelayForPaletteSwitch } from '@/components/relayfor-palette-switch'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (value: string) => value }),
}))

afterEach(() => {
  cleanup()
  localStorage.clear()
  document.documentElement.classList.remove('light', 'dark')
  document.documentElement.removeAttribute('data-relayfor-palette')
  document.body.removeAttribute('data-theme-preset')
})

describe('RelayForPaletteSwitch', () => {
  it('offers white, blue, and yellow without modifying the New API preset', async () => {
    const user = userEvent.setup()
    localStorage.setItem('newapi:theme:v1:preset', 'rose-garden')
    render(
      <ThemeProvider>
        <ThemeCustomizationProvider>
          <RelayForPaletteSwitch />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )

    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'white'
    )
    await user.click(screen.getByRole('button', { name: 'Color palette' }))
    await user.click(screen.getByRole('menuitem', { name: 'Blue' }))

    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'blue'
    )
    expect(localStorage.getItem('newapi:theme:v1:relayfor-palette')).toBe(
      'blue'
    )
    expect(localStorage.getItem('newapi:theme:v1:preset')).toBe('rose-garden')
    expect(document.body).toHaveAttribute('data-theme-preset', 'rose-garden')

    await user.click(screen.getByRole('button', { name: 'Color palette' }))
    await user.click(screen.getByRole('menuitem', { name: 'Yellow' }))
    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-palette',
      'yellow'
    )
  })
})
