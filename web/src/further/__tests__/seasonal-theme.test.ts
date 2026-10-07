import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { applyWorkspaceSeason, getWorkspacePalettePreference, selectWorkspacePalette } from '../appearance-preferences'
import { ensureWorkspaceColors, watchSeasonalTheme } from '../seasonal-theme'

let stop: (() => void) | undefined
beforeEach(() => {
  selectWorkspacePalette('blue')
  applyWorkspaceSeason('')
})
afterEach(() => {
  stop?.()
  stop = undefined
  document
    .querySelectorAll('[data-further-workspace-colors]')
    .forEach((node) => node.remove())
  delete document.documentElement.dataset.holiday
  vi.restoreAllMocks()
})

it('uses only recognized server themes and clears a finished season on refresh', async () => {
  selectWorkspacePalette('auto')
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    value: false,
  })
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
    ok: true,
    json: async () => ({ success: true, data: { theme: 'mid_autumn' } }),
  } as Response)
  stop = watchSeasonalTheme()
  await vi.waitFor(() =>
    expect(document.documentElement.dataset.holiday).toBe('mid_autumn')
  )
  ensureWorkspaceColors()
  expect(
    document.querySelectorAll('[data-further-workspace-colors]')
  ).toHaveLength(1)
  fetchMock.mockResolvedValue({
    ok: true,
    json: async () => ({ success: true, data: { theme: '' } }),
  } as Response)
  window.dispatchEvent(new Event('focus'))
  await vi.waitFor(() =>
    expect(document.documentElement.dataset.holiday).toBeUndefined()
  )
  fetchMock.mockResolvedValue({
    ok: true,
    json: async () => ({ success: true, data: { theme: 'unknown' } }),
  } as Response)
  window.dispatchEvent(new Event('focus'))
  await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3))
  expect(document.documentElement.dataset.holiday).toBeUndefined()
})

it('ignores late responses after cleanup and removes listeners', async () => {
  Object.defineProperty(document, 'hidden', {
    configurable: true,
    value: false,
  })
  let resolve!: (value: Response) => void
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done
      })
  )
  stop = watchSeasonalTheme()
  stop()
  resolve({
    ok: true,
    json: async () => ({ success: true, data: { theme: 'qixi' } }),
  } as Response)
  await Promise.resolve()
  await Promise.resolve()
  window.dispatchEvent(new Event('focus'))
  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(document.documentElement.dataset.holiday).toBeUndefined()
})

it('keeps a manually fixed colour when an older seasonal request completes', async () => {
  Object.defineProperty(document, 'hidden', { configurable: true, value: false })
  let resolve!: (value: Response) => void
  vi.spyOn(globalThis, 'fetch').mockImplementation(() => new Promise((done) => { resolve = done }))
  stop = watchSeasonalTheme()
  selectWorkspacePalette('yellow')
  resolve({ ok: true, json: async () => ({ success: true, data: { theme: 'qixi' } }) } as Response)
  await vi.waitFor(() => expect(document.documentElement.dataset.seasonHoliday).toBe('qixi'))
  expect(document.documentElement.dataset.furtherPalette).toBe('yellow')
  expect(document.documentElement.dataset.furtherPaletteMode).toBe('fixed')
  expect(document.documentElement.dataset.holiday).toBeUndefined()
  expect(getWorkspacePalettePreference().fixedPalette).toBe('yellow')
})
