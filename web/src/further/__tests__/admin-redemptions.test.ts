import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/admin.js'), 'utf8')
const shell = readFileSync(resolve(process.cwd(), '../further-ui/assets/app.js'), 'utf8')
const request = vi.fn()
const openDialog = vi.fn()
const writeText = vi.fn()
let loaders: Record<string, () => Promise<void>>
type RedemptionWindow = Window & { FurtherUI: unknown; FurtherUIResources: unknown; FurtherUIAdminOverview: unknown; FurtherUIAdminSettings: unknown; FurtherUIAdminModels: unknown }

beforeEach(() => {
  document.body.innerHTML = '<div data-redemption-rows></div><span data-redemption-page></span><button data-redemption-prev></button><button data-redemption-next></button><textarea data-generated-codes></textarea><div data-generated-redemption-title></div>'
  request.mockReset()
  openDialog.mockReset()
  writeText.mockReset().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  const runtimeWindow = window as unknown as RedemptionWindow
  runtimeWindow.FurtherUI = {
    request, openDialog, toast() {}, adminListPreference() { return false },
    number(value: number) { return String(value || 0) }, moneyFromQuota(value: number) { return `$${  value}` }, date(value: number) { return String(value || '') },
    escapeHTML(value: unknown) { return String(value).replaceAll(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]!) },
    setText(selector: string, value: string) { document.querySelectorAll(selector).forEach((element) => { element.textContent = value }) },
  }
  runtimeWindow.FurtherUIAdminOverview = { load() {} }
  runtimeWindow.FurtherUIAdminSettings = { load() {} }
  runtimeWindow.FurtherUIAdminModels = { load() {} }
  runtimeWindow.FurtherUIResources = { create(value: Record<string, () => Promise<void>>) { loaders = value; return {} } }
  window.eval(runtime)
  request.mockResolvedValue({ total: 1, total_pages: 1, items: [{ batch_id: 'batch-1', name: 'Test batch', count: 4, unused_count: 1, used_count: 1, disabled_count: 1, expired_count: 1, deletable_count: 3, items: [
    { id: 1, code: 'AVAILABLE', status: 1, quota: 20 },
    { id: 2, code: 'ALREADY-USED', status: 2, quota: 20, used_by_user_id: 2 },
    { id: 3, code: 'DISABLED', status: 3, quota: 20 },
    { id: 4, code: 'EXPIRED', status: 4, quota: 30 },
  ] }] })
})

describe('unified redemption management', () => {
  it('shows batch and code details together and batch copy includes only usable codes', async () => {
    await loaders.redemptions()
    expect(document.querySelectorAll('.redemption-batch')).toHaveLength(1)
    expect(document.querySelectorAll('.redemption-table tbody tr')).toHaveLength(4)
    expect(document.querySelector('.redemption-details')!.classList.contains('hidden')).toBe(false)
    expect(document.querySelector('.redemption-batch')!.textContent).toContain('多种面额')
    ;(document.querySelector('[data-copy-redemption-batch]') as HTMLButtonElement).click()
    expect((document.querySelector('[data-generated-codes]') as HTMLTextAreaElement).value).toBe('AVAILABLE')
    expect(openDialog).toHaveBeenCalledWith('generated-dialog')
    ;(document.querySelector('[data-copy-redemption-page]') as HTMLButtonElement).click()
    expect((document.querySelector('[data-generated-codes]') as HTMLTextAreaElement).value).toBe('AVAILABLE')
    expect(document.querySelector('[data-delete-redemption="2"]')).toBeNull()
    expect(document.querySelector('[data-edit-redemption="2"]')).toBeNull()
  })

  it('keeps collapsing batch rows painted until completion and cancels a stale exit on reopening', async () => {
    await loaders.redemptions()
    const controls = (window as unknown as RedemptionWindow).FurtherUI
    window.eval(shell)
    Object.assign(controls as object, { transitionDropdown: (window as unknown as { FurtherUI: { transitionDropdown: unknown } }).FurtherUI.transitionDropdown })
    const detail = document.querySelector<HTMLElement>('.redemption-details')!
    const toggle = document.querySelector<HTMLButtonElement>('[data-code-batch-toggle]')!
    const effects: Array<() => void> = []
    detail.animate = vi.fn(() => ({ finished: new Promise<void>(resolve => effects.push(resolve)), cancel: vi.fn() } as unknown as Animation))
    toggle.click()
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(detail).not.toHaveClass('hidden')
    expect(detail).toHaveAttribute('inert')
    toggle.click()
    effects[0](); await Promise.resolve()
    expect(detail).not.toHaveClass('hidden')
    expect(detail).not.toHaveAttribute('inert')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    effects[1](); await Promise.resolve()
    toggle.click()
    effects[2](); await Promise.resolve()
    expect(detail).toHaveClass('hidden')
    expect(detail.style.overflow).toBe('')
  })

  it('supports single code copy and preserves collapsed batch details after refresh', async () => {
    await loaders.redemptions()
    ;(document.querySelector('[data-copy-redemption="1"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith('AVAILABLE'))
    ;(document.querySelector('[data-code-batch-toggle]') as HTMLButtonElement).click()
    await loaders.redemptions()
    expect(document.querySelector('.redemption-details')!.classList.contains('hidden')).toBe(true)
    ;(document.querySelector('[data-redemption-expand-all]') as HTMLButtonElement).click()
    expect(document.querySelector('.redemption-details')!.classList.contains('hidden')).toBe(false)
  })
})
