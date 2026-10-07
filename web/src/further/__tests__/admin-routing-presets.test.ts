import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const runtime = readFileSync(resolve(process.cwd(), '../further-ui/assets/admin/admin-routing.js'), 'utf8')
const markup = readFileSync(resolve(process.cwd(), '../further-ui/admin/index.html'), 'utf8')
const request = vi.fn(), refresh = vi.fn(), toast = vi.fn()
type Routing = { bind(): void; load(): Promise<void>; resetModel(name: string): void; editModel(name: string): Promise<void>; addBinding(): void; hasUnsaved(): boolean }
const app = () => (window as unknown as { FurtherUIAdminRouting: Routing }).FurtherUIAdminRouting
const row = () => document.querySelector('[data-model-binding-row]') as HTMLElement
const input = (selector: string, target = row()) => target.querySelector(selector) as HTMLInputElement
const click = (selector: string, target = row()) => (target.querySelector(selector) as HTMLButtonElement).click()
const channels = [
  { id: 1, name: '主渠道', group: 'default', models: 'model-a', status: 1, priority: 0, weight: 5, model_mapping: { 'model-a': 'upstream-a' } },
  { id: 2, name: '相同模型', group: 'default', models: 'model-a,model-b', status: 1, priority: 4, weight: 1, model_mapping: {} },
  { id: 3, name: '其他分组', group: 'other', models: 'model-a', status: 1, priority: 100, weight: 1, model_mapping: {} },
]
beforeEach(async () => {
  document.body.innerHTML = new DOMParser().parseFromString(markup, 'text/html').body.innerHTML
  document.body.dataset.gatewayRole = '100'
  request.mockReset().mockResolvedValue(channels); refresh.mockReset().mockResolvedValue(undefined); toast.mockReset()
  Object.assign(window, { FurtherUI: { request, toast, confirm: vi.fn().mockResolvedValue(true), escapeHTML: (value: unknown) => String(value ?? '').replaceAll(/[&<>"']/g, symbol => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[symbol]!) }, FurtherUIAdminModels: { refresh } })
  window.eval(runtime)
  app().bind(); await app().load()
  ;((document.querySelector('[data-model-form]') as HTMLFormElement).elements.namedItem('model') as HTMLInputElement).value = 'model-a'
  app().resetModel('model-a'); request.mockClear()
})

describe('unified model channel routing and aliases', () => {
  it('shows upstream aliases and routing together without the removed page or dialogs', () => {
    expect(document.querySelector('[data-admin-model-tab="routing"]')).toBeNull()
    expect(document.querySelector('#model-routing-rule-dialog')).toBeNull()
    expect(document.querySelector('#model-alias-dialog')).toBeNull()
    expect(input('[data-binding-upstream]').value).toBe('upstream-a')
    expect(row().textContent).toContain('全部模型')
  })

  it('calculates main, backup and peer drafts only from same-group routes for this model', () => {
    click('[data-route-preset="primary"]'); expect(input('[data-binding-priority]').value).toBe('5')
    click('[data-route-preset="backup"]'); expect(input('[data-binding-priority]').value).toBe('3')
    click('[data-route-preset="peer"]'); expect(input('[data-binding-priority]').value).toBe('4')
    expect(input('[data-binding-weight]').value).toBe('1')
    expect(app().hasUnsaved()).toBe(true)
    expect(request).not.toHaveBeenCalled()
  })

  it('saves routing and alias changes in one channel mutation without saving model prices', async () => {
    input('[data-binding-upstream]').value = 'new-upstream'
    input('[data-binding-upstream]').dispatchEvent(new Event('input', { bubbles: true }))
    input('[data-binding-priority]').value = '7'; input('[data-binding-weight]').value = '3'
    click('[data-binding-save]'); click('[data-binding-save]')
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledOnce())
    expect(request).toHaveBeenCalledWith('/api/admin/routing/channels/1/binding', { method: 'PUT', body: { model: 'model-a', upstream_model: 'new-upstream', priority: 7, weight: 3, expected: { bound: true, upstream_model: 'upstream-a', priority: 0, weight: 5 } } })
    expect(request.mock.calls.filter(([, options]) => options?.method === 'PUT')).toHaveLength(1)
    expect(app().hasUnsaved()).toBe(false)
    expect(document.querySelector('#model-dialog')?.getAttribute('data-dialog-busy')).toBeNull()
  })

  it('adds a same-name upstream binding in the same editor', async () => {
    app().resetModel('model-b')
    ;((document.querySelector('[data-model-form]') as HTMLFormElement).elements.namedItem('model') as HTMLInputElement).value = 'model-b'
    app().addBinding()
    const added = document.querySelector('[data-binding-existing="false"]') as HTMLElement
    expect(input('[data-binding-channel-select]', added).value).toBe('1')
    expect(input('[data-binding-upstream]', added).value).toBe('model-b')
    expect(app().hasUnsaved()).toBe(true)
    click('[data-binding-save]', added)
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledOnce())
    expect(request).toHaveBeenCalledWith('/api/admin/routing/channels/1/binding', expect.objectContaining({ method: 'PUT', body: expect.objectContaining({ model: 'model-b', upstream_model: 'model-b' }) }))
  })

  it('removes just this model association using the combined endpoint', async () => {
    click('[data-binding-remove]')
    await vi.waitFor(() => expect(refresh).toHaveBeenCalledOnce())
    expect(request).toHaveBeenCalledWith('/api/admin/routing/channels/1/binding', expect.objectContaining({ method: 'DELETE', body: expect.objectContaining({ model: 'model-a' }) }))
  })

  it('retains failed drafts for retry and keeps ordinary administrators read-only', async () => {
    input('[data-binding-upstream]').dispatchEvent(new Event('input', { bubbles: true }))
    request.mockRejectedValueOnce(new Error('权限不足'))
    click('[data-binding-save]')
    await vi.waitFor(() => expect(row().textContent).toContain('保存失败：权限不足'))
    expect(app().hasUnsaved()).toBe(true)
    expect((row().querySelector('[data-binding-save]') as HTMLButtonElement).disabled).toBe(false)
    document.body.dataset.gatewayRole = '10'; app().resetModel('model-a'); request.mockClear()
    expect((row().querySelector('[data-binding-save]') as HTMLButtonElement).disabled).toBe(true)
    app().addBinding(); click('[data-binding-save]')
    expect(request).not.toHaveBeenCalled()
  })

  it('does not erase an in-flight edit when refreshed routing finishes loading', async () => {
    let finish!: (value: unknown) => void
    request.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const pending = app().editModel('model-a')
    input('[data-binding-upstream]').value = 'unsaved-upstream'
    input('[data-binding-upstream]').dispatchEvent(new Event('input', { bubbles: true }))
    finish(channels); await pending
    expect(input('[data-binding-upstream]').value).toBe('unsaved-upstream')
    expect(app().hasUnsaved()).toBe(true)
  })
})
