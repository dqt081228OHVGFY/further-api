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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RelayForMotionSwitch } from '@/components/relayfor-motion-switch'

import {
  relayForMotionEnabled,
  setRelayForMotionEnabled,
} from '../use-relayfor-motion'

afterEach(() => {
  window.localStorage.removeItem('relayfor-motion')
  delete document.documentElement.dataset.relayforMotion
  vi.unstubAllGlobals()
})

describe('RelayFor motion preference', () => {
  it('enables animations by default when the system has no reduced-motion preference', () => {
    const originalMatchMedia = window.matchMedia
    vi.stubGlobal('matchMedia', (query: string) => ({
      ...originalMatchMedia(query),
      matches: false,
    }))

    expect(relayForMotionEnabled()).toBe(true)
  })

  it('respects a reduced-motion system setting until the user enables motion', () => {
    expect(relayForMotionEnabled()).toBe(false)

    setRelayForMotionEnabled(true)

    expect(relayForMotionEnabled()).toBe(true)
    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-motion',
      'on'
    )
    expect(window.localStorage.getItem('relayfor-motion')).toBe('on')
  })

  it('lets the user disable motion again with an accessible toggle', async () => {
    setRelayForMotionEnabled(true)
    render(<RelayForMotionSwitch />)
    const user = userEvent.setup()

    const toggle = screen.getByRole('button', { name: 'Disable animations' })
    expect(toggle).toHaveAttribute('aria-pressed', 'true')
    await user.click(toggle)

    expect(
      screen.getByRole('button', { name: 'Enable animations' })
    ).toHaveAttribute('aria-pressed', 'false')
    expect(document.documentElement).toHaveAttribute(
      'data-relayfor-motion',
      'off'
    )
  })
})
