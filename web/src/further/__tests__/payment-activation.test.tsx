import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PaymentActivation } from '../feature-workspace/payment-activation'

const request = vi.hoisted(() => ({ write: vi.fn() }))
vi.mock('../feature-workspace/api', async original => ({
  ...(await original<typeof import('../feature-workspace/api')>()), ...request,
}))
beforeEach(() => {
  vi.resetAllMocks()
  Object.defineProperty(HTMLDialogElement.prototype, 'showModal', { configurable: true, value() { this.setAttribute('open', '') } })
  Object.defineProperty(HTMLDialogElement.prototype, 'close', { configurable: true, value() { this.removeAttribute('open') } })
})

describe('payment activation dialog', () => {
  it('keeps the confirmation recoverable after a failed session check', async () => {
    let reject!: (error: Error) => void
    request.write.mockImplementation(() => new Promise((_, fail) => { reject = fail }))
    render(<PaymentActivation confirmed={false} disabled={false} updated={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: '查看并确认' }))
    const dialog = screen.getByRole('dialog')
    fireEvent.click(within(dialog).getByRole('checkbox'))
    fireEvent.click(within(dialog).getByRole('button', { name: '确认收款规则' }))
    expect(within(dialog).getByRole('button', { name: '关闭' })).toBeDisabled()
    fireEvent(dialog, new Event('cancel', { cancelable: true }))
    expect(dialog).toBeVisible()
    reject(new Error('登录状态已失效，请重新登录'))
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('登录状态已失效')
    expect(within(dialog).getByRole('checkbox')).toBeChecked()
    expect(within(dialog).getByRole('button', { name: '确认收款规则' })).toBeEnabled()
  })
  it('can reopen when confirmation succeeded but the configuration snapshot did not refresh', async () => {
    request.write.mockResolvedValue({ confirmed: true })
    const updated = vi.fn().mockResolvedValue(undefined)
    render(<PaymentActivation confirmed={false} disabled={false} updated={updated} />)
    fireEvent.click(screen.getByRole('button', { name: '查看并确认' }))
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: '确认收款规则' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(request.write).toHaveBeenCalledWith('/api/option/payment_compliance', 'POST', { confirmed: true })
    expect(updated).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: '查看并确认' }))
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(screen.getByRole('checkbox')).not.toBeChecked()
  })
})
