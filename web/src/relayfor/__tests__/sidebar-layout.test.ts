import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { afterEach, beforeEach, describe, expect, it } from 'vitest'

const relayforRoot = resolve(import.meta.dirname, '../../../../relayfor-ui')
const adminHtml = readFileSync(
  resolve(relayforRoot, 'admin/index.html'),
  'utf8'
)
const appCss = readFileSync(resolve(relayforRoot, 'assets/app.css'), 'utf8')
const themeCss = readFileSync(
  resolve(relayforRoot, 'assets/gateway-theme.css'),
  'utf8'
)

let stylesheet: HTMLStyleElement

beforeEach(() => {
  const originalPage = new DOMParser().parseFromString(adminHtml, 'text/html')
  const rail = originalPage.querySelector('.nav-rail')
  if (!rail) throw new Error('Original admin navigation is missing')
  document.body.replaceChildren(rail.cloneNode(true))
  stylesheet = document.createElement('style')
  stylesheet.textContent = `${appCss}\n${themeCss}`
  document.head.append(stylesheet)
})

afterEach(() => {
  stylesheet.remove()
  document.body.replaceChildren()
})

describe('original RelayFor admin sidebar layout', () => {
  it('scrolls the rail while keeping every original navigation row and group at full size', () => {
    const rail = document.querySelector<HTMLElement>('.nav-rail')
    expect(rail).not.toBeNull()
    expect(getComputedStyle(rail!).display).toBe('flex')
    expect(getComputedStyle(rail!).flexDirection).toBe('column')
    expect(getComputedStyle(rail!).overflowY).toBe('auto')

    const rows = [...rail!.querySelectorAll<HTMLElement>('.nav-item')]
    expect(rows.length).toBeGreaterThan(10)
    for (const row of rows) {
      const style = getComputedStyle(row)
      expect(style.flexShrink, row.textContent?.trim()).toBe('0')
      expect(style.minHeight, row.textContent?.trim()).toBe('52px')
      expect(style.height, row.textContent?.trim()).toBe('52px')
    }
    for (const child of rail!.children) {
      expect(
        getComputedStyle(child).flexShrink,
        child.textContent?.trim()
      ).toBe('0')
    }
  })

  it('preserves row height when gateway sections add more navigation buttons', () => {
    const rail = document.querySelector<HTMLElement>('.nav-rail')!
    const bottom = rail.querySelector('.rail-bottom')!
    for (let index = 0; index < 12; index += 1) {
      const row = document.createElement('button')
      row.className = 'nav-item'
      row.textContent = `扩展功能 ${index + 1}`
      rail.insertBefore(row, bottom)
    }

    expect(rail.querySelectorAll('.nav-item').length).toBeGreaterThan(20)
    for (const row of rail.querySelectorAll<HTMLElement>('.nav-item')) {
      const style = getComputedStyle(row)
      expect(style.flexShrink, row.textContent?.trim()).toBe('0')
      expect(style.minHeight, row.textContent?.trim()).toBe('52px')
    }
    expect(getComputedStyle(bottom).flexShrink).toBe('0')
    expect(getComputedStyle(rail).overflowY).toBe('auto')
  })
})
