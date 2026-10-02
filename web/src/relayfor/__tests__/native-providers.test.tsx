import { render, fireEvent, screen } from '@testing-library/react'
import { useContext } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { DirectionProvider } from '@/context/direction-provider'
import { FontProvider } from '@/context/font-provider'
import { SearchContext } from '@/context/search-context'
import { SearchProvider } from '@/context/search-provider'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'

vi.mock('@/components/command-menu', () => ({
  CommandMenu: () => <div>Feature command menu</div>,
}))

afterEach(() => {
  document.documentElement.removeAttribute('data-theme')
  document.documentElement.removeAttribute('data-relayfor-palette')
  document.documentElement.removeAttribute('dir')
  document.documentElement.className = ''
})

describe('native feature providers', () => {
  it('preserves the outer RelayFor theme, font, direction and layout while mounting content', () => {
    document.documentElement.className = 'relayfor-shell'
    document.documentElement.dataset.theme = 'dark'
    document.documentElement.dataset.relayforPalette = 'yellow'
    document.documentElement.setAttribute('dir', 'rtl')
    const before = document.documentElement.attributes
    const rootAttributes = Array.from(before, (attribute) => [
      attribute.name,
      attribute.value,
    ])
    const bodyAttributes = Array.from(document.body.attributes, (attribute) => [
      attribute.name,
      attribute.value,
    ])
    render(
      <ThemeProvider isolated>
        <FontProvider isolated>
          <DirectionProvider isolated>
            <ThemeCustomizationProvider isolated>
              <p>Account settings</p>
            </ThemeCustomizationProvider>
          </DirectionProvider>
        </FontProvider>
      </ThemeProvider>
    )
    expect(screen.getByText('Account settings')).toBeInTheDocument()
    expect(
      Array.from(document.documentElement.attributes, (attribute) => [
        attribute.name,
        attribute.value,
      ])
    ).toEqual(rootAttributes)
    expect(
      Array.from(document.body.attributes, (attribute) => [
        attribute.name,
        attribute.value,
      ])
    ).toEqual(bodyAttributes)
  })

  it('leaves Ctrl K to the outer shell when a feature only needs search context', () => {
    function State() {
      const state = useContext(SearchContext)
      return (
        <p>{state?.open ? 'Feature search open' : 'Feature search closed'}</p>
      )
    }
    render(
      <SearchProvider globalShortcut={false}>
        <State />
      </SearchProvider>
    )
    fireEvent.keyDown(document, { key: 'k', ctrlKey: true })
    expect(screen.getByText('Feature search closed')).toBeInTheDocument()
    expect(screen.queryByText('Feature command menu')).not.toBeInTheDocument()
  })
})
