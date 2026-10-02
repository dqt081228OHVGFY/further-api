import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ info: vi.fn(), quote: vi.fn(), pay: vi.fn(), submit: vi.fn() }))
vi.mock('@/features/wallet/ldc-api', () => ({ getLdcInfo: mocks.info, quoteLdc: mocks.quote, payLdc: mocks.pay, submitLdcOrder: mocks.submit, sitePaymentLogo: () => '/relayfor-logo.svg' }))
vi.mock('@/hooks/use-system-config', () => ({ useSystemConfig: () => ({ logo: '/relayfor-logo.svg' }) }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (value: string) => value }) }))
vi.mock('@/components/dialog', () => ({ Dialog: ({ open, children }: { open: boolean; children: ReactNode }) => open ? <div>{children}</div> : null }))
vi.mock('@/components/ui/button', () => ({ Button: (props: React.ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props} /> }))
vi.mock('@/lib/format', () => ({ formatQuota: (value: number) => String(value) }))
vi.mock('../../../lib', () => ({ formatDuration: () => '30 天' }))

import { SubscriptionPurchaseDialog } from '../ldc-subscription-dialog'
import type { PlanRecord } from '../../../types'

const plan: PlanRecord = { plan: { id: 7, title: '节日会员', price_amount: 0.001, currency: 'USD', duration_unit: 'day', duration_value: 30, enabled: true, sort_order: 0, total_amount: 1000, quota_reset_period: 'never', max_purchase_per_user: 0, allow_wallet_overflow: true } }
const quote = { plan_id: 7, kind: 'subscription', credits: '0.10', currency: 'LDC' }
beforeEach(() => {
  mocks.info.mockResolvedValue({ enabled: true, configured: true })
  mocks.quote.mockResolvedValue(quote)
  mocks.pay.mockResolvedValue({ trade_no: 'LDC-order', status: 'pending' })
  mocks.submit.mockImplementation(() => {})
})
afterEach(() => { vi.clearAllMocks() })

describe('LDC subscription confirmation', () => {
  it('only creates an order after confirming the server quote and submits it without claiming payment success', async () => {
    const success = vi.fn(), close = vi.fn()
    render(<SubscriptionPurchaseDialog open plan={plan} onOpenChange={close} onPurchaseSuccess={success} />)
    await screen.findByText('0.10 LDC')
    expect(mocks.pay).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '确认并前往 LDC' }))
    await waitFor(() => expect(mocks.pay).toHaveBeenCalledWith({ plan_id: 7, expected_credits: '0.10' }))
    expect(mocks.submit).toHaveBeenCalledOnce()
    expect(success).not.toHaveBeenCalled()
    expect(close).not.toHaveBeenCalled()
  })
  it('canceling a confirmed quote never creates a payment order', async () => {
    const close = vi.fn()
    render(<SubscriptionPurchaseDialog open plan={plan} onOpenChange={close} />)
    await screen.findByText('0.10 LDC')
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(close).toHaveBeenCalledWith(false)
    expect(mocks.pay).not.toHaveBeenCalled()
  })
  it('keeps the payment locked through a pending request and shows real API failures', async () => {
    let reject!: (reason: Error) => void
    mocks.pay.mockImplementation(() => new Promise((_resolve, fail) => { reject = fail }))
    render(<SubscriptionPurchaseDialog open plan={plan} onOpenChange={vi.fn()} />)
    await screen.findByText('0.10 LDC')
    const button = screen.getByRole('button', { name: '确认并前往 LDC' })
    fireEvent.click(button); fireEvent.click(button)
    expect(mocks.pay).toHaveBeenCalledOnce()
    expect(button).toBeDisabled()
    reject(new Error('报价已变化，请重新确认'))
    await screen.findByRole('alert')
    expect(screen.getByRole('alert')).toHaveTextContent('报价已变化')
    expect(mocks.submit).not.toHaveBeenCalled()
  })
  it('does not offer payment when LDC configuration could not be verified', async () => {
    mocks.info.mockRejectedValue(new Error('无法读取支付状态'))
    render(<SubscriptionPurchaseDialog open plan={plan} onOpenChange={vi.fn()} />)
    await screen.findByRole('alert')
    expect(screen.getByRole('button', { name: '确认并前往 LDC' })).toBeDisabled()
    expect(mocks.quote).not.toHaveBeenCalled()
    expect(mocks.pay).not.toHaveBeenCalled()
  })
})
