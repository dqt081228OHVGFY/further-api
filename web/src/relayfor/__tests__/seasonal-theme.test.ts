import { afterEach, expect, it, vi } from 'vitest'

import { ensureWorkspaceColors, watchSeasonalTheme } from '../seasonal-theme'

let stop: (() => void) | undefined
afterEach(() => {
  stop?.()
  stop = undefined
  document
    .querySelectorAll('[data-relayfor-workspace-colors]')
    .forEach((node) => node.remove())
  delete document.documentElement.dataset.holiday
  vi.restoreAllMocks()
})

it('uses only recognized server themes and clears a finished season on refresh', async () => {
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
    document.querySelectorAll('[data-relayfor-workspace-colors]')
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
