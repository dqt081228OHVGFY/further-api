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

import { afterEach, describe, expect, it, vi } from 'vitest'

import { clearWalletHistoryRequest } from './history-url'

afterEach(() => {
  vi.restoreAllMocks()
  window.history.replaceState(null, '', '/')
})

describe('wallet history request URL', () => {
  it('preserves the outer native feature hash and browser history state', () => {
    const hash = '#native/%2Fwallet%3Fshow_history%3Dtrue'
    window.history.replaceState({ shell: 'console' }, '', `/console/${hash}`)
    const replace = vi.spyOn(window.history, 'replaceState')

    clearWalletHistoryRequest(true)

    expect(replace).not.toHaveBeenCalled()
    expect(window.location.hash).toBe(hash)
    expect(window.history.state).toEqual({ shell: 'console' })
  })

  it('consumes only show_history on the standalone route', () => {
    window.history.replaceState(
      { origin: 'wallet' },
      '',
      '/wallet?show_history=true&tab=payments#details'
    )

    clearWalletHistoryRequest(false)

    expect(window.location.pathname + window.location.search + window.location.hash)
      .toBe('/wallet?tab=payments#details')
    expect(window.history.state).toEqual({ origin: 'wallet' })
  })
})
