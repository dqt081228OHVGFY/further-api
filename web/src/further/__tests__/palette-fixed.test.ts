import { readFileSync } from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { afterEach, describe, expect, it, vi } from 'vitest'

type TestDocument = { window: Window & typeof globalThis; close: () => void }
const { JSDOM } = createRequire(import.meta.url)('jsdom') as {
  JSDOM: new (html: string, options: { url: string; runScripts: 'outside-only' }) => TestDocument
}

const source = readFileSync(path.resolve(process.cwd(), '../further-ui/assets/theme-preference.js'), 'utf8')
const controls = readFileSync(path.resolve(process.cwd(), '../further-ui/assets/appearance.js'), 'utf8')
const key = 'further-api:theme:v2:palette'
const legacy = 'further-api:theme:v1:further-palette'
const documents: TestDocument[] = []
function documentWith(season: string, stored?: Record<string, string>) {
  const dom = new JSDOM('<!doctype html><html><head></head><body><header class="top-app-bar"></header></body></html>', { url: 'https://gateway.example/', runScripts: 'outside-only' })
  documents.push(dom)
  if (season) dom.window.document.documentElement.dataset.holiday = season
  for (const [name, value] of Object.entries(stored || {})) dom.window.localStorage.setItem(name, value)
  dom.window.eval(source)
  return dom
}
afterEach(() => {
  vi.restoreAllMocks()
  documents.splice(0).forEach((dom) => dom.window.close())
})

describe('persistent fixed palette', () => {
  it.each(['white', 'blue', 'yellow', 'qixi'])('keeps the previous manual %s choice ahead of an SSR holiday on the first head execution', (colour) => {
    const dom = documentWith('national_day', { [legacy]: colour })
    const root = dom.window.document.documentElement
    expect(root.dataset.furtherPaletteMode).toBe('fixed')
    expect(root.dataset.furtherPalette).toBe(colour)
    expect(root.dataset.holiday).toBe(colour === 'qixi' ? 'qixi' : undefined)
    expect(JSON.parse(dom.window.localStorage.getItem(key)!)).toEqual({ mode: 'fixed', fixedPalette: colour })
  })

  it('freezes the current holiday once and retains it on the next document even when its SSR season differs', () => {
    const first = documentWith('national_day', { [legacy]: 'auto' })
    const saved = first.window.localStorage.getItem(key)!
    expect(JSON.parse(saved)).toEqual({ mode: 'fixed', fixedPalette: 'national_day' })
    const next = documentWith('qixi', { [key]: saved, [legacy]: 'auto' })
    expect(next.window.document.documentElement.dataset.furtherPalette).toBe('national_day')
    expect(next.window.document.documentElement.dataset.holiday).toBe('national_day')
    expect(next.window.document.documentElement.dataset.seasonHoliday).toBe('qixi')
  })

  it('follows seasons only after an explicit automatic selection and can freeze the currently visible colour', () => {
    const dom = documentWith('national_day')
    const appearance = dom.window.FurtherAppearance!
    appearance.select('blue')
    appearance.applySeason('qixi')
    expect(appearance.current()).toBe('blue')
    appearance.setMode('auto')
    expect(appearance.state().effectivePalette).toBe('qixi')
    appearance.setMode('fixed')
    appearance.applySeason('mid_autumn')
    expect(appearance.state().effectivePalette).toBe('qixi')
    expect(appearance.mode()).toBe('fixed')
  })

  it('repairs late DOM and old preference writes without changing the fixed selection', async () => {
    const dom = documentWith('national_day')
    const appearance = dom.window.FurtherAppearance!
    appearance.select('yellow')
    dom.window.localStorage.setItem(legacy, 'auto')
    const root = dom.window.document.documentElement
    root.dataset.furtherPalette = 'auto'
    root.dataset.holiday = 'qixi'
    await new Promise((done) => dom.window.setTimeout(done, 0))
    expect(root.dataset.furtherPalette).toBe('yellow')
    expect(root.dataset.furtherPaletteMode).toBe('fixed')
    expect(root.dataset.holiday).toBeUndefined()
    appearance.applySeason('mid_autumn')
    expect(root.dataset.furtherPalette).toBe('yellow')
  })

  it('keeps the current-page manual choice when persistence fails instead of restoring the older saved colour', async () => {
    const dom = documentWith('national_day', { [key]: JSON.stringify({ mode: 'fixed', fixedPalette: 'blue' }) })
    vi.spyOn(dom.window.Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('storage full') })
    dom.window.FurtherAppearance!.select('yellow')
    await new Promise((done) => dom.window.setTimeout(done, 0))
    dom.window.FurtherAppearance!.applySeason('qixi')
    expect(dom.window.document.documentElement.dataset.furtherPalette).toBe('yellow')
    expect(dom.window.FurtherAppearance!.state().fixedPalette).toBe('yellow')
  })

  it('presents explicit fixed and automatic controls and keeps manual holiday choices', () => {
    const dom = documentWith('national_day')
    Object.assign(dom.window, { FurtherUI: { openDialog: vi.fn(), closeDialog: vi.fn() } })
    dom.window.document.body.dataset.gatewayReady = 'true'
    dom.window.eval(controls)
    const root = dom.window.document.documentElement
    const fixed = dom.window.document.querySelector<HTMLButtonElement>('[data-palette-mode=fixed]')!
    const auto = dom.window.document.querySelector<HTMLButtonElement>('[data-palette-mode=auto]')!
    expect(fixed.getAttribute('aria-pressed')).toBe('true')
    auto.click()
    expect(auto.getAttribute('aria-pressed')).toBe('true')
    dom.window.FurtherAppearance!.applySeason('mid_autumn')
    fixed.click()
    expect(root.dataset.furtherPalette).toBe('mid_autumn')
    dom.window.document.querySelector<HTMLButtonElement>('[data-palette=qixi]')!.click()
    expect(root.dataset.furtherPaletteMode).toBe('fixed')
    expect(root.dataset.holiday).toBe('qixi')
    expect(dom.window.document.querySelector('[data-palette-status]')).toHaveTextContent('已固定')
  })
})
